package vertex

import (
	"bytes"
	"context"
	"fmt"

	vertexgenai "cloud.google.com/go/vertexai/genai"
	"google.golang.org/api/iterator"
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

// getClient initializes and returns a new Vertex AI client using the provided options.
// It retrieves the configuration based on the provided key from the options.
// If the key is not provided, it defaults to DefaultVertexKey.
// The function returns the initialized client and any error encountered during the process.
//
// Parameters:
//
//	options (*genai.Options): The options containing the configuration key.
//
// Returns:
//
//	client (*vertexgenai.Client): The initialized Vertex AI client.
//	err (error): Any error encountered during the client initialization.
func (p *VertexAiProvider) getClient(options *genai.Options) (client *vertexgenai.Client, err error) {
	key := options.GetString(ProviderKey)
	if key == textutils.EmptyStr {
		key = DefaultVertexKey
	}
	config := gcpsvc.Manager.Get(key)
	ctx := context.Background()
	client, err = vertexgenai.NewClient(ctx, config.ProjectId, config.Location, config.Options...)
	return
}

// prepareModel prepares a GenerativeModel for the given modelId using the provided vertexgenai.Client,
// exchange, and options. It converts the exchange into vertexgenai.Part objects for both system and user actors,
// sets the system instructions on the model if any system parts are found, and applies the provided options to the model.
//
// Parameters:
//   - modelId: The ID of the model to prepare.
//   - client: The vertexgenai.Client used to retrieve the GenerativeModel.
//   - exchange: The genai.Exchange containing the messages to convert into vertexgenai.Part objects.
//   - options: The genai.Options to apply to the model.
//
// Returns:
//   - model: The prepared vertexgenai.GenerativeModel.
//   - userParts: A slice of vertexgenai.Part objects representing the user messages.
//   - err: An error if the model is unsupported, no user messages are found, or if there is an error during conversion.
func (p *VertexAiProvider) prepareModel(modelId string, client *vertexgenai.Client,
	exchange genai.Exchange,
	options *genai.Options) (model *vertexgenai.GenerativeModel, userParts []vertexgenai.Part, err error) {
	var systemParts []vertexgenai.Part
	model = client.GenerativeModel(modelId)
	if model == nil {
		err = UnsupportedModelError.Err(modelId)
		return
	}
	systemParts, err = p.toVertexParts(genai.SystemActor, exchange)
	if err != nil {
		return
	}
	if len(systemParts) > 0 {
		model.SystemInstruction = &vertexgenai.Content{
			Parts: systemParts,
		}
	}
	userParts, err = p.toVertexParts(genai.UserActor, exchange)
	if err != nil {
		return
	}
	if len(userParts) == 0 && len(systemParts) == 0 {
		err = fmt.Errorf("no user messages found")
		return
	}
	p.setOptions(model, options)
	return

}

// Generate generates content using the specified model and exchange options.
// It initializes the client, prepares the model, and generates content based on the user parts.
// If successful, it adds the generated parts to the exchange.
//
// Parameters:
//   - modelId: The ID of the model to be used for content generation.
//   - exchange: The exchange object containing the input data for content generation.
//   - options: Additional options for the content generation process.
//
// Returns:
//   - err: An error object if any error occurs during the content generation process.
func (p *VertexAiProvider) Generate(modelId string, exchange genai.Exchange, options *genai.Options) (err error) {
	var client *vertexgenai.Client
	var model *vertexgenai.GenerativeModel
	var userParts []vertexgenai.Part
	var response *vertexgenai.GenerateContentResponse

	client, err = p.getClient(options)
	if err != nil {
		return
	}
	defer client.Close()
	model, userParts, err = p.prepareModel(modelId, client, exchange, options)
	if err != nil {
		return
	}

	response, err = model.GenerateContent(context.Background(), userParts...)
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

// GenerateStream generates a stream of content using the specified model and handles the streaming responses.
//
// Parameters:
// - modelId: The ID of the model to use for generating content.
// - exchange: The exchange object containing the input data for the model.
// - handler: A callback function to handle the streaming responses.
// - options: Additional options for generating content.
//
// Returns:
// - err: An error if the content generation or streaming fails.
//
// The function performs the following steps:
// 1. Initializes the Vertex AI client and prepares the model.
// 2. Generates a stream of content using the model and processes the responses.
// 3. Calls the handler function with the generated messages.
// 4. Merges the responses and adds the parts to the exchange object.
//
// If an error occurs at any step, the function returns the error.
func (p *VertexAiProvider) GenerateStream(modelId string, exchange genai.Exchange, handler genai.StreamingHandller,
	options *genai.Options) (err error) {
	var client *vertexgenai.Client
	var model *vertexgenai.GenerativeModel
	var userParts []vertexgenai.Part
	var resIte *vertexgenai.GenerateContentResponseIterator
	var response *vertexgenai.GenerateContentResponse

	client, err = p.getClient(options)
	if err != nil {
		return
	}
	defer client.Close()
	model, userParts, err = p.prepareModel(modelId, client, exchange, options)
	if err != nil {
		return
	}
	resIte = model.GenerateContentStream(context.Background(), userParts...)
	var current, next *vertexgenai.GenerateContentResponse
	current, err = resIte.Next()
	if err != nil {
		return
	}
	for {
		next, err = resIte.Next()
		var messages []*genai.Message
		if err != nil && err != iterator.Done {
			return err
		}
		if len(current.Candidates) == 0 || len(current.Candidates[0].Content.Parts) == 0 {
			if next != nil {
				current = next
				continue
			}
		}
		messages, err = p.partsToMessages(current.Candidates[0].Content.Parts)
		if err != nil {
			return
		}
		handler(next == nil, messages...)

		if next == nil {
			break
		}
		current = next
	}
	response = resIte.MergedResponse()
	if len(response.Candidates) == 0 {
		err = fmt.Errorf("no candidates returned")
		return
	}
	err = p.addPartsToExchange(response.Candidates[0].Content.Parts, exchange)
	return

}

// toVertexParts converts messages associated with a given actor in an exchange
// to a slice of vertexgenai.Part. It handles different MIME types and creates
// appropriate vertexgenai.Part instances based on the MIME type of each message.
//
// Parameters:
//
//	actor - the actor whose messages are to be converted.
//	exchange - the exchange containing the messages.
//
// Returns:
//
//	parts - a slice of vertexgenai.Part containing the converted messages.
//	err - an error if any occurs during the conversion process.
func (p *VertexAiProvider) toVertexParts(actor genai.Actor, exchange genai.Exchange) (parts []vertexgenai.Part, err error) {
	var msgs []*genai.Message = exchange.MsgsByActors(actor)
	if len(msgs) == 0 {
		return
	}
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

// partsToMessages converts a slice of vertexgenai.Part into a slice of pointers to genai.Message.
// It supports three types of parts: vertexgenai.Text, vertexgenai.Blob, and vertexgenai.FileData.
// For vertexgenai.Text, it creates a new text message with MIME type text/plain.
// For vertexgenai.Blob, it creates a new binary message with the provided data and MIME type.
// For vertexgenai.FileData, it creates a new file message with the provided file URI and MIME type.
// If an unsupported part type is encountered, it returns an error indicating the unsupported type.
//
// Parameters:
//
//	parts []vertexgenai.Part: A slice of parts to be converted.
//
// Returns:
//
//	msgs []*genai.Message: A slice of pointers to genai.Message.
//	err error: An error if an unsupported part type is encountered, otherwise nil.
func (p *VertexAiProvider) partsToMessages(parts []vertexgenai.Part) (msgs []*genai.Message, err error) {
	for _, part := range parts {

		switch part := part.(type) {
		case vertexgenai.Text:
			msgs = append(msgs, genai.NewTextMessage(string(part), ioutils.MimeTextPlain))
		case vertexgenai.Blob:
			msgs = append(msgs, genai.NewBinMessage(part.Data, part.MIMEType))
		case vertexgenai.FileData:
			msgs = append(msgs, genai.NewFileMessage(part.FileURI, part.MIMEType))
		default:
			err = fmt.Errorf("unsupported part type %T", part)
			return
		}
	}
	return
}

// addPartsToExchange adds various parts to the provided exchange.
// It supports parts of type vertexgenai.Text, vertexgenai.Blob, and vertexgenai.FileData.
// For each part, it calls the appropriate method on the exchange to add the part.
//
// Parameters:
//   - parts: A slice of vertexgenai.Part, which can be of type vertexgenai.Text, vertexgenai.Blob, or vertexgenai.FileData.
//   - exchange: An instance of genai.Exchange to which the parts will be added.
//
// Returns:
//   - err: An error if adding any part to the exchange fails, otherwise nil.
func (p *VertexAiProvider) addPartsToExchange(parts []vertexgenai.Part, exchange genai.Exchange) (err error) {
	for _, part := range parts {
		switch part := part.(type) {
		case vertexgenai.Text:
			_, err = exchange.AddTxtMsg(string(part), genai.AIActor)
		case vertexgenai.Blob:
			_, err = exchange.AddBinMsg(part.Data, part.MIMEType, genai.AIActor)
		case vertexgenai.FileData:
			_, err = exchange.AddFileMsg(part.FileURI, part.MIMEType, genai.AIActor)
		default:
			err = fmt.Errorf("unsupported part type %T", part)

		}
		if err != nil {
			return
		}
	}
	return
}

// setOptions configures the provided GenerativeModel with the specified options.
// It sets various parameters such as temperature, top-K, top-P, and candidate count
// based on the values present in the options. If options is nil, the function returns
// without making any changes.
//
// Parameters:
// - model: A pointer to the GenerativeModel to be configured.
// - options: A pointer to the Options containing the configuration values.
//
// Returns:
// - err: An error if any occurs during the setting of options, otherwise nil.
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

	if options.Has(genai.OptionSchema) && options.Has(genai.OptionOutputMime) {
		model.GenerationConfig.ResponseMIMEType = options.GetOutputMime(ioutils.MimeApplicationJSON)
		model.GenerationConfig.ResponseSchema = ToVertexSchema(options.GetSchema())

	}

	return
}
