package pubsub

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"cloud.google.com/go/pubsub"
	"oss.nandlabs.io/golly/messaging"
)

const (
	SchemesPubSub  = "pubsub"
	PubSubProvider = "pubsub-provider"
)

var pubsubSchemes = []string{SchemesPubSub}

type ProviderPubSub struct{}

func (p *ProviderPubSub) Schemes() (schemes []string) {
	schemes = pubsubSchemes
	return
}

func (p *ProviderPubSub) Setup() (err error) {
	return nil
}

func (p *ProviderPubSub) NewMessage(scheme string, options ...messaging.Option) (msg messaging.Message, err error) {
	baseMsg, err := messaging.NewBaseMessage()
	if err == nil {
		msg = &MessagePubSub{
			BaseMessage: baseMsg,
		}
	}
	return
}

// URL structure - pubsub://topic-id
func (p *ProviderPubSub) Send(url *url.URL, msg messaging.Message, options ...messaging.Option) (err error) {
	client, err := GetClient(url)
	if err != nil {
		return
	}
	// defer p.Close()
	topic := client.Topic(url.Host)

	message := &pubsub.Message{
		Data: []byte(msg.ReadBytes()),
	}
	ctx := context.Background()
	result := topic.Publish(ctx, message)
	id, err := result.Get(ctx)
	if err != nil {
		logger.Error(err)
		return
	}
	logger.InfoF("Message pushlished with ID : %s", id)
	return
}

// URL structure - pubsub://topic-id
func (p *ProviderPubSub) SendBatch(url *url.URL, msgs []messaging.Message, options ...messaging.Option) (err error) {
	client, err := GetClient(url)
	if err != nil {
		return
	}
	// defer p.Close()
	topic := client.Topic(url.Host)
	ctx := context.Background()
	for _, msg := range msgs {
		message := &pubsub.Message{
			Data: []byte(msg.ReadBytes()),
		}
		result := topic.Publish(ctx, message)
		id, err := result.Get(ctx)
		if err != nil {
			logger.Error(err)
			return err
		}
		logger.InfoF("Message pushlished with ID : %s", id)
	}
	return
}

// URL structure - pubsub://subscription-id
func (p *ProviderPubSub) Receive(source *url.URL, options ...messaging.Option) (msg messaging.Message, err error) {
	client, err := GetClient(source)
	if err != nil {
		return
	}
	ctx := context.Background()
	subscription := client.Subscription(source.Host)
	err = subscription.Receive(ctx, func(context context.Context, message *pubsub.Message) {
		logger.InfoF("Received message: %s\n", string(message.Data))
		// Acknowledge the message
		message.Ack()
	})
	if err != nil {
		logger.ErrorF("failed to receive message: %w", err)
		return
	}
	return
}

// URL structure - pubsub://subscription-id
func (p *ProviderPubSub) ReceiveBatch(source *url.URL, options ...messaging.Option) (msgs []messaging.Message, err error) {
	client, err := GetClient(source)
	if err != nil {
		return
	}
	ctx := context.Background()
	subscription := client.Subscription(source.Host)

	// TODO How to consume multiple messages
	err = subscription.Receive(ctx, func(context context.Context, message *pubsub.Message) {
		logger.InfoF("recieved %q", message.Data)
		message.Ack()
	})
	if err != nil {
		logger.ErrorF("error during recieved: %v", err)
	}
	return
}

func (p *ProviderPubSub) AddListener(url *url.URL, listener func(msg messaging.Message), options ...messaging.Option) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			msgs, err := p.ReceiveBatch(url, options...)
			if err != nil {
				return fmt.Errorf("failed to receive messages: %w", err)
			}
			for _, msg := range msgs {
				listener(msg)
			}

			if len(msgs) == 0 {
				time.Sleep(1 * time.Second)
			}
		}
	}
}

func (sqsp *ProviderPubSub) Close() (err error) {
	// TODO should be used to close the listener
	return
}

func (sqsp *ProviderPubSub) Id() string {
	return PubSubProvider
}
