package secrets

import (
	"context"
	"testing"
	"time"

	"oss.nandlabs.io/golly/secrets"
)

func TestNewGCPSecretStore_InvalidConfig(t *testing.T) {
	ctx := context.Background()

	// Missing project ID
	_, err := NewGCPSecretStore(ctx, nil)
	if err == nil {
		t.Error("Expected error when project ID is missing")
	}

	if err.Error() != "GCP project ID is required" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestNewGCPSecretStore_WithConfig(t *testing.T) {
	ctx := context.Background()

	cfg := &GCPSecretStoreConfig{
		ProjectID: "test-project",
		Labels: map[string]string{
			"app": "golly",
		},
		CacheTTL: 5 * time.Minute,
	}

	store, err := NewGCPSecretStore(ctx, cfg)
	if err != nil {
		// Expected if GCP credentials are not configured
		t.Logf("Skipping test (GCP not available): %v", err)
		return
	}

	if store == nil {
		t.Fatal("Expected non-nil store")
	}

	if store.projectID != "test-project" {
		t.Errorf("Expected project ID test-project, got %s", store.projectID)
	}

	if store.cacheTTL != 5*time.Minute {
		t.Errorf("Expected cache TTL 5m, got %v", store.cacheTTL)
	}
}

func TestGCPSecretStore_Provider(t *testing.T) {
	ctx := context.Background()

	cfg := &GCPSecretStoreConfig{
		ProjectID: "test-project",
	}

	store, err := NewGCPSecretStore(ctx, cfg)
	if err != nil {
		t.Logf("Skipping test (GCP not available): %v", err)
		return
	}

	if got := store.Provider(); got != GCPSecretManagerProvider {
		t.Errorf("Provider() = %q, want %q", got, GCPSecretManagerProvider)
	}
}

func TestGCPSecretStore_ClearCache(t *testing.T) {
	ctx := context.Background()

	cfg := &GCPSecretStoreConfig{
		ProjectID: "test-project",
		CacheTTL:  5 * time.Minute,
	}

	store, err := NewGCPSecretStore(ctx, cfg)
	if err != nil {
		t.Logf("Skipping test (GCP not available): %v", err)
		return
	}

	// Add dummy credential to cache
	store.cache["test-key"] = &secrets.Credential{
		Value:   []byte("test"),
		Version: "1.0",
	}

	if len(store.cache) == 0 {
		t.Error("Expected cache to have entries")
	}

	store.ClearCache()

	if len(store.cache) != 0 {
		t.Error("Expected cache to be empty after clearing")
	}
}

func TestGCPSecretStore_GetClient(t *testing.T) {
	ctx := context.Background()

	cfg := &GCPSecretStoreConfig{
		ProjectID: "test-project",
	}

	store, err := NewGCPSecretStore(ctx, cfg)
	if err != nil {
		t.Logf("Skipping test (GCP not available): %v", err)
		return
	}

	client := store.GetClient()
	if client == nil {
		t.Error("GetClient() returned nil")
	}

	defer func() { _ = store.Close() }()
}

func TestGCPSecretStore_WithLabels(t *testing.T) {
	ctx := context.Background()

	labels := map[string]string{
		"app":  "golly",
		"env":  "test",
		"team": "platform",
	}

	cfg := &GCPSecretStoreConfig{
		ProjectID: "test-project",
		Labels:    labels,
	}

	store, err := NewGCPSecretStore(ctx, cfg)
	if err != nil {
		t.Logf("Skipping test (GCP not available): %v", err)
		return
	}

	if len(store.labels) != len(labels) {
		t.Errorf("Expected %d labels, got %d", len(labels), len(store.labels))
	}

	if store.labels["app"] != "golly" {
		t.Errorf("Expected label app=golly, got %s", store.labels["app"])
	}

	defer func() { _ = store.Close() }()
}
