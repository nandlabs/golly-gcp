package gcslock

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/storage"
)

// The GCS emulator (fake-gcs-server, gcs-emulator, etc.) accepts any
// bucket name that already exists; STORAGE_EMULATOR_HOST tells the
// client to use HTTP against the fake instead of api.googleapis.com.
func newEmulatorClient(t *testing.T) *storage.Client {
	t.Helper()
	if os.Getenv("STORAGE_EMULATOR_HOST") == "" {
		t.Skip("STORAGE_EMULATOR_HOST not set; skipping GCS emulator test")
	}
	ctx := context.Background()
	c, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("storage.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func emulatorBucket() string {
	if b := os.Getenv("GCSLOCK_TEST_BUCKET"); b != "" {
		return b
	}
	return "chrono-gcslock-test"
}

func uniqueObject(t *testing.T) string {
	return fmt.Sprintf("locks/%s-%d.json", t.Name(), time.Now().UnixNano())
}

func TestEmulator_ClaimAndRenew(t *testing.T) {
	client := newEmulatorClient(t)
	ctx := context.Background()

	opts := Options{
		Bucket: emulatorBucket(),
		Object: uniqueObject(t),
		Owner:  "instance-a",
		Lease:  500 * time.Millisecond,
	}
	e := New(client, opts)
	if err := e.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(750 * time.Millisecond)
	if !e.IsLeader(ctx) {
		t.Fatalf("expected leader after emulator tick")
	}
	if err := e.Resign(ctx); err != nil {
		t.Fatalf("Resign: %v", err)
	}
}

func TestEmulator_SecondInstanceLoses(t *testing.T) {
	client := newEmulatorClient(t)
	ctx := context.Background()
	obj := uniqueObject(t)

	a := New(client, Options{Bucket: emulatorBucket(), Object: obj, Owner: "a", Lease: 500 * time.Millisecond})
	b := New(client, Options{Bucket: emulatorBucket(), Object: obj, Owner: "b", Lease: 500 * time.Millisecond})

	if err := a.Start(ctx); err != nil {
		t.Fatalf("a.Start: %v", err)
	}
	if err := b.Start(ctx); err != nil {
		t.Fatalf("b.Start: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if !a.IsLeader(ctx) {
		t.Fatalf("expected a to be leader")
	}
	if b.IsLeader(ctx) {
		t.Fatalf("expected b to NOT be leader")
	}
	_ = a.Resign(ctx)
	_ = b.Resign(ctx)
}
