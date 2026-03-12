package gs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"oss.nandlabs.io/golly/textutils"
	"oss.nandlabs.io/golly/vfs"
)

// StorageFile implements the vfs.VFile interface for GCS objects.
type StorageFile struct {
	*vfs.BaseFile
	client  *storage.Client
	fs      *StorageFS
	urlOpts *urlOpts
	// reader/writer state
	reader      io.ReadCloser
	writeBuffer *bytes.Buffer
	offset      int64
	contentType string
}

// Read reads from the GCS object.
func (f *StorageFile) Read(b []byte) (n int, err error) {
	if f.reader == nil {
		obj := f.client.Bucket(f.urlOpts.Bucket).Object(f.urlOpts.Key)
		reader, readErr := obj.NewReader(context.Background())
		if readErr != nil {
			return 0, readErr
		}
		f.reader = reader
		f.contentType = reader.Attrs.ContentType
	}
	n, err = f.reader.Read(b)
	f.offset += int64(n)
	return
}

// Write writes data to a buffer. The data is flushed to GCS on Close.
func (f *StorageFile) Write(b []byte) (n int, err error) {
	if f.writeBuffer == nil {
		f.writeBuffer = &bytes.Buffer{}
	}
	n, err = f.writeBuffer.Write(b)
	f.offset += int64(n)
	return
}

// Seek sets the offset for the next Read or Write. Only io.SeekStart with offset 0 resets the reader.
func (f *StorageFile) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekStart && offset == 0 {
		// Reset reader so next Read starts from the beginning
		if f.reader != nil {
			_ = f.reader.Close()
			f.reader = nil
		}
		f.offset = 0
		return 0, nil
	}
	return f.offset, fmt.Errorf("seek not fully supported on GCS objects")
}

// Close flushes any buffered writes to GCS and closes open readers.
func (f *StorageFile) Close() error {
	var err error
	// Flush write buffer to GCS
	if f.writeBuffer != nil && f.writeBuffer.Len() > 0 {
		ct := f.contentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		obj := f.client.Bucket(f.urlOpts.Bucket).Object(f.urlOpts.Key)
		writer := obj.NewWriter(context.Background())
		writer.ContentType = ct
		_, writeErr := io.Copy(writer, f.writeBuffer)
		if writeErr != nil {
			_ = writer.Close()
			return writeErr
		}
		err = writer.Close()
		f.writeBuffer = nil
	}
	// Close reader
	if f.reader != nil {
		closeErr := f.reader.Close()
		if err == nil {
			err = closeErr
		}
		f.reader = nil
	}
	return err
}

// ListAll lists all objects under this GCS prefix.
func (f *StorageFile) ListAll() (files []vfs.VFile, err error) {
	prefix := f.urlOpts.Key
	if prefix != "" && !strings.HasSuffix(prefix, textutils.ForwardSlashStr) {
		prefix = prefix + textutils.ForwardSlashStr
	}

	query := &storage.Query{
		Prefix: prefix,
	}

	it := f.client.Bucket(f.urlOpts.Bucket).Objects(context.Background(), query)
	for {
		attrs, iterErr := it.Next()
		if iterErr == iterator.Done {
			break
		}
		if iterErr != nil {
			return nil, iterErr
		}

		key := attrs.Name
		if attrs.Prefix != "" {
			key = attrs.Prefix
		}

		// Skip self
		if key == prefix || key == f.urlOpts.Key {
			continue
		}

		u := &url.URL{
			Scheme: GsScheme,
			Host:   f.urlOpts.Bucket,
			Path:   "/" + key,
		}
		child := newStorageFile(f.client, f.fs, &urlOpts{
			u:      u,
			Bucket: f.urlOpts.Bucket,
			Key:    key,
		})
		files = append(files, child)
	}
	return
}

// Delete deletes the GCS object.
func (f *StorageFile) Delete() error {
	obj := f.client.Bucket(f.urlOpts.Bucket).Object(f.urlOpts.Key)
	return obj.Delete(context.Background())
}

