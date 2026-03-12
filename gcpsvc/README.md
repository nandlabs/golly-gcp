# gcpsvc

GCP Service Configuration Utilities

This package provides utilities for managing Google Cloud Platform (GCP) client configurations, including project, region, credentials, endpoints, and other client options. It is designed to simplify the setup and retrieval of GCP client configurations for use in GCP service clients.

## Features

- Centralized management of GCP client options
- Support for credentials via file or JSON
- Project and region configuration
- Custom endpoint, user agent, quota project, and scopes
- Flexible retrieval of configuration based on URL or name

## Types

### Config

A struct that holds GCP client options and project/location info:

```go
type Config struct {
    Options   []option.ClientOption
    ProjectId string
    Location  string
}
```

## Usage

### Creating and Configuring a Client

```go
import (
    "google.golang.org/api/option"
    "oss.nandlabs.io/golly/golly-gcp/gcpsvc"
)

cfg := &gcpsvc.Config{}
cfg.SetProjectId("my-gcp-project")
cfg.SetRegion("us-central1")
cfg.SetAuthCredentialFile(option.ServiceAccount, "/path/to/credentials.json")
cfg.SetEndpoint("https://custom-endpoint")
cfg.SetUserAgent("my-app/1.0")
cfg.SetQuotaProject("my-quota-project")
cfg.SetScopes("scope1", "scope2")
```

### Adding Custom Options

```go
cfg.AddOption(option.WithGRPCConnectionPool(4))
```

### Managing Configs

The package provides a manager for storing and retrieving configs:

```go
gcpsvc.Manager.Add("my-key", cfg)
retrieved := gcpsvc.Manager.Get("my-key")
```

### Retrieving Config by URL or Name

```go
import "net/url"

u, _ := url.Parse("https://service.googleapis.com/resource")
config := gcpsvc.GetConfig(u, "default")
```

## License

See LICENSE file for details.
