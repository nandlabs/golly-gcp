package memorystore

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"oss.nandlabs.io/golly/cache"
)

func TestClient_SatisfiesCacheInterface(t *testing.T) {
	var _ cache.Cache[string, []byte] = (*Client)(nil)
}

func TestClient_NilConfig(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("expected ErrInvalidConfig for nil config")
	}
	if _, err := New(&Config{}); err == nil {
		t.Fatal("expected ErrInvalidConfig for empty Addrs")
	}
}

func withMiniRedis(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c, err := New(&Config{
		Addrs:      []string{mr.Addr()},
		DisableTLS: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, mr
}

func TestClient_SetGet(t *testing.T) {
	c, _ := withMiniRedis(t)
	ctx := context.Background()
	if err := c.Set(ctx, "k", []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok := c.Get(ctx, "k")
	if !ok || string(got) != "v" {
		t.Fatalf("Get(k) = (%q, %v), want (\"v\", true)", got, ok)
	}
	if _, ok := c.Get(ctx, "missing"); ok {
		t.Fatalf("Get(missing) should report ok=false")
	}
}

func TestClient_TTLExpiry(t *testing.T) {
	c, mr := withMiniRedis(t)
	ctx := context.Background()
	if err := c.SetWithTTL(ctx, "tkey", []byte("tv"), 100*time.Millisecond); err != nil {
		t.Fatalf("SetWithTTL: %v", err)
	}
	if _, ok := c.Get(ctx, "tkey"); !ok {
		t.Fatalf("expected key present within TTL")
	}
	mr.FastForward(200 * time.Millisecond)
	if _, ok := c.Get(ctx, "tkey"); ok {
		t.Fatalf("expected key absent past TTL")
	}
}

func TestClient_NoExpiry(t *testing.T) {
	c, mr := withMiniRedis(t)
	ctx := context.Background()
	_ = c.SetWithTTL(ctx, "k", []byte("v"), cache.NoExpiry)
	mr.FastForward(24 * time.Hour)
	if _, ok := c.Get(ctx, "k"); !ok {
		t.Fatalf("NoExpiry key should outlive any FastForward")
	}
}

func TestClient_DeleteAndHas(t *testing.T) {
	c, _ := withMiniRedis(t)
	ctx := context.Background()
	_ = c.Set(ctx, "x", []byte("1"))
	if !c.Has(ctx, "x") {
		t.Fatalf("Has(x) should be true after Set")
	}
	if !c.Delete(ctx, "x") {
		t.Fatalf("Delete(x) should report true on hit")
	}
	if c.Delete(ctx, "x") {
		t.Fatalf("Delete(x) should report false after key gone")
	}
}

func TestClient_ClearAndLen(t *testing.T) {
	c, _ := withMiniRedis(t)
	ctx := context.Background()
	for i, k := range []string{"a", "b", "c"} {
		_ = c.Set(ctx, k, []byte{byte('0' + i)})
	}
	if got := c.Len(ctx); got != 3 {
		t.Fatalf("Len = %d, want 3", got)
	}
	if err := c.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got := c.Len(ctx); got != 0 {
		t.Fatalf("Len after Clear = %d, want 0", got)
	}
}

func TestClient_PingValidatesConnection(t *testing.T) {
	c, _ := withMiniRedis(t)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClient_CredentialsProviderInvoked(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.RequireUserAuth("appuser", "rotating-token-v1")

	var calls int
	c, err := New(&Config{
		Addrs:      []string{mr.Addr()},
		Username:   "appuser",
		DisableTLS: true,
		CredentialsProvider: func(ctx context.Context) (string, error) {
			calls++
			return "rotating-token-v1", nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if calls == 0 {
		t.Fatalf("CredentialsProvider was never called")
	}
}