// DeleteAll deletes all objects under this prefix (for directory-like objects).
func (f *StorageFile) DeleteAll() error {
	children, err := f.ListAll()
	if err != nil {
		return err
	}
	for _, child := range children {
		childInfo, infoErr := child.Info()
		if infoErr != nil {
			// If we can't get info, try deleting directly
			if delErr := child.Delete(); delErr != nil {
				return delErr
			}
			continue
		}
		if childInfo.IsDir() {
			if delErr := child.DeleteAll(); delErr != nil {
				return delErr
			}
		} else {
			if delErr := child.Delete(); delErr != nil {
				return delErr
			}
		}
	}
	// Delete the prefix marker itself
	return f.Delete()
}

// Info returns the VFileInfo for this GCS object.
func (f *StorageFile) Info() (vfs.VFileInfo, error) {
	// Check if this is a "directory" (prefix ending with /)
	if strings.HasSuffix(f.urlOpts.Key, textutils.ForwardSlashStr) || f.urlOpts.Key == "" {
		return &StorageFileInfo{
			fs:    f.fs,
			isDir: true,
			key:   f.urlOpts.Key,
		}, nil
	}

	obj := f.client.Bucket(f.urlOpts.Bucket).Object(f.urlOpts.Key)
	attrs, err := obj.Attrs(context.Background())
	if err != nil {
		// If Attrs fails, check if it's a prefix (directory)
		query := &storage.Query{
			Prefix: f.urlOpts.Key + textutils.ForwardSlashStr,
		}
		it := f.client.Bucket(f.urlOpts.Bucket).Objects(context.Background(), query)
		_, iterErr := it.Next()
		if iterErr == nil {
			return &StorageFileInfo{
				fs:    f.fs,
				isDir: true,
				key:   f.urlOpts.Key,
			}, nil
		}
		return nil, err
	}

	return &StorageFileInfo{
		fs:           f.fs,
		isDir:        false,
		key:          attrs.Name,
		lastModified: attrs.Updated,
		size:         attrs.Size,
		contentType:  attrs.ContentType,
	}, nil
}

// Parent returns the parent directory of this file.
func (f *StorageFile) Parent() (vfs.VFile, error) {
	key := strings.TrimSuffix(f.urlOpts.Key, textutils.ForwardSlashStr)
	idx := strings.LastIndex(key, textutils.ForwardSlashStr)
	parentKey := ""
	if idx > 0 {
		parentKey = key[:idx+1]
	}
	u := &url.URL{
		Scheme: GsScheme,
		Host:   f.urlOpts.Bucket,
		Path:   "/" + parentKey,
	}
	return f.fs.Open(u)
}

// Url returns the URL of this file.
func (f *StorageFile) Url() *url.URL {
	return f.urlOpts.u
}

// ContentType returns the content type of the GCS object.
func (f *StorageFile) ContentType() string {
	if f.contentType != "" {
		return f.contentType
	}
	return "application/octet-stream"
}

// AddProperty adds metadata to the GCS object.
func (f *StorageFile) AddProperty(name, value string) error {
	ctx := context.Background()
	obj := f.client.Bucket(f.urlOpts.Bucket).Object(f.urlOpts.Key)

	// Get current metadata
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get object metadata: %w", err)
	}

	metadata := attrs.Metadata
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata[name] = value

	// Update the object metadata
	_, err = obj.Update(ctx, storage.ObjectAttrsToUpdate{Metadata: metadata})
	if err != nil {
		return fmt.Errorf("failed to update object metadata: %w", err)
	}

	logger.InfoF("Added metadata %q=%q to gs://%s/%s", name, value, f.urlOpts.Bucket, f.urlOpts.Key)
	return nil
}

// GetProperty retrieves a metadata value from the GCS object.
func (f *StorageFile) GetProperty(name string) (string, error) {
	obj := f.client.Bucket(f.urlOpts.Bucket).Object(f.urlOpts.Key)
	attrs, err := obj.Attrs(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get object metadata: %w", err)
	}

	if val, ok := attrs.Metadata[name]; ok {
		return val, nil
	}
	return "", fmt.Errorf("metadata key %q not found", name)
}

// String returns the URL string of the file.
func (f *StorageFile) String() string {
	return f.urlOpts.u.String()
}
