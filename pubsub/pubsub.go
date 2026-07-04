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
//
// It also satisfies the following v1.7.0 capability interfaces:
//   - messaging.ProducerCtx (context-aware send)
//   - messaging.ReceiverCtx (context-aware receive / listener teardown)
//   - messaging.ObservableProvider (observer hooks around Send/Receive/Ack/Nack)
//
// Keyed messages (messaging.Keyed) are mapped to Pub/Sub's OrderingKey.
// Note: the topic must be created with EnableMessageOrdering=true for
// keyed delivery — the GCP SDK returns an error otherwise, which this
// provider surfaces as-is.
type Provider struct {
	closed  atomic.Bool
	mu      sync.Mutex
	stopFns []context.CancelFunc // cancel functions for active listeners

	// observer holds a messaging.Observer for metrics / tracing hooks.
	// atomic.Value wraps observerHolder so callers can install or clear
	// the observer concurrently with hook sites reading it.
	observer atomic.Value // observerHolder
}

// observerHolder wraps a messaging.Observer so atomic.Value can store nil-safely.
type observerHolder struct{ obs messaging.Observer }

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
//
// Send delegates to SendCtx with context.Background().
func (p *Provider) Send(u *url.URL, msg messaging.Message, options ...messaging.Option) error {
	return p.SendCtx(context.Background(), u, msg, options...)
}

// SendBatch publishes a batch of messages to a Pub/Sub topic.
//
// SendBatch delegates to SendBatchCtx with context.Background().
func (p *Provider) SendBatch(u *url.URL, msgs []messaging.Message, options ...messaging.Option) error {
	return p.SendBatchCtx(context.Background(), u, msgs, options...)
}

// Receive receives a single message from a Pub/Sub subscription.
// URL format: pubsub://subscription-name
// Supported options: Timeout (seconds, required).
// The message is NOT auto-acknowledged. Call msg.Rsvp(true) to ack or msg.Rsvp(false) to nack.
//
// Receive delegates to ReceiveCtx with context.Background().
func (p *Provider) Receive(u *url.URL, options ...messaging.Option) (messaging.Message, error) {
	return p.ReceiveCtx(context.Background(), u, options...)
}

// ReceiveBatch receives a batch of messages from a Pub/Sub subscription.
// URL format: pubsub://subscription-name
// Supported options: BatchSize (default 10), Timeout (seconds, default 30).
// Messages are NOT auto-acknowledged. Call msg.Rsvp(true) to ack each message.
//
// ReceiveBatch delegates to ReceiveBatchCtx with context.Background().
func (p *Provider) ReceiveBatch(u *url.URL, options ...messaging.Option) ([]messaging.Message, error) {
	return p.ReceiveBatchCtx(context.Background(), u, options...)
}

// AddListener registers a listener that continuously receives messages from a Pub/Sub subscription.
//
// AddListener delegates to AddListenerCtx with context.Background(). The
// listener still stops when the Provider is Close()'d, but does not
// otherwise honor cancellation. Callers who need per-listener shutdown
// should use AddListenerCtx directly.
func (p *Provider) AddListener(u *url.URL, listener func(msg messaging.Message), options ...messaging.Option) error {
	return p.AddListenerCtx(context.Background(), u, listener, options...)
}

// SendCtx publishes a single message with context support. If msg implements
// messaging.Keyed and RoutingKey() is non-empty, the value is set as the
// Pub/Sub OrderingKey (the topic must have EnableMessageOrdering=true or
// the SDK returns an error). Fires the observer's OnSend hook.
func (p *Provider) SendCtx(ctx context.Context, u *url.URL, msg messaging.Message, options ...messaging.Option) (err error) {
	start := time.Now()
	defer func() {
		p.getObserver().OnSend(u, msg, err, time.Since(start))
	}()

	client, cerr := getPubSubClient(u)
	if cerr != nil {
		err = cerr
		return
	}
	defer ioutils.CloserFunc(client)

	publisher, perr := resolvePublisher(client, u)
	if perr != nil {
		err = perr
		return
	}
	defer publisher.Stop()

	message := &gpubsub.Message{
		Data: msg.ReadBytes(),
	}

	// Apply ordering key: prefer explicit option, else Keyed capability.
	optResolver := messaging.NewOptionsResolver(options...)
	if v, ok := optResolver.Get(OptOrderingKey); ok {
		publisher.EnableMessageOrdering = true
		message.OrderingKey = v.(string)
	} else if k := messaging.RoutingKeyOf(msg); k != "" {
		publisher.EnableMessageOrdering = true
		message.OrderingKey = k
	}

	// Apply attributes from message string headers
	message.Attributes = buildAttributes(msg)

	result := publisher.Publish(ctx, message)
	id, perr := result.Get(ctx)
	if perr != nil {
		err = fmt.Errorf("pubsub: publish failed: %w", perr)
		return
	}

	// Store the message ID on the message if it's a MessagePubSub
	if psMsg, ok := msg.(*MessagePubSub); ok {
		psMsg.messageId = id
	}

	logger.InfoF("Pub/Sub message published, MessageId: %s", id)
	return
}

