package gs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"oss.nandlabs.io/golly/textutils"
	"oss.nandlabs.io/golly/vfs"
)

// Compile-time check that StorageFS satisfies vfs.VFileSystemCtx so callers
// can rely on vfs.OpenCtx / CopyCtx / etc. to dispatch automatically
// instead of falling back to the goroutine-and-select wrapper.
var _ vfs.VFileSystemCtx = (*StorageFS)(nil)

// OpenCtx is the context-aware variant of Open. Open itself doesn't make
// a remote call — it just constructs a reference; reads/writes on the
// returned VFile pick up ctx from their own call sites.
func (fs *StorageFS) OpenCtx(_ context.Context, u *url.URL) (vfs.VFile, error) {
	return fs.Open(u)
}

// CreateCtx is the context-aware variant of Create.
func (fs *StorageFS) CreateCtx(ctx context.Context, u *url.URL) (vfs.VFile, error) {
	opts, err := parseURL(u)
	if err != nil {
		return nil, err
	}
	client, err := getStorageClient(opts)
	if err != nil {
		return nil, err
	}
	object := client.Bucket(opts.Bucket).Object(opts.Key)
	if _, attrErr := object.Attrs(ctx); attrErr == nil {
		return nil, fmt.Errorf("file gs://%s/%s already exists", opts.Bucket, opts.Key)
	}
	writer := object.NewWriter(ctx)
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return newStorageFile(client, fs, opts), nil
}

// MkdirAllCtx is the context-aware variant of MkdirAll.
func (fs *StorageFS) MkdirAllCtx(ctx context.Context, u *url.URL) (vfs.VFile, error) {
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
		key += textutils.ForwardSlashStr
	}
	writer := client.Bucket(opts.Bucket).Object(key).NewWriter(ctx)
	if err := writer.Close(); err != nil {
		return nil, err
	}
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

// DeleteCtx is the context-aware variant of Delete.
func (fs *StorageFS) DeleteCtx(ctx context.Context, src *url.URL) error {
	srcOpts, err := parseURL(src)
	if err != nil {
		return err
	}
	client, err := getStorageClient(srcOpts)
	if err != nil {
		return err
	}
	// Probe existence with ctx; on miss, fall through to recursive delete.
	_, attrErr := client.Bucket(srcOpts.Bucket).Object(srcOpts.Key).Attrs(ctx)
	if attrErr != nil {
		// Could be a "directory" (no real object). Use the existing
		// DeleteAll path on the VFile (which iterates internally; future
		// work could thread ctx through that path too).
		return newStorageFile(client, fs, srcOpts).DeleteAll()
	}
	return client.Bucket(srcOpts.Bucket).Object(srcOpts.Key).Delete(ctx)
}

// ListCtx is the context-aware variant of List.
func (fs *StorageFS) ListCtx(ctx context.Context, u *url.URL) ([]vfs.VFile, error) {
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
		prefix += textutils.ForwardSlashStr
	}
	it := client.Bucket(opts.Bucket).Objects(ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: textutils.ForwardSlashStr,
	})
	var files []vfs.VFile
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
		files = append(files, newStorageFile(client, fs, &urlOpts{
			u:      &url.URL{Scheme: GsScheme, Host: opts.Bucket, Path: "/" + key},
			Bucket: opts.Bucket,
			Key:    key,
		}))
	}
	return files, nil
}

// WalkCtx is the context-aware variant of Walk. ctx is checked between
// iterator pages; callers may also test ctx.Err() inside fn for finer
// cancellation granularity on long-running walks.
func (fs *StorageFS) WalkCtx(ctx context.Context, u *url.URL, fn vfs.WalkFn) error {
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
		prefix += textutils.ForwardSlashStr
	}
	it := client.Bucket(opts.Bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		child := newStorageFile(client, fs, &urlOpts{
			u:      &url.URL{Scheme: GsScheme, Host: opts.Bucket, Path: "/" + key},
			Bucket: opts.Bucket,
			Key:    key,
		})
		if walkErr := fn(child); walkErr != nil {
			if errors.Is(walkErr, vfs.ErrSkipAll) {
				return nil
			}
			if errors.Is(walkErr, vfs.ErrSkipDir) {
				// GCS has no real directories — at this level the
				// flat object listing has no children to skip, so
				// the sentinel behaves the same as continue.
				continue
			}
			return walkErr
		}
	}
	return nil
}

// CopyCtx is the context-aware variant of Copy.
func (fs *StorageFS) CopyCtx(ctx context.Context, src, dst *url.URL) error {
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

	srcFile := newStorageFile(client, fs, srcOpts)
	srcInfo, infoErr := srcFile.Info()
	if infoErr != nil || !srcInfo.IsDir() {
		return fs.copySingleObjectCtx(ctx, client, srcOpts, dstOpts)
	}

	children, err := srcFile.ListAll()
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return err
		}
		childInfo, infoErr := child.Info()
		if infoErr != nil {
			return infoErr
		}
		childKey := strings.TrimPrefix(child.Url().Path, "/")
		relativePath := strings.TrimPrefix(childKey, srcOpts.Key)
		dstKey := dstOpts.Key + relativePath
		childDstURL := &url.URL{Scheme: GsScheme, Host: dstOpts.Bucket, Path: "/" + dstKey}
		if childInfo.IsDir() {
			if copyErr := fs.CopyCtx(ctx, child.Url(), childDstURL); copyErr != nil {
				return copyErr
			}
		} else {
			childSrcOpts := &urlOpts{u: child.Url(), Bucket: srcOpts.Bucket, Key: childKey}
			childDstOpts := &urlOpts{u: childDstURL, Bucket: dstOpts.Bucket, Key: dstKey}
			if copyErr := fs.copySingleObjectCtx(ctx, client, childSrcOpts, childDstOpts); copyErr != nil {
				return copyErr
			}
		}
	}
	return nil
}

func (fs *StorageFS) copySingleObjectCtx(ctx context.Context, client *storage.Client, src, dst *urlOpts) error {
	srcObj := client.Bucket(src.Bucket).Object(src.Key)
	dstObj := client.Bucket(dst.Bucket).Object(dst.Key)
	_, err := dstObj.CopierFrom(srcObj).Run(ctx)
	return err
}

// MoveCtx is CopyCtx + DeleteCtx.
func (fs *StorageFS) MoveCtx(ctx context.Context, src, dst *url.URL) error {
	if err := fs.CopyCtx(ctx, src, dst); err != nil {
		return err
	}
	return fs.DeleteCtx(ctx, src)
}
