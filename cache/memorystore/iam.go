package memorystore

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// IAMAuthOptions configures the IAM auth-token generator returned by
// IAMAuthProvider.
type IAMAuthOptions struct {
	// CredentialsJSON is an optional service-account-key JSON. When nil,
	// Application Default Credentials (ADC) are used — typical for code
	// running on GCE / GKE / Cloud Run where the metadata server provides
	// credentials automatically.
	CredentialsJSON []byte

	// Scopes overrides the default scope ("https://www.googleapis.com/auth/cloud-platform").
	// Most users can leave this empty.
	Scopes []string
}

// IAMAuthProvider returns a callback suitable for Config.CredentialsProvider
// that produces short-lived OAuth2 access tokens for Memorystore for
// Redis 7.x IAM authentication.
//
// The returned token source caches each access token until shortly before
// expiry and transparently refreshes — go-redis calls the provider on
// every connection establishment, but token issuance hits the network
// only when a refresh is needed.
//
// Example using Application Default Credentials:
//
//	c, _ := memorystore.New(&memorystore.Config{
//	    Addrs:    []string{"10.0.0.3:6378"},
//	    Username: "iam-app@project.iam.gserviceaccount.com",
//	    CredentialsProvider: memorystore.IAMAuthProvider(memorystore.IAMAuthOptions{}),
//	})
//
// Example with a service-account key file:
//
//	keyJSON, _ := os.ReadFile("sa-key.json")
//	c, _ := memorystore.New(&memorystore.Config{
//	    Addrs:               []string{"10.0.0.3:6378"},
//	    Username:            "iam-app@project.iam.gserviceaccount.com",
//	    CredentialsProvider: memorystore.IAMAuthProvider(memorystore.IAMAuthOptions{
//	        CredentialsJSON: keyJSON,
//	    }),
//	})
//
// See "Configure Redis Auth with IAM" in the Memorystore docs:
// https://cloud.google.com/memorystore/docs/redis/about-iam-auth
func IAMAuthProvider(opts IAMAuthOptions) func(ctx context.Context) (string, error) {
	scopes := opts.Scopes
	if len(scopes) == 0 {
		scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
	}

	// Lazily initialise the token source on first call so an unreachable
	// metadata server doesn't fail New() at construction.
	var tokenSource oauth2.TokenSource

	return func(ctx context.Context) (string, error) {
		if tokenSource == nil {
			var (
				creds *google.Credentials
				err   error
			)
			if len(opts.CredentialsJSON) > 0 {
				creds, err = google.CredentialsFromJSON(ctx, opts.CredentialsJSON, scopes...)
			} else {
				creds, err = google.FindDefaultCredentials(ctx, scopes...)
			}
			if err != nil {
				return "", fmt.Errorf("load google credentials: %w", err)
			}
			tokenSource = creds.TokenSource
		}
		t, err := tokenSource.Token()
		if err != nil {
			return "", fmt.Errorf("fetch oauth2 token: %w", err)
		}
		return t.AccessToken, nil
	}
}
