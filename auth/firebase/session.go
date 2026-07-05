package firebase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"oss.nandlabs.io/golly/auth"
)

// docStore is the minimal abstraction over a Firestore collection
// used by FirestoreSessionStore. It exists so that unit tests can
// substitute an in-memory implementation without spinning up the
// Firestore emulator.
type docStore interface {
	set(ctx context.Context, id string, doc map[string]any) error
	get(ctx context.Context, id string) (map[string]any, error)
	delete(ctx context.Context, id string) error
	listByField(ctx context.Context, field, value string) ([]map[string]any, error)
}

// errDocNotFound signals a document lookup miss. It is not returned
// from the auth.SessionStore surface directly (that returns a bool),
// but the FirestoreSessionStore branches on it internally.
var errDocNotFound = errors.New("auth/firebase: document not found")

// payloadField is the Firestore field under which the JSON-encoded
// session bytes are stored. Kept as a top-level field to keep Get
// cheap and Set atomic.
const payloadField = "payload"

// subjectField is stored alongside the payload so ListByUser can run
// a native where("subject","==",id) query without hydrating every
// document first.
const subjectField = "subject"

// expiresAtField mirrors Session.ExpiresAt at the top level so callers
// (or a Firestore TTL policy) can prune expired sessions.
const expiresAtField = "expiresAt"

// FirestoreSessionStore persists auth.Session values to Firestore.
// Each session is serialized via json.Marshal into a single document
// keyed by Session.ID. The Subject and ExpiresAt fields are mirrored
// alongside the payload so they can be queried directly.
type FirestoreSessionStore struct {
	docs docStore
	// timeout caps every Firestore RPC. Zero disables the cap.
	timeout time.Duration
}

// NewFirestoreSessionStore constructs a session store backed by the
// given Firestore client and top-level collection. The returned type
// satisfies auth.SessionStore and additionally exposes ListByUser.
func NewFirestoreSessionStore(client *firestore.Client, collection string) *FirestoreSessionStore {
	if client == nil {
		panic("auth/firebase: nil firestore.Client")
	}
	if collection == "" {
		panic("auth/firebase: empty collection")
	}
	return &FirestoreSessionStore{docs: &firestoreDocStore{coll: client.Collection(collection)}}
}

// newSessionStoreWithBackend is the unit-test constructor; production
// callers should use NewFirestoreSessionStore.
func newSessionStoreWithBackend(d docStore) *FirestoreSessionStore {
	return &FirestoreSessionStore{docs: d}
}

// WithTimeout sets a per-call context deadline applied to every RPC.
// A non-positive value clears the timeout.
func (s *FirestoreSessionStore) WithTimeout(d time.Duration) *FirestoreSessionStore {
	s.timeout = d
	return s
}

// Get implements auth.SessionStore. Returns (nil, false) when the id
// is unknown or on any transport error — the interface has no error
// channel, so failures are logged only via the caller's own layer.
func (s *FirestoreSessionStore) Get(id string) (*auth.Session, bool) {
	ctx, cancel := s.ctx()
	defer cancel()
	m, err := s.docs.get(ctx, id)
	if err != nil {
		return nil, false
	}
	sess, err := decodeSession(m)
	if err != nil {
		return nil, false
	}
	return sess, true
}

// Put implements auth.SessionStore.
func (s *FirestoreSessionStore) Put(sess *auth.Session) error {
	if sess == nil || sess.ID == "" {
		return errors.New("auth/firebase: empty session id")
	}
	doc, err := encodeSession(sess)
	if err != nil {
		return err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	return s.docs.set(ctx, sess.ID, doc)
}

// Delete implements auth.SessionStore. Missing documents are treated
// as a success — callers of SessionStore.Delete typically already
// treat idempotent revocation as the desired shape.
func (s *FirestoreSessionStore) Delete(id string) error {
	ctx, cancel := s.ctx()
	defer cancel()
	err := s.docs.delete(ctx, id)
	if err != nil && errors.Is(err, errDocNotFound) {
		return nil
	}
	return err
}

// ListByUser returns every session document whose Subject equals uid.
// It is not part of auth.SessionStore but is a common enough query on
// server-side sessions to be worth exposing.
func (s *FirestoreSessionStore) ListByUser(ctx context.Context, uid string) ([]*auth.Session, error) {
	if uid == "" {
		return nil, errors.New("auth/firebase: empty user id")
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	docs, err := s.docs.listByField(ctx, subjectField, uid)
	if err != nil {
		return nil, err
	}
	out := make([]*auth.Session, 0, len(docs))
	for _, d := range docs {
		sess, err := decodeSession(d)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, nil
}

// ctx returns a context that carries the configured timeout (or a
// no-op cancel when timeout is 0).
func (s *FirestoreSessionStore) ctx() (context.Context, context.CancelFunc) {
	if s.timeout <= 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), s.timeout)
}

// encodeSession marshals a session into the on-disk shape: the JSON
// payload plus mirrored subject and expiry fields.
func encodeSession(sess *auth.Session) (map[string]any, error) {
	b, err := json.Marshal(sess)
	if err != nil {
		return nil, fmt.Errorf("auth/firebase: marshal session: %w", err)
	}
	return map[string]any{
		payloadField:   b,
		subjectField:   sess.Subject,
		expiresAtField: sess.ExpiresAt,
	}, nil
}

// decodeSession recovers a session from its stored map form.
func decodeSession(doc map[string]any) (*auth.Session, error) {
	raw, ok := doc[payloadField]
	if !ok {
		return nil, fmt.Errorf("auth/firebase: document missing %q field", payloadField)
	}
	var b []byte
	switch v := raw.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return nil, fmt.Errorf("auth/firebase: %q field has unsupported type %T", payloadField, raw)
	}
	var sess auth.Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return nil, fmt.Errorf("auth/firebase: unmarshal session: %w", err)
	}
	return &sess, nil
}

// firestoreDocStore is the production implementation of docStore that
// talks to a real Firestore collection.
type firestoreDocStore struct {
	coll *firestore.CollectionRef
}

func (f *firestoreDocStore) set(ctx context.Context, id string, doc map[string]any) error {
	_, err := f.coll.Doc(id).Set(ctx, doc)
	return err
}

func (f *firestoreDocStore) get(ctx context.Context, id string) (map[string]any, error) {
	snap, err := f.coll.Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, errDocNotFound
		}
		return nil, err
	}
	return snap.Data(), nil
}

func (f *firestoreDocStore) delete(ctx context.Context, id string) error {
	_, err := f.coll.Doc(id).Delete(ctx)
	if err != nil && status.Code(err) == codes.NotFound {
		return errDocNotFound
	}
	return err
}

func (f *firestoreDocStore) listByField(ctx context.Context, field, value string) ([]map[string]any, error) {
	it := f.coll.Where(field, "==", value).Documents(ctx)
	defer it.Stop()
	var out []map[string]any
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, snap.Data())
	}
}
