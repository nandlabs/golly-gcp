package firebase

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"oss.nandlabs.io/golly/auth"
)

// DefaultJWKSURL is the Google endpoint that returns Firebase Auth
// signing certificates as a JSON object keyed by key id (kid). Each
// value is an x509 certificate in PEM form whose public key is the
// RSA public key that signed the corresponding tokens.
const DefaultJWKSURL = "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com"

// DefaultKeyTTL is the fallback cache lifetime for JWKS entries when
// the caller does not configure WithKeyTTL. Google's certificates are
// rotated on the order of days, so an hour is a conservative default.
const DefaultKeyTTL = time.Hour

// DefaultLeeway is the clock-skew allowance used when validating exp
// and iat during Verifier.VerifyToken.
const DefaultLeeway = 60 * time.Second

// FirebaseIssuerPrefix is prepended to the projectID to derive the
// expected iss claim for Firebase-minted tokens.
const FirebaseIssuerPrefix = "https://securetoken.google.com/"

// Verifier is a Firebase Auth JWT verifier. It caches JWKS entries
// per-kid and refreshes them on demand when a token references an
// unknown kid. Verifier is safe for concurrent use.
type Verifier struct {
	projectID string
	jwksURL   string
	ttl       time.Duration
	leeway    time.Duration
	httpc     *http.Client
	now       func() time.Time

	mu   sync.RWMutex
	keys map[string]*keyEntry
}

type keyEntry struct {
	pub     *rsa.PublicKey
	expires time.Time
}

// Option customizes a Verifier at construction time.
type Option func(*Verifier)

// WithJWKSURL overrides the JWKS endpoint. Primarily used by tests.
func WithJWKSURL(url string) Option { return func(v *Verifier) { v.jwksURL = url } }

// WithHTTPClient overrides the HTTP client used to fetch JWKS.
func WithHTTPClient(c *http.Client) Option {
	return func(v *Verifier) {
		if c != nil {
			v.httpc = c
		}
	}
}

// WithKeyTTL sets the cache lifetime for fetched JWKS entries.
func WithKeyTTL(d time.Duration) Option {
	return func(v *Verifier) {
		if d > 0 {
			v.ttl = d
		}
	}
}

// WithLeeway sets the clock-skew allowance used by VerifyToken.
func WithLeeway(d time.Duration) Option {
	return func(v *Verifier) {
		if d >= 0 {
			v.leeway = d
		}
	}
}

// WithNow overrides the time source used by VerifyToken. Primarily
// used by tests to exercise expired/not-yet-valid paths.
func WithNow(fn func() time.Time) Option {
	return func(v *Verifier) {
		if fn != nil {
			v.now = fn
		}
	}
}

// NewVerifier constructs a Firebase JWT verifier for the given
// projectID. The returned *Verifier implements auth.Verifier and can
// be plugged into auth.VerifyOptions.VerifierFallback directly; the
// higher-level VerifyToken helper is recommended for typical usage.
func NewVerifier(projectID string, opts ...Option) (*Verifier, error) {
	if projectID == "" {
		return nil, errors.New("auth/firebase: projectID is required")
	}
	v := &Verifier{
		projectID: projectID,
		jwksURL:   DefaultJWKSURL,
		ttl:       DefaultKeyTTL,
		leeway:    DefaultLeeway,
		httpc:     http.DefaultClient,
		now:       time.Now,
		keys:      make(map[string]*keyEntry),
	}
	for _, o := range opts {
		o(v)
	}
	return v, nil
}

// Alg reports the algorithm this verifier validates. Firebase tokens
// are always RS256.
func (v *Verifier) Alg() string { return auth.AlgRS256 }

// Verify implements auth.Verifier by extracting the kid from the JWS
// header carried in signingInput, resolving the RSA public key (with
// a JWKS refresh on unknown kid), and validating the RS256 signature.
// It does not perform claim validation — for that use VerifyToken.
func (v *Verifier) Verify(signingInput, sig []byte) error {
	dot := indexByte(signingInput, '.')
	if dot < 0 {
		return auth.ErrInvalidToken
	}
	hdrB, err := base64.RawURLEncoding.DecodeString(string(signingInput[:dot]))
	if err != nil {
		return auth.ErrInvalidToken
	}
	var hdr auth.Header
	if err := json.Unmarshal(hdrB, &hdr); err != nil {
		return auth.ErrInvalidToken
	}
	if hdr.Alg != auth.AlgRS256 {
		return auth.ErrAlgNotAllowed
	}
	if hdr.Kid == "" {
		return fmt.Errorf("%w: token missing kid", auth.ErrKeyNotFound)
	}
	pub, err := v.keyForKid(context.Background(), hdr.Kid)
	if err != nil {
		return fmt.Errorf("%w: %v", auth.ErrKeyNotFound, err)
	}
	sum := sha256.Sum256(signingInput)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return auth.ErrInvalidSig
	}
	return nil
}

