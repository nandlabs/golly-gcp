package gcpiam

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/iam/apiv1/iampb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	gax "github.com/googleapis/gax-go/v2"
	"oss.nandlabs.io/golly/authz"
)

// IAMCaller is the minimal surface the evaluator uses on a GCP client. Both
// *resourcemanager.ProjectsClient and other service clients (e.g. the
// storage or pubsub IAM handles) satisfy it as long as they expose
// TestIamPermissions with the standard signature. Tests inject a fake.
type IAMCaller interface {
	TestIamPermissions(ctx context.Context, req *iampb.TestIamPermissionsRequest, opts ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error)
}

// PrincipalMemberFunc extracts a GCP IAM member string
// (e.g. "user:alice@example.com" or
// "serviceAccount:sa@proj.iam.gserviceaccount.com") from an
// authz.Principal. Callers supply one via WithPrincipalMember when their
// Principal implementation doesn't expose an ID()/Member() accessor.
type PrincipalMemberFunc func(p authz.Principal) string

// IAMResource is passed as the `resource any` argument to Evaluator.Can
// when the caller wants a live TestIamPermissions round-trip instead of
// consulting the cached permission map. It carries the context and the
// fully-qualified GCP resource name (e.g. projects/foo/instances/bar or
// projects/_/buckets/my-bucket).
//
// This is a workaround for the fact that authz.Policy.Can does not take a
// context.Context. Callers that don't need live checks can just pass any
// value they like (including nil) as the `resource any` argument.
type IAMResource struct {
	// Ctx is the request context used for the live call. If nil,
	// context.Background() is used.
	Ctx context.Context
	// Name is the full GCP resource identifier the permission is
	// checked against. If empty, the evaluator's configured resource
	// is used.
	Name string
}

// Option customises an Evaluator at construction time.
type Option func(*Evaluator)

// WithPrincipalMember overrides the extractor used to derive a GCP IAM
// member string from an authz.Principal. The default extractor tries
// interface{ ID() string } and then interface{ Member() string } via
// type-assertion and returns "" if neither is present — in which case
// Can returns false (see UnknownPrincipal below).
func WithPrincipalMember(fn PrincipalMemberFunc) Option {
	return func(e *Evaluator) {
		if fn != nil {
			e.extractor = fn
		}
	}
}

// WithExpectedMember restricts Can to grant only when the extracted
// principal-member equals `member`. Use this when the underlying IAM
// client is authenticated as a specific identity and you want to make
// the mismatch explicit: any Principal that resolves to a different
// member is denied without a cache lookup.
func WithExpectedMember(member string) Option {
	return func(e *Evaluator) { e.expected = member }
}

// WithInitialPermissions seeds the tracked action set so the first
// Refresh already exercises those permissions. Without this, actions
// only become tracked after the first Can call that references them,
// and Can returns false until the next Refresh populates the cache.
func WithInitialPermissions(perms ...string) Option {
	return func(e *Evaluator) {
		for _, p := range perms {
			if p != "" {
				e.tracked[p] = struct{}{}
			}
		}
	}
}

// Evaluator implements authz.Policy by consulting a Cloud IAM
// permission map cached in memory. A background goroutine re-runs
// TestIamPermissions every `refresh` interval to keep the map current.
//
// Can also accepts an *IAMResource / IAMResource as its `resource any`
// argument to force a live check that bypasses the cache — see
// IAMResource for details.
type Evaluator struct {
	caller    IAMCaller
	resource  string
	refresh   time.Duration
	extractor PrincipalMemberFunc
	expected  string

	mu      sync.RWMutex
	perms   map[string]bool     // permission -> granted (for the client identity)
	tracked map[string]struct{} // actions ever seen (or seeded via WithInitialPermissions)

	stopCh   chan struct{}
	doneCh   chan struct{}
	closedMu sync.Mutex
	closed   bool
}

// NewCachedEvaluator wires the evaluator to a
// *resourcemanager.ProjectsClient. It is a thin wrapper over
// NewCachedEvaluatorFromCaller for the common case of checking permissions
// on a project-scoped resource. Pass 0 (or negative) for `refresh` to
// disable the background refresh goroutine — the caller then invokes
// Refresh manually.
func NewCachedEvaluator(client *resourcemanager.ProjectsClient, resource string, refresh time.Duration, opts ...Option) *Evaluator {
	return NewCachedEvaluatorFromCaller(client, resource, refresh, opts...)
}

