package genai

import (
	"context"
	"errors"
	"iter"
	"testing"

	googlegenai "google.golang.org/genai"
	gollygenai "oss.nandlabs.io/golly/genai"
)

// mockGenerateAPI implements generateAPI for testing.
type mockGenerateAPI struct {
	generateFunc       func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) (*googlegenai.GenerateContentResponse, error)
	generateStreamFunc func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) iter.Seq2[*googlegenai.GenerateContentResponse, error]
}

func (m *mockGenerateAPI) GenerateContent(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) (*googlegenai.GenerateContentResponse, error) {
	return m.generateFunc(ctx, model, contents, config)
}

func (m *mockGenerateAPI) GenerateContentStream(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) iter.Seq2[*googlegenai.GenerateContentResponse, error] {
	return m.generateStreamFunc(ctx, model, contents, config)
}

func newTestProvider(mock *mockGenerateAPI) *GCPProvider {
	return &GCPProvider{
		client:      mock,
		models:      []string{"gemini-2.0-flash"},
		description: "test provider",
		version:     "1.0.0-test",
	}
}

func TestGCPProvider_Name(t *testing.T) {
	p := newTestProvider(nil)
	if p.Name() != ProviderName {
		t.Errorf("expected %q, got %q", ProviderName, p.Name())
	}
}

func TestGCPProvider_Description(t *testing.T) {
	p := newTestProvider(nil)
	if p.Description() != "test provider" {
		t.Errorf("expected 'test provider', got %q", p.Description())
	}
}

func TestGCPProvider_Version(t *testing.T) {
	p := newTestProvider(nil)
	if p.Version() != "1.0.0-test" {
		t.Errorf("expected '1.0.0-test', got %q", p.Version())
	}
}

func TestGCPProvider_Models(t *testing.T) {
	p := newTestProvider(nil)
	models := p.Models()
	if len(models) != 1 || models[0] != "gemini-2.0-flash" {
		t.Errorf("expected ['gemini-2.0-flash'], got %v", models)
	}
}

func TestGCPProvider_Close(t *testing.T) {
	p := newTestProvider(nil)
	if err := p.Close(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestGCPProvider_Generate_Success(t *testing.T) {
	mock := &mockGenerateAPI{
		generateFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) (*googlegenai.GenerateContentResponse, error) {
			if model != "gemini-2.0-flash" {
				t.Errorf("expected model 'gemini-2.0-flash', got %q", model)
			}
			return &googlegenai.GenerateContentResponse{
				Candidates: []*googlegenai.Candidate{
					{
						Index:        0,
						FinishReason: "STOP",
						Content: &googlegenai.Content{
							Role:  "model",
							Parts: []*googlegenai.Part{{Text: "Hello from Gemini!"}},
						},
					},
				},
				UsageMetadata: &googlegenai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     5,
					CandidatesTokenCount: 10,
					TotalTokenCount:      15,
				},
			}, nil
		},
	}

	p := newTestProvider(mock)
	msg := gollygenai.NewTextMessage(gollygenai.RoleUser, "Hello")
	resp, err := p.Generate(context.Background(), "gemini-2.0-flash", msg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(resp.Candidates))
	}
	if resp.Candidates[0].Message.Parts[0].Text.Content != "Hello from Gemini!" {
		t.Errorf("unexpected response text: %q", resp.Candidates[0].Message.Parts[0].Text.Content)
	}
	if resp.Meta.InputTokens != 5 {
		t.Errorf("expected 5 input tokens, got %d", resp.Meta.InputTokens)
	}
}

func TestGCPProvider_Generate_Error(t *testing.T) {
	mock := &mockGenerateAPI{
		generateFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) (*googlegenai.GenerateContentResponse, error) {
			return nil, errors.New("API error")
		},
	}

	p := newTestProvider(mock)
	msg := gollygenai.NewTextMessage(gollygenai.RoleUser, "Hello")
	_, err := p.Generate(context.Background(), "gemini-2.0-flash", msg, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	_ = err // error already validated as non-nil above
}

func TestGCPProvider_Generate_WithOptions(t *testing.T) {
	mock := &mockGenerateAPI{
		generateFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) (*googlegenai.GenerateContentResponse, error) {
			if config.MaxOutputTokens != 512 {
				t.Errorf("expected MaxOutputTokens 512, got %d", config.MaxOutputTokens)
			}
			if config.SystemInstruction == nil {
				t.Error("expected SystemInstruction to be set")
			}
			return &googlegenai.GenerateContentResponse{
				Candidates: []*googlegenai.Candidate{
					{
						Content: &googlegenai.Content{
							Role:  "model",
							Parts: []*googlegenai.Part{{Text: "response"}},
						},
					},
				},
			}, nil
		},
	}

	p := newTestProvider(mock)
	msg := gollygenai.NewTextMessage(gollygenai.RoleUser, "Hello")
	opts := gollygenai.NewOptionsBuilder().
		SetMaxTokens(512).
		Add(gollygenai.OptionSystemInstructions, "Be brief").
		Build()

	resp, err := p.Generate(context.Background(), "gemini-2.0-flash", msg, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestGCPProvider_GenerateStream_Success(t *testing.T) {
	mock := &mockGenerateAPI{
		generateStreamFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) iter.Seq2[*googlegenai.GenerateContentResponse, error] {
			return func(yield func(*googlegenai.GenerateContentResponse, error) bool) {
				resp1 := &googlegenai.GenerateContentResponse{
					Candidates: []*googlegenai.Candidate{
						{
							Content: &googlegenai.Content{
								Role:  "model",
								Parts: []*googlegenai.Part{{Text: "chunk1"}},
							},
						},
					},
				}
				if !yield(resp1, nil) {
					return
				}
				resp2 := &googlegenai.GenerateContentResponse{
					Candidates: []*googlegenai.Candidate{
						{
							Content: &googlegenai.Content{
								Role:  "model",
								Parts: []*googlegenai.Part{{Text: "chunk2"}},
							},
						},
					},
				}
				yield(resp2, nil)
			}
		},
	}

	p := newTestProvider(mock)
	msg := gollygenai.NewTextMessage(gollygenai.RoleUser, "Stream me")
	respChan, errChan := p.GenerateStream(context.Background(), "gemini-2.0-flash", msg, nil)

	var responses []*gollygenai.GenResponse
	for resp := range respChan {
		responses = append(responses, resp)
	}

	// Drain error channel
	for err := range errChan {
		t.Fatalf("unexpected stream error: %v", err)
	}

	if len(responses) != 2 {
		t.Fatalf("expected 2 streamed responses, got %d", len(responses))
	}
}

func TestGCPProvider_GenerateStream_Error(t *testing.T) {
	mock := &mockGenerateAPI{
		generateStreamFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) iter.Seq2[*googlegenai.GenerateContentResponse, error] {
			return func(yield func(*googlegenai.GenerateContentResponse, error) bool) {
				yield(nil, errors.New("stream error"))
			}
		},
	}

	p := newTestProvider(mock)
	msg := gollygenai.NewTextMessage(gollygenai.RoleUser, "Stream me")
	respChan, errChan := p.GenerateStream(context.Background(), "gemini-2.0-flash", msg, nil)

	// Drain responses
	for range respChan {
	}

	var streamErr error
	for err := range errChan {
		streamErr = err
	}

	if streamErr == nil {
		t.Fatal("expected stream error")
	}
}
