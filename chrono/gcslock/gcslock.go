package gcslock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

// Options configures a Cloud Storage-backed leader elector.
type Options struct {
	// Bucket is the GCS bucket holding the lock object. It must exist
	// prior to Start; the elector does not create buckets.
	Bucket string
	// Object is the path inside the bucket. All contenders for the
	// same leadership role MUST agree on Bucket + Object.
	Object string
	// Owner uniquely identifies THIS instance. Hostname+pid+random is
	// a reasonable default.
	Owner string
	// Lease is the renewal cadence. Renewal fires every Lease/2; the
	// persisted expiry is now + 3*Lease. Choose Lease larger than
	// expected clock skew and GC pauses.
	Lease time.Duration
}

// lockDoc is the JSON body persisted in the lock object.
type lockDoc struct {
	Owner      string    `json:"owner"`
	LeaseUntil time.Time `json:"lease_until"`
}

// backend abstracts the GCS operations the elector needs so tests can
// swap in an in-memory fake with matching precondition semantics.
type backend interface {
	// read returns the current object body and generation. ok=false
	// means the object does not exist.
	read(ctx context.Context) (body []byte, gen int64, ok bool, err error)
	// createIfMissing writes body only if the object does NOT exist.
	// It returns errPreconditionFailed when the object is already
	// present, or another error for transport failures.
	createIfMissing(ctx context.Context, body []byte) (gen int64, err error)
	// writeIfGenerationMatch writes body only if the current
	// generation equals gen. Returns errPreconditionFailed on mismatch.
	writeIfGenerationMatch(ctx context.Context, body []byte, gen int64) (newGen int64, err error)
	// deleteIfGenerationMatch deletes the object only if the current
	// generation equals gen. Returns errPreconditionFailed on mismatch.
	deleteIfGenerationMatch(ctx context.Context, gen int64) error
}

// errPreconditionFailed is the canonical sentinel returned by backend
// methods when a GenerationMatch / DoesNotExist condition fails.
var errPreconditionFailed = errors.New("gcslock: precondition failed")

// Elector implements chrono.LeaderElector on top of Cloud Storage.
type Elector struct {
	be   backend
	opts Options
	now  func() time.Time

	leader atomic.Bool
	// heldGen is the object generation we last wrote successfully;
	// used to gate renew/delete under GenerationMatch. It is only
	// accessed from the renewal goroutine and from Resign after the
	// goroutine has exited, so no lock is needed.
	heldGen int64

	startOnce sync.Once
	started   bool
	cancel    context.CancelFunc
	done      chan struct{}
}

// New constructs a Cloud Storage-backed Elector using the provided
// client. The elector does not own the client — the caller should close
// it when they're done.
func New(client *storage.Client, opts Options) *Elector {
	be := &gcsBackend{
		obj: client.Bucket(opts.Bucket).Object(opts.Object),
	}
	return newWithBackend(be, opts)
}

func newWithBackend(be backend, opts Options) *Elector {
	return &Elector{
		be:   be,
		opts: opts,
		now:  time.Now,
	}
}

// Start validates the options and launches the background renewal
// goroutine. Idempotent — repeated calls are no-ops.
func (e *Elector) Start(ctx context.Context) error {
	if err := e.validate(); err != nil {
		return err
	}
	e.startOnce.Do(func() {
		loopCtx, cancel := context.WithCancel(context.Background())
		e.cancel = cancel
		e.done = make(chan struct{})
		e.started = true
		// One synchronous attempt so callers who wait briefly can
		// observe leadership without racing the ticker.
		_ = e.tick(ctx)
		go e.run(loopCtx)
	})
	return nil
}

// IsLeader reads the atomic flag maintained by the renewal loop. It
// does NOT perform I/O and is safe from hot paths.
func (e *Elector) IsLeader(_ context.Context) bool { return e.leader.Load() }

// Resign stops the renewal loop and, if this instance owns the lock,
// deletes the object (with GenerationMatch to avoid clobbering a
// successor that has already stolen the lease).
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
	if !e.leader.Load() || e.heldGen == 0 {
		return nil
	}
	err := e.be.deleteIfGenerationMatch(ctx, e.heldGen)
	e.leader.Store(false)
	e.heldGen = 0
	if errors.Is(err, errPreconditionFailed) {
		// Someone else already stole/wrote — that's fine on resign.
		return nil
	}
	return err
}

