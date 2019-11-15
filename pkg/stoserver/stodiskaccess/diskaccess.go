// diskaccess ties together DB metadata read/write in addition to writing to disk
package stodiskaccess

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"github.com/function61/gokit/hashverifyreader"
	"github.com/function61/gokit/sliceutil"
	"github.com/function61/varasto/pkg/blobstore"
	"github.com/function61/varasto/pkg/mutexmap"
	"github.com/function61/varasto/pkg/stotypes"
	"github.com/function61/varasto/pkg/stoutils"
	"hash/crc32"
	"io"
	"io/ioutil"
)

type Controller struct {
	metadataStore  MetadataStore
	mountedDrivers map[int]blobstore.Driver // only mounted drivers
	writingBlobs   *mutexmap.M
}

func New(metadataStore MetadataStore) *Controller {
	return &Controller{
		metadataStore,
		map[int]blobstore.Driver{},
		mutexmap.New(),
	}
}

// call only during server boot (these are not threadsafe)
func (d *Controller) Define(volumeId int, driver blobstore.Driver) {
	if _, exists := d.mountedDrivers[volumeId]; exists {
		panic("driver for volumeId already defined")
	}

	d.mountedDrivers[volumeId] = driver
}

func (d *Controller) IsMounted(volumeId int) bool {
	_, mounted := d.mountedDrivers[volumeId]
	return mounted
}

// in theory we wouldn't need to do this since we could do a Fetch()-followed by Store(),
// but we can optimize by just transferring the raw on-disk format
func (d *Controller) Replicate(ctx context.Context, fromVolumeId int, toVolumeId int, ref stotypes.BlobRef) error {
	fromDriver, err := d.driverFor(fromVolumeId)
	if err != nil {
		return err
	}

	toDriver, err := d.driverFor(toVolumeId)
	if err != nil {
		return err
	}

	expectedCrc32, err := d.metadataStore.QueryBlobCrc32(ref)
	if err != nil {
		return fmt.Errorf("Replicate() QueryBlobCrc32: %v", err)
	}

	rawContent, err := fromDriver.RawFetch(ctx, ref)
	if err != nil {
		return err
	}
	defer rawContent.Close()

	crc32VerifiedReader := hashverifyreader.New(rawContent, crc32.NewIEEE(), expectedCrc32)

	if err := toDriver.RawStore(ctx, ref, crc32VerifiedReader); err != nil {
		return err
	}

	return d.metadataStore.WriteBlobReplicated(ref, toVolumeId)
}

func (d *Controller) WriteBlob(volumeId int, collId string, ref stotypes.BlobRef, content io.Reader, maybeCompressible bool) error {
	// FIXME: this will lock for a really long time if the HTTP connection breaks (TODO: benchmark for how long).
	// should we have some kind of timeoutreader?
	unlock, ok := d.writingBlobs.TryLock(ref.AsHex())
	if !ok {
		return fmt.Errorf("Another thread is currently writing blob[%s]", ref.AsHex())
	}
	defer unlock()

	// since we're writing a blob (and not replicating), for safety we'll check that we haven't
	// seen this blob before
	if exists, err := d.metadataStore.QueryBlobExists(ref); err != nil || exists {
		if err != nil { // error checking existence
			return err
		} else {
			// blob exists, which is unexpected
			return fmt.Errorf("WriteBlob() already exists: %s", ref.AsHex())
		}
	}

	// this is going to take relatively long time, so we can't keep
	// a transaction open

	driver, err := d.driverFor(volumeId)
	if err != nil {
		return err
	}

	readCounter := writeCounter{}
	verifiedContent := readCounter.Tee(stoutils.BlobHashVerifier(content, ref))

	encryptionKeyId, encryptionKey, err := d.metadataStore.QueryCollectionEncryptionKeyForNewBlobs(collId)
	if err != nil {
		return err
	}

	blobEncrypted, err := encryptAndCompressBlob(verifiedContent, encryptionKey, ref, maybeCompressible)
	if err != nil {
		return err
	}

	if err := driver.RawStore(context.TODO(), ref, bytes.NewReader(blobEncrypted.CiphertextMaybeCompressed)); err != nil {
		return fmt.Errorf("storing blob into volume %d failed: %v", volumeId, err)
	}

	meta := &BlobMeta{
		Ref:             ref,
		RealSize:        int32(readCounter.BytesWritten()),
		SizeOnDisk:      int32(len(blobEncrypted.CiphertextMaybeCompressed)),
		IsCompressed:    blobEncrypted.Compressed,
		EncryptionKeyId: encryptionKeyId,
		ExpectedCrc32:   blobEncrypted.Crc32,
	}

	if err := d.metadataStore.WriteBlobCreated(meta, volumeId); err != nil {
		return fmt.Errorf("WriteBlob() DB write: %v", err)
	}

	return nil
}

