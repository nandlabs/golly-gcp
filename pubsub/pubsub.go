package pubsub

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	gpubsub "cloud.google.com/go/pubsub/v2"
	"oss.nandlabs.io/golly/ioutils"
	"oss.nandlabs.io/golly/messaging"
)

// Option keys for Pub/Sub-specific options.
const (
	// OptTimeout is the overall timeout in seconds for receive/listener operations. Default: 30.
	OptTimeout = "Timeout"
	// OptBatchSize is the maximum number of messages for ReceiveBatch. Default: 10.
	OptBatchSize = "BatchSize"
	// OptOrderingKey is the ordering key for messages in ordered delivery topics.
	OptOrderingKey = "OrderingKey"
	// OptMaxExtension is the maximum duration (in seconds) that the message's ack deadline
	// will be extended. Applies to subscription receive settings.
	OptMaxExtension = "MaxExtension"
	// OptMaxOutstandingMessages is the maximum number of unprocessed messages the subscriber
	// will pull from the server before pausing.
	OptMaxOutstandingMessages = "MaxOutstandingMessages"
)

var pubsubSchemes = []string{PubSubScheme}

// Provider implements the messaging.Provider interface for Google Cloud Pub/Sub.
type Provider struct {
	closed  atomic.Bool
	mu      sync.Mutex
	stopFns []context.CancelFunc // cancel functions for active listeners
}

// Id returns the provider id.
func (p *Provider) Id() string {
	return PubSubProviderID
}

// Schemes returns the supported URL schemes.
func (p *Provider) Schemes() []string {
	return pubsubSchemes
}

// Setup performs initial setup (no-op for Pub/Sub).
func (p *Provider) Setup() error {
	return nil
}

// NewMessage creates a new Pub/Sub message.
func (p *Provider) NewMessage(scheme string, options ...messaging.Option) (messaging.Message, error) {
	baseMsg, err := messaging.NewBaseMessage()
	if err != nil {
		return nil, err
	}
	return &MessagePubSub{
		BaseMessage:   baseMsg,
		provider:      p,
		stringHeaders: make(map[string]string),
	}, nil
}

// Send publishes a single message to a Pub/Sub topic.
// URL format: pubsub://topic-name
// Supported options: OrderingKey.
func (p *Provider) Send(u *url.URL, msg messaging.Message, options ...messaging.Option) error {
	client, err := getPubSubClient(u)
	if err != nil {
		return err
	}
	defer ioutils.CloserFunc(client)

	publisher, err := resolvePublisher(client, u)
	if err != nil {
		return err
	}
	defer publisher.Stop()

	message := &gpubsub.Message{
		Data: msg.ReadBytes(),
	}

	// Apply ordering key if provided
	optResolver := messaging.NewOptionsResolver(options...)
	if v, ok := optResolver.Get(OptOrderingKey); ok {
		publisher.EnableMessageOrdering = true
		message.OrderingKey = v.(string)
	}

	// Apply attributes from message string headers
	message.Attributes = buildAttributes(msg)

	ctx := context.Background()
	result := publisher.Publish(ctx, message)
	id, err := result.Get(ctx)
	if err != nil {
		return fmt.Errorf("pubsub: publish failed: %w", err)
	}

	// Store the message ID on the message if it's a MessagePubSub
	if psMsg, ok := msg.(*MessagePubSub); ok {
		psMsg.messageId = id
	}

	logger.InfoF("Pub/Sub message published, MessageId: %s", id)
	return nil
}