func (e *Elector) validate() error {
	if e.opts.Bucket == "" {
		return errors.New("gcslock: Bucket required")
	}
	if e.opts.Object == "" {
		return errors.New("gcslock: Object required")
	}
	if e.opts.Owner == "" {
		return errors.New("gcslock: Owner required")
	}
	if e.opts.Lease <= 0 {
		return errors.New("gcslock: Lease must be > 0")
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

// tick runs one renewal attempt.
//
//  1. Try to create the object if it doesn't exist (first-claim path).
//  2. On precondition failure, read the object to see who owns it and
//     when the lease expires.
//  3. If we own it (renew) or it's expired (steal), overwrite with a
//     GenerationMatch precondition.
//  4. Otherwise mark not-leader.
func (e *Elector) tick(ctx context.Context) error {
	now := e.now()
	body := mustMarshal(lockDoc{Owner: e.opts.Owner, LeaseUntil: now.Add(3 * e.opts.Lease)})

	// (1) First-claim path.
	gen, err := e.be.createIfMissing(ctx, body)
	if err == nil {
		e.heldGen = gen
		e.leader.Store(true)
		return nil
	}
	if !errors.Is(err, errPreconditionFailed) {
		// Transport-level failure: preserve prior atomic value.
		return err
	}

	// (2) Object exists — read to inspect owner + lease.
	cur, curGen, ok, err := e.be.read(ctx)
	if err != nil {
		return err
	}
	if !ok {
		// Rare race: someone deleted between our create attempt and
		// the read. Next tick will retry the create path.
		e.leader.Store(false)
		return nil
	}
	doc := parseDoc(cur)

	switch {
	case doc.Owner == e.opts.Owner || !doc.LeaseUntil.After(now):
		// (3) Renew (we already hold it) or steal (expired).
		newGen, werr := e.be.writeIfGenerationMatch(ctx, body, curGen)
		if werr != nil {
			if errors.Is(werr, errPreconditionFailed) {
				// Another instance beat us to the same write; we're
				// not leader this tick. Let a future tick re-observe.
				e.leader.Store(false)
				return nil
			}
			return werr
		}
		e.heldGen = newGen
		e.leader.Store(true)
	default:
		// (4) Someone else holds a live lease.
		e.leader.Store(false)
	}
	return nil
}

// mustMarshal serialises lockDoc to JSON — the shape has no fields that
// can fail to marshal, so any error would be a programming bug.
func mustMarshal(d lockDoc) []byte {
	b, err := json.Marshal(d)
	if err != nil {
		panic(fmt.Sprintf("gcslock: marshal lockDoc: %v", err))
	}
	return b
}

// parseDoc decodes a stored body into a lockDoc. Corrupt bodies are
// treated as an expired lease so the elector can steal them cleanly.
func parseDoc(body []byte) lockDoc {
	var d lockDoc
	if err := json.Unmarshal(body, &d); err != nil {
		return lockDoc{}
	}
	return d
}

// gcsBackend is the production backend that talks to Cloud Storage.
type gcsBackend struct {
	obj *storage.ObjectHandle
}

func (b *gcsBackend) read(ctx context.Context) ([]byte, int64, bool, error) {
	r, err := b.obj.NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	defer r.Close()
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, false, err
	}
	return body, r.Attrs.Generation, true, nil
}

func (b *gcsBackend) createIfMissing(ctx context.Context, body []byte) (int64, error) {
	w := b.obj.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		if isPrecondition(err) {
			return 0, errPreconditionFailed
		}
		return 0, err
	}
	if err := w.Close(); err != nil {
		if isPrecondition(err) {
			return 0, errPreconditionFailed
		}
		return 0, err
	}
	return w.Attrs().Generation, nil
}

func (b *gcsBackend) writeIfGenerationMatch(ctx context.Context, body []byte, gen int64) (int64, error) {
	w := b.obj.If(storage.Conditions{GenerationMatch: gen}).NewWriter(ctx)
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		if isPrecondition(err) {
			return 0, errPreconditionFailed
		}
		return 0, err
	}
	if err := w.Close(); err != nil {
		if isPrecondition(err) {
			return 0, errPreconditionFailed
		}
		return 0, err
	}
	return w.Attrs().Generation, nil
}

func (b *gcsBackend) deleteIfGenerationMatch(ctx context.Context, gen int64) error {
	err := b.obj.If(storage.Conditions{GenerationMatch: gen}).Delete(ctx)
	if err == nil {
		return nil
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil
	}
	if isPrecondition(err) {
		return errPreconditionFailed
	}
	return err
}

// isPrecondition inspects a GCS/googleapi error for HTTP 412.
func isPrecondition(err error) bool {
	if err == nil {
		return false
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && gerr.Code == http.StatusPreconditionFailed {
		return true
	}
	return false
}
