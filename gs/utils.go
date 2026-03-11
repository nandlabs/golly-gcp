package gs

import (
	"errors"
	"net/url"
	"strings"
)

const (
	// GsScheme is the URL scheme for Google Cloud Storage resources.
	GsScheme = "gs"
)

// urlOpts holds parsed GCS URL components.
type urlOpts struct {
	u      *url.URL
	Bucket string
	Key    string
}

// parseURL parses a GCS URL into its bucket and key components.
// Expected format: gs://bucket-name/key/path
func parseURL(u *url.URL) (opts *urlOpts, err error) {
	if err = validateURL(u); err != nil {
		return
	}

	bucket := u.Host
	key := strings.TrimPrefix(u.Path, "/")

	opts = &urlOpts{
		u:      u,
		Bucket: bucket,
		Key:    key,
	}
	return
}

// validateURL checks that the URL is a valid GCS URL.
func validateURL(u *url.URL) error {
	if u == nil {
		return errors.New("url cannot be nil")
	}
	if u.Scheme != GsScheme {
		return errors.New("invalid URL scheme, expected 'gs'")
	}
	if u.Host == "" {
		return errors.New("invalid GCS URL, bucket name (host) is required")
	}
	return nil
}
