package genai

import (
	"fmt"
	"strings"

	googlegenai "google.golang.org/genai"
	"oss.nandlabs.io/golly/data"
	gollygenai "oss.nandlabs.io/golly/genai"
)

// buildContents converts a golly genai Message into google genai Contents.
// It separates system content (for SystemInstruction) from user/assistant content.
func buildContents(message *gollygenai.Message, options *gollygenai.Options) ([]*googlegenai.Content, *googlegenai.Content) {
	var contents []*googlegenai.Content
	var sysContent *googlegenai.Content

	sysParts := extractSystemParts(message, options)
	if len(sysParts) > 0 {
		sysContent = googlegenai.NewContentFromParts(sysParts, "user")
	}

	if message != nil && message.Role != gollygenai.RoleSystem {
		content := convertMessage(message)
		if content != nil {
			contents = append(contents, content)
		}
	}

	return contents, sysContent
}

// extractSystemParts extracts system instruction parts from message and options.
func extractSystemParts(message *gollygenai.Message, options *gollygenai.Options) []*googlegenai.Part {
	var parts []*googlegenai.Part

	if options != nil {
		if sysInstr := options.GetSystemInstructions(); sysInstr != "" {
			parts = append(parts, googlegenai.NewPartFromText(sysInstr))
		}
	}

	if message != nil && message.Role == gollygenai.RoleSystem {
		for i := range message.Parts {
			if p := convertPart(&message.Parts[i]); p != nil {
				parts = append(parts, p)
			}
		}
	}

	return parts
}

// convertMessage converts a golly genai Message to a google genai Content.
func convertMessage(msg *gollygenai.Message) *googlegenai.Content {
	var parts []*googlegenai.Part
	for i := range msg.Parts {
		if p := convertPart(&msg.Parts[i]); p != nil {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, googlegenai.NewPartFromText(""))
	}
	return googlegenai.NewContentFromParts(parts, convertRole(msg.Role))
}

// convertPart converts a golly genai Part to a google genai Part.
func convertPart(part *gollygenai.Part) *googlegenai.Part {
	switch {
	case part.Text != nil:
		return googlegenai.NewPartFromText(part.Text.Content)
	case part.File != nil:
		return googlegenai.NewPartFromURI(part.File.URI, part.MimeType)
	case part.Bin != nil:
		return googlegenai.NewPartFromBytes(part.Bin.Data, part.MimeType)
	case part.FuncCall != nil:
		p := googlegenai.NewPartFromFunctionCall(part.FuncCall.FunctionName, part.FuncCall.Arguments)
		if part.FuncCall.Id != "" {
			p.FunctionCall.ID = part.FuncCall.Id
		}
		return p
	case part.FuncResponse != nil:
		resp := make(map[string]any)
		if part.FuncResponse.Text != nil {
			resp["text"] = *part.FuncResponse.Text
		}
		if part.FuncResponse.FileURI != nil {
			resp["file_uri"] = *part.FuncResponse.FileURI
		}
		if part.FuncResponse.Data != nil {
			resp["data"] = string(part.FuncResponse.Data)
		}
		return googlegenai.NewPartFromFunctionResponse(part.Name, resp)
	default:
		return nil
	}
}

// convertRole maps golly genai Role to google genai Role.
func convertRole(role gollygenai.Role) googlegenai.Role {
	switch role {
	case gollygenai.RoleUser:
		return "user"
	case gollygenai.RoleAssistant:
		return "model"
	default:
		return "user"
	}
}

// buildGenerateConfig constructs a GenerateContentConfig from golly genai Options.
func buildGenerateConfig(options *gollygenai.Options, systemInstruction *googlegenai.Content) *googlegenai.GenerateContentConfig {
	config := &googlegenai.GenerateContentConfig{}

	if systemInstruction != nil {
		config.SystemInstruction = systemInstruction
	}

	if options == nil {
		return config
	}

	if options.Has(gollygenai.OptionMaxTokens) {
		config.MaxOutputTokens = int32(options.GetMaxTokens(DefaultMaxTokens))
	}

	if options.Has(gollygenai.OptionTemperature) {
		v := options.GetTemperature(0.7)
		config.Temperature = &v
	}

	if options.Has(gollygenai.OptionTopP) {
		v := options.GetTopP(0.9)
		config.TopP = &v
	}

	if options.Has(gollygenai.OptionTopK) {
		v := float32(options.GetTopK(40))
		config.TopK = &v
	}

	if options.Has(gollygenai.OptionCandidateCount) {
		config.CandidateCount = int32(options.GetCandidateCount(1))
	}

	if stopWords := options.GetStopWords(nil); len(stopWords) > 0 {
		config.StopSequences = stopWords
	}

	if options.Has(gollygenai.OptionPresencePenalty) {
		v := float32(options.GetPresencePenalty(0))
		config.PresencePenalty = &v
	}

	if options.Has(gollygenai.OptionFrequencyPenalty) {
		v := float32(options.GetFrequencyPenalty(0))
		config.FrequencyPenalty = &v
	}

	if options.Has(gollygenai.OptionSeed) {
		v := int32(options.GetSeed(0))
		config.Seed = &v
	}

	if options.Has(gollygenai.OptionOutputMime) {
		config.ResponseMIMEType = options.GetOutputMime("")
	}

	if schema := options.GetSchema(); schema != nil {
		config.ResponseSchema = convertSchema(schema)
	}

	return config
}

