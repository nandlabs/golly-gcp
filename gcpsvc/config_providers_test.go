package gcpsvc

import (
	"testing"

	"google.golang.org/api/option"
)

func TestConfig_SetProjectId(t *testing.T) {
	cfg := &Config{}
	cfg.SetProjectId("my-project")
	if cfg.ProjectId != "my-project" {
		t.Errorf("expected 'my-project', got %q", cfg.ProjectId)
	}
}

func TestConfig_SetRegion(t *testing.T) {
	cfg := &Config{}
	cfg.SetRegion("us-central1")
	if cfg.Location != "us-central1" {
		t.Errorf("expected 'us-central1', got %q", cfg.Location)
	}
}

func TestConfig_SetAuthCredentialFile(t *testing.T) {
	cfg := &Config{}
	opts := cfg.SetAuthCredentialFile(option.ServiceAccount, "/path/to/key.json")
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
	if len(cfg.Options) != 1 {
		t.Fatalf("expected cfg.Options to have 1 entry, got %d", len(cfg.Options))
	}
}

func TestConfig_SetAuthCredentialJSON(t *testing.T) {
	cfg := &Config{}
	jsonData := []byte(`{"type":"service_account"}`)
	opts := cfg.SetAuthCredentialJSON(option.ServiceAccount, jsonData)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}

func TestConfig_SetEndpoint(t *testing.T) {
	cfg := &Config{}
	opts := cfg.SetEndpoint("https://custom.endpoint.com")
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}

func TestConfig_SetUserAgent(t *testing.T) {
	cfg := &Config{}
	opts := cfg.SetUserAgent("my-agent/1.0")
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}

func TestConfig_SetQuotaProject(t *testing.T) {
	cfg := &Config{}
	opts := cfg.SetQuotaProject("billing-project")
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}

func TestConfig_SetScopes(t *testing.T) {
	cfg := &Config{}
	opts := cfg.SetScopes("https://www.googleapis.com/auth/cloud-platform", "https://www.googleapis.com/auth/devstorage.full_control")
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}

func TestConfig_AddOption(t *testing.T) {
	cfg := &Config{}
	cfg.AddOption(option.WithEndpoint("https://example.com"))
	if len(cfg.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(cfg.Options))
	}
}

func TestConfig_MultipleOptions(t *testing.T) {
	cfg := &Config{}
	cfg.SetEndpoint("https://ep1.com")
	cfg.SetUserAgent("agent/1.0")
	cfg.SetQuotaProject("proj")
	cfg.SetScopes("scope1")
	cfg.AddOption(option.WithEndpoint("https://ep2.com"))
	if len(cfg.Options) != 5 {
		t.Fatalf("expected 5 options, got %d", len(cfg.Options))
	}
}

func TestConfig_NilOptionsInitialized(t *testing.T) {
	cfg := &Config{}
	if cfg.Options != nil {
		t.Fatal("expected Options to be nil initially")
	}
	cfg.SetEndpoint("https://example.com")
	if cfg.Options == nil {
		t.Fatal("expected Options to be initialized after SetEndpoint")
	}
}

func TestConfig_DeprecatedSetCredentialFile(t *testing.T) {
	cfg := &Config{}
	opts := cfg.SetCredentialFile("/path/to/creds.json") //nolint:staticcheck
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}

func TestConfig_DeprecatedSetCredentialJSON(t *testing.T) {
	cfg := &Config{}
	opts := cfg.SetCredentialJSON([]byte(`{"type":"service_account"}`)) //nolint:staticcheck
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}
