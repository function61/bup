package stotypes

import (
	"time"
)

type Node struct {
	ID   string
	Addr string
	Name string
}

type Client struct {
	ID        string
	Created   time.Time
	AuthToken string
	Name      string
}

type ReplicationPolicy struct {
	ID             string
	Name           string
	DesiredVolumes []int
}

type Volume struct {
	ID            int
	UUID          string
	Label         string
	Description   string
	SerialNumber  string
	Technology    string
	Enclosure     string
	EnclosureSlot int // 0 = not defined
	Manufactured  time.Time
	WarrantyEnds  time.Time
	Quota         int64
	BlobSizeTotal int64 // @ compressed & deduplicated
	BlobCount     int64
}

type VolumeMount struct {
	ID         string
	Volume     int
	Node       string
	Driver     VolumeDriverKind
	DriverOpts string
}

type Directory struct {
	ID          string
	Parent      string
	Name        string
	Description string
	Type        string
	Metadata    map[string]string
	Sensitivity int // 0(for all eyes) 1(a bit sensitive) 2(for my eyes only)
}

type Collection struct {
	ID             string
	Created        time.Time // earliest of all changesets' file create/update timestamps
	Directory      string
	Name           string
	Description    string
	Sensitivity    int // 0(for all eyes) 1(a bit sensitive) 2(for my eyes only)
	DesiredVolumes []int
	Head           string
	EncryptionKey  [32]byte
	Changesets     []CollectionChangeset
	Metadata       map[string]string
	Tags           []string
}

type CollectionChangeset struct {
	ID           string
	Parent       string
	Created      time.Time
	FilesCreated []File
	FilesUpdated []File
	FilesDeleted []string
}

func (c *CollectionChangeset) AnyChanges() bool {
	return (len(c.FilesCreated) + len(c.FilesUpdated) + len(c.FilesDeleted)) > 0
}

type File struct {
	Path     string
	Sha256   string
	Created  time.Time
	Modified time.Time
	Size     int64
	BlobRefs []string // TODO: use explicit datatype?
}

type Blob struct {
	Ref                       BlobRef
	Coll                      string // collection that owns (& encrypts) this. many collections can refer to this same blob
	Volumes                   []int
	VolumesPendingReplication []int
	Referenced                bool // aborted uploads (ones that do not get referenced by a commit) could leave orphaned blobs
	IsCompressed              bool
	Size                      int32
	SizeOnDisk                int32 // after optional compression
	Crc32                     []byte
}

type IntegrityVerificationJob struct {
	ID                   string
	Started              time.Time
	Completed            time.Time
	VolumeId             int
	LastCompletedBlobRef BlobRef
	BytesScanned         uint64
	ErrorsFound          int
	Report               string
}

type Config struct {
	Key   string
	Value string
}

func NewChangeset(
	id string,
	parent string,
	created time.Time,
	filesCreated []File,
	filesUpdated []File,
	filesDeleted []string,
) CollectionChangeset {
	return CollectionChangeset{
		ID:           id,
		Parent:       parent,
		Created:      created,
		FilesCreated: filesCreated,
		FilesUpdated: filesUpdated,
		FilesDeleted: filesDeleted,
	}
}

func NewDirectory(id string, parent string, name string, typ string) *Directory {
	return &Directory{
		ID:       id,
		Parent:   parent,
		Name:     name,
		Metadata: map[string]string{},
		Type:     typ,
	}
}
