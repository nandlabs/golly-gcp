package gs

import (
	"net/url"
	"testing"
)

func TestParseURL_Valid(t *testing.T) {
	u, _ := url.Parse("gs://my-bucket/path/to/file.txt")
	opts, err := parseURL(u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Bucket != "my-bucket" {
		t.Errorf("expected bucket 'my-bucket', got %q", opts.Bucket)
	}
	if opts.Key != "path/to/file.txt" {
		t.Errorf("expected key 'path/to/file.txt', got %q", opts.Key)
	}
}

func TestParseURL_BucketOnly(t *testing.T) {
	u, _ := url.Parse("gs://my-bucket")
	opts, err := parseURL(u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Bucket != "my-bucket" {
		t.Errorf("expected bucket 'my-bucket', got %q", opts.Bucket)
	}
	if opts.Key != "" {
		t.Errorf("expected empty key, got %q", opts.Key)
	}
}

func TestParseURL_BucketWithTrailingSlash(t *testing.T) {
	u, _ := url.Parse("gs://my-bucket/")
	opts, err := parseURL(u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Bucket != "my-bucket" {
		t.Errorf("expected bucket 'my-bucket', got %q", opts.Bucket)
	}
	if opts.Key != "" {
		t.Errorf("expected empty key for trailing slash, got %q", opts.Key)
	}
}

func TestParseURL_NilURL(t *testing.T) {
	_, err := parseURL(nil)
	if err == nil {
		t.Fatal("expected error for nil URL")
	}
}

func TestParseURL_WrongScheme(t *testing.T) {
	u, _ := url.Parse("s3://my-bucket/key")
	_, err := parseURL(u)
	if err == nil {
		t.Fatal("expected error for wrong scheme")
	}
}

func TestParseURL_NoBucket(t *testing.T) {
	u := &url.URL{Scheme: "gs"}
	_, err := parseURL(u)
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestValidateURL_Valid(t *testing.T) {
	u, _ := url.Parse("gs://bucket/key")
	if err := validateURL(u); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateURL_Nil(t *testing.T) {
	if err := validateURL(nil); err == nil {
		t.Fatal("expected error for nil URL")
	}
}

func TestValidateURL_WrongScheme(t *testing.T) {
	u, _ := url.Parse("http://bucket/key")
	if err := validateURL(u); err == nil {
		t.Fatal("expected error for wrong scheme")
	}
}

func TestValidateURL_EmptyHost(t *testing.T) {
	u := &url.URL{Scheme: "gs"}
	if err := validateURL(u); err == nil {
		t.Fatal("expected error for empty host")
	}
}
