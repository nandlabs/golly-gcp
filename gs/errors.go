package gs

import (
	"errors"
	"fmt"
	"net/http"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"oss.nandlabs.io/golly/vfs"
)

// mapGCSErr translates a Google Cloud Storage SDK error into a golly/vfs
// sentinel so callers can switch on outcomes with errors.Is across
// backends. It returns nil for a nil input, and returns the original
// error wrapped with a matching vfs sentinel when the GCS error maps
// cleanly. If no mapping is known, the original error is returned
// unchanged so callers still see the SDK detail.
//
// Callers should wrap SDK errors at each callsite:
//
//	if _, err := obj.Attrs(ctx); err != nil {
//	    return nil, mapGCSErr(err)
//	}
//
// The returned error preserves the original chain (via fmt.Errorf %w),
// so errors.Is(err, vfs.ErrNotExist) AND the SDK-typed error assertion
// both continue to work.
func mapGCSErr(err error) error {
	if err == nil {
		return nil
	}

	// Prefer the typed sentinels the storage SDK exports — stable
	// across SDK versions.
	if errors.Is(err, storage.ErrObjectNotExist) ||
		errors.Is(err, storage.ErrBucketNotExist) {
		return fmt.Errorf("%w: %w", vfs.ErrNotExist, err)
	}

	// Fallback: some errors surface only as googleapi.Error with an
	// HTTP status. Map 404 / 403 / 401 to the matching sentinel.
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		switch gErr.Code {
		case http.StatusNotFound:
			return fmt.Errorf("%w: %w", vfs.ErrNotExist, err)
		case http.StatusForbidden, http.StatusUnauthorized:
			return fmt.Errorf("%w: %w", vfs.ErrPermission, err)
		}
	}

	return err
}
