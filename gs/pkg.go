package gs

import (
	"oss.nandlabs.io/golly/l3"
	"oss.nandlabs.io/golly/vfs"
)

var (
	logger = l3.Get()
)

func init() {
	storageFs := &StorageFS{
		BaseVFS: &vfs.BaseVFS{VFileSystem: &StorageFS{}},
	}
	vfs.GetManager().Register(storageFs)
}
