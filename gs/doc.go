// Package gs implements the golly VFS (Virtual File System) interface for Google Cloud Storage.
//
// It registers itself with the golly VFS manager on import, supporting the "gs" URL scheme.
// URLs follow the format: gs://bucket-name/key/path
//
// Configuration is resolved via the gcpsvc package. Register a gcpsvc.Config
// before using the gs package:
//
//	cfg := &gcpsvc.Config{
//		ProjectId: "my-project",
//		Location:  "us-central1",
//	}
//	cfg.SetCredentialFile("/path/to/credentials.json")
//	gcpsvc.Manager.Register("gs", cfg)
//
//	// Then use via the VFS manager:
//	file, err := vfs.GetManager().OpenRaw("gs://my-bucket/path/to/file.txt")
package gs
