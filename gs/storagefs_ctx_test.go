package gs

import (
	"testing"

	"oss.nandlabs.io/golly/vfs"
)

// TestStorageFS_ImplementsVFileSystemCtx is the load-bearing test for this
// downstream impl: as long as StorageFS satisfies vfs.VFileSystemCtx, the
// package-level dispatchers in golly (vfs.OpenCtx, vfs.CopyCtx, …) will
// route through our ctx-aware methods rather than the goroutine
// fallback. Real ctx propagation correctness past that boundary is the
// google-cloud-go SDK's responsibility — it takes context on every public
// call.
//
// An end-to-end cancellation test against a fake GCS endpoint is left to
// integration tests in the consuming project.
func TestStorageFS_ImplementsVFileSystemCtx(t *testing.T) {
	var _ vfs.VFileSystemCtx = (*StorageFS)(nil)
}

// TestStorageFS_ImplementsLister guarantees vfs.ListIter dispatches
// through StorageFS.ListIter rather than falling back to the eager
// List slice (which would materialize million-object prefixes into
// one allocation).
func TestStorageFS_ImplementsLister(t *testing.T) {
	var _ vfs.Lister = (*StorageFS)(nil)
}

// TestStorageFile_ImplementsRangeReader guarantees vfs.ReadRange
// dispatches to GCS's native NewRangeReader instead of Seek+Read
// (which on cloud backends typically downloads the whole object).
func TestStorageFile_ImplementsRangeReader(t *testing.T) {
	var _ vfs.RangeReader = (*StorageFile)(nil)
}
