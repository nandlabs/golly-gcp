package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
	secrets "oss.nandlabs.io/golly/secrets"
)

const (
	GCPSecretManagerProvider = "gcp-secret-manager"
)

// GCPSecretStore implements the Store interface using GCP Secret Manager
type GCPSecretStore struct {
	client    *secretmanager.Client
	projectID string
	labels    map[string]string
	mutex     sync.RWMutex
	cache     map[string]*secrets.Credential
	cacheTTL  time.Duration
	lastSync  map[string]time.Time
}

// GCPSecretStoreConfig holds configuration for creating a GCPSecretStore
type GCPSecretStoreConfig struct {
	ProjectID string            // GCP project ID
	Labels    map[string]string // Labels for organizing secrets
	CacheTTL  time.Duration     // Cache TTL (0 = no caching)
}

// NewGCPSecretStore creates a new GCP Secret Manager-backed store
func NewGCPSecretStore(ctx context.Context, cfg *GCPSecretStoreConfig) (*GCPSecretStore, error) {
	if cfg == nil {
		cfg = &GCPSecretStoreConfig{}
	}

	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("GCP project ID is required")
	}

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP Secret Manager client: %w", err)
	}

	return &GCPSecretStore{
		client:    client,
		projectID: cfg.ProjectID,
		labels:    cfg.Labels,
		cache:     make(map[string]*secrets.Credential),
		cacheTTL:  cfg.CacheTTL,
		lastSync:  make(map[string]time.Time),
	}, nil
}

// Get retrieves a credential from GCP Secret Manager
func (gs *GCPSecretStore) Get(key string, ctx context.Context) (*secrets.Credential, error) {
	gs.mutex.RLock()
	defer gs.mutex.RUnlock()

	// Check cache
	if cached, ok := gs.cache[key]; ok {
		if gs.cacheTTL == 0 || time.Since(gs.lastSync[key]) < gs.cacheTTL {
			return cached, nil
		}
	}

	// Build the secret name
	secretName := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", gs.projectID, key)

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: secretName,
	}

	result, err := gs.client.AccessSecretVersion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret from GCP Secret Manager: %w", err)
	}

	cred := &secrets.Credential{
		LastUpdated: time.Now(),
		MetaData:    make(map[string]interface{}),
	}

	// Parse secret value
	secretData := result.Payload.Data
	if secretData != nil {
		// Try to parse as JSON credential
		var credData map[string]interface{}
		if err := json.Unmarshal(secretData, &credData); err == nil {
			// Extract credential fields
			if value, ok := credData["value"]; ok {
				cred.Value = []byte(fmt.Sprintf("%v", value))
			}
			if version, ok := credData["version"].(string); ok {
				cred.Version = version
			}
			if metadata, ok := credData["metadata"].(map[string]interface{}); ok {
				cred.MetaData = metadata
			}
		} else {
			// Store raw secret data
			cred.Value = secretData
		}
	}

	// Add GCP metadata
	cred.MetaData["gcp_version"] = result.Name

	// Update cache
	gs.cache[key] = cred
	gs.lastSync[key] = time.Now()

	return cred, nil
}

// Write stores a credential in GCP Secret Manager
func (gs *GCPSecretStore) Write(key string, credential *secrets.Credential, ctx context.Context) error {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	// Prepare secret data
	secretData := map[string]interface{}{
		"value":        string(credential.Value),
		"version":      credential.Version,
		"last_updated": credential.LastUpdated.Unix(),
	}

	if credential.MetaData != nil {
		secretData["metadata"] = credential.MetaData
	}

	secretString, err := json.Marshal(secretData)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	parentName := fmt.Sprintf("projects/%s", gs.projectID)
	secretName := fmt.Sprintf("projects/%s/secrets/%s", gs.projectID, key)

	// Check if secret exists
	getSecretReq := &secretmanagerpb.GetSecretRequest{
		Name: secretName,
	}

	_, err = gs.client.GetSecret(ctx, getSecretReq)
	if err != nil {
		// Secret doesn't exist, create it
		createSecretReq := &secretmanagerpb.CreateSecretRequest{
			Parent:   parentName,
			SecretId: key,
			Secret: &secretmanagerpb.Secret{
				Replication: &secretmanagerpb.Replication{
					Replication: &secretmanagerpb.Replication_Automatic_{
						Automatic: &secretmanagerpb.Replication_Automatic{},
					},
				},
				Labels: gs.labels,
			},
		}

		secret, err := gs.client.CreateSecret(ctx, createSecretReq)
		if err != nil {
			return fmt.Errorf("failed to create secret in GCP Secret Manager: %w", err)
		}

		// Add secret version
		addVersionReq := &secretmanagerpb.AddSecretVersionRequest{
			Parent: secret.Name,
			Payload: &secretmanagerpb.SecretPayload{
				Data: secretString,
			},
		}

		_, err = gs.client.AddSecretVersion(ctx, addVersionReq)
		if err != nil {
			return fmt.Errorf("failed to add secret version: %w", err)
		}
	} else {
		// Secret exists, add a new version
		addVersionReq := &secretmanagerpb.AddSecretVersionRequest{
			Parent: secretName,
			Payload: &secretmanagerpb.SecretPayload{
				Data: secretString,
			},
		}

		_, err := gs.client.AddSecretVersion(ctx, addVersionReq)
		if err != nil {
			return fmt.Errorf("failed to add secret version: %w", err)
		}
	}

	// Update cache
	gs.cache[key] = credential
	gs.lastSync[key] = time.Now()

	return nil
}

// Delete removes a credential from GCP Secret Manager
func (gs *GCPSecretStore) Delete(key string, ctx context.Context) error {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	secretName := fmt.Sprintf("projects/%s/secrets/%s", gs.projectID, key)

	deleteReq := &secretmanagerpb.DeleteSecretRequest{
		Name: secretName,
	}

	err := gs.client.DeleteSecret(ctx, deleteReq)
	if err != nil {
		return fmt.Errorf("failed to delete secret from GCP Secret Manager: %w", err)
	}

	// Remove from cache
	delete(gs.cache, key)
	delete(gs.lastSync, key)

	return nil
}

// List lists all credentials
func (gs *GCPSecretStore) List(ctx context.Context) ([]string, error) {
	gs.mutex.RLock()
	defer gs.mutex.RUnlock()

	parentName := fmt.Sprintf("projects/%s", gs.projectID)
	listReq := &secretmanagerpb.ListSecretsRequest{
		Parent: parentName,
	}

	iter := gs.client.ListSecrets(ctx, listReq)

	var results []string
	for {
		secret, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}

		// Extract secret name from the full resource name
		secretName := secret.Name[len("projects/"+gs.projectID+"/secrets/"):]
		results = append(results, secretName)
	}

	return results, nil
}

// Provider returns the provider name
func (gs *GCPSecretStore) Provider() string {
	return GCPSecretManagerProvider
}

// Close closes the underlying client connection
func (gs *GCPSecretStore) Close() error {
	return gs.client.Close()
}

// ClearCache clears the in-memory cache
func (gs *GCPSecretStore) ClearCache() {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	gs.cache = make(map[string]*secrets.Credential)
	gs.lastSync = make(map[string]time.Time)
}

// GetClient returns the underlying GCP Secret Manager client for advanced operations
func (gs *GCPSecretStore) GetClient() *secretmanager.Client {
	return gs.client
}
