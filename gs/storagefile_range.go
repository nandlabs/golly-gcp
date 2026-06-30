package gs

import (
	"context"
	"fmt"
	"io"

	"oss.nandlabs.io/golly/vfs"
)

// Compile-time check that StorageFile implements the optional
// vfs.RangeReader capability so vfs.ReadRange dispatches through us —
// translating the caller's [off, off+length) directly to a GCS ranged
// read rather than falling back to Seek+Read.
var _ vfs.RangeReader = (*StorageFile)(nil)

// ReadRange returns up to length bytes starting at off via GCS's
// native ranged reader. A length of 0 (or any negative value) means
// "read to EOF from off"; see cloud.google.com/go/storage's
// Object.NewRangeReader semantics where length == -1 ⇒ to EOF.
//
// Only the requested bytes are transferred over the wire, regardless
// of the object's total size.
func (f *StorageFile) ReadRange(ctx context.Context, off, length int64) ([]byte, error) {
	if off < 0 {
		return nil, fmt.Errorf("gs: negative offset %d", off)
	}
	// NewRangeReader uses -1 to mean "to EOF". Map our zero-length to it.
	wantLen := length
	if length <= 0 {
		wantLen = -1
	}

	r, err := f.client.Bucket(f.urlOpts.Bucket).Object(f.urlOpts.Key).NewRangeReader(ctx, off, wantLen)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if length > 0 && len(buf) == 0 {
		return nil, io.EOF
	}
	return buf, nil
}
