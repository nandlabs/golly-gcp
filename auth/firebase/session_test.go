package firebase

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"oss.nandlabs.io/golly/auth"
)

// memDocStore is an in-memory docStore that mirrors Firestore's shape
// (map[string]any per document) so it exercises the same encode/decode
// paths as the real implementation.
type memDocStore struct {
	mu   sync.Mutex
	docs map[string]map[string]any
}

func newMemDocStore() *memDocStore {
	return &memDocStore{docs: make(map[string]map[string]any)}
}

func (m *memDocStore) set(_ context.Context, id string, doc map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Clone to guard against accidental caller-side mutation.
	clone := make(map[string]any, len(doc))
	for k, v := range doc {
		clone[k] = v
	}
	m.docs[id] = clone
	return nil
}

func (m *memDocStore) get(_ context.Context, id string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[id]
	if !ok {
		return nil, errDocNotFound
	}
	return d, nil
}

func (m *memDocStore) delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.docs[id]; !ok {
		return errDocNotFound
	}
	delete(m.docs, id)
	return nil
}

func (m *memDocStore) listByField(_ context.Context, field, value string) ([]map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []map[string]any
	for _, d := range m.docs {
		if v, ok := d[field].(string); ok && v == value {
			out = append(out, d)
		}
	}
	return out, nil
}

func newTestStore() *FirestoreSessionStore {
	return newSessionStoreWithBackend(newMemDocStore())
}

func mkSession(id, sub string) *auth.Session {
	now := time.Now().UTC().Truncate(time.Second)
	return &auth.Session{
		ID:        id,
		Subject:   sub,
		CreatedAt: now,
		LastSeen:  now,
		ExpiresAt: now.Add(time.Hour),
		Data:      map[string]any{"role": "user"},
	}
}

func TestSessionStore_PutGetRoundTrip(t *testing.T) {
	s := newTestStore()
	sess := mkSession("sid-1", "user-1")
	if err := s.Put(sess); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := s.Get(sess.ID)
	if !ok {
		t.Fatal("Get: not found after Put")
	}
	if got.ID != sess.ID || got.Subject != sess.Subject {
		t.Errorf("got %+v want %+v", got, sess)
	}
	if !got.ExpiresAt.Equal(sess.ExpiresAt) {
		t.Errorf("ExpiresAt: got %v want %v", got.ExpiresAt, sess.ExpiresAt)
	}
	if got.Data["role"] != "user" {
		t.Errorf("Data lost in round trip: %+v", got.Data)
	}
}

func TestSessionStore_GetMissing(t *testing.T) {
	s := newTestStore()
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get on empty store returned ok")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	s := newTestStore()
	if err := s.Put(mkSession("sid-1", "user-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("sid-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get("sid-1"); ok {
		t.Fatal("session still present after Delete")
	}
	// Idempotent: deleting a missing session is not an error.
	if err := s.Delete("sid-1"); err != nil {
		t.Fatalf("Delete (idempotent): %v", err)
	}
}

func TestSessionStore_PutEmptyIDFails(t *testing.T) {
	s := newTestStore()
	err := s.Put(&auth.Session{ID: ""})
	if err == nil {
		t.Fatal("Put with empty id: expected error")
	}
	if err := s.Put(nil); err == nil {
		t.Fatal("Put(nil): expected error")
	}
}

func TestSessionStore_ListByUser(t *testing.T) {
	s := newTestStore()
	for i, tc := range []struct {
		id, sub string
	}{
		{"a", "user-1"},
		{"b", "user-1"},
		{"c", "user-2"},
	} {
		if err := s.Put(mkSession(tc.id, tc.sub)); err != nil {
			t.Fatalf("Put[%d]: %v", i, err)
		}
	}
	got, err := s.ListByUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	ids := make([]string, len(got))
	for i, g := range got {
		ids[i] = g.ID
	}
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Errorf("ListByUser user-1: got %v want [a b]", ids)
	}

	empty, err := s.ListByUser(context.Background(), "user-3")
	if err != nil {
		t.Fatalf("ListByUser (empty): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ListByUser user-3: got %d, want 0", len(empty))
	}

	if _, err := s.ListByUser(context.Background(), ""); err == nil {
		t.Fatal("ListByUser(\"\"): expected error")
	}
}

func TestSessionStore_PayloadIsJSON(t *testing.T) {
	// Confirm the stored document really uses json.Marshal — exercise
	// the payload directly via the in-memory backend to catch drift.
	backend := newMemDocStore()
	s := newSessionStoreWithBackend(backend)
	sess := mkSession("sid-1", "user-1")
	if err := s.Put(sess); err != nil {
		t.Fatal(err)
	}
	doc := backend.docs[sess.ID]
	raw, ok := doc[payloadField]
	if !ok {
		t.Fatalf("payload field missing; doc=%v", doc)
	}
	b, ok := raw.([]byte)
	if !ok {
		t.Fatalf("payload has type %T, want []byte", raw)
	}
	var round auth.Session
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if round.Subject != sess.Subject {
		t.Errorf("json round-trip subject: got %q want %q", round.Subject, sess.Subject)
	}
	if s2, ok := doc[subjectField].(string); !ok || s2 != sess.Subject {
		t.Errorf("mirrored subject field: got %v want %q", doc[subjectField], sess.Subject)
	}
}

func TestSessionStore_PayloadStringBackend(t *testing.T) {
	// Some Firestore setups return the payload as a string rather than
	// []byte after a round trip; decodeSession should accept both.
	backend := newMemDocStore()
	s := newSessionStoreWithBackend(backend)
	sess := mkSession("sid-1", "user-1")
	if err := s.Put(sess); err != nil {
		t.Fatal(err)
	}
	doc := backend.docs[sess.ID]
	if b, ok := doc[payloadField].([]byte); ok {
		doc[payloadField] = string(b)
	}
	got, ok := s.Get(sess.ID)
	if !ok {
		t.Fatal("Get after string re-encode: not found")
	}
	if got.Subject != sess.Subject {
		t.Errorf("subject: got %q want %q", got.Subject, sess.Subject)
	}
}

// stubBackendErr forces every Get to fail — exercises the (nil,false)
// contract on transport errors.
type stubBackendErr struct{}

func (stubBackendErr) set(context.Context, string, map[string]any) error { return errors.New("boom") }
func (stubBackendErr) get(context.Context, string) (map[string]any, error) {
	return nil, errors.New("boom")
}
func (stubBackendErr) delete(context.Context, string) error { return errors.New("boom") }
func (stubBackendErr) listByField(context.Context, string, string) ([]map[string]any, error) {
	return nil, errors.New("boom")
}

func TestSessionStore_GetSwallowsTransportError(t *testing.T) {
	s := newSessionStoreWithBackend(stubBackendErr{})
	if _, ok := s.Get("x"); ok {
		t.Fatal("Get: expected (nil,false) on transport error")
	}
}

func TestSessionStore_DeletePropagatesTransportError(t *testing.T) {
	s := newSessionStoreWithBackend(stubBackendErr{})
	if err := s.Delete("x"); err == nil {
		t.Fatal("Delete: expected error to propagate")
	}
}

// Compile-time assertion: FirestoreSessionStore satisfies auth.SessionStore.
var _ auth.SessionStore = (*FirestoreSessionStore)(nil)