// toGenResponse converts a google genai GenerateContentResponse to a golly genai GenResponse.
func toGenResponse(resp *googlegenai.GenerateContentResponse) *gollygenai.GenResponse {
	if resp == nil {
		return nil
	}

	genResp := &gollygenai.GenResponse{
		Meta: gollygenai.ResponseMeta{},
	}

	if resp.UsageMetadata != nil {
		genResp.Meta.InputTokens = int(resp.UsageMetadata.PromptTokenCount)
		genResp.Meta.OutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
		genResp.Meta.TotalTokens = int(resp.UsageMetadata.TotalTokenCount)
		genResp.Meta.CachedTokens = int(resp.UsageMetadata.CachedContentTokenCount)
		genResp.Meta.ThinkingTokens = int(resp.UsageMetadata.ThoughtsTokenCount)
	}

	for _, candidate := range resp.Candidates {
		c := gollygenai.Candidate{
			Index:        int(candidate.Index),
			FinishReason: mapFinishReason(candidate.FinishReason),
		}
		if candidate.Content != nil {
			c.Message = convertFromGoogleContent(candidate.Content)
		}
		if candidate.GroundingMetadata != nil {
			c.Groundings = convertGroundingMetadata(candidate.GroundingMetadata)
		}
		genResp.Candidates = append(genResp.Candidates, c)
	}

	return genResp
}

// convertFromGoogleContent converts a google genai Content to a golly genai Message.
func convertFromGoogleContent(content *googlegenai.Content) *gollygenai.Message {
	msg := &gollygenai.Message{
		Role: convertFromGoogleRole(content.Role),
	}
	for _, part := range content.Parts {
		if p := convertFromGooglePart(part); p != nil {
			msg.Parts = append(msg.Parts, *p)
		}
	}
	return msg
}

// convertFromGooglePart converts a google genai Part to a golly genai Part.
func convertFromGooglePart(part *googlegenai.Part) *gollygenai.Part {
	if part == nil {
		return nil
	}

	// Thinking/reasoning parts (Thought=true with text content)
	if part.Thought && part.Text != "" {
		return &gollygenai.Part{
			Text: &gollygenai.TextPart{Content: part.Text},
			Attributes: map[string]interface{}{
				"thought": true,
			},
		}
	}

	switch {
	case part.Text != "":
		return &gollygenai.Part{
			Text: &gollygenai.TextPart{Content: part.Text},
		}
	case part.InlineData != nil:
		return &gollygenai.Part{
			MimeType: part.InlineData.MIMEType,
			Bin:      &gollygenai.BinPart{Data: part.InlineData.Data},
		}
	case part.FileData != nil:
		return &gollygenai.Part{
			MimeType: part.FileData.MIMEType,
			File:     &gollygenai.FilePart{URI: part.FileData.FileURI},
		}
	case part.FunctionCall != nil:
		return &gollygenai.Part{
			Name: part.FunctionCall.Name,
			FuncCall: &gollygenai.FuncCallPart{
				Id:           part.FunctionCall.ID,
				FunctionName: part.FunctionCall.Name,
				Arguments:    part.FunctionCall.Args,
			},
		}
	case part.FunctionResponse != nil:
		p := &gollygenai.Part{
			Name:         part.FunctionResponse.Name,
			FuncResponse: &gollygenai.FuncResponsePart{},
		}
		if text, ok := part.FunctionResponse.Response["text"].(string); ok {
			p.FuncResponse.Text = &text
		}
		return p
	default:
		return nil
	}
}

