package gs

import (
	"testing"
	"time"
)

func TestStorageFileInfo_Name(t *testing.T) {
	info := &StorageFileInfo{key: "path/to/file.txt"}
	if info.Name() != "path/to/file.txt" {
		t.Errorf("expected 'path/to/file.txt', got %q", info.Name())
	}
}

func TestStorageFileInfo_Size(t *testing.T) {
	info := &StorageFileInfo{size: 1024}
	if info.Size() != 1024 {
		t.Errorf("expected 1024, got %d", info.Size())
	}
}

func TestStorageFileInfo_Mode(t *testing.T) {
	info := &StorageFileInfo{}
	if info.Mode() != 0 {
		t.Errorf("expected 0, got %v", info.Mode())
	}
}

func TestStorageFileInfo_ModTime(t *testing.T) {
	now := time.Now()
	info := &StorageFileInfo{lastModified: now}
	if !info.ModTime().Equal(now) {
		t.Errorf("expected %v, got %v", now, info.ModTime())
	}
}

func TestStorageFileInfo_IsDir_True(t *testing.T) {
	info := &StorageFileInfo{isDir: true}
	if !info.IsDir() {
		t.Error("expected IsDir true")
	}
}

func TestStorageFileInfo_IsDir_False(t *testing.T) {
	info := &StorageFileInfo{isDir: false}
	if info.IsDir() {
		t.Error("expected IsDir false")
	}
}

func TestStorageFileInfo_Sys(t *testing.T) {
	fs := &StorageFS{}
	info := &StorageFileInfo{fs: fs}
	if info.Sys() != fs {
		t.Error("expected Sys() to return the filesystem")
	}
}

func TestStorageFileInfo_String(t *testing.T) {
	info := &StorageFileInfo{
		key:   "test.txt",
		size:  100,
		isDir: false,
	}
	s := info.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

func TestStorageFS_Schemes(t *testing.T) {
	fs := &StorageFS{}
	schemes := fs.Schemes()
	if len(schemes) != 1 || schemes[0] != "gs" {
		t.Errorf("expected ['gs'], got %v", schemes)
	}
}
