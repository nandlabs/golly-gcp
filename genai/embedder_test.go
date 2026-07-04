package genai

import (
	"context"
	"errors"
	"testing"

	googlegenai "google.golang.org/genai"
	gollygenai "oss.nandlabs.io/golly/genai"
)

// Compile-time check that GCPProvider is recognized as a golly Embedder from
// the test binary too — mirrors the assertion in embedder.go.
var _ gollygenai.Embedder = (*GCPProvider)(nil)

func TestEmbed_SingleInput(t *testing.T) {
	expected := []float32{0.1, 0.2, 0.3}
	var seenModel string
	var seenContents []*googlegenai.Content

	mock := &mockGenerateAPI{
		embedFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.EmbedContentConfig) (*googlegenai.EmbedContentResponse, error) {
			seenModel = model
			seenContents = contents
			return &googlegenai.EmbedContentResponse{
				Embeddings: []*googlegenai.ContentEmbedding{
					{Values: expected},
				},
				Metadata: &googlegenai.EmbedContentMetadata{BillableCharacterCount: 5},
			}, nil
		},
	}
	p := newTestProvider(mock)

	req := &gollygenai.EmbedRequest{
		Model: "text-embedding-004",
		Inputs: []gollygenai.Part{
			{Text: &gollygenai.TextPart{Content: "hello"}},
		},
	}
	resp, err := p.Embed(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if seenModel != "text-embedding-004" {
		t.Errorf("expected model 'text-embedding-004', got %q", seenModel)
	}
	if len(seenContents) != 1 {
		t.Fatalf("expected 1 content passed to SDK, got %d", len(seenContents))
	}
	if len(seenContents[0].Parts) != 1 || seenContents[0].Parts[0].Text != "hello" {
		t.Errorf("expected single text part 'hello', got %+v", seenContents[0].Parts)
	}
	if len(resp.Vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(resp.Vectors))
	}
	if len(resp.Vectors[0]) != 3 || resp.Vectors[0][0] != 0.1 {
		t.Errorf("expected vector [0.1 0.2 0.3], got %v", resp.Vectors[0])
	}
	if resp.Meta == nil {
		t.Fatal("expected non-nil Meta populated from BillableCharacterCount")
	}
	if resp.Meta.InputTokens != 5 {
		t.Errorf("expected InputTokens=5, got %d", resp.Meta.InputTokens)
	}
}

func TestEmbed_MultipleInputs(t *testing.T) {
	var seenContents []*googlegenai.Content

	mock := &mockGenerateAPI{
		embedFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.EmbedContentConfig) (*googlegenai.EmbedContentResponse, error) {
			seenContents = contents
			out := make([]*googlegenai.ContentEmbedding, 0, len(contents))
			for i := range contents {
				out = append(out, &googlegenai.ContentEmbedding{
					Values: []float32{float32(i), float32(i) + 0.5},
				})
			}
			return &googlegenai.EmbedContentResponse{Embeddings: out}, nil
		},
	}
	p := newTestProvider(mock)

	req := &gollygenai.EmbedRequest{
		Inputs: []gollygenai.Part{
			{Text: &gollygenai.TextPart{Content: "first"}},
			{Text: &gollygenai.TextPart{Content: "second"}},
			{Text: &gollygenai.TextPart{Content: "third"}},
		},
	}
	resp, err := p.Embed(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seenContents) != 3 {
		t.Fatalf("expected SDK to receive 3 contents (batch-native), got %d", len(seenContents))
	}
	if len(resp.Vectors) != 3 {
		t.Fatalf("expected 3 vectors back, got %d", len(resp.Vectors))
	}
	// Verify batch order alignment.
	expectedTexts := []string{"first", "second", "third"}
	for i, want := range expectedTexts {
		if got := seenContents[i].Parts[0].Text; got != want {
			t.Errorf("input %d: expected text %q, got %q", i, want, got)
		}
		if resp.Vectors[i][0] != float32(i) {
			t.Errorf("vector %d: expected first component %v, got %v", i, float32(i), resp.Vectors[i][0])
		}
	}
}

func TestEmbed_ClientError_Propagates(t *testing.T) {
	mock := &mockGenerateAPI{
		embedFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.EmbedContentConfig) (*googlegenai.EmbedContentResponse, error) {
			return nil, errors.New("boom")
		},
	}
	p := newTestProvider(mock)

	_, err := p.Embed(context.Background(), &gollygenai.EmbedRequest{
		Model: "text-embedding-004",
		Inputs: []gollygenai.Part{
			{Text: &gollygenai.TextPart{Content: "hi"}},
		},
	})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !errors.Is(err, err) { // sanity: wrapped error exists
		t.Fatal("expected non-nil error chain")
	}
	// The provider wraps the error with an explanatory prefix; make sure the
	// wrapped inner error is still reachable.
	if !containsString(err.Error(), "boom") {
		t.Errorf("expected inner error 'boom' to be surfaced, got %q", err.Error())
	}
}

func TestEmbed_NilRequest(t *testing.T) {
	p := newTestProvider(&mockGenerateAPI{})
	_, err := p.Embed(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestEmbed_EmptyInputs(t *testing.T) {
	called := false
	mock := &mockGenerateAPI{
		embedFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.EmbedContentConfig) (*googlegenai.EmbedContentResponse, error) {
			called = true
			return &googlegenai.EmbedContentResponse{}, nil
		},
	}
	p := newTestProvider(mock)

	resp, err := p.Embed(context.Background(), &gollygenai.EmbedRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected no SDK call for empty inputs (short-circuit)")
	}
	if len(resp.Vectors) != 0 {
		t.Errorf("expected 0 vectors, got %d", len(resp.Vectors))
	}
}

func TestEmbed_NilEmbeddingInResponse(t *testing.T) {
	mock := &mockGenerateAPI{
		embedFunc: func(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.EmbedContentConfig) (*googlegenai.EmbedContentResponse, error) {
			return &googlegenai.EmbedContentResponse{
				Embeddings: []*googlegenai.ContentEmbedding{
					{Values: []float32{1.0}},
					nil,
				},
			}, nil
		},
	}
	p := newTestProvider(mock)
	resp, err := p.Embed(context.Background(), &gollygenai.EmbedRequest{
		Inputs: []gollygenai.Part{
			{Text: &gollygenai.TextPart{Content: "a"}},
			{Text: &gollygenai.TextPart{Content: "b"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Vectors) != 2 {
		t.Fatalf("expected 2 vectors preserved (including nil), got %d", len(resp.Vectors))
	}
	if resp.Vectors[1] != nil {
		t.Errorf("expected nil vector for nil ContentEmbedding, got %v", resp.Vectors[1])
	}
}

// containsString is a tiny helper to avoid pulling in strings just for tests.
func containsString(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
