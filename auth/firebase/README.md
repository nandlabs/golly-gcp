# auth/firebase

Firebase Authentication adapter for [`oss.nandlabs.io/golly/auth`](https://pkg.go.dev/oss.nandlabs.io/golly/auth).

Ships two pluggable pieces:

- **`Verifier`** — a JWKS-backed JWT verifier for Firebase Auth /
  Google Identity Platform tokens. It fetches the public
  certificates from
  `https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com`,
  caches them per-kid with a configurable TTL (default 1h), and
  refreshes on demand when a token references an unknown kid. It
  validates the RS256 signature, `iss = https://securetoken.google.com/<projectID>`,
  `aud = <projectID>`, `exp` / `iat` (with configurable leeway) and
  a non-empty `sub`. Custom Firebase claims are surfaced on
  `Claims.Extra`.
- **`FirestoreSessionStore`** — an `auth.SessionStore` backed by
  Cloud Firestore. Sessions are serialized via `json.Marshal` into a
  document keyed by session id. The subject and expiry are mirrored
  as top-level fields so callers can query them directly (a
  `ListByUser(ctx, uid)` helper is provided).

## Quick start — verify a Firebase JWT in a `rest` handler

```go
import (
    "net/http"

    "oss.nandlabs.io/golly-gcp/auth/firebase"
    "oss.nandlabs.io/golly/rest"
)

func main() {
    v, err := firebase.NewVerifier("my-firebase-project")
    if err != nil { panic(err) }
    r := rest.NewRouter()
    r.Get("/me", func(w http.ResponseWriter, req *http.Request) {
        tok := req.Header.Get("Authorization")[len("Bearer "):]
        claims, err := v.VerifyToken(req.Context(), tok)
        if err != nil { http.Error(w, err.Error(), http.StatusUnauthorized); return }
        w.Write([]byte("hello " + claims.Subject))
    })
    http.ListenAndServe(":8080", r)
}
```

## Session store

```go
client, _ := firestore.NewClient(ctx, "my-project")
store := firebase.NewFirestoreSessionStore(client, "sessions")
sm := auth.NewSessionManager(store, auth.SessionConfig{Absolute: 24 * time.Hour})
```

`ListByUser(ctx, uid)` returns every active session document whose
`Subject` field matches `uid` — handy for a "sign out everywhere"
action or an admin UI. A Firestore TTL policy on the `expiresAt`
field is the recommended way to prune expired sessions server-side.
