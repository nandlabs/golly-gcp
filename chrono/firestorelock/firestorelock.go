package firestorelock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Options configures a Firestore-backed leader elector.
type Options struct {
	// Collection is the Firestore collection that holds lock documents.
	// One lock == one document keyed by DocID inside this collection.
	Collection string
	// DocID is the well-known name of the lock document. All contenders
	// for the same leadership role MUST agree on Collection + DocID.
	DocID string
	// Owner is the identity of THIS instance. It should be globally
	// unique across contenders (hostname+pid+random is a good default).
	Owner string
	// Lease is the renewal cadence. The elector will attempt renewal
	// every Lease/2, and the persisted expiry is now + 3*Lease. Choose
	// Lease to be larger than expected clock skew and GC pauses.
	Lease time.Duration
}

// The elector persists this shape in Firestore. Field tags map to the
// document keys advertised in the package docs.
type lockDoc struct {
	Owner      string    `firestore:"owner"`
	LeaseUntil time.Time `firestore:"lease_until"`
}

// lockState is the snapshot the tick logic reads inside a transaction.
type lockState struct {
	Exists     bool
	Owner      string
	LeaseUntil time.Time
}

// opKind describes what the tick decision wants the backend to persist.
type opKind int

const (
	opNone opKind = iota
	opWrite
	opDelete
)

type lockOp struct {
	Kind       opKind
	Owner      string
	LeaseUntil time.Time
}

// txFn is invoked by backend.doTx with a consistent snapshot of the lock
// document. It returns the operation the backend should apply atomically
// against that snapshot.
type txFn func(cur lockState) (lockOp, error)

// backend abstracts the actual Firestore transaction so tests can swap
// in an in-memory fake without spinning up an emulator.
type backend interface {
	doTx(ctx context.Context, fn txFn) error
}

// Elector implements chrono.LeaderElector on top of Firestore.
type Elector struct {
	be   backend
	opts Options
	now  func() time.Time

	leader atomic.Bool

	startOnce sync.Once
	started   bool
	cancel    context.CancelFunc
	done      chan struct{}
}

// New constructs a Firestore-backed Elector using the provided client.
// The elector does not take ownership of the client — the caller must
// close it when they're done.
func New(client *firestore.Client, opts Options) *Elector {
	be := &firestoreBackend{
		client: client,
		docRef: client.Collection(opts.Collection).Doc(opts.DocID),
	}
	return newWithBackend(be, opts)
}

// newWithBackend is the internal constructor used by both New and tests.
func newWithBackend(be backend, opts Options) *Elector {
	return &Elector{
		be:   be,
		opts: opts,
		now:  time.Now,
	}
}

// Start validates configuration and launches the background renewal
// goroutine. It is safe to call more than once; subsequent calls are
// no-ops. Start returns before the first renewal completes; call
// IsLeader after at least one Lease/2 tick to observe the outcome.
func (e *Elector) Start(ctx context.Context) error {
	if err := e.validate(); err != nil {
		return err
	}
	e.startOnce.Do(func() {
		loopCtx, cancel := context.WithCancel(context.Background())
		e.cancel = cancel
		e.done = make(chan struct{})
		e.started = true
		// Do one synchronous attempt so callers who wait a moment can
		// observe leadership without racing the ticker.
		_ = e.tick(ctx)
		go e.run(loopCtx)
	})
	return nil
}

// IsLeader reads the atomic leadership flag maintained by the renewal
// loop. It does NOT perform I/O and is safe to call from hot paths.
func (e *Elector) IsLeader(_ context.Context) bool {
	return e.leader.Load()
}

// Resign stops the renewal loop and, if this instance currently owns
// the lock, deletes the document inside a transaction so that a fresh
// contender can claim immediately.
func (e *Elector) Resign(ctx context.Context) error {
	if !e.started {
		return nil
	}
	if e.cancel != nil {
		e.cancel()
	}
	if e.done != nil {
		<-e.done
	}
	if !e.leader.Load() {
		return nil
	}
	// Clear ownership atomically.
	err := e.be.doTx(ctx, func(cur lockState) (lockOp, error) {
		if !cur.Exists || cur.Owner != e.opts.Owner {
			return lockOp{Kind: opNone}, nil
		}
		return lockOp{Kind: opDelete}, nil
	})
	e.leader.Store(false)
	return err
}

func (e *Elector) validate() error {
	if e.opts.Collection == "" {
		return errors.New("firestorelock: Collection required")
	}
	if e.opts.DocID == "" {
		return errors.New("firestorelock: DocID required")
	}
	if e.opts.Owner == "" {
		return errors.New("firestorelock: Owner required")
	}
	if e.opts.Lease <= 0 {
		return errors.New("firestorelock: Lease must be > 0")
	}
	return nil
}

func (e *Elector) run(ctx context.Context) {
	defer close(e.done)
	interval := e.opts.Lease / 2
	if interval <= 0 {
		interval = e.opts.Lease
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = e.tick(ctx)
		}
	}
}

// tick runs one renewal attempt. It sets the leader flag according to
// the transaction outcome and returns any transport error unchanged.
func (e *Elector) tick(ctx context.Context) error {
	var claim bool
	err := e.be.doTx(ctx, func(cur lockState) (lockOp, error) {
		now := e.now()
		expiry := now.Add(3 * e.opts.Lease)
		switch {
		case !cur.Exists || !cur.LeaseUntil.After(now):
			// Free (or expired): claim it.
			claim = true
			return lockOp{Kind: opWrite, Owner: e.opts.Owner, LeaseUntil: expiry}, nil
		case cur.Owner == e.opts.Owner:
			// We already own it: renew.
			claim = true
			return lockOp{Kind: opWrite, Owner: e.opts.Owner, LeaseUntil: expiry}, nil
		default:
			// Someone else holds a live lease.
			claim = false
			return lockOp{Kind: opNone}, nil
		}
	})
	if err != nil {
		// Transport-level failures should NOT flip an existing leader to
		// non-leader — the lease-until in Firestore is the source of
		// truth; letting the lease expire from an outside observer's
		// point of view is the correct fail-safe.
		return err
	}
	e.leader.Store(claim)
	return nil
}

// firestoreBackend is the production backend that runs each decision
// inside a Firestore transaction.
type firestoreBackend struct {
	client *firestore.Client
	docRef *firestore.DocumentRef
}

func (b *firestoreBackend) doTx(ctx context.Context, fn txFn) error {
	return b.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		var cur lockState
		snap, err := tx.Get(b.docRef)
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return fmt.Errorf("firestorelock: read %s: %w", b.docRef.Path, err)
			}
			cur.Exists = false
		} else {
			cur.Exists = true
			var doc lockDoc
			if derr := snap.DataTo(&doc); derr != nil {
				return fmt.Errorf("firestorelock: decode %s: %w", b.docRef.Path, derr)
			}
			cur.Owner = doc.Owner
			cur.LeaseUntil = doc.LeaseUntil
		}
		op, err := fn(cur)
		if err != nil {
			return err
		}
		switch op.Kind {
		case opWrite:
			return tx.Set(b.docRef, lockDoc{Owner: op.Owner, LeaseUntil: op.LeaseUntil})
		case opDelete:
			return tx.Delete(b.docRef)
		}
		return nil
	})
}
