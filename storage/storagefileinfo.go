package storage

import (
	"os"
	"time"
)

type StorageFileInfo struct {
	key          string
	size         int64
	lastModified time.Time
}

func (f *StorageFileInfo) Name() string {
	return f.key
}

func (f *StorageFileInfo) Size() int64 {
	return f.size
}

func (f *StorageFileInfo) Mode() os.FileMode {
	// Not applicable for GCP Storage objects, return default value
	return 0
}

func (f *StorageFileInfo) ModTime() time.Time {
	return f.lastModified
}

func (f *StorageFileInfo) IsDir() bool {
	// Not applicable for GCP Storage objects, return default value
	return false
}

func (f *StorageFileInfo) Sys() interface{} {
	// Not applicable for GCP Storage objects, return default value
	return nil
}
