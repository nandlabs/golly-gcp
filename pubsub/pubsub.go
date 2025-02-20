package pubsub

import (
	"context"
	"errors"
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
	subscription := client.Subscription(source.Host)

	optionResolver := messaging.NewOptionsResolver(options...)
	timeoutVal, has := optionResolver.Get("Timeout")
	if !has {
		return nil, errors.New("please provide timeout of the messages to consume")
	}
	timeout := timeoutVal.(int)
	timeoutDuration := time.Duration(timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)

	// channel to capture the message
	messageChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		err := subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
			logger.InfoF("Received message: %s\n", string(msg.Data))
			messageChan <- string(msg.Data)
			msg.Ack()
			cancel()
		})
		if err != nil {
			errChan <- err
		}
	}()

	select {
	case message := <-messageChan:
		baseMsg, _ := messaging.NewBaseMessage()
		baseMsg.SetBodyStr(string(message))
		msg = &MessagePubSub{
			BaseMessage: baseMsg,
		}
		return msg, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, errors.New("no messages found")
	}
}

// URL structure - pubsub://subscription-id
func (p *ProviderPubSub) ReceiveBatch(source *url.URL, options ...messaging.Option) (msgs []messaging.Message, err error) {
	client, err := GetClient(source)
	if err != nil {
		return
	}
	subscription := client.Subscription(source.Host)

	fmt.Println(options)
	optionResolver := messaging.NewOptionsResolver(options...)
	batchSizeVal, has := optionResolver.Get("BatchSize")
	if !has {
		// handle if the batchsize is not passed
		// do we throw an error?
		return nil, errors.New("please provide batchsize of the messages to consume")
	}

	batchSize := batchSizeVal.(int)
	timeoutVal, has := optionResolver.Get("Timeout")
	if !has {
		return nil, errors.New("please provide timeout of the messages to consume")
	}
	timeout := timeoutVal.(int)
	timeoutDuration := time.Duration(timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)

	err = subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		baseMsg, _ := messaging.NewBaseMessage()
		baseMsg.SetBodyStr(string(msg.Data))
		receivedMessage := &MessagePubSub{
			BaseMessage: baseMsg,
		}
		msgs = append(msgs, receivedMessage)

		msg.Ack()

		if len(msgs) >= batchSize {
			cancel()
		}
	})
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, errors.New("no messages found")
	}
	return
}

func (p *ProviderPubSub) AddListener(u *url.URL, listener func(msg messaging.Message), options ...messaging.Option) (err error) {
	ctx := context.Background()

	client, err := GetClient(u)
	if err != nil {
		return
	}
	subscription := client.Subscription(u.Host)
	logger.Info("Starting listener for messages..")
	err = subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		baseMsg, _ := messaging.NewBaseMessage()
		baseMsg.SetBodyStr(string(msg.Data))
		receivedMessage := &MessagePubSub{
			BaseMessage: baseMsg,
		}
		listener(receivedMessage)
		msg.Ack()
	})
	if err != nil {
		return
	}
	return
}

func (sqsp *ProviderPubSub) Close() (err error) {
	// TODO should be used to close the listener
	return
}

func (sqsp *ProviderPubSub) Id() string {
	return PubSubProvider
}
