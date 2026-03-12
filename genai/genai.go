package genai

import (
	"context"
	"fmt"

	googlegenai "google.golang.org/genai"
	"oss.nandlabs.io/golly-gcp/gcpsvc"
	gollygenai "oss.nandlabs.io/golly/genai"
)

const (
	// ProviderName is the name of the provider.
	ProviderName = "google-genai"
	// ProviderVersion is the version of the provider.
	ProviderVersion = "1.0.0"
	// ProviderDescription is the description of the provider.
	ProviderDescription = "Google GenAI provider for Vertex AI, Gemini API, and Model Garden via google.golang.org/genai SDK"
	// DefaultMaxTokens is the default max tokens if not specified in options.
	DefaultMaxTokens = 4096
)

// Backend selects which Google GenAI backend to use.
type Backend = googlegenai.Backend

// Backend constants re-exported for convenience.
var (
	// BackendVertexAI targets the Vertex AI endpoint (requires ProjectId/Location).
	BackendVertexAI = googlegenai.BackendVertexAI
	// BackendGeminiAPI targets the Gemini API endpoint (requires APIKey).
	BackendGeminiAPI = googlegenai.BackendGeminiAPI
)

// GCPProvider implements the golly genai.Provider interface using Google's GenAI SDK.
// It supports Vertex AI, Gemini API, and Model Garden backends.
type GCPProvider struct {
	client      generateAPI
	models      []string
	description string
	version     string
}

// ProviderConfig contains configuration for creating a GCPProvider.
type ProviderConfig struct {
	// CfgName is the name of the gcpsvc.Config registered with gcpsvc.Manager.
	// Used to resolve ProjectId and Location when not explicitly set.
	CfgName string
	// ProjectId is the GCP project ID. Overrides the value from gcpsvc config.
	ProjectId string
	// Location is the GCP region/location. Overrides the value from gcpsvc config.
	Location string
	// APIKey is the API key for Gemini API backend. When set and Backend is not
	// explicitly configured, the provider defaults to BackendGeminiAPI.
	APIKey string
	// Backend selects the Google GenAI backend (BackendVertexAI or BackendGeminiAPI).
	// When zero, it is inferred: BackendGeminiAPI if APIKey is set, else BackendVertexAI.
	Backend Backend
	// Models is the list of model IDs supported by this provider instance.
	Models []string
	// Description is a custom description for the provider.
	Description string
	// Version is a custom version for the provider.
	Version string
}

// NewGCPProvider creates a new GCPProvider with the given configuration.
// It resolves the Google GenAI client using the provided config or gcpsvc manager.
func NewGCPProvider(ctx context.Context, config *ProviderConfig) (gollygenai.Provider, error) {
	if config == nil {
		config = &ProviderConfig{}
	}

	projectId := config.ProjectId
	location := config.Location

	// Resolve from gcpsvc config if not explicitly set
	if (projectId == "" || location == "") && config.APIKey == "" {
		gcpCfg := gcpsvc.Manager.Get(config.CfgName)
		if gcpCfg != nil {
			if projectId == "" {
				projectId = gcpCfg.ProjectId
			}
			if location == "" {
				location = gcpCfg.Location
			}
		}
	}

	clientCfg := &googlegenai.ClientConfig{
		Project:  projectId,
		Location: location,
		APIKey:   config.APIKey,
	}

	// Determine backend
	switch {
	case config.Backend != 0:
		clientCfg.Backend = config.Backend
	case config.APIKey != "":
		clientCfg.Backend = googlegenai.BackendGeminiAPI
	default:
		clientCfg.Backend = googlegenai.BackendVertexAI
	}

	client, err := googlegenai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create GenAI client: %w", err)
	}

	desc := config.Description
	if desc == "" {
		desc = ProviderDescription
	}
	ver := config.Version
	if ver == "" {
		ver = ProviderVersion
	}

	return &GCPProvider{
		client:      client.Models,
		models:      config.Models,
		description: desc,
		version:     ver,
	}, nil
}

// Name returns the name of the provider.
func (p *GCPProvider) Name() string { return ProviderName }

// Description returns a brief description of the provider.
func (p *GCPProvider) Description() string { return p.description }

// Version returns the version of the provider.
func (p *GCPProvider) Version() string { return p.version }

// Models returns the list of model IDs supported by this provider instance.
func (p *GCPProvider) Models() []string { return p.models }

// Close releases provider resources. The GenAI client does not hold persistent
// connections, so this is a no-op.
func (p *GCPProvider) Close() error { return nil }

// Generate performs a synchronous inference call using the Google GenAI API.
func (p *GCPProvider) Generate(ctx context.Context, model string, message *gollygenai.Message, options *gollygenai.Options) (*gollygenai.GenResponse, error) {
	contents, sysContent := buildContents(message, options)
	config := buildGenerateConfig(options, sysContent)

	resp, err := p.client.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("genai GenerateContent API call failed: %w", err)
	}

	return toGenResponse(resp), nil
}

// GenerateStream performs a streaming inference call using the Google GenAI API.
func (p *GCPProvider) GenerateStream(ctx context.Context, model string, message *gollygenai.Message, options *gollygenai.Options) (<-chan *gollygenai.GenResponse, <-chan error) {
	responseChan := make(chan *gollygenai.GenResponse, 10)
	errorChan := make(chan error, 1)

	go func() {
		defer close(responseChan)
		defer close(errorChan)

		contents, sysContent := buildContents(message, options)
		config := buildGenerateConfig(options, sysContent)

		for resp, err := range p.client.GenerateContentStream(ctx, model, contents, config) {
			if err != nil {
				errorChan <- fmt.Errorf("genai streaming error: %w", err)
				return
			}

			select {
			case <-ctx.Done():
				errorChan <- ctx.Err()
				return
			default:
			}

			genResp := toGenResponse(resp)
			if genResp != nil {
				responseChan <- genResp
			}
		}
	}()

	return responseChan, errorChan
}
