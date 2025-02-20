package gs

import (
	"fmt"
	"os"
	"time"

	"oss.nandlabs.io/golly/vfs"
)

type StorageFileInfo struct {
	fs           vfs.VFileSystem
	isDir        bool
	key          string
	lastModified time.Time
	size         int64
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
	return f.isDir
}

func (f *StorageFileInfo) Sys() interface{} {
	// Not applicable for GCP Storage objects, return default value
	return f.fs
}

// String returns a string representation of the file info.
func (f *StorageFileInfo) String() string {
	return fmt.Sprintf("StorageFileInfo{Name: %s, Size: %d, ModTime: %v, IsDir: %t}", f.key, f.size, f.lastModified, f.isDir)
}
