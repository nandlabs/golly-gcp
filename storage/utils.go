package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"cloud.google.com/go/storage"
	"oss.nandlabs.io/golly-gcp/gcpsvc"
	"oss.nandlabs.io/golly/textutils"
)

type UrlOpts struct {
	u      *url.URL
	Host   string
	Bucket string
	Key    string
}

// Structure of URL will be
// storage://bucketName/folderName.../fileName
// there can be multiple folders

func parseUrl(url *url.URL) (opts *UrlOpts, err error) {
	err = validateUrl(url)
	if err != nil {
		return
	}
	host := url.Host
	pathParams := strings.Split(url.Path, "/")
	bucket := pathParams[0]
	var b bytes.Buffer
	for _, item := range pathParams[1:] {
		b.WriteString("/")
		b.WriteString(item)
	}
	key := b.String()
	opts = &UrlOpts{
		u:      url,
		Host:   host,
		Bucket: bucket,
		Key:    key,
	}
	return
}

func validateUrl(u *url.URL) (err error) {
	pathsElements := strings.Split(u.Path, "/")
	if len(pathsElements) == 1 {
		// only bucket provided
		return nil
	} else if len(pathsElements) >= 2 {
		// Bucket and object path provided
		return nil
	} else {
		//return error as it's not a valid url with bucket missing
		return errors.New("invalid url with bucket missing")
	}
}

func (urlOpts *UrlOpts) CreateStorageClient() (client *storage.Client, err error) {
	gcpConfig := gcpsvc.Manager.Get(gcpsvc.ExtractKey(urlOpts.u))
	if gcpConfig.ProjectId == textutils.EmptyStr {
		gcpConfig = gcpsvc.Manager.Get("gcs")
	}
	client, err = storage.NewClient(context.Background(), gcpConfig.Options...)
	return
}

func handleStoragePath(ctx context.Context, client *storage.Client, storageURL string) error {
	// Validate and parse the URL
	if !strings.HasPrefix(storageURL, "gcs://") {
		return fmt.Errorf("invalid URL format, must start with 'storage://'")
	}

	path := strings.TrimPrefix(storageURL, "gcs://")
	components := strings.Split(path, "/")

	if len(components) < 1 {
		return fmt.Errorf("invalid URL, must specify at least a bucket name")
	}

	// Extract bucket name and the "path" inside the bucket
	bucketName := components[0]
	objectPath := strings.Join(components[1:], "/")

	// Determine if the last component is a bucket or file
	if objectPath == "" {
		// No object path provided, create an empty bucket
		return createBucket(ctx, client, bucketName)
	}

	// If objectPath is provided, treat it as a file to create
	return createFile(ctx, client, bucketName, objectPath)
}

func createBucket(ctx context.Context, client *storage.Client, bucketName string) (err error) {
	// create the bucket
	err = client.Bucket(bucketName).Create(ctx, "project-id", nil)
	if err != nil {
		logger.ErrorF("failed to create bucket %q: %v", bucketName, err)
		return
	}
	logger.InfoF("Bucket %q created successfully.", bucketName)
	return
}

func createFile(ctx context.Context, client *storage.Client, bucketName string, objectName string) (err error) {
	bucket := client.Bucket(bucketName)
	object := bucket.Object(objectName)

	wc := object.NewWriter(ctx)

	if err = wc.Close(); err != nil {
		logger.ErrorF("failed to close writer: %v", err)
		return
	}
	return
}
