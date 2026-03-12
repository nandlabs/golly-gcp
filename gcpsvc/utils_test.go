package gcpsvc

import (
	"net/url"
	"testing"
)

func TestGetConfig_NilURL(t *testing.T) {
	cfg := &Config{ProjectId: "fallback-project"}
	Manager.Register("test-svc", cfg)
	defer Manager.Unregister("test-svc")

	result := GetConfig(nil, "test-svc")
	if result == nil {
		t.Fatal("expected config from name fallback, got nil")
	}
	if result.ProjectId != "fallback-project" {
		t.Errorf("expected ProjectId 'fallback-project', got %q", result.ProjectId)
	}
}

func TestGetConfig_ByHost(t *testing.T) {
	cfg := &Config{ProjectId: "host-project"}
	Manager.Register("my-host", cfg)
	defer Manager.Unregister("my-host")

	u, _ := url.Parse("pubsub://my-host/topic")
	result := GetConfig(u, "fallback")
	if result == nil {
		t.Fatal("expected config from host lookup, got nil")
	}
	if result.ProjectId != "host-project" {
		t.Errorf("expected ProjectId 'host-project', got %q", result.ProjectId)
	}
}

func TestGetConfig_ByHostAndPath(t *testing.T) {
	cfg := &Config{ProjectId: "hostpath-project"}
	// URL path includes leading slash, so key is "my-host//topic"
	Manager.Register("my-host//topic", cfg)
	defer Manager.Unregister("my-host//topic")

	u, _ := url.Parse("pubsub://my-host/topic")
	result := GetConfig(u, "fallback")
	if result == nil {
		t.Fatal("expected config from host+path lookup, got nil")
	}
	if result.ProjectId != "hostpath-project" {
		t.Errorf("expected ProjectId 'hostpath-project', got %q", result.ProjectId)
	}
}

func TestGetConfig_FallbackToName(t *testing.T) {
	cfg := &Config{ProjectId: "name-project"}
	Manager.Register("my-service", cfg)
	defer Manager.Unregister("my-service")

	u, _ := url.Parse("pubsub://unknown-host/topic")
	result := GetConfig(u, "my-service")
	if result == nil {
		t.Fatal("expected config from name fallback, got nil")
	}
	if result.ProjectId != "name-project" {
		t.Errorf("expected ProjectId 'name-project', got %q", result.ProjectId)
	}
}

func TestGetConfig_NotFound(t *testing.T) {
	u, _ := url.Parse("pubsub://nonexistent/topic")
	result := GetConfig(u, "nonexistent-service")
	if result != nil {
		t.Errorf("expected nil config for nonexistent key, got %+v", result)
	}
}

func TestGetConfig_EmptyHost(t *testing.T) {
	cfg := &Config{ProjectId: "name-project"}
	Manager.Register("svc", cfg)
	defer Manager.Unregister("svc")

	u := &url.URL{Scheme: "pubsub", Path: "/some-path"}
	result := GetConfig(u, "svc")
	if result == nil {
		t.Fatal("expected config from name fallback, got nil")
	}
	if result.ProjectId != "name-project" {
		t.Errorf("expected ProjectId 'name-project', got %q", result.ProjectId)
	}
}