// does decrypt(optional_decompress(blobOnDisk))
// verifies blob integrity for you!
func (d *Controller) Fetch(ref stotypes.BlobRef, encryptionKeys []stotypes.KeyEnvelope, volumeId int) (io.ReadCloser, error) {
	driver, err := d.driverFor(volumeId)
	if err != nil {
		return nil, err
	}

	meta, err := d.metadataStore.QueryBlobMetadata(ref, encryptionKeys)
	if err != nil {
		return nil, err
	}

	body, err := driver.RawFetch(context.TODO(), ref)
	if err != nil {
		return nil, err
	}
	// body.Close() will be called by readCloseWrapper

	// reads crc32-verified ciphertext which contains maybe_gzipped(plaintext)
	crc32VerifiedCiphertextReader := hashverifyreader.New(body, crc32.NewIEEE(), meta.ExpectedCrc32)

	aesDecrypter, err := aes.NewCipher(meta.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("Fetch() AES cipher: %v", err)
	}

	decrypted := &cipher.StreamReader{S: cipher.NewCTR(aesDecrypter, deriveIvFromBlobRef(ref)), R: crc32VerifiedCiphertextReader}

	// assume no compression ..
	uncompressedReader := io.Reader(decrypted)

	if meta.IsCompressed { // .. but patch in decompression step if assumption incorrect
		gzipReader, err := gzip.NewReader(decrypted)
		if err != nil {
			return nil, fmt.Errorf("Fetch() gzip: %v", err)
		}

		uncompressedReader = gzipReader
	}

	blobDecryptedUncompressed := stoutils.BlobHashVerifier(uncompressedReader, ref)

	return &readCloseWrapper{blobDecryptedUncompressed, body}, nil
}

// currently looks for the first ID mounted on this node, but in the future could use richer heuristics:
// - is the HDD currently spinning
// - best latency & bandwidth
func (d *Controller) BestVolumeId(volumeIds []int) (int, error) {
	for _, volumeId := range volumeIds {
		if d.IsMounted(volumeId) {
			return volumeId, nil
		}
	}

	return 0, stotypes.ErrBlobNotAccessibleOnThisNode
}

// runs a scrub for a blob in a given volume to detect errors
// https://en.wikipedia.org/wiki/Data_scrubbing
// we could actually just do a Fetch() but that would require access to the encryption keys.
// this way we can verify on-disk integrity without encryption keys.
func (d *Controller) Scrub(ref stotypes.BlobRef, volumeId int) (int64, error) {
	driver, err := d.driverFor(volumeId)
	if err != nil {
		return 0, err
	}

	expectedCrc32, err := d.metadataStore.QueryBlobCrc32(ref)
	if err != nil {
		return 0, err
	}

	body, err := driver.RawFetch(context.TODO(), ref)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	verifiedReader := hashverifyreader.New(body, crc32.NewIEEE(), expectedCrc32)

	bytesRead, err := io.Copy(ioutil.Discard, verifiedReader)
	return bytesRead, err
}

func (d *Controller) driverFor(volumeId int) (blobstore.Driver, error) {
	driver, found := d.mountedDrivers[volumeId]
	if !found {
		return nil, fmt.Errorf("volume %d not found", volumeId)
	}

	return driver, nil
}

func encrypt(key []byte, plaintext io.Reader, br stotypes.BlobRef) ([]byte, error) {
	aesEncrypter, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	streamCipher := cipher.NewCTR(aesEncrypter, deriveIvFromBlobRef(br))

	var cipherText bytes.Buffer

	ciphertextWriter := &cipher.StreamWriter{S: streamCipher, W: &cipherText}

	// Copy the input to the output buffer, encrypting as we go.
	if _, err := io.Copy(ciphertextWriter, plaintext); err != nil {
		return nil, err
	}

	return cipherText.Bytes(), nil
}

// used for encryptAndCompressBlob()
type blobResult struct {
	CiphertextMaybeCompressed []byte
	Compressed                bool
	Crc32                     []byte
}

// does encrypt(maybe_compress(plaintext))
func encryptAndCompressBlob(
	contentReader io.Reader,
	encryptionKey []byte,
	ref stotypes.BlobRef,
	maybeCompressible bool,
) (*blobResult, error) {
	content, err := ioutil.ReadAll(contentReader)
	if err != nil {
		return nil, err
	}

	contentMaybeCompressed := content
	contentIsCompressed := false

	/*	Here are perf measurements from my year 2012 CPU

		concurrency=2 | by trying compression = 25.1 MB/s
		concurrency=2 | no compression = 50.0 MB/s
		concurrency=4 | no compression = 65.7 MB/s
										66.3 MB/s
	*/
	if maybeCompressible {
		var compressed bytes.Buffer
		compressedWriter := gzip.NewWriter(&compressed)

		if _, err := compressedWriter.Write(content); err != nil {
			return nil, err
		}

		if err := compressedWriter.Close(); err != nil {
			return nil, err
		}

		compressionRatio := float64(compressed.Len()) / float64(len(content))

		wellCompressible := compressionRatio < 0.9

		if wellCompressible {
			contentMaybeCompressed = compressed.Bytes()
			contentIsCompressed = true
		}
	}

	ciphertextMaybeCompressed, err := encrypt(encryptionKey, bytes.NewReader(contentMaybeCompressed), ref)
	if err != nil {
		return nil, err
	}

	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(ciphertextMaybeCompressed))

	return &blobResult{
		CiphertextMaybeCompressed: ciphertextMaybeCompressed,
		Compressed:                contentIsCompressed,
		Crc32:                     crc,
	}, nil
}

func deriveIvFromBlobRef(br stotypes.BlobRef) []byte {
	return br.AsSha256Sum()[0:aes.BlockSize]
}
