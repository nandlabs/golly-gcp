// Package firebase provides a Firebase Authentication adapter for the
// golly auth primitives.
//
// It ships two building blocks that plug into oss.nandlabs.io/golly/auth:
//
//   - Verifier: a JWKS-backed JWT verifier for tokens minted by Firebase
//     Authentication / Google Identity Platform. It fetches the public
//     x509 certificates from Google's securetoken endpoint, caches them
//     per-kid with a configurable TTL, and validates the RS256 signature
//     plus the issuer, audience, expiry and subject claims.
//
//   - FirestoreSessionStore: an auth.SessionStore implementation backed
//     by Cloud Firestore. Sessions are serialized as JSON documents
//     keyed by session id and can be queried by their subject via the
//     ListByUser helper.
//
// Neither type registers itself with any manager: callers wire them
// explicitly into their golly auth verification or session middleware.
package firebase
