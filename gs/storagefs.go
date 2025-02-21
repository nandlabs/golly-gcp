package gs

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"oss.nandlabs.io/golly/textutils"
	"oss.nandlabs.io/golly/vfs"
)

const (
	GsFileScheme = "gs"
)

var fsSchemes = []string{GsFileScheme}

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
	_, attrerr := object.Attrs(context.Background())
	if attrerr == nil {
		return nil, fmt.Errorf("file %s already exists", urlopts.Key)
	}
	writer := object.NewWriter(context.Background())
	err = writer.Close()
	if err != nil {
		return
	}
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
	path := urlopts.Key
	if !strings.HasSuffix(path, textutils.ForwardSlashStr) {
		path = path + textutils.ForwardSlashStr
	}
	bucket := client.Bucket(urlopts.Bucket)

	object := bucket.Object(path)
	_, attrErr := object.Attrs(context.Background())
	if attrErr == nil {
		return nil, fmt.Errorf("folder %s already exists", path)
	}
	writer := object.NewWriter(context.Background())
	err = writer.Close()
	if err != nil {
		return
	}
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
