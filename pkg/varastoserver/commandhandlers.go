package varastoserver

import (
	"errors"
	"fmt"
	"github.com/function61/eventkit/command"
	"github.com/function61/eventkit/eventlog"
	"github.com/function61/eventkit/httpcommand"
	"github.com/function61/gokit/cryptorandombytes"
	"github.com/function61/gokit/httpauth"
	"github.com/function61/varasto/pkg/blobdriver"
	"github.com/function61/varasto/pkg/varastotypes"
	"github.com/function61/varasto/pkg/varastoutils"
	"github.com/gorilla/mux"
	"go.etcd.io/bbolt"
	"net/http"
)

// we are currently using the command pattern very wrong!
type cHandlers struct {
	db   *bolt.DB
	conf *ServerConfig
}

func (c *cHandlers) VolumeCreate(cmd *VolumeCreate, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	allVolumes := []varastotypes.Volume{}
	if err := VolumeRepository.Each(volumeAppender(&allVolumes), tx); err != nil {
		return err
	}

	if err := VolumeRepository.Update(&varastotypes.Volume{
		ID:    len(allVolumes) + 1,
		UUID:  varastoutils.NewVolumeUuid(),
		Label: cmd.Name,
		Quota: mebibytesToBytes(cmd.Quota),
	}, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) VolumeChangeQuota(cmd *VolumeChangeQuota, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	vol, err := QueryWithTx(tx).Volume(cmd.Id)
	if err != nil {
		return err
	}

	vol.Quota = mebibytesToBytes(cmd.Quota)

	if err := VolumeRepository.Update(vol, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) VolumeChangeDescription(cmd *VolumeChangeDescription, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	vol, err := QueryWithTx(tx).Volume(cmd.Id)
	if err != nil {
		return err
	}

	vol.Description = cmd.Description

	if err := VolumeRepository.Update(vol, tx); err != nil {
		return err
	}

	return tx.Commit()
}

// FIXME: name ends in 2 because conflicts with types.VolumeMount
func (c *cHandlers) VolumeMount2(cmd *VolumeMount2, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	vol, err := QueryWithTx(tx).Volume(cmd.Id)
	if err != nil {
		return err
	}

	// TODO: grab driver instance by this spec?
	mountSpec := &varastotypes.VolumeMount{
		ID:         varastoutils.NewVolumeMountId(),
		Volume:     vol.ID,
		Node:       c.conf.SelfNode.ID,
		Driver:     varastotypes.VolumeDriverKindLocalFs,
		DriverOpts: cmd.DriverOpts,
	}

	// try mounting the volume
	mount := blobdriver.NewLocalFs(vol.UUID, mountSpec.DriverOpts, nil)
	if err := mount.Mountable(); err != nil {
		return err
	}

	if err := VolumeMountRepository.Update(mountSpec, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) VolumeUnmount(cmd *VolumeUnmount, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	mount, err := QueryWithTx(tx).VolumeMount(cmd.Id)
	if err != nil {
		return err
	}

	if err := VolumeMountRepository.Delete(mount, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) DirectoryCreate(cmd *DirectoryCreate, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := DirectoryRepository.Update(&varastotypes.Directory{
		ID:     varastoutils.NewDirectoryId(),
		Parent: cmd.Parent,
		Name:   cmd.Name,
	}, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) DirectoryDelete(cmd *DirectoryDelete, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	dir, err := QueryWithTx(tx).Directory(cmd.Id)
	if err != nil {
		return err
	}

	collections, err := QueryWithTx(tx).CollectionsByDirectory(dir.ID)
	if err != nil {
		return err
	}

	subDirs, err := QueryWithTx(tx).SubDirectories(dir.ID)
	if err != nil {
		return err
	}

	if len(collections) > 0 {
		return fmt.Errorf("Cannot delete directory because it has %d collection(s)", len(collections))
	}

	if len(subDirs) > 0 {
		return fmt.Errorf("Cannot delete directory because it has %d directory(s)", len(subDirs))
	}

	if err := DirectoryRepository.Delete(dir, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) DirectoryRename(cmd *DirectoryRename, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	dir, err := QueryWithTx(tx).Directory(cmd.Id)
	if err != nil {
		return err
	}

	dir.Name = cmd.Name

	if err := DirectoryRepository.Update(dir, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) DirectoryChangeSensitivity(cmd *DirectoryChangeSensitivity, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	dir, err := QueryWithTx(tx).Directory(cmd.Id)
	if err != nil {
		return err
	}

	dir.Sensitivity = cmd.Sensitivity

	if err := DirectoryRepository.Update(dir, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) DirectoryMove(cmd *DirectoryMove, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	dirToMove, err := QueryWithTx(tx).Directory(cmd.Id)
	if err != nil {
		return err
	}

	// verify that new parent exists
	newParent, err := QueryWithTx(tx).Directory(cmd.Directory)
	if err != nil {
		return err
	}

	if dirToMove.ID == newParent.ID {
		return errors.New("dir cannot be its own parent, dawg")
	}

	dirToMove.Parent = newParent.ID

	if err := DirectoryRepository.Update(dirToMove, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) CollectionMove(cmd *CollectionMove, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// check for existence
	_, err = QueryWithTx(tx).Directory(cmd.Directory)
	if err != nil {
		return err
	}

	coll, err := QueryWithTx(tx).Collection(cmd.Collection)
	if err != nil {
		return err
	}

	coll.Directory = cmd.Directory

	if err := CollectionRepository.Update(coll, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) CollectionChangeDescription(cmd *CollectionChangeDescription, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	coll, err := QueryWithTx(tx).Collection(cmd.Collection)
	if err != nil {
		return err
	}

	coll.Description = cmd.Description

	if err := CollectionRepository.Update(coll, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) CollectionRename(cmd *CollectionRename, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	coll, err := QueryWithTx(tx).Collection(cmd.Collection)
	if err != nil {
		return err
	}

	coll.Name = cmd.Name

	if err := CollectionRepository.Update(coll, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) ClientCreate(cmd *ClientCreate, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := ClientRepository.Update(&varastotypes.Client{
		ID:        varastoutils.NewClientId(),
		Name:      cmd.Name,
		AuthToken: cryptorandombytes.Base64Url(32),
	}, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (c *cHandlers) ClientRemove(cmd *ClientRemove, ctx *command.Ctx) error {
	tx, err := c.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := ClientRepository.Delete(&varastotypes.Client{
		ID: cmd.Id,
	}, tx); err != nil {
		return err
	}

	return tx.Commit()
}

func registerCommandEndpoints(
	router *mux.Router,
	eventLog eventlog.Log,
	cmdHandlers CommandHandlers,
	mwares httpauth.MiddlewareChainMap,
) {
	router.HandleFunc("/command/{commandName}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		commandName := mux.Vars(r)["commandName"]

		httpErr := httpcommand.Serve(
			w,
			r,
			mwares,
			commandName,
			Allocators,
			cmdHandlers,
			eventLog)
		if httpErr != nil {
			if !httpErr.ErrorResponseAlreadySentByMiddleware() {
				http.Error(
					w,
					httpErr.ErrorCode+": "+httpErr.Description,
					httpErr.StatusCode) // making many assumptions here
			}
		} else {
			// no-op => ok
			w.Write([]byte(`{}`))
		}
	})).Methods(http.MethodPost)
}

func mebibytesToBytes(mebibytes int) int64 {
	return int64(mebibytes * 1024 * 1024)
}
