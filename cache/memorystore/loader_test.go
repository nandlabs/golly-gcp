package memorystore

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSweep_ReturnsZero(t *testing.T) {
	c, _ := withMiniRedis(t)
	ctx := context.Background()

	// Populate a few keys, some with a TTL that has already lapsed.
	_ = c.Set(ctx, "keep", []byte("v"))
	_ = c.SetWithTTL(ctx, "gone", []byte("v"), 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	if got := c.Sweep(); got != 0 {
		t.Fatalf("Sweep on Redis backend must be a no-op returning 0; got %d", got)
	}
}

func TestGetOrLoad_MissThenLoads(t *testing.T) {
	c, _ := withMiniRedis(t)
	ctx := context.Background()

	var calls atomic.Int32
	load := func(context.Context) ([]byte, error) {
		calls.Add(1)
		return []byte("loaded"), nil
	}

	v, err := c.GetOrLoad(ctx, "k", time.Minute, load)
	if err != nil {
		t.Fatalf("GetOrLoad: %v", err)
	}
	if string(v) != "loaded" {
		t.Fatalf("got %q, want %q", v, "loaded")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("load calls = %d, want 1", got)
	}

	// The loader must have written through to the cache.
	got, ok := c.Get(ctx, "k")
	if !ok || string(got) != "loaded" {
		t.Fatalf("post-load Get(k) = (%q, %v), want (\"loaded\", true)", got, ok)
	}
}

func TestGetOrLoad_HitSkipsLoad(t *testing.T) {
	c, _ := withMiniRedis(t)
	ctx := context.Background()

	if err := c.Set(ctx, "k", []byte("preloaded")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var calls atomic.Int32
	load := func(context.Context) ([]byte, error) {
		calls.Add(1)
		return []byte("should-not-run"), nil
	}

	v, err := c.GetOrLoad(ctx, "k", time.Minute, load)
	if err != nil {
		t.Fatalf("GetOrLoad: %v", err)
	}
	if string(v) != "preloaded" {
		t.Fatalf("got %q, want %q", v, "preloaded")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("load must not run on hit; called %d times", got)
	}
}

func TestGetOrLoad_LoadErrorPropagates(t *testing.T) {
	c, _ := withMiniRedis(t)
	ctx := context.Background()

	sentinel := errors.New("upstream unavailable")
	load := func(context.Context) ([]byte, error) {
		return nil, sentinel
	}

	if _, err := c.GetOrLoad(ctx, "k", time.Minute, load); !errors.Is(err, sentinel) {
		t.Fatalf("GetOrLoad err = %v, want %v", err, sentinel)
	}

	// Errors must not populate the cache — the next caller must be able
	// to retry (and, in this test, still see the miss).
	if _, ok := c.Get(ctx, "k"); ok {
		t.Fatalf("failed load must not be cached")
	}
}

func TestGetOrLoad_SingleFlight_Concurrent(t *testing.T) {
	c, _ := withMiniRedis(t)
	ctx := context.Background()

	var calls atomic.Int32
	release := make(chan struct{})
	load := func(context.Context) ([]byte, error) {
		calls.Add(1)
		// Block long enough for every goroutine to converge on the
		// single-flight before we let the load complete.
		<-release
		return []byte("shared"), nil
	}

	const N = 32
	var wg sync.WaitGroup
	results := make([][]byte, N)
	errs := make([]error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := c.GetOrLoad(ctx, "sfkey", time.Minute, load)
			results[i] = v
			errs[i] = err
		}(i)
	}

	// Give every goroutine a moment to enter singleflight.Do before we
	// unblock the sole load in flight.
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 load invocation under single-flight, got %d", got)
	}
	for i := 0; i < N; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: err = %v", i, errs[i])
		}
		if string(results[i]) != "shared" {
			t.Fatalf("goroutine %d: got %q, want %q", i, results[i], "shared")
		}
	}
}
