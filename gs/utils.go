package gs

import (
	"context"
	"errors"
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

func GetConfigFromStrUrl(u string) (gcpConfig gcpsvc.Config, err error) {
	parsedUrl, err := url.Parse(u)
	if err != nil {
		return
	}
	return GetConfig(parsedUrl)
}

func GetConfig(u *url.URL) (gcpConfig gcpsvc.Config, err error) {
	urlOpts, err := parseUrl(u)
	if err != nil {
		logger.ErrorF("error during parse url: %v", err)
		return
	}
	gcpConfig = gcpsvc.Manager.Get(gcpsvc.ExtractKey(urlOpts.u))
	if gcpConfig.ProjectId == textutils.EmptyStr {
		gcpConfig = gcpsvc.Manager.Get("gs")
	}
	return
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

	path := strings.TrimPrefix(url.String(), "gs://")
	components := strings.Split(path, "/")

	bucketName := components[0]
	objectPath := strings.Join(components[1:], "/")

	// TODO add a check if the object path is actually a file or a folder
	// file should contain the extension and filder should contain / in the end

	opts = &UrlOpts{
		u:      url,
		Host:   host,
		Bucket: bucketName,
		Key:    objectPath,
	}
	return
}

func validateUrl(u *url.URL) (err error) {
	storageUrl := u.String()
	if !strings.HasPrefix(storageUrl, "gs://") {
		return errors.New("invalid URL format, must start with 'storage://'")
	}

	path := strings.TrimPrefix(storageUrl, "gs://")
	components := strings.Split(path, "/")

	if len(components) < 1 {
		return errors.New("invalid URL, must specify at least a bucket name")
	}

	// Extract bucket name and the "path" inside the bucket
	objectPath := strings.Join(components[1:], "/")

	// Determine if the last component is a bucket or file
	if objectPath == "" {
		// No object path provided, create an empty bucket
		// do not allow to create a bucket
		// throw error
		logger.Error("cannot create a bucket. please use console for the same")
		return errors.New("cannot create a bucket. please use console for the same")
	}
	return
}

func (urlOpts *UrlOpts) CreateStorageClient() (client *storage.Client, err error) {
	gcpConfig := gcpsvc.Manager.Get(gcpsvc.ExtractKey(urlOpts.u))
	if gcpConfig.ProjectId == textutils.EmptyStr {
		gcpConfig = gcpsvc.Manager.Get("gs")
	}
	// make it 3 step verification check
	// check for url.host, gs, if none is present then set default conifg
	client, err = storage.NewClient(context.Background(), gcpConfig.Options...)
	return
}
