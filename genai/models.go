package genai

import gollygenai "oss.nandlabs.io/golly/genai"

// Compile-time assertion that GCPProvider satisfies the optional golly genai
// CapabilityProvider interface. This is how the golly v1.8.0 model router
// discovers structured model metadata (capabilities, context windows, pricing)
// for capability-based routing — the router type-asserts each Provider to
// CapabilityProvider and folds its ModelCatalog into the ModelRegistry.
var _ gollygenai.CapabilityProvider = (*GCPProvider)(nil)

// vertexCatalog is the authoritative, static metadata for the Vertex AI /
// Gemini model surface this provider serves. The Provider field on every entry
// is ProviderName so the router's registry keys (provider/model) line up with
// this provider's registered name.
//
// Capabilities and token limits are provider-owned facts. Pricing figures are
// public list prices in USD per million tokens; operator RoutingConfig can
// override them (see golly genai RoutingConfig.CostOverrides).
//
// pricing as of 2026-07
var vertexCatalog = []gollygenai.ModelInfo{
	// ---- Gemini 2.5 ----
	{
		Name:        "gemini-2.5-pro",
		Provider:    ProviderName,
		DisplayName: "Gemini 2.5 Pro",
		Capabilities: []gollygenai.Capability{
			gollygenai.CapText, gollygenai.CapChat, gollygenai.CapStreaming,
			gollygenai.CapVision, gollygenai.CapAudio, gollygenai.CapToolCalling,
			gollygenai.CapJSONMode, gollygenai.CapReasoning,
		},
		MaxInputTokens:    1048576,
		MaxOutputTokens:   65536,
		InputCostPerMTok:  1.25, // pricing as of 2026-07 (<=200k prompt tier)
		OutputCostPerMTok: 10.0,
		Metadata:          map[string]string{"family": "gemini-2.5", "tier": "pro"},
	},
	{
		Name:        "gemini-2.5-flash",
		Provider:    ProviderName,
		DisplayName: "Gemini 2.5 Flash",
		Capabilities: []gollygenai.Capability{
			gollygenai.CapText, gollygenai.CapChat, gollygenai.CapStreaming,
			gollygenai.CapVision, gollygenai.CapAudio, gollygenai.CapToolCalling,
			gollygenai.CapJSONMode, gollygenai.CapReasoning,
		},
		MaxInputTokens:    1048576,
		MaxOutputTokens:   65536,
		InputCostPerMTok:  0.30, // pricing as of 2026-07
		OutputCostPerMTok: 2.50,
		Metadata:          map[string]string{"family": "gemini-2.5", "tier": "flash"},
	},
	{
		Name:        "gemini-2.5-flash-lite",
		Provider:    ProviderName,
		DisplayName: "Gemini 2.5 Flash-Lite",
		Capabilities: []gollygenai.Capability{
			gollygenai.CapText, gollygenai.CapChat, gollygenai.CapStreaming,
			gollygenai.CapVision, gollygenai.CapToolCalling, gollygenai.CapJSONMode,
		},
		MaxInputTokens:    1048576,
		MaxOutputTokens:   65536,
		InputCostPerMTok:  0.10, // pricing as of 2026-07
		OutputCostPerMTok: 0.40,
		Metadata:          map[string]string{"family": "gemini-2.5", "tier": "flash-lite"},
	},

	// ---- Gemini 2.0 ----
	{
		Name:        "gemini-2.0-flash",
		Provider:    ProviderName,
		DisplayName: "Gemini 2.0 Flash",
		Capabilities: []gollygenai.Capability{
			gollygenai.CapText, gollygenai.CapChat, gollygenai.CapStreaming,
			gollygenai.CapVision, gollygenai.CapAudio, gollygenai.CapToolCalling,
			gollygenai.CapJSONMode,
		},
		MaxInputTokens:    1048576,
		MaxOutputTokens:   8192,
		InputCostPerMTok:  0.15, // pricing as of 2026-07
		OutputCostPerMTok: 0.60,
		Metadata:          map[string]string{"family": "gemini-2.0", "tier": "flash"},
	},
	{
		Name:        "gemini-2.0-flash-lite",
		Provider:    ProviderName,
		DisplayName: "Gemini 2.0 Flash-Lite",
		Capabilities: []gollygenai.Capability{
			gollygenai.CapText, gollygenai.CapChat, gollygenai.CapStreaming,
			gollygenai.CapVision, gollygenai.CapToolCalling, gollygenai.CapJSONMode,
		},
		MaxInputTokens:    1048576,
		MaxOutputTokens:   8192,
		InputCostPerMTok:  0.075, // pricing as of 2026-07
		OutputCostPerMTok: 0.30,
		Metadata:          map[string]string{"family": "gemini-2.0", "tier": "flash-lite"},
	},

	// ---- Embeddings ----
	// Embedding models are routed through golly's EmbeddingRouter, not the
	// generate-shaped ModelRouter. They advertise only CapEmbeddings.
	{
		Name:             "text-embedding-005",
		Provider:         ProviderName,
		DisplayName:      "Text Embedding 005",
		Capabilities:     []gollygenai.Capability{gollygenai.CapEmbeddings},
		MaxInputTokens:   2048,
		InputCostPerMTok: 0.025, // pricing as of 2026-07
		Metadata:         map[string]string{"family": "text-embedding"},
	},
	{
		Name:             "text-multilingual-embedding-002",
		Provider:         ProviderName,
		DisplayName:      "Text Multilingual Embedding 002",
		Capabilities:     []gollygenai.Capability{gollygenai.CapEmbeddings},
		MaxInputTokens:   2048,
		InputCostPerMTok: 0.025, // pricing as of 2026-07
		Metadata:         map[string]string{"family": "text-multilingual-embedding"},
	},
}

// ModelCatalog returns the authoritative metadata for every model this provider
// serves. It returns a defensive copy so callers (and the router registry)
// cannot mutate the package-level catalog. Implements gollygenai.CapabilityProvider.
func (p *GCPProvider) ModelCatalog() []gollygenai.ModelInfo {
	out := make([]gollygenai.ModelInfo, len(vertexCatalog))
	copy(out, vertexCatalog)
	return out
}

// ModelInfoFor returns metadata for a single model id, if known. The bool is
// false for unknown models. Implements gollygenai.CapabilityProvider.
func (p *GCPProvider) ModelInfoFor(model string) (gollygenai.ModelInfo, bool) {
	for i := range vertexCatalog {
		if vertexCatalog[i].Name == model {
			return vertexCatalog[i], true
		}
	}
	return gollygenai.ModelInfo{}, false
}
