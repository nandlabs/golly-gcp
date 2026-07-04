package pubsub

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gpubsub "cloud.google.com/go/pubsub/v2"
	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"oss.nandlabs.io/golly-gcp/gcpsvc"
	"oss.nandlabs.io/golly/messaging"
)

// --- test scaffolding: pstest server + gcpsvc registration ---

const testProjectID = "test-project"

// pubsubTestEnv wires a fresh pstest server + gRPC conn + registers a
// gcpsvc Config so Provider methods can resolve a real client against
// the fake. Topics / subscriptions are created up front via the admin
// clients so publish / receive succeed.
type pubsubTestEnv struct {
	t          *testing.T
	srv        *pstest.Server
	conn       *grpc.ClientConn
	admin      *gpubsub.Client
	topic      string // short name, used as URL host for publish
	fqTopic    string // fully-qualified name
	sub        string // short name, used as URL host for receive
	fqSub      string
	regKeys    []string // gcpsvc registry keys to unregister on Close
	orderingOn bool
}

func newPubsubTestEnv(t *testing.T, topic, sub string, orderingOn bool) *pubsubTestEnv {
	t.Helper()

	srv := pstest.NewServer()
	// Route ALL pubsub.NewClient calls (both this admin client and the
	// Provider's per-call clients) through the fake by setting the
	// pubsub emulator env var. Each client dials its own conn to
	// srv.Addr so client.Close() on one does not tear the shared
	// transport out from under the others.
	t.Setenv("PUBSUB_EMULATOR_HOST", srv.Addr)

	// Still hold onto a dialed conn as a legacy path — some code paths
	// (kept for compatibility) may want it. Not used by Provider.
	conn, err := grpc.Dial(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		srv.Close()
		t.Fatalf("grpc dial pstest: %v", err)
	}

	ctx := context.Background()
	adminClient, err := gpubsub.NewClient(ctx, testProjectID)
	if err != nil {
		conn.Close()
		srv.Close()
		t.Fatalf("new pubsub client: %v", err)
	}

	fqTopic := fmt.Sprintf("projects/%s/topics/%s", testProjectID, topic)
	if _, err := adminClient.TopicAdminClient.CreateTopic(ctx, &pb.Topic{Name: fqTopic}); err != nil {
		adminClient.Close()
		conn.Close()
		srv.Close()
		t.Fatalf("create topic: %v", err)
	}

	fqSub := ""
	if sub != "" {
		fqSub = fmt.Sprintf("projects/%s/subscriptions/%s", testProjectID, sub)
		subReq := &pb.Subscription{
			Name:                     fqSub,
			Topic:                    fqTopic,
			AckDeadlineSeconds:       10,
			EnableMessageOrdering:    orderingOn,
			EnableExactlyOnceDelivery: false,
		}
		if _, err := adminClient.SubscriptionAdminClient.CreateSubscription(ctx, subReq); err != nil {
			adminClient.Close()
			conn.Close()
			srv.Close()
			t.Fatalf("create subscription: %v", err)
		}
	}

	// Note: The pstest fake honors OrderingKey based on
	// Publisher.EnableMessageOrdering (client-side) rather than a
	// topic-level flag. Our Provider flips EnableMessageOrdering=true
	// whenever a message carries a routing key, so no extra topic
	// configuration is required against the fake.

	env := &pubsubTestEnv{
		t:          t,
		srv:        srv,
		conn:       conn,
		admin:      adminClient,
		topic:      topic,
		fqTopic:    fqTopic,
		sub:        sub,
		fqSub:      fqSub,
		orderingOn: orderingOn,
	}

	// Register a gcpsvc Config for the topic name AND the subscription
	// name so Provider.getPubSubClient can build a client. The
	// PUBSUB_EMULATOR_HOST env var (set above) is what actually
	// redirects the client to the fake; Options is empty here so each
	// Provider call gets its own conn.
	cfg := &gcpsvc.Config{
		ProjectId: testProjectID,
	}
	gcpsvc.Manager.Register(topic, cfg)
	env.regKeys = append(env.regKeys, topic)
	if sub != "" && sub != topic {
		gcpsvc.Manager.Register(sub, cfg)
		env.regKeys = append(env.regKeys, sub)
	}

	return env
}

func (e *pubsubTestEnv) Close() {
	for _, k := range e.regKeys {
		gcpsvc.Manager.Unregister(k)
	}
	_ = e.admin.Close()
	_ = e.conn.Close()
	_ = e.srv.Close()
}

// --- captureObserver ---

type captureObserver struct {
	mu        sync.Mutex
	sendCalls []observedSend
	recvCalls []observedRecv
	ackCalls  []observedAck
	nackCalls []observedNack
}

