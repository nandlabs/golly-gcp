package gs

import (
	"errors"
	"io/fs"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"oss.nandlabs.io/golly/vfs"
)

func TestMapGCSErr_Nil(t *testing.T) {
	if err := mapGCSErr(nil); err != nil {
		t.Errorf("mapGCSErr(nil) = %v, want nil", err)
	}
}

func TestMapGCSErr_ObjectNotExist_MapsToNotExist(t *testing.T) {
	got := mapGCSErr(storage.ErrObjectNotExist)
	if !errors.Is(got, vfs.ErrNotExist) {
		t.Errorf("ErrObjectNotExist did not map to vfs.ErrNotExist: %v", got)
	}
	// vfs.ErrNotExist wraps fs.ErrNotExist, so stdlib checks still work.
	if !errors.Is(got, fs.ErrNotExist) {
		t.Error("mapped error should still satisfy fs.ErrNotExist")
	}
	// The original SDK sentinel remains in the chain.
	if !errors.Is(got, storage.ErrObjectNotExist) {
		t.Error("mapped error should still satisfy storage.ErrObjectNotExist")
	}
}

func TestMapGCSErr_BucketNotExist_MapsToNotExist(t *testing.T) {
	got := mapGCSErr(storage.ErrBucketNotExist)
	if !errors.Is(got, vfs.ErrNotExist) {
		t.Errorf("ErrBucketNotExist did not map to vfs.ErrNotExist: %v", got)
	}
	if !errors.Is(got, storage.ErrBucketNotExist) {
		t.Error("mapped error should still satisfy storage.ErrBucketNotExist")
	}
}

func TestMapGCSErr_HTTP404_MapsToNotExist(t *testing.T) {
	src := &googleapi.Error{Code: 404, Message: "not found"}
	got := mapGCSErr(src)
	if !errors.Is(got, vfs.ErrNotExist) {
		t.Errorf("HTTP 404 did not map to vfs.ErrNotExist: %v", got)
	}
	if !errors.Is(got, fs.ErrNotExist) {
		t.Error("mapped error should still satisfy fs.ErrNotExist")
	}
	// The original googleapi.Error remains in the chain.
	var gErr *googleapi.Error
	if !errors.As(got, &gErr) {
		t.Error("wrapped error should still be a *googleapi.Error")
	}
}

func TestMapGCSErr_HTTP403_MapsToPermission(t *testing.T) {
	src := &googleapi.Error{Code: 403, Message: "forbidden"}
	got := mapGCSErr(src)
	if !errors.Is(got, vfs.ErrPermission) {
		t.Errorf("HTTP 403 did not map to vfs.ErrPermission: %v", got)
	}
	if !errors.Is(got, fs.ErrPermission) {
		t.Error("mapped error should still satisfy fs.ErrPermission")
	}
}

func TestMapGCSErr_HTTP401_MapsToPermission(t *testing.T) {
	src := &googleapi.Error{Code: 401, Message: "unauthorized"}
	got := mapGCSErr(src)
	if !errors.Is(got, vfs.ErrPermission) {
		t.Errorf("HTTP 401 did not map to vfs.ErrPermission: %v", got)
	}
}

func TestMapGCSErr_UnknownCode_PassthroughUnchanged(t *testing.T) {
	src := &googleapi.Error{Code: 500, Message: "internal"}
	got := mapGCSErr(src)
	if errors.Is(got, vfs.ErrNotExist) || errors.Is(got, vfs.ErrPermission) {
		t.Errorf("unknown-code error should not map to a sentinel: %v", got)
	}
	if !errors.Is(got, src) {
		t.Error("unknown-code error should be returned unchanged (same instance)")
	}
}

func TestMapGCSErr_UnknownError_PassthroughUnchanged(t *testing.T) {
	src := errors.New("some random error unrelated to GCS")
	got := mapGCSErr(src)
	if errors.Is(got, vfs.ErrNotExist) || errors.Is(got, vfs.ErrPermission) {
		t.Errorf("unknown error should not map to a sentinel: %v", got)
	}
	if !errors.Is(got, src) {
		t.Error("unknown error should be returned unchanged (same instance)")
	}
}
