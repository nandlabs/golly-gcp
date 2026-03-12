package genai

import (
	"testing"

	googlegenai "google.golang.org/genai"
	"oss.nandlabs.io/golly/data"
	gollygenai "oss.nandlabs.io/golly/genai"
)

// --- convertPart tests ---

func TestConvertPart_Text(t *testing.T) {
	part := &gollygenai.Part{
		Text: &gollygenai.TextPart{Content: "hello world"},
	}
	result := convertPart(part)
	if result == nil {
		t.Fatal("expected non-nil part")
	}
	if result.Text != "hello world" {
		t.Errorf("expected text 'hello world', got %q", result.Text)
	}
}

func TestConvertPart_File(t *testing.T) {
	part := &gollygenai.Part{
		MimeType: "image/png",
		File:     &gollygenai.FilePart{URI: "gs://bucket/image.png"},
	}
	result := convertPart(part)
	if result == nil {
		t.Fatal("expected non-nil part")
	}
	if result.FileData == nil {
		t.Fatal("expected FileData to be set")
	}
	if result.FileData.FileURI != "gs://bucket/image.png" {
		t.Errorf("expected URI 'gs://bucket/image.png', got %q", result.FileData.FileURI)
	}
}

func TestConvertPart_Bin(t *testing.T) {
	part := &gollygenai.Part{
		MimeType: "application/pdf",
		Bin:      &gollygenai.BinPart{Data: []byte("pdf-data")},
	}
	result := convertPart(part)
	if result == nil {
		t.Fatal("expected non-nil part")
	}
	if result.InlineData == nil {
		t.Fatal("expected InlineData to be set")
	}
	if string(result.InlineData.Data) != "pdf-data" {
		t.Errorf("expected data 'pdf-data', got %q", result.InlineData.Data)
	}
}

func TestConvertPart_FuncCall(t *testing.T) {
	part := &gollygenai.Part{
		FuncCall: &gollygenai.FuncCallPart{
			Id:           "call-1",
			FunctionName: "get_weather",
			Arguments:    map[string]any{"city": "NYC"},
		},
	}
	result := convertPart(part)
	if result == nil {
		t.Fatal("expected non-nil part")
	}
	if result.FunctionCall == nil {
		t.Fatal("expected FunctionCall to be set")
	}
	if result.FunctionCall.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %q", result.FunctionCall.Name)
	}
	if result.FunctionCall.ID != "call-1" {
		t.Errorf("expected ID 'call-1', got %q", result.FunctionCall.ID)
	}
}

func TestConvertPart_FuncResponse(t *testing.T) {
	text := "sunny"
	part := &gollygenai.Part{
		Name: "get_weather",
		FuncResponse: &gollygenai.FuncResponsePart{
			Text: &text,
		},
	}
	result := convertPart(part)
	if result == nil {
		t.Fatal("expected non-nil part")
	}
	if result.FunctionResponse == nil {
		t.Fatal("expected FunctionResponse to be set")
	}
	if result.FunctionResponse.Name != "get_weather" {
		t.Errorf("expected name 'get_weather', got %q", result.FunctionResponse.Name)
	}
}

func TestConvertPart_Nil(t *testing.T) {
	part := &gollygenai.Part{} // no fields set
	result := convertPart(part)
	if result != nil {
		t.Errorf("expected nil for empty part, got %+v", result)
	}
}

// --- convertRole tests ---

func TestConvertRole_User(t *testing.T) {
	role := convertRole(gollygenai.RoleUser)
	if role != "user" {
		t.Errorf("expected 'user', got %q", role)
	}
}

func TestConvertRole_Assistant(t *testing.T) {
	role := convertRole(gollygenai.RoleAssistant)
	if role != "model" {
		t.Errorf("expected 'model', got %q", role)
	}
}

func TestConvertRole_Default(t *testing.T) {
	role := convertRole(gollygenai.RoleSystem)
	if role != "user" {
		t.Errorf("expected 'user' for system role default, got %q", role)
	}
}

// --- convertFromGoogleRole tests ---

func TestConvertFromGoogleRole_User(t *testing.T) {
	role := convertFromGoogleRole("user")
	if role != gollygenai.RoleUser {
		t.Errorf("expected RoleUser, got %v", role)
	}
}

func TestConvertFromGoogleRole_Model(t *testing.T) {
	role := convertFromGoogleRole("model")
	if role != gollygenai.RoleAssistant {
		t.Errorf("expected RoleAssistant, got %v", role)
	}
}

