package stoserver

import (
	"errors"
	"github.com/function61/gokit/logex"
	"github.com/function61/varasto/pkg/blorm"
	"github.com/function61/varasto/pkg/stoserver/stodb"
	"github.com/function61/varasto/pkg/stotypes"
	"github.com/function61/varasto/pkg/stoutils"
	"go.etcd.io/bbolt"
	"log"
)

var (
	configBucketKey     = []byte("config")
	configBucketNodeKey = []byte("nodeId")
)

func bootstrap(db *bolt.DB, logger *log.Logger) error {
	logl := logex.Levels(logger)

	if err := bootstrapRepos(db); err != nil {
		return err
	}

	tx, err := db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	newNode := &stotypes.Node{
		ID:   stoutils.NewNodeId(),
		Addr: "localhost:8066",
		Name: "dev",
	}

	logl.Info.Printf("generated nodeId: %s", newNode.ID)

	results := []error{
		stodb.NodeRepository.Update(newNode, tx),
		stodb.DirectoryRepository.Update(stotypes.NewDirectory("root", "", "root"), tx),
		stodb.ReplicationPolicyRepository.Update(&stotypes.ReplicationPolicy{
			ID:             "default",
			Name:           "Default replication policy",
			DesiredVolumes: []int{1, 2}, // FIXME: this assumes 1 and 2 will be created soon..
		}, tx),
		bootstrapSetNodeId(newNode.ID, tx),
	}

	if err := allOk(results); err != nil {
		return err
	}

	return tx.Commit()
}

func bootstrapSetNodeId(nodeId string, tx *bolt.Tx) error {
	// errors if already exists
	configBucket, err := tx.CreateBucket(configBucketKey)
	if err != nil {
		return err
	}

	return configBucket.Put(configBucketNodeKey, []byte(nodeId))
}

func getSelfNodeId(tx *bolt.Tx) (string, error) {
	configBucket := tx.Bucket(configBucketKey)
	if configBucket == nil {
		return "", blorm.ErrNotFound
	}

	nodeId := string(configBucket.Get(configBucketNodeKey))
	if nodeId == "" {
		return "", errors.New("config bucket node ID not found")
	}

	return nodeId, nil
}

func bootstrapRepos(db *bolt.DB) error {
	tx, err := db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, repo := range repoByRecordType {
		if err := repo.Bootstrap(tx); err != nil {
			return err
		}
	}

	return tx.Commit()
}