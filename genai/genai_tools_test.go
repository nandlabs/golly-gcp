package genai

import (
	"testing"

	googlegenai "google.golang.org/genai"
	"oss.nandlabs.io/golly/data"
	gollygenai "oss.nandlabs.io/golly/genai"
)

// --- Tools plumbing tests ---

func TestBuildConfig_ToolsPassedThrough(t *testing.T) {
	tool := gollygenai.Tool{
		Function: &gollygenai.FunctionDecl{
			Name:        "get_weather",
			Description: "Get the weather for a location",
			Parameters: &data.Schema{
				Type:     "object",
				Required: []string{"city"},
				Properties: map[string]*data.Schema{
					"city": {Type: "string", Description: "City name"},
				},
			},
		},
	}
	opts := gollygenai.NewOptionsBuilder().SetTools(tool).Build()

	config := buildGenerateConfig(opts, nil)
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if len(config.Tools) != 1 {
		t.Fatalf("expected 1 Tool on config, got %d", len(config.Tools))
	}
	if len(config.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 FunctionDeclaration, got %d", len(config.Tools[0].FunctionDeclarations))
	}
	fd := config.Tools[0].FunctionDeclarations[0]
	if fd.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %q", fd.Name)
	}
	if fd.Description != "Get the weather for a location" {
		t.Errorf("expected function description propagated, got %q", fd.Description)
	}
	if fd.Parameters == nil {
		t.Fatal("expected Parameters schema to be translated")
	}
	if fd.Parameters.Type != googlegenai.TypeObject {
		t.Errorf("expected object schema type, got %v", fd.Parameters.Type)
	}
	if _, ok := fd.Parameters.Properties["city"]; !ok {
		t.Error("expected 'city' property translated onto schema")
	}
}

func TestBuildConfig_ToolsMultiple(t *testing.T) {
	tools := []gollygenai.Tool{
		{Function: &gollygenai.FunctionDecl{Name: "a"}},
		{Function: &gollygenai.FunctionDecl{Name: "b"}},
	}
	opts := gollygenai.NewOptionsBuilder().SetTools(tools...).Build()

	config := buildGenerateConfig(opts, nil)
	if len(config.Tools) != 2 {
		t.Fatalf("expected 2 Tools, got %d", len(config.Tools))
	}
	names := []string{
		config.Tools[0].FunctionDeclarations[0].Name,
		config.Tools[1].FunctionDeclarations[0].Name,
	}
	if names[0] != "a" || names[1] != "b" {
		t.Errorf("expected [a b], got %v", names)
	}
}

func TestBuildConfig_ToolsNilFunctionSkipped(t *testing.T) {
	tools := []gollygenai.Tool{
		{Function: nil},
		{Function: &gollygenai.FunctionDecl{Name: "kept"}},
	}
	opts := gollygenai.NewOptionsBuilder().SetTools(tools...).Build()

	config := buildGenerateConfig(opts, nil)
	if len(config.Tools) != 1 {
		t.Fatalf("expected 1 Tool (nil skipped), got %d", len(config.Tools))
	}
	if config.Tools[0].FunctionDeclarations[0].Name != "kept" {
		t.Errorf("expected 'kept', got %q", config.Tools[0].FunctionDeclarations[0].Name)
	}
}

func TestBuildConfig_NoTools(t *testing.T) {
	opts := gollygenai.NewOptionsBuilder().Build()
	config := buildGenerateConfig(opts, nil)
	if config.Tools != nil {
		t.Errorf("expected nil Tools when unset, got %+v", config.Tools)
	}
	if config.ToolConfig != nil {
		t.Errorf("expected nil ToolConfig when unset, got %+v", config.ToolConfig)
	}
}

// --- ToolChoice translation tests ---

func TestBuildConfig_ToolChoice_Auto(t *testing.T) {
	opts := gollygenai.NewOptionsBuilder().
		SetToolChoice(gollygenai.NewToolChoice(gollygenai.ToolChoiceAuto)).
		Build()
	config := buildGenerateConfig(opts, nil)
	if config.ToolConfig == nil || config.ToolConfig.FunctionCallingConfig == nil {
		t.Fatal("expected ToolConfig.FunctionCallingConfig to be set")
	}
	if config.ToolConfig.FunctionCallingConfig.Mode != googlegenai.FunctionCallingConfigModeAuto {
		t.Errorf("expected AUTO mode, got %v", config.ToolConfig.FunctionCallingConfig.Mode)
	}
	if len(config.ToolConfig.FunctionCallingConfig.AllowedFunctionNames) != 0 {
		t.Error("expected no AllowedFunctionNames for auto mode")
	}
}

func TestBuildConfig_ToolChoice_Any(t *testing.T) {
	opts := gollygenai.NewOptionsBuilder().
		SetToolChoice(gollygenai.NewToolChoice(gollygenai.ToolChoiceRequired)).
		Build()
	config := buildGenerateConfig(opts, nil)
	if config.ToolConfig.FunctionCallingConfig.Mode != googlegenai.FunctionCallingConfigModeAny {
		t.Errorf("expected ANY mode for required, got %v", config.ToolConfig.FunctionCallingConfig.Mode)
	}
	if len(config.ToolConfig.FunctionCallingConfig.AllowedFunctionNames) != 0 {
		t.Error("expected no AllowedFunctionNames for unnamed required")
	}
}

func TestBuildConfig_ToolChoice_None(t *testing.T) {
	opts := gollygenai.NewOptionsBuilder().
		SetToolChoice(gollygenai.NewToolChoice(gollygenai.ToolChoiceNone)).
		Build()
	config := buildGenerateConfig(opts, nil)
	if config.ToolConfig.FunctionCallingConfig.Mode != googlegenai.FunctionCallingConfigModeNone {
		t.Errorf("expected NONE mode, got %v", config.ToolConfig.FunctionCallingConfig.Mode)
	}
}

func TestBuildConfig_ToolChoice_Named(t *testing.T) {
	opts := gollygenai.NewOptionsBuilder().
		SetToolChoice(gollygenai.NewNamedToolChoice("get_weather")).
		Build()
	config := buildGenerateConfig(opts, nil)
	fcc := config.ToolConfig.FunctionCallingConfig
	if fcc.Mode != googlegenai.FunctionCallingConfigModeAny {
		t.Errorf("expected ANY mode for named choice, got %v", fcc.Mode)
	}
	if len(fcc.AllowedFunctionNames) != 1 || fcc.AllowedFunctionNames[0] != "get_weather" {
		t.Errorf("expected AllowedFunctionNames=[get_weather], got %v", fcc.AllowedFunctionNames)
	}
}

func TestBuildConfig_ToolChoice_UnknownMode(t *testing.T) {
	opts := gollygenai.NewOptionsBuilder().
		SetToolChoice(&gollygenai.ToolChoice{Mode: "bogus"}).
		Build()
	config := buildGenerateConfig(opts, nil)
	// Unknown mode should degrade to AUTO rather than break the request.
	if config.ToolConfig.FunctionCallingConfig.Mode != googlegenai.FunctionCallingConfigModeAuto {
		t.Errorf("expected fallback AUTO mode for unknown ToolChoiceMode, got %v", config.ToolConfig.FunctionCallingConfig.Mode)
	}
}

func TestConvertToolChoice_Nil(t *testing.T) {
	if got := convertToolChoice(nil); got != nil {
		t.Errorf("expected nil ToolConfig for nil ToolChoice, got %+v", got)
	}
}