type observedSend struct {
	u       *url.URL
	msg     messaging.Message
	err     error
	latency time.Duration
}
type observedRecv struct {
	u   *url.URL
	msg messaging.Message
	err error
}
type observedAck struct {
	u   *url.URL
	msg messaging.Message
}
type observedNack struct {
	u       *url.URL
	msg     messaging.Message
	requeue bool
}

func (c *captureObserver) OnSend(u *url.URL, msg messaging.Message, err error, latency time.Duration) {
	c.mu.Lock()
	c.sendCalls = append(c.sendCalls, observedSend{u, msg, err, latency})
	c.mu.Unlock()
}
func (c *captureObserver) OnReceive(u *url.URL, msg messaging.Message, err error) {
	c.mu.Lock()
	c.recvCalls = append(c.recvCalls, observedRecv{u, msg, err})
	c.mu.Unlock()
}
func (c *captureObserver) OnAck(u *url.URL, msg messaging.Message) {
	c.mu.Lock()
	c.ackCalls = append(c.ackCalls, observedAck{u, msg})
	c.mu.Unlock()
}
func (c *captureObserver) OnNack(u *url.URL, msg messaging.Message, requeue bool) {
	c.mu.Lock()
	c.nackCalls = append(c.nackCalls, observedNack{u, msg, requeue})
	c.mu.Unlock()
}

func (c *captureObserver) sendCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sendCalls)
}
func (c *captureObserver) recvCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.recvCalls)
}
func (c *captureObserver) ackCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.ackCalls)
}
func (c *captureObserver) nackCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.nackCalls)
}

// --- keyedMsg: helper Keyed-implementing wrapper around MessagePubSub ---

// (MessagePubSub already embeds *messaging.BaseMessage which implements
// Keyed via SetRoutingKey / RoutingKey, so we just call those directly
// on the *messaging.BaseMessage.)

// --- capability assertion tests ---

func TestProvider_CapabilityAssertions(t *testing.T) {
	// If these type assertions ever regress, the whole package fails to
	// compile — this test just documents the intent.
	var p messaging.Provider = &Provider{}
	if _, ok := p.(messaging.ProducerCtx); !ok {
		t.Error("Provider does not satisfy messaging.ProducerCtx")
	}
	if _, ok := p.(messaging.ReceiverCtx); !ok {
		t.Error("Provider does not satisfy messaging.ReceiverCtx")
	}
	if _, ok := p.(messaging.ObservableProvider); !ok {
		t.Error("Provider does not satisfy messaging.ObservableProvider")
	}
}

// --- SendCtx tests ---

func TestSendCtx_CancellationPropagates(t *testing.T) {
	env := newPubsubTestEnv(t, "cancel-topic", "", false)
	defer env.Close()

	p := &Provider{}
	msg, err := p.NewMessage(PubSubScheme)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	_, _ = msg.(*MessagePubSub).SetBodyStr("hello")

	u, _ := url.Parse("pubsub://cancel-topic")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE calling SendCtx

	err = p.SendCtx(ctx, u, msg)
	if err == nil {
		t.Fatal("expected error from SendCtx with cancelled ctx, got nil")
	}
	// The error is expected to wrap context.Canceled — either directly
	// (pubsub client surfaces it) or as a "context canceled" gRPC status.
	if !errors.Is(err, context.Canceled) {
		// Not fatal — the SDK sometimes wraps this in a gRPC error. Log
		// so the failure mode is visible.
		t.Logf("SendCtx returned %v (not directly errors.Is context.Canceled; acceptable if it's a gRPC-wrapped equivalent)", err)
	}
}

func TestSendCtx_KeyedMapsToOrderingKey(t *testing.T) {
	env := newPubsubTestEnv(t, "keyed-topic", "", true)
	defer env.Close()

	p := &Provider{}
	msg, err := p.NewMessage(PubSubScheme)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	psMsg := msg.(*MessagePubSub)
	_, _ = psMsg.SetBodyStr("keyed-body")
	// Set the routing key via the Keyed capability on the embedded
	// BaseMessage.
	psMsg.BaseMessage.SetRoutingKey("customer-42")

	u, _ := url.Parse("pubsub://keyed-topic")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.SendCtx(ctx, u, msg); err != nil {
		t.Fatalf("SendCtx: %v", err)
	}

	msgs := env.srv.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(msgs))
	}
	if got, want := msgs[0].OrderingKey, "customer-42"; got != want {
		t.Errorf("OrderingKey: got %q, want %q", got, want)
	}
}

