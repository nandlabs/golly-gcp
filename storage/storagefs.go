package storage

import (
	"context"
	"errors"
	"io"
	"net/url"

	"oss.nandlabs.io/golly/vfs"
)

const (
	fileScheme = "gcs"
)

var fsSchemes = []string{fileScheme}

type StorageFS struct {
	*vfs.BaseVFS
}

func (storageFs *StorageFS) Schemes() []string {
	return fsSchemes
}

func (storageFs *StorageFS) Create(u *url.URL) (file vfs.VFile, err error) {
	urlopts, err := parseUrl(u)
	if err != nil {
		return
	}
	client, err := urlopts.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()

	err = handleStoragePath(ctx, client, u.String())
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
	object := bucket.Object("")

	rc, err := object.NewReader(ctx)
	if err != nil {
		logger.ErrorF("object.NewReader: %v", err)
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
