package gcpiam

import (
	"context"
	"testing"
)

func TestCan_IAMResource_LiveCall_Granted(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("storage.objects.get")
	e := NewCachedEvaluatorFromCaller(fc, "unused", 0)
	defer e.Close()

	p := &principalWithID{id: "user:alice@example.com"}
	res := IAMResource{
		Ctx:  context.Background(),
		Name: "projects/_/buckets/photos",
	}
	if !e.Can(p, "storage.objects.get", res) {
		t.Fatal("live check granted permission: want true")
	}
	// The fake should have been hit exactly once — this bypassed the cache.
	if got := fc.callCount(); got != 1 {
		t.Fatalf("want 1 live call, got %d", got)
	}
	// And the request must carry the IAMResource.Name, not the evaluator's configured resource.
	fc.mu.Lock()
	got := fc.lastReq
	fc.mu.Unlock()
	if got == nil {
		t.Fatal("no request recorded")
	}
	if got.GetResource() != "projects/_/buckets/photos" {
		t.Fatalf("resource: want %q, got %q", "projects/_/buckets/photos", got.GetResource())
	}
	if len(got.GetPermissions()) != 1 || got.GetPermissions()[0] != "storage.objects.get" {
		t.Fatalf("permissions: want [storage.objects.get], got %v", got.GetPermissions())
	}
}

func TestCan_IAMResource_LiveCall_Denied(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller() // nothing granted
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0)
	defer e.Close()

	p := &principalWithID{id: "user:alice@example.com"}
	res := &IAMResource{
		Ctx:  context.Background(),
		Name: "projects/_/buckets/photos",
	}
	if e.Can(p, "storage.objects.delete", res) {
		t.Fatal("live check ungranted permission: want false")
	}
}

func TestCan_IAMResource_NilCtx_UsesBackground(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get")
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0)
	defer e.Close()

	p := &principalWithID{id: "user:alice@example.com"}
	// Ctx is nil — must not panic; evaluator falls back to context.Background().
	res := IAMResource{Name: "projects/demo/zones/us-central1-a/instances/vm1"}
	if !e.Can(p, "compute.instances.get", res) {
		t.Fatal("nil ctx: want true")
	}
}

func TestCan_IAMResource_EmptyName_FallsBackToEvaluatorResource(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get")
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0)
	defer e.Close()

	p := &principalWithID{id: "user:alice@example.com"}
	res := IAMResource{Ctx: context.Background()} // no Name
	if !e.Can(p, "compute.instances.get", res) {
		t.Fatal("empty name: want true")
	}
	fc.mu.Lock()
	got := fc.lastReq.GetResource()
	fc.mu.Unlock()
	if got != "projects/demo" {
		t.Fatalf("resource: want fallback %q, got %q", "projects/demo", got)
	}
}

func TestCan_IAMResource_UnknownPrincipal_ReturnsFalse(t *testing.T) {
	t.Parallel()
	fc := newFakeCaller("compute.instances.get")
	e := NewCachedEvaluatorFromCaller(fc, "projects/demo", 0)
	defer e.Close()

	res := IAMResource{Ctx: context.Background(), Name: "projects/demo"}
	// nil principal → extractor returns "" → false, no live call made.
	if e.Can(nil, "compute.instances.get", res) {
		t.Fatal("nil principal on live path: want false")
	}
	if got := fc.callCount(); got != 0 {
		t.Fatalf("expected zero live calls for unknown principal, got %d", got)
	}
}
