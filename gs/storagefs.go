package gs

import (
	"io"
	"net/url"

	"oss.nandlabs.io/golly/vfs"
)

const (
	fileScheme = "gs"
)

var fsSchemes = []string{fileScheme}

type StorageFS struct {
	*vfs.BaseVFS
}

func (storageFs *StorageFS) Schemes() []string {
	return fsSchemes
}

// TODO should not be allowed to create a bucket
// should be able to create folders and files
func (storageFs *StorageFS) Create(u *url.URL) (file vfs.VFile, err error) {
	urlopts, err := parseUrl(u)
	if err != nil {
		return
	}
	client, err := urlopts.CreateStorageClient()
	if err != nil {
		return
	}

	bucket := client.Bucket(urlopts.Bucket)
	object := bucket.Object(urlopts.Key)
	file = &StorageFile{
		bucket:     bucket,
		fs:         storageFs,
		storageObj: object,
		urlOpts:    urlopts,
		closers:    make([]io.Closer, 0),
	}
	return
}

func (storageFs *StorageFS) Mkdir(u *url.URL) (file vfs.VFile, err error) {

	return storageFs.MkdirAll(u)
}

func (storageFs *StorageFS) MkdirAll(u *url.URL) (file vfs.VFile, err error) {
	urlopts, err := parseUrl(u)
	if err != nil {
		return
	}
	client, err := urlopts.CreateStorageClient()
	if err != nil {
		return
	}

	bucket := client.Bucket(urlopts.Bucket)
	object := bucket.Object(urlopts.Key + "/")
	file = &StorageFile{
		bucket:     bucket,
		fs:         storageFs,
		storageObj: object,
		urlOpts:    urlopts,
		closers:    make([]io.Closer, 0),
	}
	return
}

// Open location provided of the Storage Bucket
func (storageFs *StorageFS) Open(u *url.URL) (file vfs.VFile, err error) {
	urlopts, err := parseUrl(u)
	if err != nil {
		return
	}
	client, err := urlopts.CreateStorageClient()
	if err != nil {
		return
	}
	bucket := client.Bucket(urlopts.Bucket)
	object := bucket.Object(urlopts.Key)

	file = &StorageFile{
		bucket:     bucket,
		fs:         storageFs,
		storageObj: object,
		urlOpts:    urlopts,
		closers:    make([]io.Closer, 0),
	}
	return
}