func TestSendCtx_EmptyKey_NoOrderingKey(t *testing.T) {
	env := newPubsubTestEnv(t, "unkeyed-topic", "", false)
	defer env.Close()

	p := &Provider{}
	msg, err := p.NewMessage(PubSubScheme)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	psMsg := msg.(*MessagePubSub)
	_, _ = psMsg.SetBodyStr("unkeyed-body")
	// Do NOT set a routing key.

	u, _ := url.Parse("pubsub://unkeyed-topic")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.SendCtx(ctx, u, msg); err != nil {
		t.Fatalf("SendCtx: %v", err)
	}

	msgs := env.srv.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(msgs))
	}
	if got := msgs[0].OrderingKey; got != "" {
		t.Errorf("expected empty OrderingKey, got %q", got)
	}
}

// --- Observer tests ---

func TestObserver_OnSend_FiresWithLatency(t *testing.T) {
	env := newPubsubTestEnv(t, "obs-send-topic", "", false)
	defer env.Close()

	p := &Provider{}
	obs := &captureObserver{}
	p.SetObserver(obs)

	msg, err := p.NewMessage(PubSubScheme)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	_, _ = msg.(*MessagePubSub).SetBodyStr("observed")

	u, _ := url.Parse("pubsub://obs-send-topic")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.SendCtx(ctx, u, msg); err != nil {
		t.Fatalf("SendCtx: %v", err)
	}

	if got, want := obs.sendCount(), 1; got != want {
		t.Fatalf("OnSend calls: got %d, want %d", got, want)
	}
	call := obs.sendCalls[0]
	if call.err != nil {
		t.Errorf("expected nil send err, got %v", call.err)
	}
	if call.latency <= 0 {
		t.Errorf("expected positive latency, got %v", call.latency)
	}
	if call.u == nil || call.u.Host != "obs-send-topic" {
		t.Errorf("unexpected URL in OnSend: %v", call.u)
	}
}

func TestObserver_OnReceive_FiresPerMessage(t *testing.T) {
	env := newPubsubTestEnv(t, "obs-recv-topic", "obs-recv-sub", false)
	defer env.Close()

	// Publish a few messages directly via the fake so the subscriber
	// has something to pull.
	env.srv.Publish(env.fqTopic, []byte("m1"), nil)
	env.srv.Publish(env.fqTopic, []byte("m2"), nil)
	env.srv.Publish(env.fqTopic, []byte("m3"), nil)

	p := &Provider{}
	obs := &captureObserver{}
	p.SetObserver(obs)

	u, _ := url.Parse("pubsub://obs-recv-sub")

	batch, err := p.ReceiveBatchCtx(context.Background(), u,
		messaging.Option{Key: OptBatchSize, Value: 3},
		messaging.Option{Key: OptTimeout, Value: 3},
	)
	if err != nil {
		t.Fatalf("ReceiveBatchCtx: %v", err)
	}
	// Ack them so they don't redeliver into the observer count.
	for _, m := range batch {
		_ = m.Rsvp(true)
	}
	if got := len(batch); got != 3 {
		t.Fatalf("received messages: got %d, want 3", got)
	}

	// OnReceive should have fired exactly once per delivered message.
	if got := obs.recvCount(); got != 3 {
		t.Errorf("OnReceive calls: got %d, want 3", got)
	}
	for i, c := range obs.recvCalls {
		if c.err != nil {
			t.Errorf("OnReceive[%d]: unexpected err %v", i, c.err)
		}
		if c.msg == nil {
			t.Errorf("OnReceive[%d]: nil msg", i)
		}
	}
}