// Keyset returns an auth.Verifier bound to a specific kid. It is
// suitable for wiring into auth.VerifyOptions.Keyset when the caller
// prefers explicit per-kid dispatch over Verifier's own header parse.
func (v *Verifier) Keyset(kid string) (auth.Verifier, error) {
	pub, err := v.keyForKid(context.Background(), kid)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", auth.ErrKeyNotFound, err)
	}
	return auth.NewRSVerifier(auth.AlgRS256, pub)
}

// VerifyToken parses, signature-checks and claim-validates a compact
// Firebase-issued JWT. It enforces:
//   - alg = RS256
//   - iss = https://securetoken.google.com/<projectID>
//   - aud contains <projectID>
//   - exp is in the future (with configured leeway)
//   - iat is in the past  (with configured leeway)
//   - sub is non-empty
//
// The returned Claims carry the RFC 7519 registered fields plus any
// Firebase custom claims exposed via Claims.Extra.
func (v *Verifier) VerifyToken(ctx context.Context, token string) (*auth.Claims, error) {
	// Prime the JWKS cache before delegating to auth.Verify so that
	// keyForKid uses the caller-supplied context on the network fetch.
	if kid := peekKid(token); kid != "" {
		if _, err := v.keyForKid(ctx, kid); err != nil {
			return nil, fmt.Errorf("%w: %v", auth.ErrKeyNotFound, err)
		}
	}
	claims, err := auth.Verify(token, auth.VerifyOptions{
		Algs:             []string{auth.AlgRS256},
		VerifierFallback: v,
		Issuer:           FirebaseIssuerPrefix + v.projectID,
		Audience:         v.projectID,
		Leeway:           v.leeway,
		Now:              v.now,
	})
	if err != nil {
		return nil, err
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: sub claim is empty", auth.ErrInvalidToken)
	}
	if claims.IssuedAt != nil {
		if claims.IssuedAt.After(v.now().Add(v.leeway)) {
			return nil, auth.ErrNotYetValid
		}
	}
	return claims, nil
}

// keyForKid returns the cached public key for kid, refreshing the
// JWKS on a cache miss or when the cached entry has expired.
func (v *Verifier) keyForKid(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	now := v.now()
	v.mu.RLock()
	entry, ok := v.keys[kid]
	v.mu.RUnlock()
	if ok && now.Before(entry.expires) {
		return entry.pub, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	v.mu.RLock()
	entry, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("kid %q not present in JWKS", kid)
	}
	return entry.pub, nil
}

// refresh fetches the JWKS and replaces the in-memory cache. It uses
// a lock across the network call so concurrent misses collapse into a
// single fetch.
func (v *Verifier) refresh(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("auth/firebase: JWKS fetch returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var raw map[string]string
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("auth/firebase: decode JWKS: %w", err)
	}
	if len(raw) == 0 {
		return errors.New("auth/firebase: JWKS response is empty")
	}
	next := make(map[string]*keyEntry, len(raw))
	exp := v.now().Add(v.ttl)
	for kid, certPEM := range raw {
		pub, err := parseRSAPublicKeyFromCertPEM([]byte(certPEM))
		if err != nil {
			return fmt.Errorf("auth/firebase: kid %q: %w", kid, err)
		}
		next[kid] = &keyEntry{pub: pub, expires: exp}
	}
	v.keys = next
	return nil
}

// parseRSAPublicKeyFromCertPEM extracts the RSA public key from an
// x509 certificate encoded as PEM. Non-RSA keys are rejected — the
// Firebase endpoint always returns RSA certs.
func parseRSAPublicKeyFromCertPEM(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse cert: %w", err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("certificate public key is %T, want *rsa.PublicKey", cert.PublicKey)
	}
	return pub, nil
}

// peekKid extracts the kid header from a compact JWT without any
// signature check. It returns "" on any parse error — the subsequent
// Verify pass will surface the specific problem.
func peekKid(token string) string {
	dot := strings.IndexByte(token, '.')
	if dot < 0 {
		return ""
	}
	hb, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return ""
	}
	var h auth.Header
	if err := json.Unmarshal(hb, &h); err != nil {
		return ""
	}
	return h.Kid
}

// indexByte is a byte-slice equivalent of strings.IndexByte; inlined
// here to avoid a string conversion on the hot signature-verify path.
func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
