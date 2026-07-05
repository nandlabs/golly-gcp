package firebase

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"oss.nandlabs.io/golly/auth"
)

// testSigner bundles an RSA key + kid to keep test tokens compact.
type testSigner struct {
	kid string
	key *rsa.PrivateKey
}

func newTestSigner(t *testing.T, kid string) *testSigner {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return &testSigner{kid: kid, key: k}
}

// certPEM returns a self-signed x509 certificate PEM whose public key
// matches s.key — the same shape Google's securetoken endpoint hands
// out for Firebase Auth tokens.
func (s *testSigner) certPEM(t *testing.T) string {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &s.key.PublicKey, s.key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// mint builds a signed compact JWT using RS256 with the signer's kid
// baked into the header.
func (s *testSigner) mint(t *testing.T, claims map[string]any) string {
	t.Helper()
	hdr := map[string]any{"alg": auth.AlgRS256, "typ": "JWT", "kid": s.kid}
	hb, _ := json.Marshal(hdr)
	pb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// jwksServer serves a JSON map of kid -> cert PEM. Every request bumps
// the atomic hit counter so tests can assert on refresh behavior.
type jwksServer struct {
	*httptest.Server
	hits atomic.Int64
	body atomic.Value // string
}

func newJWKSServer(t *testing.T, initial map[string]string) *jwksServer {
	t.Helper()
	js := &jwksServer{}
	js.set(initial)
	js.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		js.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(js.body.Load().(string)))
	}))
	t.Cleanup(js.Close)
	return js
}

func (js *jwksServer) set(m map[string]string) {
	b, _ := json.Marshal(m)
	js.body.Store(string(b))
}

// baseClaims returns a valid claim set for the given signer and
// projectID; individual tests mutate the map before minting.
func baseClaims(projectID, sub string) map[string]any {
	now := time.Now()
	return map[string]any{
		"iss": FirebaseIssuerPrefix + projectID,
		"aud": projectID,
		"sub": sub,
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}
}