// SendBatch publishes a batch of messages to a Pub/Sub topic.
// Pub/Sub batches messages automatically via the client library.
// All messages are published asynchronously and then we wait for all results.
func (p *Provider) SendBatch(u *url.URL, msgs []messaging.Message, options ...messaging.Option) error {
	if len(msgs) == 0 {
		return nil
	}

	client, err := getPubSubClient(u)
	if err != nil {
		return err
	}
	defer ioutils.CloserFunc(client)

	publisher, err := resolvePublisher(client, u)
	if err != nil {
		return err
	}
	defer publisher.Stop()

	optResolver := messaging.NewOptionsResolver(options...)
	if v, ok := optResolver.Get(OptOrderingKey); ok {
		publisher.EnableMessageOrdering = true
		_ = v // ordering key set per-message below
	}

	ctx := context.Background()
	results := make([]*gpubsub.PublishResult, len(msgs))
	for i, msg := range msgs {
		message := &gpubsub.Message{
			Data:       msg.ReadBytes(),
			Attributes: buildAttributes(msg),
		}
		if v, ok := optResolver.Get(OptOrderingKey); ok {
			message.OrderingKey = v.(string)
		}
		results[i] = publisher.Publish(ctx, message)
	}

	// Wait for all publishes to complete
	var firstErr error
	for i, result := range results {
		id, err := result.Get(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("pubsub: batch publish failed at index %d: %w", i, err)
			}
			continue
		}
		if psMsg, ok := msgs[i].(*MessagePubSub); ok {
			psMsg.messageId = id
		}
	}

	if firstErr != nil {
		return firstErr
	}

	logger.InfoF("Pub/Sub batch sent %d messages to topic %s", len(msgs), u.Host)
	return nil
}

// Receive receives a single message from a Pub/Sub subscription.
// URL format: pubsub://subscription-name
// Supported options: Timeout (seconds, required).
// The message is NOT auto-acknowledged. Call msg.Rsvp(true) to ack or msg.Rsvp(false) to nack.
func (p *Provider) Receive(u *url.URL, options ...messaging.Option) (messaging.Message, error) {
	client, err := getPubSubClient(u)
	if err != nil {
		return nil, err
	}
	defer ioutils.CloserFunc(client)

	sub, err := resolveSubscriber(client, u)
	if err != nil {
		return nil, err
	}

	optResolver := messaging.NewOptionsResolver(options...)

	// Determine timeout
	timeout := 30 * time.Second
	if v, ok := optResolver.Get(OptTimeout); ok {
		timeout = time.Duration(v.(int)) * time.Second
	}

	// We only want one message, so limit concurrency
	sub.ReceiveSettings.MaxOutstandingMessages = 1

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	msgChan := make(chan *gpubsub.Message, 1)
	errChan := make(chan error, 1)

	go func() {
		err := sub.Receive(ctx, func(_ context.Context, m *gpubsub.Message) {
			msgChan <- m
			cancel() // stop receiving after first message
		})
		if err != nil {
			errChan <- err
		}
	}()

	select {
	case m := <-msgChan:
		return p.toMessage(m), nil
	case err := <-errChan:
		return nil, fmt.Errorf("pubsub: receive failed: %w", err)
	case <-ctx.Done():
		return nil, fmt.Errorf("pubsub: no messages available within timeout")
	}
}

// ReceiveBatch receives a batch of messages from a Pub/Sub subscription.
// URL format: pubsub://subscription-name
// Supported options: BatchSize (default 10), Timeout (seconds, default 30).
// Messages are NOT auto-acknowledged. Call msg.Rsvp(true) to ack each message.
func (p *Provider) ReceiveBatch(u *url.URL, options ...messaging.Option) ([]messaging.Message, error) {
	client, err := getPubSubClient(u)
	if err != nil {
		return nil, err
	}
	defer ioutils.CloserFunc(client)

	sub, err := resolveSubscriber(client, u)
	if err != nil {
		return nil, err
	}

	optResolver := messaging.NewOptionsResolver(options...)

	batchSize := 10
	if v, ok := optResolver.Get(OptBatchSize); ok {
		batchSize = v.(int)
		if batchSize < 1 {
			batchSize = 1
		}
	}

	timeout := 30 * time.Second
	if v, ok := optResolver.Get(OptTimeout); ok {
		timeout = time.Duration(v.(int)) * time.Second
	}

	sub.ReceiveSettings.MaxOutstandingMessages = batchSize

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var (
		mu   sync.Mutex
		msgs []messaging.Message
	)

	err = sub.Receive(ctx, func(_ context.Context, m *gpubsub.Message) {
		mu.Lock()
		msgs = append(msgs, p.toMessage(m))
		if len(msgs) >= batchSize {
			cancel()
		}
		mu.Unlock()
	})
	if err != nil && ctx.Err() == nil {
		return nil, fmt.Errorf("pubsub: receive batch failed: %w", err)
	}

	if len(msgs) == 0 {
		return nil, fmt.Errorf("pubsub: no messages available within timeout")
	}

	return msgs, nil
}

