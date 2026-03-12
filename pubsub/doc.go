// Package pubsub provides a Google Cloud Pub/Sub (v2) implementation of the golly messaging.Provider interface.
//
// It uses cloud.google.com/go/pubsub/v2 and supports publishing messages to topics
// and receiving messages from subscriptions using the standard golly messaging URL scheme:
//
//   - Publish:   pubsub://topic-name
//   - Subscribe: pubsub://subscription-name
//
// The provider auto-registers with the golly messaging manager on import.
// Configuration is resolved via gcpsvc.GetConfig using a 3-step lookup.
//
// Usage:
//
//	import _ "oss.nandlabs.io/golly-gcp/pubsub"
//
//	mgr := messaging.GetManager()
//	msg, _ := mgr.NewMessage("pubsub")
//	msg.SetBodyStr("hello world")
//	u, _ := url.Parse("pubsub://my-topic")
//	mgr.Send(u, msg)
package pubsub
