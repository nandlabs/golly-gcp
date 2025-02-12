package storage

import (
	"oss.nandlabs.io/golly/l3"
	"oss.nandlabs.io/golly/vfs"
)

var (
	logger = l3.Get()
)

func init() {
	storageFs := &StorageFS{}
	vfs.GetManager().Register(storageFs)
}