func TestConvertFromGoogleRole_Default(t *testing.T) {
	role := convertFromGoogleRole("unknown")
	if role != gollygenai.RoleAssistant {
		t.Errorf("expected RoleAssistant for unknown, got %v", role)
	}
}

// --- convertFromGooglePart tests ---

func TestConvertFromGooglePart_Nil(t *testing.T) {
	result := convertFromGooglePart(nil)
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestConvertFromGooglePart_Text(t *testing.T) {
	part := &googlegenai.Part{Text: "hello"}
	result := convertFromGooglePart(part)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Text == nil || result.Text.Content != "hello" {
		t.Errorf("expected text 'hello', got %+v", result.Text)
	}
}

func TestConvertFromGooglePart_Thought(t *testing.T) {
	part := &googlegenai.Part{Text: "thinking...", Thought: true}
	result := convertFromGooglePart(part)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Attributes == nil {
		t.Fatal("expected Attributes map")
	}
	if v, ok := result.Attributes["thought"]; !ok || v != true {
		t.Errorf("expected thought=true attribute, got %+v", result.Attributes)
	}
}

func TestConvertFromGooglePart_InlineData(t *testing.T) {
	part := &googlegenai.Part{
		InlineData: &googlegenai.Blob{
			MIMEType: "image/jpeg",
			Data:     []byte("image-data"),
		},
	}
	result := convertFromGooglePart(part)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Bin == nil {
		t.Fatal("expected Bin to be set")
	}
	if result.MimeType != "image/jpeg" {
		t.Errorf("expected MimeType 'image/jpeg', got %q", result.MimeType)
	}
}

func TestConvertFromGooglePart_FileData(t *testing.T) {
	part := &googlegenai.Part{
		FileData: &googlegenai.FileData{
			MIMEType: "video/mp4",
			FileURI:  "gs://bucket/video.mp4",
		},
	}
	result := convertFromGooglePart(part)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.File == nil {
		t.Fatal("expected File to be set")
	}
	if result.File.URI != "gs://bucket/video.mp4" {
		t.Errorf("expected URI 'gs://bucket/video.mp4', got %q", result.File.URI)
	}
}

func TestConvertFromGooglePart_FunctionCall(t *testing.T) {
	part := &googlegenai.Part{
		FunctionCall: &googlegenai.FunctionCall{
			ID:   "fc-1",
			Name: "search",
			Args: map[string]any{"q": "test"},
		},
	}
	result := convertFromGooglePart(part)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.FuncCall == nil {
		t.Fatal("expected FuncCall to be set")
	}
	if result.FuncCall.FunctionName != "search" {
		t.Errorf("expected name 'search', got %q", result.FuncCall.FunctionName)
	}
	if result.FuncCall.Id != "fc-1" {
		t.Errorf("expected id 'fc-1', got %q", result.FuncCall.Id)
	}
}

func TestConvertFromGooglePart_FunctionResponse(t *testing.T) {
	part := &googlegenai.Part{
		FunctionResponse: &googlegenai.FunctionResponse{
			Name:     "search",
			Response: map[string]any{"text": "result here"},
		},
	}
	result := convertFromGooglePart(part)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.FuncResponse == nil {
		t.Fatal("expected FuncResponse to be set")
	}
	if result.FuncResponse.Text == nil || *result.FuncResponse.Text != "result here" {
		t.Errorf("expected text 'result here', got %+v", result.FuncResponse.Text)
	}
}

func TestConvertFromGooglePart_Empty(t *testing.T) {
	part := &googlegenai.Part{}
	result := convertFromGooglePart(part)
	if result != nil {
		t.Errorf("expected nil for empty part, got %+v", result)
	}
}

// --- convertFromGoogleContent tests ---

func TestConvertFromGoogleContent(t *testing.T) {
	content := &googlegenai.Content{
		Role: "user",
		Parts: []*googlegenai.Part{
			{Text: "hello"},
			{Text: "world"},
		},
	}
	msg := convertFromGoogleContent(content)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Role != gollygenai.RoleUser {
		t.Errorf("expected RoleUser, got %v", msg.Role)
	}
	if len(msg.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(msg.Parts))
	}
	if msg.Parts[0].Text.Content != "hello" {
		t.Errorf("expected 'hello', got %q", msg.Parts[0].Text.Content)
	}
}

// --- convertMessage tests ---

