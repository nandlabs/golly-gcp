package genai

import (
	"context"
	"iter"

	googlegenai "google.golang.org/genai"
)

// generateAPI abstracts the Google GenAI models client methods used by the provider.
// This interface enables testing with mock implementations.
type generateAPI interface {
	GenerateContent(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) (*googlegenai.GenerateContentResponse, error)
	GenerateContentStream(ctx context.Context, model string, contents []*googlegenai.Content, config *googlegenai.GenerateContentConfig) iter.Seq2[*googlegenai.GenerateContentResponse, error]
}
