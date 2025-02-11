package pubsub

import (
	"context"
	"net/url"

	"cloud.google.com/go/pubsub"
	"oss.nandlabs.io/golly-gcp/gcpsvc"
	"oss.nandlabs.io/golly/textutils"
)

func GetClient(url *url.URL) (client *pubsub.Client, err error) {
	gcpConfig := gcpsvc.Manager.Get(gcpsvc.ExtractKey(url))
	if gcpConfig.ProjectId == textutils.EmptyStr {
		gcpConfig = gcpsvc.Manager.Get("pubsub")
	}
	client, err = pubsub.NewClient(context.Background(), gcpConfig.ProjectId)
	return
}