func TestConvertMessage(t *testing.T) {
	msg := gollygenai.NewTextMessage(gollygenai.RoleUser, "test prompt")
	content := convertMessage(msg)
	if content == nil {
		t.Fatal("expected non-nil content")
	}
	if content.Role != "user" {
		t.Errorf("expected role 'user', got %q", content.Role)
	}
	if len(content.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(content.Parts))
	}
	if content.Parts[0].Text != "test prompt" {
		t.Errorf("expected 'test prompt', got %q", content.Parts[0].Text)
	}
}

func TestConvertMessage_EmptyParts(t *testing.T) {
	msg := &gollygenai.Message{Role: gollygenai.RoleUser}
	content := convertMessage(msg)
	if len(content.Parts) != 1 {
		t.Fatalf("expected 1 empty fallback part, got %d", len(content.Parts))
	}
	if content.Parts[0].Text != "" {
		t.Errorf("expected empty text, got %q", content.Parts[0].Text)
	}
}

// --- mapFinishReason tests ---

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		input    googlegenai.FinishReason
		expected gollygenai.FinishReason
	}{
		{"STOP", gollygenai.FinishReasonStop},
		{"MAX_TOKENS", gollygenai.FinishReasonLength},
		{"SAFETY", gollygenai.FinishReasonContentFilter},
		{"RECITATION", gollygenai.FinishReasonContentFilter},
		{"BLOCKLIST", gollygenai.FinishReasonContentFilter},
		{"PROHIBITED_CONTENT", gollygenai.FinishReasonContentFilter},
		{"SPII", gollygenai.FinishReasonContentFilter},
		{"MALFORMED_FUNCTION_CALL", gollygenai.FinishReasonFunctionCall},
		{"OTHER", gollygenai.FinishReasonUnknown},
		{"", gollygenai.FinishReasonInProgress},
		{"SOME_NEW_REASON", gollygenai.FinishReasonUnknown},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := mapFinishReason(tt.input)
			if result != tt.expected {
				t.Errorf("mapFinishReason(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// --- extractSystemParts tests ---

func TestExtractSystemParts_FromOptions(t *testing.T) {
	opts := gollygenai.NewOptionsBuilder().
		Add(gollygenai.OptionSystemInstructions, "Be helpful").
		Build()
	parts := extractSystemParts(nil, opts)
	if len(parts) != 1 {
		t.Fatalf("expected 1 system part, got %d", len(parts))
	}
	if parts[0].Text != "Be helpful" {
		t.Errorf("expected 'Be helpful', got %q", parts[0].Text)
	}
}

func TestExtractSystemParts_FromSystemMessage(t *testing.T) {
	msg := gollygenai.NewTextMessage(gollygenai.RoleSystem, "System prompt")
	parts := extractSystemParts(msg, nil)
	if len(parts) != 1 {
		t.Fatalf("expected 1 system part, got %d", len(parts))
	}
}

func TestExtractSystemParts_Both(t *testing.T) {
	msg := gollygenai.NewTextMessage(gollygenai.RoleSystem, "From message")
	opts := gollygenai.NewOptionsBuilder().
		Add(gollygenai.OptionSystemInstructions, "From options").
		Build()
	parts := extractSystemParts(msg, opts)
	if len(parts) != 2 {
		t.Fatalf("expected 2 system parts, got %d", len(parts))
	}
}

func TestExtractSystemParts_None(t *testing.T) {
	parts := extractSystemParts(nil, nil)
	if len(parts) != 0 {
		t.Fatalf("expected 0 system parts, got %d", len(parts))
	}
}

// --- buildContents tests ---

func TestBuildContents_UserMessage(t *testing.T) {
	msg := gollygenai.NewTextMessage(gollygenai.RoleUser, "Hello")
	contents, sysContent := buildContents(msg, nil)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	if sysContent != nil {
		t.Error("expected nil sysContent for user message")
	}
}

func TestBuildContents_SystemMessage(t *testing.T) {
	msg := gollygenai.NewTextMessage(gollygenai.RoleSystem, "System")
	contents, sysContent := buildContents(msg, nil)
	if len(contents) != 0 {
		t.Fatalf("expected 0 user contents for system-only message, got %d", len(contents))
	}
	if sysContent == nil {
		t.Fatal("expected non-nil sysContent for system message")
	}
}

// --- buildGenerateConfig tests ---

func TestBuildGenerateConfig_NilOptions(t *testing.T) {
	config := buildGenerateConfig(nil, nil)
	if config == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestBuildGenerateConfig_WithSystemInstruction(t *testing.T) {
	sys := googlegenai.NewContentFromParts([]*googlegenai.Part{googlegenai.NewPartFromText("Be helpful")}, "user")
	config := buildGenerateConfig(nil, sys)
	if config.SystemInstruction == nil {
		t.Fatal("expected SystemInstruction to be set")
	}
}

func TestBuildGenerateConfig_WithOptions(t *testing.T) {
	opts := gollygenai.NewOptionsBuilder().
		SetMaxTokens(1024).
		SetTemperature(0.5).
		SetTopK(10).
		SetSeed(42).
		SetStopWords("END", "STOP").
		Build()
	config := buildGenerateConfig(opts, nil)
	if config.MaxOutputTokens != 1024 {
		t.Errorf("expected MaxOutputTokens 1024, got %d", config.MaxOutputTokens)
	}
	if config.Temperature == nil || *config.Temperature != 0.5 {
		t.Errorf("expected Temperature 0.5")
	}
	if config.TopK == nil || *config.TopK != 10 {
		t.Errorf("expected TopK 10")
	}
	if config.Seed == nil || *config.Seed != 42 {
		t.Errorf("expected Seed 42")
	}
	if len(config.StopSequences) != 2 {
		t.Errorf("expected 2 stop sequences, got %d", len(config.StopSequences))
	}
}

// --- toGenResponse tests ---

func TestToGenResponse_Nil(t *testing.T) {
	result := toGenResponse(nil)
	if result != nil {
		t.Errorf("expected nil for nil response, got %+v", result)
	}
}

func TestToGenResponse_WithUsageMetadata(t *testing.T) {
	resp := &googlegenai.GenerateContentResponse{
		UsageMetadata: &googlegenai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:        10,
			CandidatesTokenCount:    20,
			TotalTokenCount:         30,
			CachedContentTokenCount: 5,
			ThoughtsTokenCount:      3,
		},
	}
	result := toGenResponse(resp)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Meta.InputTokens != 10 {
		t.Errorf("expected InputTokens 10, got %d", result.Meta.InputTokens)
	}
	if result.Meta.OutputTokens != 20 {
		t.Errorf("expected OutputTokens 20, got %d", result.Meta.OutputTokens)
	}
	if result.Meta.TotalTokens != 30 {
		t.Errorf("expected TotalTokens 30, got %d", result.Meta.TotalTokens)
	}
	if result.Meta.CachedTokens != 5 {
		t.Errorf("expected CachedTokens 5, got %d", result.Meta.CachedTokens)
	}
	if result.Meta.ThinkingTokens != 3 {
		t.Errorf("expected ThinkingTokens 3, got %d", result.Meta.ThinkingTokens)
	}
}

func TestToGenResponse_WithCandidates(t *testing.T) {
	resp := &googlegenai.GenerateContentResponse{
		Candidates: []*googlegenai.Candidate{
			{
				Index:        0,
				FinishReason: "STOP",
				Content: &googlegenai.Content{
					Role:  "model",
					Parts: []*googlegenai.Part{{Text: "response text"}},
				},
			},
		},
	}
	result := toGenResponse(resp)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(result.Candidates))
	}
	c := result.Candidates[0]
	if c.FinishReason != gollygenai.FinishReasonStop {
		t.Errorf("expected FinishReasonStop, got %q", c.FinishReason)
	}
	if c.Message == nil {
		t.Fatal("expected non-nil message")
	}
	if c.Message.Role != gollygenai.RoleAssistant {
		t.Errorf("expected RoleAssistant, got %v", c.Message.Role)
	}
}

// --- convertGroundingMetadata tests ---

func TestConvertGroundingMetadata_Web(t *testing.T) {
	gm := &googlegenai.GroundingMetadata{
		GroundingChunks: []*googlegenai.GroundingChunk{
			{Web: &googlegenai.GroundingChunkWeb{Title: "Example", URI: "https://example.com"}},
		},
	}
	groundings := convertGroundingMetadata(gm)
	if len(groundings) != 1 {
		t.Fatalf("expected 1 grounding, got %d", len(groundings))
	}
	if groundings[0].Source != "web" {
		t.Errorf("expected source 'web', got %q", groundings[0].Source)
	}
	if groundings[0].URI != "https://example.com" {
		t.Errorf("expected URI 'https://example.com', got %q", groundings[0].URI)
	}
}

func TestConvertGroundingMetadata_RetrievedContext(t *testing.T) {
	gm := &googlegenai.GroundingMetadata{
		GroundingChunks: []*googlegenai.GroundingChunk{
			{RetrievedContext: &googlegenai.GroundingChunkRetrievedContext{Title: "Doc", URI: "gs://bucket/doc"}},
		},
	}
	groundings := convertGroundingMetadata(gm)
	if len(groundings) != 1 {
		t.Fatalf("expected 1 grounding, got %d", len(groundings))
	}
	if groundings[0].Source != "retrieved" {
		t.Errorf("expected source 'retrieved', got %q", groundings[0].Source)
	}
}

func TestConvertGroundingMetadata_Empty(t *testing.T) {
	gm := &googlegenai.GroundingMetadata{}
	groundings := convertGroundingMetadata(gm)
	if len(groundings) != 0 {
		t.Errorf("expected 0 groundings, got %d", len(groundings))
	}
}

// --- convertSchema tests ---

func TestConvertSchema_Nil(t *testing.T) {
	result := convertSchema(nil)
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestConvertSchema_Basic(t *testing.T) {
	schema := &data.Schema{
		Type:        "object",
		Description: "A test object",
		Title:       "TestObj",
		Required:    []string{"name"},
	}
	result := convertSchema(schema)
	if result == nil {
		t.Fatal("expected non-nil schema")
	}
	if result.Description != "A test object" {
		t.Errorf("expected description 'A test object', got %q", result.Description)
	}
	if result.Title != "TestObj" {
		t.Errorf("expected title 'TestObj', got %q", result.Title)
	}
	if len(result.Required) != 1 || result.Required[0] != "name" {
		t.Errorf("expected Required ['name'], got %v", result.Required)
	}
}

func TestConvertSchema_WithProperties(t *testing.T) {
	schema := &data.Schema{
		Type: "object",
		Properties: map[string]*data.Schema{
			"name": {Type: "string", Description: "The name"},
			"age":  {Type: "integer", Description: "The age"},
		},
	}
	result := convertSchema(schema)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if len(result.Properties) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(result.Properties))
	}
	if result.Properties["name"].Description != "The name" {
		t.Errorf("expected 'The name', got %q", result.Properties["name"].Description)
	}
}

