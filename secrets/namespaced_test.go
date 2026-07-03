package secrets

import (
	"context"
	"errors"
	"testing"

	secrets "oss.nandlabs.io/golly/secrets"
)

// fakeStore is a minimal secrets.Store used to observe what keys the
// underlying store receives after Namespaced qualifies them. It stands in
// for the concrete *GCPSecretStore so we can exercise the wiring without
// booting a real GCP client.
type fakeStore struct {
	getKey    string
	writeKey  string
	writeCred *secrets.Credential
	getCalls  int
	writeCall int
	getErr    error
	writeErr  error
	returnCr  *secrets.Credential
}

func (f *fakeStore) Get(key string, _ context.Context) (*secrets.Credential, error) {
	f.getCalls++
	f.getKey = key
	return f.returnCr, f.getErr
}

func (f *fakeStore) Write(key string, credential *secrets.Credential, _ context.Context) error {
	f.writeCall++
	f.writeKey = key
	f.writeCred = credential
	return f.writeErr
}

func (f *fakeStore) Provider() string { return "fake" }

// TestNamespaced_PrefixApplied verifies that a Get against a namespaced
// wrapper forwards a fully-qualified key to the inner store — proving the
// wire-up done by NewNamespacedGCPSecretStore's second step (secrets.Namespace).
func TestNamespaced_PrefixApplied(t *testing.T) {
	fake := &fakeStore{returnCr: &secrets.Credential{Value: []byte("v")}}
	store := secrets.Namespace(fake, "tenants/acme")

	if _, err := store.Get("mykey", context.Background()); err != nil {
		t.Fatalf("Get returned unexpected error: %v", err)
	}

	if fake.getCalls != 1 {
		t.Fatalf("expected inner Get called once, got %d", fake.getCalls)
	}
	if want := "tenants/acme/mykey"; fake.getKey != want {
		t.Errorf("inner store saw key %q, want %q", fake.getKey, want)
	}

	// Write should also be prefixed.
	cred := &secrets.Credential{Value: []byte("secret")}
	if err := store.Write("mykey", cred, context.Background()); err != nil {
		t.Fatalf("Write returned unexpected error: %v", err)
	}
	if want := "tenants/acme/mykey"; fake.writeKey != want {
		t.Errorf("inner store saw write key %q, want %q", fake.writeKey, want)
	}
}

// TestNamespaced_AuthorizerVetoesGet proves that a denying Authorizer
// short-circuits Get with ErrForbidden — the inner store is never touched.
func TestNamespaced_AuthorizerVetoesGet(t *testing.T) {
	fake := &fakeStore{}
	deny := func(_ context.Context, op secrets.Op, key string) error {
		if op != secrets.OpGet {
			t.Errorf("Authorizer got op %q, want %q", op, secrets.OpGet)
		}
		return secrets.ErrForbidden
	}
	store := secrets.Namespace(fake, "tenants/acme", secrets.WithAuthorizer(deny))

	_, err := store.Get("mykey", context.Background())
	if !errors.Is(err, secrets.ErrForbidden) {
		t.Fatalf("Get error = %v, want wrapping ErrForbidden", err)
	}
	if fake.getCalls != 0 {
		t.Errorf("inner store was called %d times; expected 0 (denied)", fake.getCalls)
	}
}

// TestNamespaced_AuthorizerVetoesWrite proves the same short-circuit path
// for the write op.
func TestNamespaced_AuthorizerVetoesWrite(t *testing.T) {
	fake := &fakeStore{}
	deny := func(_ context.Context, op secrets.Op, key string) error {
		if op != secrets.OpWrite {
			t.Errorf("Authorizer got op %q, want %q", op, secrets.OpWrite)
		}
		return secrets.ErrForbidden
	}
	store := secrets.Namespace(fake, "tenants/acme", secrets.WithAuthorizer(deny))

	err := store.Write("mykey", &secrets.Credential{Value: []byte("x")}, context.Background())
	if !errors.Is(err, secrets.ErrForbidden) {
		t.Fatalf("Write error = %v, want wrapping ErrForbidden", err)
	}
	if fake.writeCall != 0 {
		t.Errorf("inner store was called %d times; expected 0 (denied)", fake.writeCall)
	}
}

// TestNewNamespacedGCPSecretStore_ConfigError proves the convenience
// constructor propagates errors from NewGCPSecretStore instead of returning
// a broken wrapper.
func TestNewNamespacedGCPSecretStore_ConfigError(t *testing.T) {
	store, err := NewNamespacedGCPSecretStore(context.Background(), nil, "tenants/acme")
	if err == nil {
		t.Fatal("expected error when config is missing project ID")
	}
	if store != nil {
		t.Error("expected nil store when constructor errors")
	}
}