// AddListener registers a listener that continuously receives messages from a Pub/Sub subscription.
// The listener runs in a goroutine and can be stopped by calling Close on the provider.
// URL format: pubsub://subscription-name
// Supported options: Timeout (total listener duration in seconds, 0 = indefinite),
// MaxOutstandingMessages, MaxExtension.
// Messages are NOT auto-acknowledged. The listener callback must call msg.Rsvp(true) to ack.
func (p *Provider) AddListener(u *url.URL, listener func(msg messaging.Message), options ...messaging.Option) error {
	client, err := getPubSubClient(u)
	if err != nil {
		return err
	}

	sub, err := resolveSubscriber(client, u)
	if err != nil {
		ioutils.CloserFunc(client)
		return err
	}

	optResolver := messaging.NewOptionsResolver(options...)

	// Configure subscriber receive settings
	if v, ok := optResolver.Get(OptMaxOutstandingMessages); ok {
		sub.ReceiveSettings.MaxOutstandingMessages = v.(int)
	}
	if v, ok := optResolver.Get(OptMaxExtension); ok {
		sub.ReceiveSettings.MaxExtension = time.Duration(v.(int)) * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	if v, ok := optResolver.Get(OptTimeout); ok {
		timeout := time.Duration(v.(int)) * time.Second
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	}

	p.mu.Lock()
	p.stopFns = append(p.stopFns, cancel)
	p.mu.Unlock()

	go func() {
		defer cancel()
		defer ioutils.CloserFunc(client)
		logger.InfoF("Pub/Sub listener started for subscription %s", u.Host)

		err := sub.Receive(ctx, func(_ context.Context, m *gpubsub.Message) {
			if p.closed.Load() {
				m.Nack()
				return
			}
			listener(p.toMessage(m))
		})
		if err != nil && ctx.Err() == nil {
			logger.ErrorF("Pub/Sub listener error: %v", err)
		}

		logger.InfoF("Pub/Sub listener stopped for subscription %s", u.Host)
	}()

	return nil
}

// Close stops all active listeners and releases resources.
func (p *Provider) Close() error {
	p.closed.Store(true)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, cancel := range p.stopFns {
		cancel()
	}
	p.stopFns = nil
	return nil
}

// toMessage converts a Pub/Sub message to a MessagePubSub.
// The underlying Pub/Sub message is retained for Ack/Nack via Rsvp.
func (p *Provider) toMessage(m *gpubsub.Message) *MessagePubSub {
	baseMsg, _ := messaging.NewBaseMessage()
	_, _ = baseMsg.SetBodyBytes(m.Data)

	// Map Pub/Sub message attributes to headers
	stringHeaders := make(map[string]string, len(m.Attributes))
	for k, v := range m.Attributes {
		baseMsg.SetStrHeader(k, v)
		stringHeaders[k] = v
	}

	return &MessagePubSub{
		BaseMessage:   baseMsg,
		pubsubMessage: m,
		messageId:     m.ID,
		provider:      p,
		stringHeaders: stringHeaders,
	}
}

// buildAttributes converts message headers to Pub/Sub message attributes.
// Returns the tracked string headers from MessagePubSub, or nil if the message
// is not a *MessagePubSub.
func buildAttributes(msg messaging.Message) map[string]string {
	if psMsg, ok := msg.(*MessagePubSub); ok {
		return psMsg.stringHeaders
	}
	return nil
}
