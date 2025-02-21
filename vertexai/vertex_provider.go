package vertexai

import (
	"bytes"
	"context"
	"fmt"

	vertexgenai "cloud.google.com/go/vertexai/genai"
	"oss.nandlabs.io/golly-gcp/gcpsvc"
	"oss.nandlabs.io/golly/errutils"
	"oss.nandlabs.io/golly/genai"
	"oss.nandlabs.io/golly/ioutils"
	"oss.nandlabs.io/golly/textutils"
)

const (
	DefaultVertexKey                      = "default-vertex-ai"
	ProviderKey                           = "provider_key"
	VertexAIDefaultTopK           int     = 0.0
	VertexAIDefaultCandidateCount int     = 1
	VertexAIDefaultTemp           float32 = 0.5
	VertexAIDefaultTopP           float32 = 0.0
)

var UnsupportedModelError = errutils.NewCustomError("unsupported model %s")
var UnsupportedOperationError = errutils.NewCustomError("unsupported operation %s")

// VertexAiProvider is a provider for VertexAI. This implements the genai.Provider interface from golly.
type VertexAiProvider struct {
	name        string
	description string
	version     string
}

// NewVertexAiProvider creates a new VertexAiProvider.
func NewVertexAiProvider() *VertexAiProvider {
	return &VertexAiProvider{
		name:        "VertexAI",
		description: "VertexAI provider",
		version:     "0.0.1",
	}
}

// Name returns the name of the provider.
func (p *VertexAiProvider) Name() string {
	return p.name
}

// Description returns the description of the provider.
func (p *VertexAiProvider) Description() string {
	return p.description
}

// Version returns the version of the provider.
func (p *VertexAiProvider) Version() string {
	return p.version
}

// Models returns the models of the provider.
func (p *VertexAiProvider) Models() ([]*genai.Model, error) {

	return nil, UnsupportedOperationError.Err("Models()")
}

func (p *VertexAiProvider) Generate(modelId string, exchange genai.Exchange, options *genai.Options) (err error) {
	var client *vertexgenai.Client
	var systemParts []vertexgenai.Part
	var userParts []vertexgenai.Part
	var response *vertexgenai.GenerateContentResponse
	key := options.GetString(ProviderKey)
	if key == textutils.EmptyStr {
		key = DefaultVertexKey
	}
	config := gcpsvc.Manager.Get(key)
	ctx := context.Background()
	client, err = vertexgenai.NewClient(ctx, config.ProjectId, config.Location, config.Options...)
	if err != nil {
		return
	}
	defer client.Close()
	model := client.GenerativeModel(modelId)
	if model == nil {
		err = UnsupportedModelError.Err(modelId)
		return
	}
	systemMessages := exchange.MsgsByActors(genai.SystemActor)
	if len(systemMessages) > 0 {
		systemParts, err = p.toVertexParts(systemMessages, exchange)
		if err != nil {
			return
		}
		model.SystemInstruction = &vertexgenai.Content{
			Parts: systemParts,
		}
	}
	userMessages := exchange.MsgsByActors(genai.UserActor)
	if len(userMessages) > 0 {
		userParts, err = p.toVertexParts(userMessages, exchange)
		if err != nil {
			return
		}
	}

	p.setOptions(model, options)
	response, err = model.GenerateContent(ctx, userParts...)
	if err != nil {
		return
	}
	if len(response.Candidates) == 0 {
		err = fmt.Errorf("no candidates returned")
		return
	}
	err = p.addPartsToExchange(response.Candidates[0].Content.Parts, exchange)

	return
}

func (p *VertexAiProvider) GenerateStream(model string, exchange genai.Exchange, handler genai.StreamingHandller,
	options *genai.Options) (err error) {

	return

}

func (p *VertexAiProvider) toVertexParts(msgs []*genai.Message, exchange genai.Exchange) (parts []vertexgenai.Part, err error) {
	for _, msg := range msgs {
		var part vertexgenai.Part
		switch msg.Mime() {
		case ioutils.MimeTextPlain, ioutils.MimeApplicationJSON, ioutils.MimeTextHTML, ioutils.MimeMarkDown, ioutils.MimeTextYAML:
			part = vertexgenai.Text(msg.String())
		default:
			if msg.URL() != nil {
				part = vertexgenai.FileData{
					MIMEType: msg.Mime(),
					FileURI:  msg.URL().String(),
				}

			} else {
				buf := new(bytes.Buffer)
				_, err = msg.WriteTo(buf)
				if err != nil {
					return
				}
				part = vertexgenai.Blob{
					MIMEType: msg.Mime(),
					Data:     buf.Bytes(),
				}
			}
		}
		parts = append(parts, part)
	}
	return
}

func (p *VertexAiProvider) addPartsToExchange(parts []vertexgenai.Part, exchange genai.Exchange) (err error) {
	for _, part := range parts {
		switch part.(type) {
		case vertexgenai.Text:
			text := part.(vertexgenai.Text)
			_, err = exchange.AddTxtMsg(string(text), genai.AIActor)
		case vertexgenai.Blob:
			blob := part.(vertexgenai.Blob)
			_, err = exchange.AddBinMsg(blob.Data, blob.MIMEType, genai.AIActor)
		case vertexgenai.FileData:
			file := part.(vertexgenai.FileData)
			_, err = exchange.AddFileMsg(file.FileURI, file.MIMEType, genai.AIActor)
		default:
			err = fmt.Errorf("unsupported part type %T", part)

		}

	}
	return
}

func (p *VertexAiProvider) setOptions(model *vertexgenai.GenerativeModel, options *genai.Options) (err error) {

	if options == nil {
		return
	}

	if options.Has(genai.OptionTemperature) {
		model.SetTemperature(options.GetTemperature(VertexAIDefaultTemp))
	}

	if options.Has(genai.OptionTopK) {
		model.SetTopK(int32(options.GetTopK(VertexAIDefaultTopK)))
	}

	if options.Has(genai.OptionTopP) {
		model.SetTopP(options.GetTopP(VertexAIDefaultTopP))
	}

	if options.Has(genai.OptionCandidateCount) {
		model.SetCandidateCount(int32(options.GetCandidateCount(VertexAIDefaultCandidateCount)))
	}

	return
}