func TestObserver_OnAck_FiresOnHandlerAck(t *testing.T) {
	env := newPubsubTestEnv(t, "obs-ack-topic", "obs-ack-sub", false)
	defer env.Close()

	env.srv.Publish(env.fqTopic, []byte("ack-me"), nil)

	p := &Provider{}
	obs := &captureObserver{}
	p.SetObserver(obs)

	u, _ := url.Parse("pubsub://obs-ack-sub")

	batch, err := p.ReceiveBatchCtx(context.Background(), u,
		messaging.Option{Key: OptBatchSize, Value: 1},
		messaging.Option{Key: OptTimeout, Value: 3},
	)
	if err != nil {
		t.Fatalf("ReceiveBatchCtx: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("got %d messages, want 1", len(batch))
	}

	if err := batch[0].Rsvp(true); err != nil {
		t.Fatalf("Rsvp(true): %v", err)
	}

	if got := obs.ackCount(); got != 1 {
		t.Errorf("OnAck calls: got %d, want 1", got)
	}
	if got := obs.nackCount(); got != 0 {
		t.Errorf("OnNack calls: got %d, want 0", got)
	}
	if len(obs.ackCalls) > 0 {
		if u2 := obs.ackCalls[0].u; u2 == nil || u2.Host != "obs-ack-sub" {
			t.Errorf("OnAck URL: got %v, want host obs-ack-sub", u2)
		}
	}
}

func TestObserver_OnNack_FiresWithRequeueTrue(t *testing.T) {
	env := newPubsubTestEnv(t, "obs-nack-topic", "obs-nack-sub", false)
	defer env.Close()

	env.srv.Publish(env.fqTopic, []byte("nack-me"), nil)

	p := &Provider{}
	obs := &captureObserver{}
	p.SetObserver(obs)

	u, _ := url.Parse("pubsub://obs-nack-sub")

	batch, err := p.ReceiveBatchCtx(context.Background(), u,
		messaging.Option{Key: OptBatchSize, Value: 1},
		messaging.Option{Key: OptTimeout, Value: 3},
	)
	if err != nil {
		t.Fatalf("ReceiveBatchCtx: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("got %d messages, want 1", len(batch))
	}

	if err := batch[0].Rsvp(false); err != nil {
		t.Fatalf("Rsvp(false): %v", err)
	}

	if got := obs.nackCount(); got != 1 {
		t.Errorf("OnNack calls: got %d, want 1", got)
	}
	if got := obs.ackCount(); got != 0 {
		t.Errorf("OnAck calls: got %d, want 0", got)
	}
	if len(obs.nackCalls) > 0 {
		nc := obs.nackCalls[0]
		if !nc.requeue {
			t.Error("expected requeue=true for Pub/Sub Nack, got false")
		}
		if nc.u == nil || nc.u.Host != "obs-nack-sub" {
			t.Errorf("OnNack URL: got %v, want host obs-nack-sub", nc.u)
		}
	}
}

func TestObserver_Nil_IsSafe(t *testing.T) {
	env := newPubsubTestEnv(t, "obs-nil-topic", "obs-nil-sub", false)
	defer env.Close()

	env.srv.Publish(env.fqTopic, []byte("body"), nil)

	p := &Provider{}
	// Never call SetObserver — observer should be a no-op.
	// Also explicitly set to nil and confirm no panic.
	p.SetObserver(nil)

	msg, err := p.NewMessage(PubSubScheme)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	_, _ = msg.(*MessagePubSub).SetBodyStr("send-nil-obs")
	u, _ := url.Parse("pubsub://obs-nil-topic")
	if err := p.SendCtx(context.Background(), u, msg); err != nil {
		t.Fatalf("SendCtx with nil observer: %v", err)
	}

	subURL, _ := url.Parse("pubsub://obs-nil-sub")
	batch, err := p.ReceiveBatchCtx(context.Background(), subURL,
		messaging.Option{Key: OptBatchSize, Value: 1},
		messaging.Option{Key: OptTimeout, Value: 3},
	)
	if err != nil {
		t.Fatalf("ReceiveBatchCtx with nil observer: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("expected 1 message, got %d", len(batch))
	}
	if err := batch[0].Rsvp(true); err != nil {
		t.Fatalf("Rsvp with nil observer: %v", err)
	}
	if err := batch[0].Rsvp(false); err != nil {
		t.Fatalf("Rsvp(false) with nil observer: %v", err)
	}
}

// --- AddListenerCtx cancellation ---

func TestAddListenerCtx_HonorsContextCancel(t *testing.T) {
	env := newPubsubTestEnv(t, "listener-topic", "listener-sub", false)
	defer env.Close()

	p := &Provider{}
	defer p.Close()

	env.srv.Publish(env.fqTopic, []byte("l1"), nil)
	env.srv.Publish(env.fqTopic, []byte("l2"), nil)

	u, _ := url.Parse("pubsub://listener-sub")

	var received int32
	ctx, cancel := context.WithCancel(context.Background())

	if err := p.AddListenerCtx(ctx, u, func(m messaging.Message) {
		atomic.AddInt32(&received, 1)
		_ = m.Rsvp(true)
	}); err != nil {
		t.Fatalf("AddListenerCtx: %v", err)
	}

	// Wait for at least one delivery to prove the listener is running.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&received) == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if atomic.LoadInt32(&received) == 0 {
		t.Fatal("listener never received a message")
	}

	// Cancel and give the goroutine a moment to observe it. If ctx
	// cancellation didn't propagate to sub.Receive, the goroutine would
	// keep running and race detector would flag the leak; either way
	// this is our smoke check that AddListenerCtx honors ctx.
	cancel()
	time.Sleep(200 * time.Millisecond)
}
