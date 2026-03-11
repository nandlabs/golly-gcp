package pubsub

import (
	"fmt"

	gpubsub "cloud.google.com/go/pubsub/v2"
	"oss.nandlabs.io/golly/messaging"
)

// MessagePubSub wraps BaseMessage and adds Pub/Sub-specific fields for acknowledgement.
type MessagePubSub struct {
	*messaging.BaseMessage
	// pubsubMessage is the underlying Pub/Sub message, used for Ack/Nack.
	// Nil for outbound (sent) messages.
	pubsubMessage *gpubsub.Message
	// messageId is the server-assigned message ID after publishing.
	messageId string
	// provider is a back-reference to the provider.
	provider *Provider
	// stringHeaders tracks all headers as string key-value pairs for Pub/Sub attributes.
	stringHeaders map[string]string
}

// trackHeader stores the string representation of a header value.
func (m *MessagePubSub) trackHeader(key, value string) {
	if m.stringHeaders == nil {
		m.stringHeaders = make(map[string]string)
	}
	m.stringHeaders[key] = value
}

func (m *MessagePubSub) SetHeader(key string, value []byte) {
	m.BaseMessage.SetHeader(key, value)
	m.trackHeader(key, string(value))
}

func (m *MessagePubSub) SetStrHeader(key string, value string) {
	m.BaseMessage.SetStrHeader(key, value)
	m.trackHeader(key, value)
}

func (m *MessagePubSub) SetBoolHeader(key string, value bool) {
	m.BaseMessage.SetBoolHeader(key, value)
	m.trackHeader(key, fmt.Sprintf("%v", value))
}

func (m *MessagePubSub) SetIntHeader(key string, value int) {
	m.BaseMessage.SetIntHeader(key, value)
	m.trackHeader(key, fmt.Sprintf("%d", value))
}

func (m *MessagePubSub) SetInt8Header(key string, value int8) {
	m.BaseMessage.SetInt8Header(key, value)
	m.trackHeader(key, fmt.Sprintf("%d", value))
}

func (m *MessagePubSub) SetInt16Header(key string, value int16) {
	m.BaseMessage.SetInt16Header(key, value)
	m.trackHeader(key, fmt.Sprintf("%d", value))
}

func (m *MessagePubSub) SetInt32Header(key string, value int32) {
	m.BaseMessage.SetInt32Header(key, value)
	m.trackHeader(key, fmt.Sprintf("%d", value))
}

func (m *MessagePubSub) SetInt64Header(key string, value int64) {
	m.BaseMessage.SetInt64Header(key, value)
	m.trackHeader(key, fmt.Sprintf("%d", value))
}

func (m *MessagePubSub) SetFloatHeader(key string, value float32) {
	m.BaseMessage.SetFloatHeader(key, value)
	m.trackHeader(key, fmt.Sprintf("%g", value))
}

func (m *MessagePubSub) SetFloat64Header(key string, value float64) {
	m.BaseMessage.SetFloat64Header(key, value)
	m.trackHeader(key, fmt.Sprintf("%g", value))
}

// Rsvp acknowledges or rejects a received Pub/Sub message.
// If accept is true, the message is acknowledged (removed from subscription).
// If accept is false, the message is nacked (made available for redelivery).
// For outbound messages (no underlying Pub/Sub message), this is a no-op.
func (m *MessagePubSub) Rsvp(accept bool, options ...messaging.Option) error {
	if m.pubsubMessage == nil {
		return nil
	}
	if accept {
		m.pubsubMessage.Ack()
	} else {
		m.pubsubMessage.Nack()
	}
	return nil
}