func newVerifier(t *testing.T, projectID string, jwks *jwksServer, opts ...Option) *Verifier {
	t.Helper()
	full := append([]Option{WithJWKSURL(jwks.URL)}, opts...)
	v, err := NewVerifier(projectID, full...)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

func TestVerify_ValidToken_Success(t *testing.T) {
	const proj = "my-proj"
	signer := newTestSigner(t, "kid-1")
	jwks := newJWKSServer(t, map[string]string{signer.kid: signer.certPEM(t)})
	v := newVerifier(t, proj, jwks)

	claims := baseClaims(proj, "user-42")
	claims["email"] = "alice@example.com"
	token := signer.mint(t, claims)

	got, err := v.VerifyToken(context.Background(), token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if got.Subject != "user-42" {
		t.Errorf("subject: got %q want %q", got.Subject, "user-42")
	}
	if len(got.Audience) != 1 || got.Audience[0] != proj {
		t.Errorf("audience: got %v want [%s]", got.Audience, proj)
	}
	if got.Issuer != FirebaseIssuerPrefix+proj {
		t.Errorf("issuer: got %q", got.Issuer)
	}
	if v, ok := got.Extra["email"].(string); !ok || v != "alice@example.com" {
		t.Errorf("custom email claim not surfaced: %v", got.Extra)
	}
}

func TestVerify_BadSignature_Fails(t *testing.T) {
	const proj = "my-proj"
	signer := newTestSigner(t, "kid-1")
	jwks := newJWKSServer(t, map[string]string{signer.kid: signer.certPEM(t)})
	v := newVerifier(t, proj, jwks)

	token := signer.mint(t, baseClaims(proj, "user-42"))
	// Simplest tamper: replace the last four chars of the signature
	// with a fixed run so any JOSE base64url decode still succeeds but
	// the resulting bytes don't verify.
	tampered := token[:len(token)-4] + "AAAA"

	if _, err := v.VerifyToken(context.Background(), tampered); !errors.Is(err, auth.ErrInvalidSig) {
		t.Fatalf("VerifyToken: got %v, want ErrInvalidSig", err)
	}
}

func TestVerify_WrongIssuer_Fails(t *testing.T) {
	const proj = "my-proj"
	signer := newTestSigner(t, "kid-1")
	jwks := newJWKSServer(t, map[string]string{signer.kid: signer.certPEM(t)})
	v := newVerifier(t, proj, jwks)

	claims := baseClaims(proj, "user-42")
	claims["iss"] = "https://securetoken.google.com/other-project"
	token := signer.mint(t, claims)

	if _, err := v.VerifyToken(context.Background(), token); !errors.Is(err, auth.ErrIssuerMismatch) {
		t.Fatalf("VerifyToken: got %v, want ErrIssuerMismatch", err)
	}
}

func TestVerify_WrongAudience_Fails(t *testing.T) {
	const proj = "my-proj"
	signer := newTestSigner(t, "kid-1")
	jwks := newJWKSServer(t, map[string]string{signer.kid: signer.certPEM(t)})
	v := newVerifier(t, proj, jwks)

	claims := baseClaims(proj, "user-42")
	claims["aud"] = "some-other-project"
	token := signer.mint(t, claims)

	if _, err := v.VerifyToken(context.Background(), token); !errors.Is(err, auth.ErrAudMismatch) {
		t.Fatalf("VerifyToken: got %v, want ErrAudMismatch", err)
	}
}

func TestVerify_ExpiredToken_Fails(t *testing.T) {
	const proj = "my-proj"
	signer := newTestSigner(t, "kid-1")
	jwks := newJWKSServer(t, map[string]string{signer.kid: signer.certPEM(t)})
	// Set leeway to 0 so the small buffer doesn't rescue the token.
	v := newVerifier(t, proj, jwks, WithLeeway(0))

	claims := baseClaims(proj, "user-42")
	claims["iat"] = time.Now().Add(-2 * time.Hour).Unix()
	claims["exp"] = time.Now().Add(-time.Hour).Unix()
	token := signer.mint(t, claims)

	if _, err := v.VerifyToken(context.Background(), token); !errors.Is(err, auth.ErrExpired) {
		t.Fatalf("VerifyToken: got %v, want ErrExpired", err)
	}
}

func TestVerify_EmptySubject_Fails(t *testing.T) {
	const proj = "my-proj"
	signer := newTestSigner(t, "kid-1")
	jwks := newJWKSServer(t, map[string]string{signer.kid: signer.certPEM(t)})
	v := newVerifier(t, proj, jwks)

	claims := baseClaims(proj, "")
	token := signer.mint(t, claims)

	_, err := v.VerifyToken(context.Background(), token)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("VerifyToken: got %v, want ErrInvalidToken", err)
	}
}

func TestVerify_UnknownKid_RefreshesJWKS(t *testing.T) {
	const proj = "my-proj"
	signerOld := newTestSigner(t, "kid-old")
	signerNew := newTestSigner(t, "kid-new")
	// Start with only the old kid published.
	jwks := newJWKSServer(t, map[string]string{signerOld.kid: signerOld.certPEM(t)})
	v := newVerifier(t, proj, jwks)

	// Prime the cache with a valid old-kid token.
	if _, err := v.VerifyToken(context.Background(), signerOld.mint(t, baseClaims(proj, "user-old"))); err != nil {
		t.Fatalf("prime: %v", err)
	}
	primeHits := jwks.hits.Load()
	if primeHits == 0 {
		t.Fatalf("expected at least one JWKS fetch during priming")
	}

	// Rotate the JWKS to publish the new kid too.
	jwks.set(map[string]string{
		signerOld.kid: signerOld.certPEM(t),
		signerNew.kid: signerNew.certPEM(t),
	})

	// A token signed with the new kid should force a JWKS refresh.
	claims := baseClaims(proj, "user-new")
	token := signerNew.mint(t, claims)
	got, err := v.VerifyToken(context.Background(), token)
	if err != nil {
		t.Fatalf("VerifyToken (rotate): %v", err)
	}
	if got.Subject != "user-new" {
		t.Errorf("subject: got %q", got.Subject)
	}
	if jwks.hits.Load() <= primeHits {
		t.Errorf("expected another JWKS fetch on unknown kid; hits stayed at %d", jwks.hits.Load())
	}
}

func TestNewVerifier_RejectsEmptyProject(t *testing.T) {
	if _, err := NewVerifier(""); err == nil {
		t.Fatal("expected error for empty projectID")
	}
}

func TestVerifier_Alg(t *testing.T) {
	v, err := NewVerifier("p")
	if err != nil {
		t.Fatal(err)
	}
	if v.Alg() != auth.AlgRS256 {
		t.Errorf("Alg: got %q", v.Alg())
	}
}