// NewCachedEvaluatorFromCaller is the general constructor: any client
// that satisfies IAMCaller (a Storage bucket IAM handle, a Pub/Sub
// topic handle, a Cloud Run service client, etc.) can be plugged in.
// Tests use it to inject a fake caller.
func NewCachedEvaluatorFromCaller(caller IAMCaller, resource string, refresh time.Duration, opts ...Option) *Evaluator {
	e := &Evaluator{
		caller:    caller,
		resource:  resource,
		refresh:   refresh,
		extractor: defaultPrincipalMember,
		perms:     make(map[string]bool),
		tracked:   make(map[string]struct{}),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	if refresh > 0 {
		go e.loop()
	} else {
		// No background loop — signal doneCh so Close is a no-op-safe.
		close(e.doneCh)
	}
	return e
}

// defaultPrincipalMember tries a couple of common accessor shapes before
// giving up. Callers with a bespoke Principal type should inject an
// extractor via WithPrincipalMember.
func defaultPrincipalMember(p authz.Principal) string {
	if p == nil {
		return ""
	}
	if withID, ok := p.(interface{ ID() string }); ok {
		return withID.ID()
	}
	if withMember, ok := p.(interface{ Member() string }); ok {
		return withMember.Member()
	}
	return ""
}

func (e *Evaluator) loop() {
	defer close(e.doneCh)
	t := time.NewTicker(e.refresh)
	defer t.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), e.refresh)
			_ = e.Refresh(ctx)
			cancel()
		}
	}
}

// Refresh re-runs TestIamPermissions for every action that has been
// seen (or seeded) and replaces the cached permission map. Safe for
// concurrent use with Can. Returns whatever error the underlying IAM
// call returned; the cache is left untouched on error.
func (e *Evaluator) Refresh(ctx context.Context) error {
	if e == nil || e.caller == nil {
		return errors.New("gcpiam: evaluator has no caller")
	}
	e.mu.RLock()
	if len(e.tracked) == 0 {
		e.mu.RUnlock()
		return nil
	}
	perms := make([]string, 0, len(e.tracked))
	for p := range e.tracked {
		perms = append(perms, p)
	}
	e.mu.RUnlock()

	resp, err := e.caller.TestIamPermissions(ctx, &iampb.TestIamPermissionsRequest{
		Resource:    e.resource,
		Permissions: perms,
	})
	if err != nil {
		return fmt.Errorf("gcpiam: TestIamPermissions on %q: %w", e.resource, err)
	}
	granted := make(map[string]bool, len(resp.GetPermissions()))
	for _, p := range resp.GetPermissions() {
		granted[p] = true
	}
	// Any tracked action not in the response is explicitly not-granted.
	next := make(map[string]bool, len(perms))
	for _, p := range perms {
		next[p] = granted[p]
	}
	e.mu.Lock()
	e.perms = next
	e.mu.Unlock()
	return nil
}

// Can satisfies authz.Policy.
//
// If `resource` is an IAMResource or *IAMResource, Can bypasses the cache
// and makes a live TestIamPermissions call using the embedded context.
// Otherwise it consults the cached permission map for the evaluator's
// configured resource. Any action not yet observed is added to the
// tracked set (the next Refresh will populate its cache entry) and Can
// returns false for that call.
func (e *Evaluator) Can(p authz.Principal, action string, resource any) bool {
	if e == nil || action == "" {
		return false
	}
	// Live path: caller wants a fresh answer and supplied a context via IAMResource.
	if live, name, ctx, ok := extractIAMResource(resource); ok {
		if !e.principalOK(p) {
			_ = live
			return false
		}
		return e.liveCheck(ctx, name, action)
	}
	if !e.principalOK(p) {
		return false
	}
	e.mu.RLock()
	cached, cachedOK := e.perms[action]
	_, tracked := e.tracked[action]
	e.mu.RUnlock()
	if !tracked {
		e.mu.Lock()
		e.tracked[action] = struct{}{}
		e.mu.Unlock()
	}
	if !cachedOK {
		return false
	}
	return cached
}

// principalOK returns false when the extractor yields an empty member
// (an "unknown" principal) or when WithExpectedMember was set and the
// extracted member doesn't match.
func (e *Evaluator) principalOK(p authz.Principal) bool {
	member := e.extractor(p)
	if member == "" {
		return false
	}
	if e.expected != "" && member != e.expected {
		return false
	}
	return true
}

func (e *Evaluator) liveCheck(ctx context.Context, name, action string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if name == "" {
		name = e.resource
	}
	resp, err := e.caller.TestIamPermissions(ctx, &iampb.TestIamPermissionsRequest{
		Resource:    name,
		Permissions: []string{action},
	})
	if err != nil {
		return false
	}
	for _, got := range resp.GetPermissions() {
		if got == action {
			return true
		}
	}
	return false
}

// extractIAMResource unwraps IAMResource / *IAMResource from the any.
// Returns (present, name, ctx, ok).
func extractIAMResource(resource any) (IAMResource, string, context.Context, bool) {
	switch r := resource.(type) {
	case IAMResource:
		return r, r.Name, r.Ctx, true
	case *IAMResource:
		if r == nil {
			return IAMResource{}, "", nil, false
		}
		return *r, r.Name, r.Ctx, true
	}
	return IAMResource{}, "", nil, false
}

// Close stops the background refresh goroutine. Safe to call multiple
// times. Returns nil — kept as an error return for symmetry with io.Closer
// so future implementations can surface a shutdown failure.
func (e *Evaluator) Close() error {
	if e == nil {
		return nil
	}
	e.closedMu.Lock()
	if e.closed {
		e.closedMu.Unlock()
		return nil
	}
	e.closed = true
	close(e.stopCh)
	e.closedMu.Unlock()
	<-e.doneCh
	return nil
}
