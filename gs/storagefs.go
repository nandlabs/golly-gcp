package gs

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"oss.nandlabs.io/golly/textutils"
	"oss.nandlabs.io/golly/vfs"
)

var fsSchemes = []string{GsScheme}

// StorageFS implements the vfs.VFileSystem interface for Google Cloud Storage.
type StorageFS struct {
	*vfs.BaseVFS
}

// Schemes returns the URL schemes supported by this filesystem.
func (fs *StorageFS) Schemes() []string {
	return fsSchemes
}

// Create creates a new empty object at the given GCS URL.
func (fs *StorageFS) Create(u *url.URL) (vfs.VFile, error) {
	opts, err := parseURL(u)
	if err != nil {
		return nil, err
	}

	client, err := getStorageClient(opts)
	if err != nil {
		return nil, err
	}

	bucket := client.Bucket(opts.Bucket)
	object := bucket.Object(opts.Key)

	// Check if object already exists
	_, attrErr := object.Attrs(context.Background())
	if attrErr == nil {
		return nil, fmt.Errorf("file gs://%s/%s already exists", opts.Bucket, opts.Key)
	}

	// Create empty object
	writer := object.NewWriter(context.Background())
	if err = writer.Close(); err != nil {
		return nil, err
	}

	return newStorageFile(client, fs, opts), nil
}

// Mkdir creates a directory marker (key ending with /) in GCS.
func (fs *StorageFS) Mkdir(u *url.URL) (vfs.VFile, error) {
	return fs.MkdirAll(u)
}

// MkdirAll creates a directory marker in GCS. Since GCS has no real directories,
// this creates a zero-byte object with a trailing slash.
func (fs *StorageFS) MkdirAll(u *url.URL) (vfs.VFile, error) {
	opts, err := parseURL(u)
	if err != nil {
		return nil, err
	}

	client, err := getStorageClient(opts)
	if err != nil {
		return nil, err
	}

	key := opts.Key
	if !strings.HasSuffix(key, textutils.ForwardSlashStr) {
		key = key + textutils.ForwardSlashStr
	}

	bucket := client.Bucket(opts.Bucket)
	object := bucket.Object(key)

	writer := object.NewWriter(context.Background())
	if err = writer.Close(); err != nil {
		return nil, err
	}

	// Update opts with directory key
	dirOpts := &urlOpts{
		u: &url.URL{
			Scheme: GsScheme,
			Host:   opts.Bucket,
			Path:   "/" + key,
		},
		Bucket: opts.Bucket,
		Key:    key,
	}

	return newStorageFile(client, fs, dirOpts), nil
}

// Open opens a GCS object at the given URL. It does not validate existence.
func (fs *StorageFS) Open(u *url.URL) (vfs.VFile, error) {
	opts, err := parseURL(u)
	if err != nil {
		return nil, err
	}

	client, err := getStorageClient(opts)
	if err != nil {
		return nil, err
	}

	return newStorageFile(client, fs, opts), nil
}

// newStorageFile creates a new StorageFile instance.
func newStorageFile(client *storage.Client, fs *StorageFS, opts *urlOpts) *StorageFile {
	f := &StorageFile{
		client:  client,
		fs:      fs,
		urlOpts: opts,
	}
	f.BaseFile = &vfs.BaseFile{VFile: f}
	return f
}

// Copy copies a GCS object from src to dst. If src is a directory, copies all children recursively.
func (fs *StorageFS) Copy(src, dst *url.URL) error {
	srcOpts, err := parseURL(src)
	if err != nil {
		return err
	}

	dstOpts, err := parseURL(dst)
	if err != nil {
		return err
	}

	client, err := getStorageClient(srcOpts)
	if err != nil {
		return err
	}

	// Check if source is a "directory" (prefix)
	srcFile := newStorageFile(client, fs, srcOpts)
	srcInfo, err := srcFile.Info()
	if err != nil {
		// Not a directory, copy single object
		return fs.copySingleObject(client, srcOpts, dstOpts)
	}

	if !srcInfo.IsDir() {
		return fs.copySingleObject(client, srcOpts, dstOpts)
	}

	// Copy all children
	children, err := srcFile.ListAll()
	if err != nil {
		return err
	}

	for _, child := range children {
		childInfo, infoErr := child.Info()
		if infoErr != nil {
			return infoErr
		}
		childKey := strings.TrimPrefix(child.Url().Path, "/")
		relativePath := strings.TrimPrefix(childKey, srcOpts.Key)
		dstKey := dstOpts.Key + relativePath

		childDstURL := &url.URL{
			Scheme: GsScheme,
			Host:   dstOpts.Bucket,
			Path:   "/" + dstKey,
		}

		if childInfo.IsDir() {
			if copyErr := fs.Copy(child.Url(), childDstURL); copyErr != nil {
				return copyErr
			}
		} else {
			childSrcOpts := &urlOpts{u: child.Url(), Bucket: srcOpts.Bucket, Key: childKey}
			childDstOpts := &urlOpts{u: childDstURL, Bucket: dstOpts.Bucket, Key: dstKey}
			if copyErr := fs.copySingleObject(client, childSrcOpts, childDstOpts); copyErr != nil {
				return copyErr
			}
		}
	}

	return nil
}

// copySingleObject copies a single GCS object using server-side copy.
func (fs *StorageFS) copySingleObject(client *storage.Client, src, dst *urlOpts) error {
	srcObj := client.Bucket(src.Bucket).Object(src.Key)
	dstObj := client.Bucket(dst.Bucket).Object(dst.Key)

	_, err := dstObj.CopierFrom(srcObj).Run(context.Background())
	return err
}

