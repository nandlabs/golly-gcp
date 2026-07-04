package genai

import (
	"context"
	"fmt"

	googlegenai "google.golang.org/genai"
	gollygenai "oss.nandlabs.io/golly/genai"
)

// Compile-time assertion that GCPProvider satisfies the optional golly genai
// Embedder interface. Callers may type-assert on the Provider to access the
// batch embedding API.
var _ gollygenai.Embedder = (*GCPProvider)(nil)

// Embed generates embedding vectors for one or more inputs using the Vertex
// AI / Gemini EmbedContent endpoint. Each input Part in req.Inputs produces
// one vector in the response, in the same order.
//
// Text parts are wired to the model as text content; file parts are wired via
// URI; binary parts are wired as inline bytes with their MimeType. Unknown /
// empty parts are treated as empty text so the response length still aligns
// with req.Inputs.
//
// req.Model overrides the embedding-model id; when empty, the caller is
// expected to have configured a default on the SDK client — the empty string
// is passed through unchanged so the SDK's own default takes over.
func (p *GCPProvider) Embed(ctx context.Context, req *gollygenai.EmbedRequest) (*gollygenai.EmbedResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("genai Embed: nil request")
	}
	if len(req.Inputs) == 0 {
		return &gollygenai.EmbedResponse{Vectors: nil}, nil
	}

	contents := embedInputsToContents(req.Inputs)

	resp, err := p.client.EmbedContent(ctx, req.Model, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("genai EmbedContent API call failed: %w", err)
	}

	return toEmbedResponse(resp), nil
}

// embedInputsToContents converts a slice of golly genai Parts into the Google
// GenAI SDK's []*Content. Each input Part becomes its own Content with a
// single Part so the SDK produces one vector per input.
func embedInputsToContents(inputs []gollygenai.Part) []*googlegenai.Content {
	contents := make([]*googlegenai.Content, 0, len(inputs))
	for i := range inputs {
		p := convertPart(&inputs[i])
		if p == nil {
			// Preserve alignment with request order: empty text placeholder.
			p = googlegenai.NewPartFromText("")
		}
		contents = append(contents, googlegenai.NewContentFromParts([]*googlegenai.Part{p}, "user"))
	}
	return contents
}

// toEmbedResponse converts the SDK EmbedContentResponse into a golly
// EmbedResponse. Vector values are copied as-is (both sides use float32).
func toEmbedResponse(resp *googlegenai.EmbedContentResponse) *gollygenai.EmbedResponse {
	if resp == nil {
		return &gollygenai.EmbedResponse{}
	}

	out := &gollygenai.EmbedResponse{
		Vectors: make([][]float32, 0, len(resp.Embeddings)),
	}
	for _, e := range resp.Embeddings {
		if e == nil {
			out.Vectors = append(out.Vectors, nil)
			continue
		}
		out.Vectors = append(out.Vectors, e.Values)
	}

	// Vertex reports a billable character count on the response metadata;
	// surface it via InputTokens as a rough proxy so callers have a usage
	// signal without inventing a new field.
	if resp.Metadata != nil && resp.Metadata.BillableCharacterCount > 0 {
		out.Meta = &gollygenai.ResponseMeta{
			InputTokens: int(resp.Metadata.BillableCharacterCount),
			TotalTokens: int(resp.Metadata.BillableCharacterCount),
		}
	}

	return out
}