func TestConvertSchema_WithItems(t *testing.T) {
	schema := &data.Schema{
		Type:  "array",
		Items: &data.Schema{Type: "string"},
	}
	result := convertSchema(schema)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Items == nil {
		t.Fatal("expected Items to be set")
	}
}

func TestConvertSchema_WithEnum(t *testing.T) {
	schema := &data.Schema{
		Type: "string",
		Enum: []any{"red", "green", "blue"},
	}
	result := convertSchema(schema)
	if len(result.Enum) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(result.Enum))
	}
}

func TestConvertSchema_WithConstraints(t *testing.T) {
	maxItems := 10
	minItems := 1
	format := "email"
	pattern := "^[a-z]+$"
	maxLen := 100
	minLen := 1
	nullable := true
	schema := &data.Schema{
		Type:      "string",
		Nullable:  nullable,
		Format:    &format,
		Pattern:   &pattern,
		MaxItems:  &maxItems,
		MinItems:  &minItems,
		MaxLength: &maxLen,
		MinLength: &minLen,
	}
	result := convertSchema(schema)
	if result.Format != "email" {
		t.Errorf("expected format 'email', got %q", result.Format)
	}
	if result.Pattern != "^[a-z]+$" {
		t.Errorf("expected pattern, got %q", result.Pattern)
	}
	if result.MaxItems == nil || *result.MaxItems != 10 {
		t.Error("expected MaxItems 10")
	}
	if result.MinItems == nil || *result.MinItems != 1 {
		t.Error("expected MinItems 1")
	}
	if result.Nullable == nil || *result.Nullable != true {
		t.Error("expected Nullable true")
	}
}

// --- convertSchemaType tests ---

func TestConvertSchemaType(t *testing.T) {
	tests := []struct {
		input    string
		expected googlegenai.Type
	}{
		{"string", googlegenai.TypeString},
		{"integer", googlegenai.TypeInteger},
		{"number", googlegenai.TypeNumber},
		{"boolean", googlegenai.TypeBoolean},
		{"array", googlegenai.TypeArray},
		{"object", googlegenai.TypeObject},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertSchemaType(tt.input)
			if result != tt.expected {
				t.Errorf("convertSchemaType(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
