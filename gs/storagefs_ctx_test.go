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
