package firestorelock

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

// The emulator tests use the same fake-emulator-project convention as
// Google Cloud examples: any project string is accepted by the
// emulator, but every test uses a unique docID so they don't step on
// each other.
const emulatorProject = "golly-emulator"

func newEmulatorClient(t *testing.T) *firestore.Client {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set; skipping Firestore emulator test")
	}
	ctx := context.Background()
	c, err := firestore.NewClient(ctx, emulatorProject)
	if err != nil {
		t.Fatalf("firestore.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func uniqueDocID(t *testing.T) string {
	return fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
}

func TestEmulator_ClaimAndRenew(t *testing.T) {
	client := newEmulatorClient(t)
	ctx := context.Background()

	opts := Options{
		Collection: "chrono-locks",
		DocID:      uniqueDocID(t),
		Owner:      "instance-a",
		Lease:      500 * time.Millisecond,
	}
	e := New(client, opts)
	if err := e.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Give the loop at least one full renewal.
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
	docID := uniqueDocID(t)

	a := New(client, Options{Collection: "chrono-locks", DocID: docID, Owner: "a", Lease: 500 * time.Millisecond})
	b := New(client, Options{Collection: "chrono-locks", DocID: docID, Owner: "b", Lease: 500 * time.Millisecond})

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
