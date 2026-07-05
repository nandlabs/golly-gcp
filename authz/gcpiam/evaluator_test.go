package gcpiam

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/iam/apiv1/iampb"
	gax "github.com/googleapis/gax-go/v2"
	"oss.nandlabs.io/golly/authz"
)

// principalWithID is a test Principal that also exposes ID() — the
// default extractor picks it up via type-assertion.
type principalWithID struct {
	id    string
	roles []string
	caps  []string
}

func (p *principalWithID) ID() string             { return p.id }
func (p *principalWithID) Roles() []string        { return p.roles }
func (p *principalWithID) Capabilities() []string { return p.caps }

// fakeCaller records every TestIamPermissions call and returns the
// permissions currently marked as granted. Tests drive it by mutating
// grant / unGrant between refreshes.
type fakeCaller struct {
	mu       sync.Mutex
	granted  map[string]bool // permission -> granted on this resource
	calls    int
	lastReq  *iampb.TestIamPermissionsRequest
	err      error
	callHook func()
}

func newFakeCaller(granted ...string) *fakeCaller {
	m := make(map[string]bool, len(granted))
	for _, g := range granted {
		m[g] = true
	}
	return &fakeCaller{granted: m}
}

func (f *fakeCaller) TestIamPermissions(_ context.Context, req *iampb.TestIamPermissionsRequest, _ ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error) {
	f.mu.Lock()
	f.calls++
	f.lastReq = req
	hook := f.callHook
	err := f.err
	grantedCopy := make(map[string]bool, len(f.granted))
	for k, v := range f.granted {
		grantedCopy[k] = v
	}
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	if err != nil {
		return nil, err
	}
	out := &iampb.TestIamPermissionsResponse{}
	for _, p := range req.GetPermissions() {
		if grantedCopy[p] {
			out.Permissions = append(out.Permissions, p)
		}
	}
	return out, nil
}

func (f *fakeCaller) setGranted(perms ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.granted = map[string]bool{}
	for _, p := range perms {
		f.granted[p] = true
	}
}

func (f *fakeCaller) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestCachedEvaluator_HitsCacheBetweenRefresh(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get")
	// refresh=0 disables the background loop; we drive Refresh manually.
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0,
		WithInitialPermissions("compute.instances.get"))
	defer func() { _ = e.Close() }()

	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := fc.callCount(); got != 1 {
		t.Fatalf("expected 1 call after Refresh, got %d", got)
	}

	p := &principalWithID{id: "user:alice@example.com"}
	for i := 0; i < 5; i++ {
		if !e.Can(p, "compute.instances.get", nil) {
			t.Fatalf("Can #%d: expected true from cache", i)
		}
	}
	if got := fc.callCount(); got != 1 {
		t.Fatalf("expected cache hits, got %d additional calls", got-1)
	}
}

func TestCachedEvaluator_RefreshUpdatesCache(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get")
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0,
		WithInitialPermissions("compute.instances.get"))
	defer func() { _ = e.Close() }()

	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	p := &principalWithID{id: "user:alice@example.com"}
	if !e.Can(p, "compute.instances.get", nil) {
		t.Fatal("Can before revoke: want true")
	}

	// IAM revokes the permission; next Refresh should reflect that.
	fc.setGranted()
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("second Refresh: %v", err)
	}
	if e.Can(p, "compute.instances.get", nil) {
		t.Fatal("Can after revoke: want false")
	}

	// Re-grant; cache flips back after another Refresh.
	fc.setGranted("compute.instances.get")
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("third Refresh: %v", err)
	}
	if !e.Can(p, "compute.instances.get", nil) {
		t.Fatal("Can after re-grant: want true")
	}
}

func TestCachedEvaluator_MissingPermission_ReturnsFalse(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get") // .list is NOT granted
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0,
		WithInitialPermissions("compute.instances.get", "compute.instances.list"))
	defer func() { _ = e.Close() }()

	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	p := &principalWithID{id: "user:alice@example.com"}
	if !e.Can(p, "compute.instances.get", nil) {
		t.Fatal("granted permission: want true")
	}
	if e.Can(p, "compute.instances.list", nil) {
		t.Fatal("ungranted permission: want false")
	}
	if e.Can(p, "compute.instances.delete", nil) {
		t.Fatal("never-observed permission: want false (uncached => deny)")
	}
}

func TestCachedEvaluator_UnknownPrincipal_ReturnsFalse(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get")
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0,
		WithInitialPermissions("compute.instances.get"))
	defer func() { _ = e.Close() }()
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// authz.BasicPrincipal does not expose ID() or Member() — the
	// default extractor returns "" and Can must deny.
	unknown := &authz.BasicPrincipal{RoleList: []string{"viewer"}}
	if e.Can(unknown, "compute.instances.get", nil) {
		t.Fatal("unknown principal (no ID accessor): want false")
	}

	// A principal with an ID that doesn't match WithExpectedMember is
	// also denied — this is the "wrong identity" case.
	e2 := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0,
		WithInitialPermissions("compute.instances.get"),
		WithExpectedMember("serviceAccount:svc@proj.iam.gserviceaccount.com"))
	defer func() { _ = e2.Close() }()
	if err := e2.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh (e2): %v", err)
	}
	stranger := &principalWithID{id: "user:mallory@example.com"}
	if e2.Can(stranger, "compute.instances.get", nil) {
		t.Fatal("wrong-member principal: want false")
	}

	// The expected member is granted.
	svc := &principalWithID{id: "serviceAccount:svc@proj.iam.gserviceaccount.com"}
	if !e2.Can(svc, "compute.instances.get", nil) {
		t.Fatal("expected member: want true")
	}
}

func TestCachedEvaluator_BackgroundLoopRefreshes(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get")
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 20*time.Millisecond,
		WithInitialPermissions("compute.instances.get"))
	defer func() { _ = e.Close() }()

	// Wait for at least one background refresh to land.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fc.callCount() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if fc.callCount() < 1 {
		t.Fatalf("background loop never refreshed: calls=%d", fc.callCount())
	}
	p := &principalWithID{id: "user:alice@example.com"}
	if !e.Can(p, "compute.instances.get", nil) {
		t.Fatal("after background refresh: want true")
	}
}

func TestCachedEvaluator_RefreshError_LeavesCacheIntact(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get")
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0,
		WithInitialPermissions("compute.instances.get"))
	defer func() { _ = e.Close() }()
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	p := &principalWithID{id: "user:alice@example.com"}
	if !e.Can(p, "compute.instances.get", nil) {
		t.Fatal("pre-error: want true")
	}
	// Now IAM fails; the cache from the previous successful Refresh
	// should not be clobbered.
	fc.mu.Lock()
	fc.err = errors.New("boom")
	fc.mu.Unlock()
	if err := e.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh with err: want error")
	}
	if !e.Can(p, "compute.instances.get", nil) {
		t.Fatal("post-error: cache should still say true")
	}
}

func TestCachedEvaluator_Close_Idempotent(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller()
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 20*time.Millisecond)
	if err := e.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
