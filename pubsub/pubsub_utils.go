package pubsub

import (
	"context"
	"fmt"
	"net/url"

	"cloud.google.com/go/pubsub/v2"
	"oss.nandlabs.io/golly-gcp/gcpsvc"
)

const (
	// PubSubScheme is the URL scheme for Pub/Sub.
	PubSubScheme = "pubsub"
	// PubSubProviderID is the provider identifier.
	PubSubProviderID = "pubsub-provider"
)

// getPubSubClient creates a Pub/Sub client using the gcpsvc config resolved for the given URL.
// If no config is registered, it falls back to creating a client with Application Default Credentials.
func getPubSubClient(u *url.URL) (*pubsub.Client, error) {
	cfg := gcpsvc.GetConfig(u, PubSubScheme)
	if cfg == nil || cfg.ProjectId == "" {
		return nil, fmt.Errorf("pubsub: no GCP config with ProjectId registered for %v", u)
	}
	client, err := pubsub.NewClient(context.Background(), cfg.ProjectId, cfg.Options...)
	if err != nil {
		return nil, fmt.Errorf("pubsub: failed to create client: %w", err)
	}
	return client, nil
}

// resolvePublisher returns a *pubsub.Publisher for the given URL.
// URL format: pubsub://topic-name
func resolvePublisher(client *pubsub.Client, u *url.URL) (*pubsub.Publisher, error) {
	topicName := u.Host
	if topicName == "" {
		return nil, fmt.Errorf("pubsub: topic name (URL host) is required")
	}
	return client.Publisher(topicName), nil
}

// resolveSubscriber returns a *pubsub.Subscriber for the given URL.
// URL format: pubsub://subscription-name
func resolveSubscriber(client *pubsub.Client, u *url.URL) (*pubsub.Subscriber, error) {
	subName := u.Host
	if subName == "" {
		return nil, fmt.Errorf("pubsub: subscription name (URL host) is required")
	}
	return client.Subscriber(subName), nil
}
