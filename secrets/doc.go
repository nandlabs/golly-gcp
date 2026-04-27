// Package secrets provides Google Cloud Secret Manager integration for Golly credential management.
//
// This package implements the Store interface from oss.nandlabs.io/golly/secrets
// using Google Cloud Secret Manager as the backend storage for credentials.
//
// # Features
//
// - Full integration with Golly credential types and metadata
// - Automatic secret creation if not exists
// - Label-based organization and filtering
// - Automatic version management for audit trail
// - In-memory caching with configurable TTL
// - Thread-safe concurrent operations
// - JSON-based credential storage
// - Automatic encryption at rest via GCP
//
// # Basic Usage
//
// Create a new store:
//
//	store, err := NewGCPSecretStore(ctx, &GCPSecretStoreConfig{
//	    ProjectID: "my-gcp-project",
//	})
//	defer store.Close()
//
// Write a credential:
//
//	cred := &secrets.Credential{
//	    Value:       []byte("secret-value"),
//	    LastUpdated: time.Now(),
//	    Version:     "1.0",
//	}
//	err := store.Write("my-secret", cred, ctx)
//
// Read a credential:
//
//	cred, err := store.Get("my-secret", ctx)
//
// Delete a credential:
//
//	err := store.Delete("my-secret", ctx)
//
// List all credentials:
//
//	keys, err := store.List(ctx)
//
// # GCP Requirements
//
// - Valid GCP credentials configured (via GOOGLE_APPLICATION_CREDENTIALS or gcloud auth)
// - Secret Manager API enabled in the GCP project
// - IAM permissions for secret.create, secret.get, secret.delete, secret.list, and versions.access
//
// # Caching
//
// The store supports optional in-memory caching to improve performance:
//
//	store, err := NewGCPSecretStore(ctx, &GCPSecretStoreConfig{
//	    ProjectID: "my-gcp-project",
//	    CacheTTL:  5 * time.Minute,
//	})
//
// # Version Management
//
// GCP Secret Manager automatically maintains a version history for each secret.
// Each write operation creates a new version, enabling:
// - Audit trails of all changes
// - Easy rollback to previous versions
// - Concurrent version access
//
// # Thread Safety
//
// All operations are protected by internal mutexes and are safe for concurrent use.
// Remember to call Close() when done to release the client connection.
package secrets
