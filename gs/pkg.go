package gs

import (
	"context"

	"cloud.google.com/go/storage"
	"oss.nandlabs.io/golly-gcp/gcpsvc"
	"oss.nandlabs.io/golly/l3"
	"oss.nandlabs.io/golly/vfs"
)

var logger = l3.Get()

func init() {
	storageFs := &StorageFS{}
	storageFs.BaseVFS = &vfs.BaseVFS{VFileSystem: storageFs}
	vfs.GetManager().Register(storageFs)
}

// getStorageClient creates a GCS client using the gcpsvc config resolved for the given urlOpts.
func getStorageClient(opts *urlOpts) (*storage.Client, error) {
	cfg := gcpsvc.GetConfig(opts.u, GsScheme)
	if cfg == nil {
		// Fallback: load default GCS client without gcpsvc registration
		return storage.NewClient(context.Background())
	}
	return storage.NewClient(context.Background(), cfg.Options...)
}
