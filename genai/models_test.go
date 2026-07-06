package genai

import (
	"testing"

	gollygenai "oss.nandlabs.io/golly/genai"
)

// newCatalogProvider returns a credential-free GCPProvider. The capability
// catalog is static, so ModelCatalog/ModelInfoFor/Name need no live client;
// the existing newTestProvider(nil) helper (genai_test.go) supplies one.
func newCatalogProvider() *GCPProvider { return newTestProvider(nil) }

func TestModelCatalog_NonEmpty(t *testing.T) {
	p := newCatalogProvider()
	catalog := p.ModelCatalog()
	if len(catalog) == 0 {
		t.Fatal("ModelCatalog returned an empty catalog")
	}
	for _, m := range catalog {
		if m.Name == "" {
			t.Errorf("model has empty Name: %+v", m)
		}
		if m.Provider != p.Name() {
			t.Errorf("model %q Provider = %q, want %q", m.Name, m.Provider, p.Name())
		}
		if len(m.Capabilities) == 0 {
			t.Errorf("model %q has no capabilities", m.Name)
		}
	}
}

func TestModelCatalog_ReturnsCopy(t *testing.T) {
	p := newCatalogProvider()
	first := p.ModelCatalog()
	if len(first) == 0 {
		t.Fatal("empty catalog")
	}
	// Mutate the returned slice.
	first[0] = gollygenai.ModelInfo{Name: "mutated"}

	second := p.ModelCatalog()
	if second[0].Name == "mutated" {
		t.Error("mutating the returned slice affected a subsequent ModelCatalog call")
	}
}

func TestModelInfoFor_Known(t *testing.T) {
	p := newCatalogProvider()
	info, ok := p.ModelInfoFor("gemini-2.5-pro")
	if !ok {
		t.Fatal("ModelInfoFor(gemini-2.5-pro) = false, want true")
	}
	if !info.Has(gollygenai.CapVision) {
		t.Error("gemini-2.5-pro should advertise CapVision")
	}
	if !info.Has(gollygenai.CapReasoning) {
		t.Error("gemini-2.5-pro should advertise CapReasoning")
	}
}

func TestModelInfoFor_Unknown(t *testing.T) {
	p := newCatalogProvider()
	if _, ok := p.ModelInfoFor("no-such-model"); ok {
		t.Error("ModelInfoFor(no-such-model) = true, want false")
	}
}

func TestEmbeddingModels_OnlyEmbeddings(t *testing.T) {
	p := newCatalogProvider()
	info, ok := p.ModelInfoFor("text-embedding-005")
	if !ok {
		t.Fatal("ModelInfoFor(text-embedding-005) = false, want true")
	}
	if !info.Has(gollygenai.CapEmbeddings) {
		t.Error("text-embedding-005 should advertise CapEmbeddings")
	}
	if info.Has(gollygenai.CapText) {
		t.Error("text-embedding-005 should NOT advertise CapText")
	}
}

func TestCapabilityProvider_Conformance(t *testing.T) {
	var p gollygenai.Provider = newCatalogProvider()
	if _, ok := p.(gollygenai.CapabilityProvider); !ok {
		t.Fatal("GCPProvider does not satisfy gollygenai.CapabilityProvider at runtime")
	}
}

// TestRouter_Integration is an end-to-end smoke test: build a ProviderSet and
// ModelRegistry from the provider, confirm the catalog reaches the registry,
// and confirm a CapabilityStrategy ranks a vision task onto a Gemini model.
func TestRouter_Integration(t *testing.T) {
	p := newCatalogProvider()
	set := gollygenai.NewProviderSet(p)

	registry, err := gollygenai.NewModelRegistry(set, gollygenai.RoutingConfig{})
	if err != nil {
		t.Fatalf("NewModelRegistry: %v", err)
	}

	all := registry.All()
	if len(all) != len(p.ModelCatalog()) {
		t.Errorf("registry.All() has %d models, want %d (the full catalog)", len(all), len(p.ModelCatalog()))
	}
	// Registry must contain a known catalog entry keyed by (provider, model).
	if _, ok := registry.Get(p.Name(), "gemini-2.5-pro"); !ok {
		t.Error("registry missing gemini-2.5-pro")
	}

	strategy := gollygenai.NewCapabilityStrategy()
	task := gollygenai.Task{RequiredCapabilities: []gollygenai.Capability{gollygenai.CapVision}}
	ranked := strategy.Rank(task, registry)
	if len(ranked) == 0 {
		t.Fatal("CapabilityStrategy ranked no candidates for a vision task")
	}

	var sawVisionGemini bool
	for _, c := range ranked {
		info, ok := registry.Get(c.Provider, c.Model)
		if !ok || !info.Has(gollygenai.CapVision) {
			t.Errorf("ranked candidate %s/%s is not vision-capable", c.Provider, c.Model)
		}
		if c.Provider == p.Name() {
			sawVisionGemini = true
		}
	}
	if !sawVisionGemini {
		t.Error("expected a vision-capable Gemini model in the ranking")
	}
}
