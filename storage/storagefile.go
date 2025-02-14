package storage

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
	Location *url.URL
	fs       vfs.VFileSystem
	closers  []io.Closer
}

func (storageFile *StorageFile) Read(b []byte) (body int, err error) {
	parsedUrl, err := parseUrl(storageFile.Url())
	if err != nil {
		return
	}
	client, err := parsedUrl.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	bucket := client.Bucket(parsedUrl.Bucket)
	// TODO Object name comes from URL
	object := bucket.Object(parsedUrl.Key)
	reader, err := object.NewReader(ctx)
	storageFile.closers = append(storageFile.closers, reader)
	if err != nil {
		return
	}
	defer storageFile.Close()

	body, err = reader.Read(b)
	if err == io.EOF {
		reader.Close()
	}
	if err != nil {
		return
	}
	return
}

func (storageFile *StorageFile) Write(b []byte) (numBytes int, err error) {
	parsedUrl, err := parseUrl(storageFile.Url())
	if err != nil {
		return
	}
	client, err := parsedUrl.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	bucket := client.Bucket(parsedUrl.Bucket)
	// TODO Object name comes from URL
	object := bucket.Object("")

	writer := object.NewWriter(ctx)
	storageFile.closers = append(storageFile.closers, writer)
	defer storageFile.Close()

	// TODO Object name comes from URL
	logger.InfoF("Uploading data to gs://%s/%s\n", parsedUrl.Bucket, "objectName")
	if _, err = writer.Write(b); err != nil {
		logger.ErrorF("writer.Write: %v", err)
		return
	}
	return
}

func (storageFile *StorageFile) ListAll() (files []vfs.VFile, err error) {
	parsedUrl, err := parseUrl(storageFile.Url())
	if err != nil {
		return
	}
	client, err := parsedUrl.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	bucket := client.Bucket(parsedUrl.Bucket)

	it := bucket.Objects(ctx, nil)
	logger.InfoF("Files in bucket %s:\n", parsedUrl.Bucket)
	for {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			logger.InfoF("Bucket.Objects: %v\n", err)
			return nil, err
		}
		u, _ := url.Parse("https://storage.googleapis.com" + textutils.ForwardSlashStr + objAttrs.Bucket + textutils.ForwardSlashStr + objAttrs.Name)
		files = append(files, &StorageFile{
			Location: u,
		})
	}
	return
}

func (storageFile *StorageFile) Info() (file vfs.VFileInfo, err error) {
	parsedUrl, err := parseUrl(storageFile.Url())
	if err != nil {
		return
	}
	client, err := parsedUrl.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	bucket := client.Bucket(parsedUrl.Bucket)
	object := bucket.Object(parsedUrl.Key)

	attrs, err := object.Attrs(ctx)
	if err != nil {
		logger.ErrorF("object.Attrs: %v", err)
		return
	}

	file = &StorageFileInfo{
		key:          attrs.Name,
		size:         attrs.Size,
		lastModified: attrs.Updated,
	}

	return
}

func (storageFile *StorageFile) AddProperty(name, value string) (err error) {
	parsedUrl, err := parseUrl(storageFile.Url())
	if err != nil {
		return
	}
	client, err := parsedUrl.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	bucket := client.Bucket(parsedUrl.Bucket)
	object := bucket.Object(parsedUrl.Key)

	attrs, err := object.Attrs(ctx)
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
	_, err = object.Update(ctx, storage.ObjectAttrsToUpdate{Metadata: newMetadata})
	if err != nil {
		logger.ErrorF("object.Update: %v", err)
		return
	}
	logger.InfoF("Added metadata %q=%q to gs://%s/%s\n", name, value, attrs.Bucket, attrs.Name)
	return
}

func (storageFile *StorageFile) GetProperty(name string) (value string, err error) {
	parsedUrl, err := parseUrl(storageFile.Url())
	if err != nil {
		return
	}
	client, err := parsedUrl.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	bucket := client.Bucket(parsedUrl.Bucket)
	object := bucket.Object(parsedUrl.Key)

	attrs, err := object.Attrs(ctx)
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
	return storageFile.Location
}

func (storageFile *StorageFile) Delete() (err error) {
	parsedUrl, err := parseUrl(storageFile.Url())
	if err != nil {
		return
	}
	client, err := parsedUrl.CreateStorageClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	bucket := client.Bucket(parsedUrl.Bucket)
	object := bucket.Object(parsedUrl.Key)

	if err = object.Delete(ctx); err != nil {
		logger.ErrorF("object.Delete: %v", err)
		return
	}
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
