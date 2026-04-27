# GCP Secret Manager Store

This package provides a Golly Store implementation backed by Google Cloud Secret Manager.

## Features

- **Full Credential Support**: Works with all Golly credential types
- **Version Management**: Automatic version tracking for secrets
- **Label Support**: Organize and tag secrets with GCP labels
- **In-Memory Caching**: Optional caching with TTL
- **JSON Storage**: Stores credentials as JSON for easy integration
- **Automatic Encryption**: GCP Secret Manager encrypts all secrets by default

## Creating a GCP Secret Store

### Basic Configuration

```go
store, err := NewGCPSecretStore(ctx, &GCPSecretStoreConfig{
    ProjectID: "my-gcp-project",
})
```

### With Labels

```go
store, err := NewGCPSecretStore(ctx, &GCPSecretStoreConfig{
    ProjectID: "my-gcp-project",
    Labels: map[string]string{
        "app":  "golly",
        "env":  "production",
    },
})
```

### With Caching

```go
store, err := NewGCPSecretStore(ctx, &GCPSecretStoreConfig{
    ProjectID: "my-gcp-project",
    CacheTTL:  5 * time.Minute,
})
```

## Usage

### Writing a Credential

```go
cred := &secrets.Credential{
    Value:       []byte("secret-api-key"),
    LastUpdated: time.Now(),
    Version:     "1.0",
}

err := store.Write("my-api-key", cred, context.Background())
```

### Reading a Credential

```go
cred, err := store.Get("my-api-key", context.Background())
```

### Deleting a Credential

```go
err := store.Delete("my-api-key", context.Background())
```

### Listing All Credentials

```go
keys, err := store.List(context.Background())
```

## IAM Permissions

Minimum IAM role required: `roles/secretmanager.secretAccessor`

For secret creation and management, use: `roles/secretmanager.admin`

Or create a custom role with these permissions:

```
secretmanager.secrets.create
secretmanager.secrets.get
secretmanager.secrets.delete
secretmanager.secrets.list
secretmanager.versions.access
secretmanager.versions.add
```

## Advanced Usage

### Using the Secret Manager Client Directly

```go
client := store.GetClient()
// Use client for advanced operations
```

### Clearing the Cache

```go
store.ClearCache()
```

### Closing the Store

```go
err := store.Close()
```

## Storage Format

Credentials are stored as JSON in GCP Secret Manager:

```json
{
  "value": "secret-value",
  "version": "1.0",
  "last_updated": 1682505600,
  "metadata": {
    "provider": "GCP",
    "project": "my-project"
  },
  "gcp_version": "projects/my-project/secrets/my-key/versions/1"
}
```

## Version Management

GCP Secret Manager automatically maintains version history. Each write creates a new version:

- Latest version is always accessible via `versions/latest`
- Previous versions are retained for rollback
- Automatic versioning ensures audit trail

## Performance Considerations

- GCP Secret Manager includes Client Libraries in Google Cloud quotas
- Use caching to reduce API calls and latency
- Consider regional endpoints for latency optimization
- Enable replication for multi-region deployments

## Error Handling

```go
cred, err := store.Get("nonexistent", context.Background())
if err != nil {
    if strings.Contains(err.Error(), "NotFound") {
        log.Println("Secret not found")
    }
}
```

## Thread Safety

The store is thread-safe for concurrent operations due to internal mutex protection.

## Environment Setup

Ensure your GCP credentials are configured:

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/credentials.json
```

Or use Application Default Credentials (ADC):

```bash
gcloud auth application-default login
```
