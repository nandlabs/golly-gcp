package firestorelock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeBackend is an in-memory serializable substitute for the Firestore
// transaction backend. All operations acquire the same mutex so the
// semantics match Firestore's serializable transactions on a single
// document.
type fakeBackend struct {
	mu       sync.Mutex
	state    lockState
	txCalls  atomic.Int64
	failNext atomic.Int64 // if >0, next N calls fail with errInjected
}

var errInjected = errors.New("injected transport error")

func newFakeBackend() *fakeBackend { return &fakeBackend{} }

func (b *fakeBackend) doTx(ctx context.Context, fn txFn) error {
	b.txCalls.Add(1)
	if n := b.failNext.Load(); n > 0 {
		b.failNext.Add(-1)
		return errInjected
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	cur := b.state
	op, err := fn(cur)
	if err != nil {
		return err
	}
	switch op.Kind {
	case opWrite:
		b.state = lockState{Exists: true, Owner: op.Owner, LeaseUntil: op.LeaseUntil}
	case opDelete:
		b.state = lockState{}
	}
	return nil
}

// snapshot returns the current stored state under the lock.
func (b *fakeBackend) snapshot() lockState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// mockClock is a monotonic advancer used to drive tick logic
// deterministically.
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
		Collection: "locks",
		DocID:      "sched",
		Owner:      owner,
		Lease:      10 * time.Second,
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
	st := be.snapshot()
	if st.Owner != "instance-a" {
		t.Fatalf("owner = %q, want instance-a", st.Owner)
	}
	if want := clock.Now().Add(30 * time.Second); !st.LeaseUntil.Equal(want) {
		t.Fatalf("lease_until = %v, want %v", st.LeaseUntil, want)
	}
}

func TestRenew_RetainsLeadershipAcrossLoops(t *testing.T) {
	be := newFakeBackend()
	clock := newMockClock(time.Unix(1_700_000_000, 0))
	e := newTestElector(be, "instance-a", clock)

	if err := e.tick(context.Background()); err != nil {
		t.Fatalf("tick1: %v", err)
	}
	first := be.snapshot().LeaseUntil

	clock.Advance(4 * time.Second) // less than lease; still leader
	if err := e.tick(context.Background()); err != nil {
		t.Fatalf("tick2: %v", err)
	}
	if !e.IsLeader(context.Background()) {
		t.Fatalf("expected still leader after renewal")
	}
	second := be.snapshot().LeaseUntil
	if !second.After(first) {
		t.Fatalf("lease_until did not advance: first=%v second=%v", first, second)
	}
	if be.snapshot().Owner != "instance-a" {
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

	// Advance beyond the 3*Lease expiry.
	clock.Advance(31 * time.Second)

	challenger := newTestElector(be, "instance-b", clock)
	if err := challenger.tick(context.Background()); err != nil {
		t.Fatalf("challenger tick: %v", err)
	}
	if !challenger.IsLeader(context.Background()) {
		t.Fatalf("challenger should have stolen expired lease")
	}
	if be.snapshot().Owner != "instance-b" {
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
	// Only a small advance — well inside the lease.
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
	if be.snapshot().Owner != "instance-a" {
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
	// Start does one synchronous tick, so we should already be leader.
	if !e.IsLeader(context.Background()) {
		t.Fatalf("expected leader after Start")
	}
	if err := e.Resign(context.Background()); err != nil {
		t.Fatalf("Resign: %v", err)
	}
	if e.IsLeader(context.Background()) {
		t.Fatalf("expected NOT leader after Resign")
	}
	if be.snapshot().Exists {
		t.Fatalf("expected doc cleared, got %+v", be.snapshot())
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
	// b resigns without owning the doc: must NOT delete instance-a's ownership.
	if err := b.Resign(context.Background()); err != nil {
		t.Fatalf("b Resign: %v", err)
	}
	if be.snapshot().Owner != "instance-a" {
		t.Fatalf("b Resign clobbered a's ownership: %+v", be.snapshot())
	}
	if err := a.Resign(context.Background()); err != nil {
		t.Fatalf("a Resign: %v", err)
	}
}

// TestIsLeader_ReturnsAtomicallyFast asserts IsLeader does not perform
// I/O — it should complete in microseconds regardless of backend
// latency. We install a backend whose doTx blocks forever and verify
// IsLeader still returns instantly.
func TestIsLeader_ReturnsAtomicallyFast(t *testing.T) {
	be := &blockingBackend{ch: make(chan struct{})}
	defer close(be.ch)
	clock := newMockClock(time.Unix(1_700_000_000, 0))
	e := newTestElector(be, "instance-a", clock)
	// Set leader true directly to exercise the read path.
	e.leader.Store(true)

	// The call must return well under a millisecond. Give ourselves a
	// generous 100ms envelope to keep the test hermetic under load.
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

// blockingBackend is a backend whose doTx never returns until ch closes.
// Used only in TestIsLeader_ReturnsAtomicallyFast.
type blockingBackend struct{ ch chan struct{} }

func (b *blockingBackend) doTx(ctx context.Context, fn txFn) error {
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
		"no collection": {DocID: "x", Owner: "y", Lease: time.Second},
		"no docid":      {Collection: "c", Owner: "y", Lease: time.Second},
		"no owner":      {Collection: "c", DocID: "x", Lease: time.Second},
		"no lease":      {Collection: "c", DocID: "x", Owner: "y"},
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
