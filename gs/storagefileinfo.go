package gs

import (
	"fmt"
	"os"
	"time"

	"oss.nandlabs.io/golly/vfs"
)

// StorageFileInfo implements the vfs.VFileInfo interface for GCS objects.
type StorageFileInfo struct {
	fs           vfs.VFileSystem
	isDir        bool
	key          string
	lastModified time.Time
	size         int64
	contentType  string
}

// Name returns the object key.
func (f *StorageFileInfo) Name() string {
	return f.key
}

// Size returns the size of the object in bytes.
func (f *StorageFileInfo) Size() int64 {
	return f.size
}

// Mode returns the file mode bits. Not applicable for GCS, returns 0.
func (f *StorageFileInfo) Mode() os.FileMode {
	return 0
}

// ModTime returns the last modified time of the object.
func (f *StorageFileInfo) ModTime() time.Time {
	return f.lastModified
}

// IsDir returns true if the GCS object represents a directory (prefix).
func (f *StorageFileInfo) IsDir() bool {
	return f.isDir
}

// Sys returns the underlying VFileSystem.
func (f *StorageFileInfo) Sys() interface{} {
	return f.fs
}

// String returns a string representation of the file info.
func (f *StorageFileInfo) String() string {
	return fmt.Sprintf("StorageFileInfo{Name: %s, Size: %d, ModTime: %v, IsDir: %t}", f.key, f.size, f.lastModified, f.isDir)
}