// SendBatchCtx publishes a batch of messages with context support.
// Each message honors messaging.Keyed the same way as SendCtx. The
// observer's OnSend hook fires once per successfully-published message
// (matching the LocalProvider semantics: hook is per-message).
func (p *Provider) SendBatchCtx(ctx context.Context, u *url.URL, msgs []messaging.Message, options ...messaging.Option) error {
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
	explicitKey, hasExplicitKey := optResolver.Get(OptOrderingKey)

	obs := p.getObserver()

	// Enable ordering upfront if any message will carry an ordering key,
	// so the publisher applies the correct sequencing.
	if hasExplicitKey {
		publisher.EnableMessageOrdering = true
	} else {
		for _, m := range msgs {
			if k := messaging.RoutingKeyOf(m); k != "" {
				publisher.EnableMessageOrdering = true
				break
			}
		}
	}

	results := make([]*gpubsub.PublishResult, len(msgs))
	starts := make([]time.Time, len(msgs))
	for i, msg := range msgs {
		message := &gpubsub.Message{
			Data:       msg.ReadBytes(),
			Attributes: buildAttributes(msg),
		}
		if hasExplicitKey {
			message.OrderingKey = explicitKey.(string)
		} else if k := messaging.RoutingKeyOf(msg); k != "" {
			message.OrderingKey = k
		}
		starts[i] = time.Now()
		results[i] = publisher.Publish(ctx, message)
	}

	// Wait for all publishes to complete
	var firstErr error
	for i, result := range results {
		id, perr := result.Get(ctx)
		latency := time.Since(starts[i])
		if perr != nil {
			obs.OnSend(u, msgs[i], perr, latency)
			if firstErr == nil {
				firstErr = fmt.Errorf("pubsub: batch publish failed at index %d: %w", i, perr)
			}
			continue
		}
		obs.OnSend(u, msgs[i], nil, latency)
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

// ReceiveCtx receives a single message with context support. ctx is
// composed with the OptTimeout deadline: cancelling ctx aborts the
// receive early. Fires the observer's OnReceive hook.
func (p *Provider) ReceiveCtx(ctx context.Context, u *url.URL, options ...messaging.Option) (messaging.Message, error) {
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
	// Force immediate shutdown when ctx cancels: this method hands the
	// message to the caller unacked, so from the Subscriber's POV every
	// callback returns with an in-flight message. Without a 0-timeout
	// shutdown the Subscriber would block waiting for those messages to
	// be acked, which never happens on this synchronous return path.
	sub.ReceiveSettings.ShutdownOptions = &gpubsub.ShutdownOptions{Timeout: 0}

	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	msgChan := make(chan *gpubsub.Message, 1)
	errChan := make(chan error, 1)

	obs := p.getObserver()

	go func() {
		rerr := sub.Receive(rctx, func(_ context.Context, m *gpubsub.Message) {
			select {
			case msgChan <- m:
			default:
				// buffer already full — nack this extra so it can redeliver
				m.Nack()
			}
			cancel() // stop receiving after first message
		})
		if rerr != nil {
			errChan <- rerr
		}
	}()

	select {
	case m := <-msgChan:
		out := p.toMessage(u, m)
		obs.OnReceive(u, out, nil)
		return out, nil
	case rerr := <-errChan:
		e := fmt.Errorf("pubsub: receive failed: %w", rerr)
		obs.OnReceive(u, nil, e)
		return nil, e
	case <-rctx.Done():
		if ctx.Err() != nil {
			obs.OnReceive(u, nil, ctx.Err())
			return nil, ctx.Err()
		}
		e := fmt.Errorf("pubsub: no messages available within timeout")
		obs.OnReceive(u, nil, e)
		return nil, e
	}
}

// ReceiveBatchCtx receives a batch of messages with context support.
// ctx is composed with the OptTimeout deadline. Fires OnReceive per
// message.
func (p *Provider) ReceiveBatchCtx(ctx context.Context, u *url.URL, options ...messaging.Option) ([]messaging.Message, error) {
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
	// See ReceiveCtx: force immediate shutdown so cancelled receives
	// don't block on unacked messages that this method hands back to
	// the caller for later Rsvp.
	sub.ReceiveSettings.ShutdownOptions = &gpubsub.ShutdownOptions{Timeout: 0}

	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	obs := p.getObserver()

	var (
		mu   sync.Mutex
		msgs []messaging.Message
	)

	err = sub.Receive(rctx, func(_ context.Context, m *gpubsub.Message) {
		mu.Lock()
		out := p.toMessage(u, m)
		msgs = append(msgs, out)
		size := len(msgs)
		mu.Unlock()
		obs.OnReceive(u, out, nil)
		if size >= batchSize {
			cancel()
		}
	})
	if err != nil && rctx.Err() == nil {
		e := fmt.Errorf("pubsub: receive batch failed: %w", err)
		obs.OnReceive(u, nil, e)
		return nil, e
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if len(msgs) == 0 {
		e := fmt.Errorf("pubsub: no messages available within timeout")
		obs.OnReceive(u, nil, e)
		return nil, e
	}

	return msgs, nil
}

// AddListenerCtx registers a listener with context support. Cancelling
// ctx tears down the underlying streaming pull. Provider.Close still
// stops all active listeners (including ones started via AddListener).
func (p *Provider) AddListenerCtx(ctx context.Context, u *url.URL, listener func(msg messaging.Message), options ...messaging.Option) error {
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

	// Derive lctx from caller's ctx so cancellation is honored. If a
	// timeout was requested, wrap ctx with it.
	var (
		lctx   context.Context
		cancel context.CancelFunc
	)
	if v, ok := optResolver.Get(OptTimeout); ok {
		timeout := time.Duration(v.(int)) * time.Second
		lctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		lctx, cancel = context.WithCancel(ctx)
	}

	p.mu.Lock()
	p.stopFns = append(p.stopFns, cancel)
	p.mu.Unlock()

	obs := p.getObserver()

	go func() {
		defer cancel()
		defer ioutils.CloserFunc(client)
		logger.InfoF("Pub/Sub listener started for subscription %s", u.Host)

		rerr := sub.Receive(lctx, func(_ context.Context, m *gpubsub.Message) {
			if p.closed.Load() {
				m.Nack()
				return
			}
			out := p.toMessage(u, m)
			obs.OnReceive(u, out, nil)
			listener(out)
		})
		if rerr != nil && lctx.Err() == nil {
			logger.ErrorF("Pub/Sub listener error: %v", rerr)
		}

		logger.InfoF("Pub/Sub listener stopped for subscription %s", u.Host)
	}()

	return nil
}

// SetObserver installs (or clears, with nil) the observer for metrics /
// tracing hooks. Safe for concurrent calls; hook sites snapshot the
// current observer.
func (p *Provider) SetObserver(obs messaging.Observer) {
	p.observer.Store(observerHolder{obs: obs})
}

// getObserver returns the currently-installed observer, or a no-op
// implementation when none is installed. Hook sites call this and
// invoke the returned observer unconditionally.
func (p *Provider) getObserver() messaging.Observer {
	if v := p.observer.Load(); v != nil {
		h := v.(observerHolder)
		if h.obs != nil {
			return h.obs
		}
	}
	return noopObserver{}
}

// noopObserver is a local sentinel used when no observer is installed.
type noopObserver struct{}

func (noopObserver) OnSend(*url.URL, messaging.Message, error, time.Duration) {}
func (noopObserver) OnReceive(*url.URL, messaging.Message, error)             {}
func (noopObserver) OnAck(*url.URL, messaging.Message)                        {}
func (noopObserver) OnNack(*url.URL, messaging.Message, bool)                 {}

// Compile-time assertions that Provider satisfies the v1.7.0 capability
// interfaces.
var (
	_ messaging.Provider           = (*Provider)(nil)
	_ messaging.ProducerCtx        = (*Provider)(nil)
	_ messaging.ReceiverCtx        = (*Provider)(nil)
	_ messaging.ObservableProvider = (*Provider)(nil)
)

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
// The underlying Pub/Sub message is retained for Ack/Nack via Rsvp. u
// is the source subscription URL, stored on the wrapper so observer
// hooks fired from Rsvp know which subscription the message came from.
func (p *Provider) toMessage(u *url.URL, m *gpubsub.Message) *MessagePubSub {
	baseMsg, _ := messaging.NewBaseMessage()
	_, _ = baseMsg.SetBodyBytes(m.Data)

	// Map Pub/Sub message attributes to headers
	stringHeaders := make(map[string]string, len(m.Attributes))
	for k, v := range m.Attributes {
		baseMsg.SetStrHeader(k, v)
		stringHeaders[k] = v
	}

	// Preserve the OrderingKey via the Keyed capability so downstream
	// consumers can read it back with messaging.RoutingKeyOf.
	if m.OrderingKey != "" {
		baseMsg.SetRoutingKey(m.OrderingKey)
	}

	return &MessagePubSub{
		BaseMessage:   baseMsg,
		pubsubMessage: m,
		messageId:     m.ID,
		provider:      p,
		stringHeaders: stringHeaders,
		rsvpURL:       u,
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