// convertFromGoogleRole maps a google genai role string to a golly genai Role.
func convertFromGoogleRole(role string) gollygenai.Role {
	switch role {
	case "user":
		return gollygenai.RoleUser
	case "model":
		return gollygenai.RoleAssistant
	default:
		return gollygenai.RoleAssistant
	}
}

// mapFinishReason maps a google genai FinishReason to a golly genai FinishReason.
func mapFinishReason(reason googlegenai.FinishReason) gollygenai.FinishReason {
	switch reason {
	case "STOP":
		return gollygenai.FinishReasonStop
	case "MAX_TOKENS":
		return gollygenai.FinishReasonLength
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return gollygenai.FinishReasonContentFilter
	case "MALFORMED_FUNCTION_CALL":
		return gollygenai.FinishReasonFunctionCall
	case "OTHER":
		return gollygenai.FinishReasonUnknown
	default:
		if reason == "" {
			return gollygenai.FinishReasonInProgress
		}
		return gollygenai.FinishReasonUnknown
	}
}

// convertGroundingMetadata converts google genai GroundingMetadata to golly GroundingInfo slice.
func convertGroundingMetadata(gm *googlegenai.GroundingMetadata) []gollygenai.GroundingInfo {
	var groundings []gollygenai.GroundingInfo
	for _, chunk := range gm.GroundingChunks {
		if chunk.Web != nil {
			groundings = append(groundings, gollygenai.GroundingInfo{
				Name:   chunk.Web.Title,
				Source: "web",
				URI:    chunk.Web.URI,
			})
		}
		if chunk.RetrievedContext != nil {
			groundings = append(groundings, gollygenai.GroundingInfo{
				Name:   chunk.RetrievedContext.Title,
				Source: "retrieved",
				URI:    chunk.RetrievedContext.URI,
			})
		}
	}
	return groundings
}

// convertSchema converts a golly data.Schema to a google genai Schema.
func convertSchema(s *data.Schema) *googlegenai.Schema {
	if s == nil {
		return nil
	}

	gs := &googlegenai.Schema{
		Type:        convertSchemaType(s.Type),
		Description: s.Description,
		Required:    s.Required,
		Title:       s.Title,
	}
	if s.Nullable {
		gs.Nullable = &s.Nullable
	}

	if s.Format != nil {
		gs.Format = *s.Format
	}
	if s.Pattern != nil {
		gs.Pattern = *s.Pattern
	}
	if s.Maximum != nil {
		gs.Maximum = s.Maximum
	}
	if s.Minimum != nil {
		gs.Minimum = s.Minimum
	}
	if s.MaxItems != nil {
		v := int64(*s.MaxItems)
		gs.MaxItems = &v
	}
	if s.MinItems != nil {
		v := int64(*s.MinItems)
		gs.MinItems = &v
	}
	if s.MaxLength != nil {
		v := int64(*s.MaxLength)
		gs.MaxLength = &v
	}
	if s.MinLength != nil {
		v := int64(*s.MinLength)
		gs.MinLength = &v
	}
	if s.MaxProperties != nil {
		v := int64(*s.MaxProperties)
		gs.MaxProperties = &v
	}
	if s.MinProperties != nil {
		v := int64(*s.MinProperties)
		gs.MinProperties = &v
	}

	if s.Default != nil {
		gs.Default = s.Default
	}
	if s.Example != nil {
		gs.Example = s.Example
	}

	// Convert enum values to strings
	if len(s.Enum) > 0 {
		for _, e := range s.Enum {
			gs.Enum = append(gs.Enum, fmt.Sprintf("%v", e))
		}
	}

	if s.Items != nil {
		gs.Items = convertSchema(s.Items)
	}

	if len(s.Properties) > 0 {
		gs.Properties = make(map[string]*googlegenai.Schema, len(s.Properties))
		for k, v := range s.Properties {
			gs.Properties[k] = convertSchema(v)
		}
	}

	if len(s.AnyOf) > 0 {
		for _, a := range s.AnyOf {
			gs.AnyOf = append(gs.AnyOf, convertSchema(a))
		}
	}

	return gs
}

// convertSchemaType maps golly data schema type strings to google genai Type constants.
func convertSchemaType(t string) googlegenai.Type {
	switch strings.ToLower(t) {
	case "string":
		return googlegenai.TypeString
	case "number":
		return googlegenai.TypeNumber
	case "integer":
		return googlegenai.TypeInteger
	case "boolean":
		return googlegenai.TypeBoolean
	case "array":
		return googlegenai.TypeArray
	case "object":
		return googlegenai.TypeObject
	default:
		return googlegenai.TypeUnspecified
	}
}
