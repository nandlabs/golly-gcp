package pubsub

import "oss.nandlabs.io/golly/messaging"

type MessagePubSub struct {
	*messaging.BaseMessage
}

func (m *MessagePubSub) Rsvp(yes bool, options ...messaging.Option) (err error) {
	return
}
