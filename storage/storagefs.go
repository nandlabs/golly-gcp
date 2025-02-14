package storage

import (
	"context"
	"errors"
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
	object := bucket.Object(urlopts.Key + "/")

	wc := object.NewWriter(context.Background())

	if err = wc.Close(); err != nil {
		logger.ErrorF("failed to close writer: %v", err)
		return
	}
	return
}

func (storageFs *StorageFS) Mkdir(u *url.URL) (file vfs.VFile, err error) {
	err = errors.New("operation Mkdir not supported")
	return
}

func (storageFs *StorageFS) MkdirAll(u *url.URL) (file vfs.VFile, err error) {
	err = errors.New("operation MkdirAll not supported")
	return
}

// Open location provided of the Storage Bucket
// TODO what is the purpose of this function? what does it open?
// Opening a single file?
func (storageFs *StorageFS) Open(u *url.URL) (file vfs.VFile, err error) {
	urlopts, err := parseUrl(u)
	if err != nil {
		return
	}
	client, err := urlopts.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	bucket := client.Bucket(urlopts.Bucket)
	// TODO Object name comes from URL
	object := bucket.Object(urlopts.Key)
	rc, err := object.NewReader(ctx)
	if err != nil {
		logger.ErrorF("object.NewReader: %v\n", err)
		return
	}
	defer rc.Close()
	_, err = io.ReadAll(rc)
	// TODO :: what is the expectation of this function
	file = &StorageFile{
		Location: u,
		fs:       storageFs,
	}
	return
}
