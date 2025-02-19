package gs

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"oss.nandlabs.io/golly/textutils"
	"oss.nandlabs.io/golly/vfs"
)

type StorageFile struct {
	*vfs.BaseFile
	bucket     *storage.BucketHandle
	closers    []io.Closer
	fs         vfs.VFileSystem
	storageObj *storage.ObjectHandle
	urlOpts    *UrlOpts
}

func (storageFile *StorageFile) Read(b []byte) (numBytes int, err error) {
	ctx := context.Background()
	reader, err := storageFile.storageObj.NewReader(ctx)
	storageFile.closers = append(storageFile.closers, reader)
	if err != nil {
		return
	}
	numBytes, err = reader.Read(b)
	return
}

func (storageFile *StorageFile) Write(b []byte) (numBytes int, err error) {

	ctx := context.Background()
	writer := storageFile.storageObj.NewWriter(ctx)
	storageFile.closers = append(storageFile.closers, writer)
	numBytes, err = writer.Write(b)
	return
}

func (storageFile *StorageFile) ListAll() (files []vfs.VFile, err error) {
	ctx := context.Background()
	logger.InfoF("Listing all objects in bucket %q using ListAll for path %s", storageFile.bucket.BucketName(), storageFile.urlOpts.u.Path)
	it := storageFile.bucket.Objects(ctx, &storage.Query{
		Prefix: storageFile.urlOpts.Key,
	})
	for {
		var vFile vfs.VFile
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		subPath := ""

		if attrs.Prefix == textutils.EmptyStr {

			subPath = attrs.Name
		} else {
			subPath = attrs.Prefix
		}
		if storageFile.urlOpts.Key != "" && (subPath == storageFile.urlOpts.Key+textutils.ForwardSlashStr || subPath == storageFile.urlOpts.Key) {
			continue
		}
		u := &url.URL{
			Scheme: storageFile.urlOpts.u.Scheme,
			Host:   storageFile.urlOpts.u.Host,
			Path:   subPath,
		}
		vFile, err = storageFile.fs.Open(u)
		if err != nil {
			return nil, err
		}
		files = append(files, vFile)

	}

	return
}

func (storageFile *StorageFile) Info() (file vfs.VFileInfo, err error) {
	ctx := context.Background()
	attrs, err := storageFile.storageObj.Attrs(ctx)
	if err != nil {
		logger.ErrorF("object.Attrs: %v", err)
		return
	}

	file = &StorageFileInfo{
		fs:           storageFile.fs,
		isDir:        attrs.Prefix != textutils.EmptyStr,
		key:          attrs.Name,
		lastModified: attrs.Updated,
		size:         attrs.Size,
	}

	return
}

func (storageFile *StorageFile) AddProperty(name, value string) (err error) {

	ctx := context.Background()

	attrs, err := storageFile.storageObj.Attrs(ctx)
	if err != nil {
		logger.ErrorF("object.Attrs: %v", err)
		return
	}

	newMetadata := attrs.Metadata
	if newMetadata == nil {
		newMetadata = make(map[string]string)
	}
	newMetadata[name] = value

	// update the object
	_, err = storageFile.storageObj.Update(ctx, storage.ObjectAttrsToUpdate{Metadata: newMetadata})
	if err != nil {
		logger.ErrorF("object.Update: %v", err)
		return
	}
	logger.InfoF("Added metadata %q=%q to gs://%s/%s\n", name, value, attrs.Bucket, attrs.Name)
	return
}

func (storageFile *StorageFile) GetProperty(name string) (value string, err error) {

	ctx := context.Background()

	attrs, err := storageFile.storageObj.Attrs(ctx)
	if err != nil {
		logger.ErrorF("object.Attrs: %v", err)
		return
	}

	if value, ok := attrs.Metadata[name]; ok {
		return value, nil
	}

	return "", fmt.Errorf("metadata key %q not found", name)
}

func (storageFile *StorageFile) Url() *url.URL {
	return storageFile.urlOpts.u
}

func (storageFile *StorageFile) Delete() (err error) {

	ctx := context.Background()

	err = storageFile.storageObj.Delete(ctx)
	return
}

func (storageFile *StorageFile) Close() (err error) {
	if len(storageFile.closers) > 0 {
		for _, closable := range storageFile.closers {
			err = closable.Close()
		}
	}
	return
}

func (storageFile *StorageFile) String() string {
	return storageFile.urlOpts.u.String()
}