// Delete deletes the object at the given URL. If it's a directory, deletes all children.
func (fs *StorageFS) Delete(src *url.URL) error {
	srcOpts, err := parseURL(src)
	if err != nil {
		return err
	}

	client, err := getStorageClient(srcOpts)
	if err != nil {
		return err
	}

	srcFile := newStorageFile(client, fs, srcOpts)
	srcInfo, infoErr := srcFile.Info()

	if infoErr == nil && srcInfo.IsDir() {
		return srcFile.DeleteAll()
	}

	return srcFile.Delete()
}

// List lists all direct children of the given GCS prefix.
func (fs *StorageFS) List(u *url.URL) ([]vfs.VFile, error) {
	opts, err := parseURL(u)
	if err != nil {
		return nil, err
	}

	client, err := getStorageClient(opts)
	if err != nil {
		return nil, err
	}

	prefix := opts.Key
	if prefix != "" && !strings.HasSuffix(prefix, textutils.ForwardSlashStr) {
		prefix = prefix + textutils.ForwardSlashStr
	}

	query := &storage.Query{
		Prefix:    prefix,
		Delimiter: textutils.ForwardSlashStr,
	}

	var files []vfs.VFile
	it := client.Bucket(opts.Bucket).Objects(context.Background(), query)
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

		if key == prefix {
			continue
		}

		childURL := &url.URL{
			Scheme: GsScheme,
			Host:   opts.Bucket,
			Path:   "/" + key,
		}
		child := newStorageFile(client, fs, &urlOpts{
			u:      childURL,
			Bucket: opts.Bucket,
			Key:    key,
		})
		files = append(files, child)
	}

	return files, nil
}

// Walk traverses the GCS prefix tree recursively, calling fn for each file.
func (fs *StorageFS) Walk(u *url.URL, fn vfs.WalkFn) error {
	opts, err := parseURL(u)
	if err != nil {
		return err
	}

	client, err := getStorageClient(opts)
	if err != nil {
		return err
	}

	prefix := opts.Key
	if prefix != "" && !strings.HasSuffix(prefix, textutils.ForwardSlashStr) {
		prefix = prefix + textutils.ForwardSlashStr
	}

	query := &storage.Query{
		Prefix: prefix,
	}

	it := client.Bucket(opts.Bucket).Objects(context.Background(), query)
	for {
		attrs, iterErr := it.Next()
		if iterErr == iterator.Done {
			break
		}
		if iterErr != nil {
			return iterErr
		}

		key := attrs.Name
		if key == prefix {
			continue
		}

		childURL := &url.URL{
			Scheme: GsScheme,
			Host:   opts.Bucket,
			Path:   "/" + key,
		}
		child := newStorageFile(client, fs, &urlOpts{
			u:      childURL,
			Bucket: opts.Bucket,
			Key:    key,
		})
		if walkErr := fn(child); walkErr != nil {
			return walkErr
		}
	}

	return nil
}

// Move moves a GCS object from src to dst (copy + delete).
func (fs *StorageFS) Move(src, dst *url.URL) error {
	if err := fs.Copy(src, dst); err != nil {
		return err
	}
	return fs.Delete(src)
}

// Find finds files under the given location that match the filter.
func (fs *StorageFS) Find(location *url.URL, filter vfs.FileFilter) ([]vfs.VFile, error) {
	var files []vfs.VFile
	err := fs.Walk(location, func(file vfs.VFile) error {
		pass, filterErr := filter(file)
		if filterErr != nil {
			return filterErr
		}
		if pass {
			files = append(files, file)
		}
		return nil
	})
	return files, err
}

// DeleteMatching deletes files that match the given filter.
func (fs *StorageFS) DeleteMatching(location *url.URL, filter vfs.FileFilter) error {
	files, err := fs.Find(location, filter)
	if err != nil {
		return err
	}
	for _, file := range files {
		if delErr := file.Delete(); delErr != nil {
			return delErr
		}
	}
	return nil
}

// CopyRaw copies from src URL string to dst URL string.
func (fs *StorageFS) CopyRaw(src, dst string) error {
	srcURL, err := url.Parse(src)
	if err != nil {
		return err
	}
	dstURL, err := url.Parse(dst)
	if err != nil {
		return err
	}
	return fs.Copy(srcURL, dstURL)
}

// CreateRaw creates a file at the given URL string.
func (fs *StorageFS) CreateRaw(raw string) (vfs.VFile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return fs.Create(u)
}

// DeleteRaw deletes the object at the given URL string.
func (fs *StorageFS) DeleteRaw(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	return fs.Delete(u)
}

// ListRaw lists objects at the given URL string.
func (fs *StorageFS) ListRaw(raw string) ([]vfs.VFile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return fs.List(u)
}

// MkdirRaw creates a directory at the given URL string.
func (fs *StorageFS) MkdirRaw(raw string) (vfs.VFile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return fs.Mkdir(u)
}

// MkdirAllRaw creates a directory at the given URL string.
func (fs *StorageFS) MkdirAllRaw(raw string) (vfs.VFile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return fs.MkdirAll(u)
}

// MoveRaw moves from src URL string to dst URL string.
func (fs *StorageFS) MoveRaw(src, dst string) error {
	srcURL, err := url.Parse(src)
	if err != nil {
		return err
	}
	dstURL, err := url.Parse(dst)
	if err != nil {
		return err
	}
	return fs.Move(srcURL, dstURL)
}

// OpenRaw opens the GCS object at the given URL string.
func (fs *StorageFS) OpenRaw(raw string) (vfs.VFile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return fs.Open(u)
}
