package gcslock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeBackend simulates GCS generation semantics against an in-memory
// object slot. Every mutating operation advances the generation; every
// precondition check must match the current generation exactly.
type fakeBackend struct {
	mu       sync.Mutex
	exists   bool
	body     []byte
	gen      int64
	calls    atomic.Int64
	failNext atomic.Int64
}

var errInjected = errors.New("injected transport error")

func newFakeBackend() *fakeBackend { return &fakeBackend{gen: 100} }

func (b *fakeBackend) read(ctx context.Context) ([]byte, int64, bool, error) {
	b.calls.Add(1)
	if n := b.failNext.Load(); n > 0 {
		b.failNext.Add(-1)
		return nil, 0, false, errInjected
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.exists {
		return nil, 0, false, nil
	}
	buf := make([]byte, len(b.body))
	copy(buf, b.body)
	return buf, b.gen, true, nil
}

func (b *fakeBackend) createIfMissing(ctx context.Context, body []byte) (int64, error) {
	b.calls.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.exists {
		return 0, errPreconditionFailed
	}
	b.exists = true
	b.body = append([]byte(nil), body...)
	b.gen++
	return b.gen, nil
}

func (b *fakeBackend) writeIfGenerationMatch(ctx context.Context, body []byte, gen int64) (int64, error) {
	b.calls.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.exists || b.gen != gen {
		return 0, errPreconditionFailed
	}
	b.body = append([]byte(nil), body...)
	b.gen++
	return b.gen, nil
}

func (b *fakeBackend) deleteIfGenerationMatch(ctx context.Context, gen int64) error {
	b.calls.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.exists {
		return nil
	}
	if b.gen != gen {
		return errPreconditionFailed
	}
	b.exists = false
	b.body = nil
	b.gen++
	return nil
}

// snapshot returns a copy of the fake's stored state.
func (b *fakeBackend) snapshot() (exists bool, body []byte, gen int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.exists {
		return false, nil, b.gen
	}
	buf := make([]byte, len(b.body))
	copy(buf, b.body)
	return true, buf, b.gen
}

type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock(t time.Time) *mockClock { return &mockClock{now: t} }

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestElector(be backend, owner string, clock *mockClock) *Elector {
	e := newWithBackend(be, Options{
		Bucket: "test-bucket",
		Object: "chrono/sched.lock",
		Owner:  owner,
		Lease:  10 * time.Second,
	})
	e.now = clock.Now
	return e
}

func TestClaim_FirstAcquirer(t *testing.T) {
	be := newFakeBackend()
	clock := newMockClock(time.Unix(1_700_000_000, 0))
	e := newTestElector(be, "instance-a", clock)

	if err := e.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !e.IsLeader(context.Background()) {
		t.Fatalf("expected leader after first tick")
	}
	exists, body, _ := be.snapshot()
	if !exists {
		t.Fatalf("expected object created")
	}
	doc := parseDoc(body)
	if doc.Owner != "instance-a" {
		t.Fatalf("owner = %q, want instance-a", doc.Owner)
	}
	if want := clock.Now().Add(30 * time.Second); !doc.LeaseUntil.Equal(want) {
		t.Fatalf("lease_until = %v, want %v", doc.LeaseUntil, want)
	}
}

func TestRenew_RetainsLeadershipAcrossLoops(t *testing.T) {
	be := newFakeBackend()
	clock := newMockClock(time.Unix(1_700_000_000, 0))
	e := newTestElector(be, "instance-a", clock)

	if err := e.tick(context.Background()); err != nil {
		t.Fatalf("tick1: %v", err)
	}
	_, body1, gen1 := be.snapshot()
	first := parseDoc(body1).LeaseUntil

	clock.Advance(4 * time.Second)
	if err := e.tick(context.Background()); err != nil {
		t.Fatalf("tick2: %v", err)
	}
	if !e.IsLeader(context.Background()) {
		t.Fatalf("expected still leader after renewal")
	}
	_, body2, gen2 := be.snapshot()
	second := parseDoc(body2).LeaseUntil
	if !second.After(first) {
		t.Fatalf("lease_until did not advance: first=%v second=%v", first, second)
	}
	if gen2 <= gen1 {
		t.Fatalf("generation did not advance across renewals: %d -> %d", gen1, gen2)
	}
	if parseDoc(body2).Owner != "instance-a" {
		t.Fatalf("owner drifted after renew")
	}
}

func TestSteal_AfterLeaseExpiry(t *testing.T) {
	be := newFakeBackend()
	clock := newMockClock(time.Unix(1_700_000_000, 0))

	incumbent := newTestElector(be, "instance-a", clock)
	if err := incumbent.tick(context.Background()); err != nil {
		t.Fatalf("incumbent tick: %v", err)
	}
	clock.Advance(31 * time.Second)

	challenger := newTestElector(be, "instance-b", clock)
	if err := challenger.tick(context.Background()); err != nil {
		t.Fatalf("challenger tick: %v", err)
	}
	if !challenger.IsLeader(context.Background()) {
		t.Fatalf("challenger should have stolen expired lease")
	}
	_, body, _ := be.snapshot()
	if parseDoc(body).Owner != "instance-b" {
		t.Fatalf("owner should be challenger after steal")
	}
}

func TestSecondInstance_NotLeader_WhileFirstAlive(t *testing.T) {
	be := newFakeBackend()
	clock := newMockClock(time.Unix(1_700_000_000, 0))

	a := newTestElector(be, "instance-a", clock)
	b := newTestElector(be, "instance-b", clock)

	if err := a.tick(context.Background()); err != nil {
		t.Fatalf("a tick: %v", err)
	}
	clock.Advance(2 * time.Second)
	if err := b.tick(context.Background()); err != nil {
		t.Fatalf("b tick: %v", err)
	}
	if !a.IsLeader(context.Background()) {
		t.Fatalf("a should still be leader")
	}
	if b.IsLeader(context.Background()) {
		t.Fatalf("b should NOT be leader")
	}
	_, body, _ := be.snapshot()
	if parseDoc(body).Owner != "instance-a" {
		t.Fatalf("stored owner should remain instance-a")
	}
}

func TestResign_ClearsDocOrObject_WhenLeader(t *testing.T) {
	be := newFakeBackend()
	clock := newMockClock(time.Unix(1_700_000_000, 0))
	e := newTestElector(be, "instance-a", clock)

	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !e.IsLeader(context.Background()) {
		t.Fatalf("expected leader after Start")
	}
	if err := e.Resign(context.Background()); err != nil {
		t.Fatalf("Resign: %v", err)
	}
	if e.IsLeader(context.Background()) {
		t.Fatalf("expected NOT leader after Resign")
	}
	exists, _, _ := be.snapshot()
	if exists {
		t.Fatalf("expected object cleared on Resign")
	}
}

func TestResign_NoOp_WhenNotLeader(t *testing.T) {
	be := newFakeBackend()
	clock := newMockClock(time.Unix(1_700_000_000, 0))

	a := newTestElector(be, "instance-a", clock)
	b := newTestElector(be, "instance-b", clock)

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("a Start: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("b Start: %v", err)
	}
	if !a.IsLeader(context.Background()) || b.IsLeader(context.Background()) {
		t.Fatalf("expected a leader, b not — got a=%v b=%v", a.IsLeader(context.Background()), b.IsLeader(context.Background()))
	}
	if err := b.Resign(context.Background()); err != nil {
		t.Fatalf("b Resign: %v", err)
	}
	exists, body, _ := be.snapshot()
	if !exists || parseDoc(body).Owner != "instance-a" {
		t.Fatalf("b Resign clobbered a's ownership: exists=%v body=%q", exists, string(body))
	}
	if err := a.Resign(context.Background()); err != nil {
		t.Fatalf("a Resign: %v", err)
	}
}

func TestIsLeader_ReturnsAtomicallyFast(t *testing.T) {
	be := &blockingBackend{ch: make(chan struct{})}
	defer close(be.ch)
	clock := newMockClock(time.Unix(1_700_000_000, 0))
	e := newTestElector(be, "instance-a", clock)
	e.leader.Store(true)

	done := make(chan bool, 1)
	start := time.Now()
	go func() { done <- e.IsLeader(context.Background()) }()
	select {
	case v := <-done:
		if !v {
			t.Fatalf("IsLeader returned false, want true")
		}
		if time.Since(start) > 100*time.Millisecond {
			t.Fatalf("IsLeader took too long — likely blocked on I/O")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("IsLeader did not return within 100ms — likely blocked on I/O")
	}
}

// blockingBackend blocks every backend call until ch closes.
type blockingBackend struct{ ch chan struct{} }

func (b *blockingBackend) read(ctx context.Context) ([]byte, int64, bool, error) {
	select {
	case <-ctx.Done():
		return nil, 0, false, ctx.Err()
	case <-b.ch:
		return nil, 0, false, nil
	}
}
func (b *blockingBackend) createIfMissing(ctx context.Context, body []byte) (int64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-b.ch:
		return 0, nil
	}
}
func (b *blockingBackend) writeIfGenerationMatch(ctx context.Context, body []byte, gen int64) (int64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-b.ch:
		return 0, nil
	}
}
func (b *blockingBackend) deleteIfGenerationMatch(ctx context.Context, gen int64) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.ch:
		return nil
	}
}

func TestStart_IsIdempotent(t *testing.T) {
	be := newFakeBackend()
	clock := newMockClock(time.Unix(1_700_000_000, 0))
	e := newTestElector(be, "instance-a", clock)
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	_ = e.Resign(context.Background())
}

func TestValidate_RejectsBadOptions(t *testing.T) {
	be := newFakeBackend()
	cases := map[string]Options{
		"no bucket": {Object: "x", Owner: "y", Lease: time.Second},
		"no object": {Bucket: "b", Owner: "y", Lease: time.Second},
		"no owner":  {Bucket: "b", Object: "x", Lease: time.Second},
		"no lease":  {Bucket: "b", Object: "x", Owner: "y"},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			e := newWithBackend(be, opts)
			if err := e.Start(context.Background()); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}
