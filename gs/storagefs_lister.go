package gs

import (
	"context"
	"io"
	"net/url"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"oss.nandlabs.io/golly/textutils"
	"oss.nandlabs.io/golly/vfs"
)

// Compile-time check that StorageFS implements the optional Lister
// capability so vfs.ListIter dispatches through us rather than falling
// back to the eager List slice.
var _ vfs.Lister = (*StorageFS)(nil)

// ListIter returns a paginated FileIterator over the GCS prefix at u.
// Each Next() call advances the underlying storage.ObjectIterator, which
// fetches the next page lazily — a million-object bucket prefix never
// materializes in a single slice.
func (fs *StorageFS) ListIter(ctx context.Context, u *url.URL) (vfs.FileIterator, error) {
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
	return &gcsFileIterator{
		fs:     fs,
		bucket: opts.Bucket,
		prefix: prefix,
		client: client,
		it:     it,
	}, nil
}

// gcsFileIterator yields VFiles one at a time from a storage.ObjectIterator.
// Not safe for concurrent use.
type gcsFileIterator struct {
	fs     *StorageFS
	bucket string
	prefix string
	client *storage.Client
	it     *storage.ObjectIterator
	done   bool
}

// Next returns the next VFile in the prefix. Returns io.EOF when
// exhausted; honors ctx cancellation between calls.
func (g *gcsFileIterator) Next(ctx context.Context) (vfs.VFile, error) {
	if g.done {
		return nil, io.EOF
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for {
		attrs, err := g.it.Next()
		if err == iterator.Done {
			g.done = true
			return nil, io.EOF
		}
		if err != nil {
			return nil, err
		}
		key := attrs.Name
		if attrs.Prefix != "" {
			key = attrs.Prefix
		}
		if key == g.prefix {
			continue
		}
		return newStorageFile(g.client, g.fs, &urlOpts{
			u:      &url.URL{Scheme: GsScheme, Host: g.bucket, Path: "/" + key},
			Bucket: g.bucket,
			Key:    key,
		}), nil
	}
}

// Close marks the iterator finished. Idempotent; safe to call without
// consuming to EOF (the underlying storage.ObjectIterator's resources
// are released when garbage collected).
func (g *gcsFileIterator) Close() error {
	g.done = true
	return nil
}
