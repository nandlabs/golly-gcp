# genai

Google Cloud GenAI implementation of the [golly genai](https://pkg.go.dev/oss.nandlabs.io/golly/genai) `Provider` interface.

> Uses the **[google.golang.org/genai](https://pkg.go.dev/google.golang.org/genai)** SDK
> for a unified interface to Google's generative AI models — supports
> **Vertex AI**, **Gemini API**, and **Model Garden** backends.

---

- [Installation](#installation)
- [Features](#features)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [Usage](#usage)
- [Supported Content Types](#supported-content-types)
- [Options](#options)
- [Tool Use (Function Calling)](#tool-use-function-calling)
- [Streaming](#streaming)
- [Schema / Structured Output](#schema--structured-output)
- [Error Handling](#error-handling)
- [API Reference](#api-reference)
- [Prerequisites](#prerequisites)
- [Contributing](#contributing)

---

## Installation

```bash
go get oss.nandlabs.io/golly-gcp/genai
```

## Features

- **Generate** — synchronous inference via the Google GenAI API
- **GenerateStream** — streaming inference via the Google GenAI streaming API
- **Multi-model** — works with any model available through Google GenAI (Gemini, Model Garden, etc.)
- **Text, Images, Files, and Binary data** — send text, inline images/audio/video, file URIs, and raw binary
- **Tool use** — full function calling support (send tool calls, receive tool results)
- **System prompts** — via options or system-role messages
- **Inference config** — max tokens, temperature, top-p, top-k, candidate count, stop sequences, penalties, seed
- **Structured output** — response MIME type and JSON schema for controlled output
- **Token usage** — input, output, total, cached, and thinking token counts in response metadata
- **Thinking/reasoning** — thinking parts preserved with `thought` attribute
- **Grounding** — web and retrieved context grounding metadata mapped to `GroundingInfo`
- **Dual backend** — supports Vertex AI (default) and Gemini API via explicit `Backend` selection
- **Config resolution** — leverages `gcpsvc` for GCP project and location management

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  Application                                                 │
│                                                              │
│  genai.Provider interface                                    │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  genai.GCPProvider                                  │  │
│  │                                                        │  │
│  │  ┌──────────────────────────────────────────────────┐  │  │
│  │  │  generateAPI (interface)                         │  │  │
│  │  │                                                  │  │  │
│  │  │  • GenerateContent(ctx, model, ...) → response   │  │  │
│  │  │  • GenerateContentStream(ctx, ...) → iter.Seq2   │  │  │
│  │  └──────────────────────────────────────────────────┘  │  │
│  │                                                        │  │
│  │  ┌──────────────┐   ┌─────────────────────────────┐    │  │
│  │  │  utils.go     │  │  pkg.go                     │    │  │
│  │  │  • convert    │  │  • generateAPI interface     │    │  │
│  │  │  • build      │  │  • gcpsvc integration       │    │  │
│  │  │  • map types  │  │  • logger                   │    │  │
│  │  └──────────────┘   └─────────────────────────────┘    │  │
│  └────────────────────────────────────────────────────────┘  │
│                              │                               │
│                              ▼                               │
│                     Google GenAI SDK                         │
│                 (Vertex AI / Gemini API)                     │
└──────────────────────────────────────────────────────────────┘
```

## Configuration

### Using `ProviderConfig`

```go
provider, err := gcpgenai.NewGCPProvider(ctx, &gcpgenai.ProviderConfig{
    // GCP project and location (for Vertex AI backend)
    ProjectId: "my-project",
    Location:  "us-central1",

    // Explicitly select the backend
    Backend: gcpgenai.BackendVertexAI,

    // Or resolve from a named gcpsvc configuration
    CfgName: "my-genai-config",

    // List of model IDs this provider instance supports
    Models: []string{
        "gemini-2.5-pro",
        "gemini-2.0-flash",
    },
})
```

### Using Gemini API (API Key)

```go
provider, err := gcpgenai.NewGCPProvider(ctx, &gcpgenai.ProviderConfig{
    APIKey:  "your-api-key",
    Backend: gcpgenai.BackendGeminiAPI,
    Models:  []string{"gemini-2.0-flash"},
})
```

### Configuration Resolution

The provider resolves GCP project and location in this order:

1. **Explicit `ProjectId` / `Location`** — uses the values directly from `ProviderConfig`
2. **Named config (`CfgName`)** — looks up a named config from `gcpsvc.Manager`
3. **Environment / ADC** — the underlying SDK uses Application Default Credentials

The `Backend` field lets you explicitly select `BackendVertexAI` or `BackendGeminiAPI`.
When zero, it is inferred: `BackendGeminiAPI` if `APIKey` is set, else `BackendVertexAI`.

## Usage

### Basic Text Generation

```go
package main

import (
    "context"
    "fmt"
    "log"

    gcpgenai "oss.nandlabs.io/golly-gcp/genai"
    "oss.nandlabs.io/golly/genai"
)

func main() {
    provider, err := gcpgenai.NewGCPProvider(context.Background(), &gcpgenai.ProviderConfig{
        ProjectId: "my-project",
        Location:  "us-central1",
        Backend:   gcpgenai.BackendVertexAI,
        Models:    []string{"gemini-2.0-flash"},
    })
    if err != nil {
        log.Fatal(err)
    }
    defer provider.Close()

    msg := genai.NewTextMessage(genai.RoleUser, "What is Google Gemini?")

    options := genai.NewOptionsBuilder().
        SetMaxTokens(1024).
        SetTemperature(0.7).
        Add(genai.OptionSystemInstructions, "You are a helpful assistant.").
        Build()

    resp, err := provider.Generate(
        context.Background(),
        "gemini-2.0-flash",
        msg,
        options,
    )
    if err != nil {
        log.Fatal(err)
    }

    for _, candidate := range resp.Candidates {
        for _, part := range candidate.Message.Parts {
            if part.Text != nil {
                fmt.Println(part.Text.Text)
            }
        }
    }

    fmt.Printf("Tokens: in=%d out=%d total=%d\n",
        resp.Meta.InputTokens, resp.Meta.OutputTokens, resp.Meta.TotalTokens)
}
```

### Streaming

```go
msg := genai.NewTextMessage(genai.RoleUser, "Tell me a story about Go.")

respChan, errChan := provider.GenerateStream(
    context.Background(),
    "gemini-2.0-flash",
    msg,
    nil,
)

for resp := range respChan {
    for _, candidate := range resp.Candidates {
        if candidate.Message != nil {
            for _, part := range candidate.Message.Parts {
                if part.Text != nil {
                    fmt.Print(part.Text.Text)
                }
            }
        }
    }
}

if err := <-errChan; err != nil {
    log.Fatal(err)
}
```

## Supported Content Types

### Text

```go
msg := genai.NewTextMessage(genai.RoleUser, "Hello, world!")
```

### Images (Inline)

```go
imageData, _ := os.ReadFile("photo.png")
msg := &genai.Message{
    Role: genai.RoleUser,
    Parts: []genai.Part{
        {Text: &genai.TextPart{Text: "What's in this image?"}},
        {MimeType: "image/png", Bin: &genai.BinPart{Data: imageData}},
    },
}
```

### File URI (Cloud Storage)

```go
msg := &genai.Message{
    Role: genai.RoleUser,
    Parts: []genai.Part{
        {Text: &genai.TextPart{Text: "Summarize this document."}},
        {MimeType: "application/pdf", File: &genai.FilePart{URI: "gs://my-bucket/report.pdf"}},
    },
}
```

## Options

| Option                | Builder Method           | Description                                   |
| --------------------- | ------------------------ | --------------------------------------------- |
| `max_tokens`          | `SetMaxTokens(n)`        | Maximum number of tokens to generate          |
| `temperature`         | `SetTemperature(f)`      | Randomness (0.0–2.0)                          |
| `top_p`               | `SetTopP(f)`             | Nucleus sampling threshold                    |
| `top_k`               | `SetTopK(n)`             | Top-K sampling                                |
| `candidate_count`     | `SetCandidateCount(n)`   | Number of response candidates                 |
| `stop_words`          | `SetStopWords(s...)`     | Stop sequences                                |
| `presence_penalty`    | `Add(key, val)`          | Presence penalty                              |
| `frequency_penalty`   | `Add(key, val)`          | Frequency penalty                             |
| `seed`                | `Add(key, val)`          | Random seed for deterministic output          |
| `output_mime`         | `Add(key, val)`          | Response MIME type (e.g., `application/json`) |
| `schema`              | `Add(key, *data.Schema)` | Response schema for structured output         |
| `system_instructions` | `Add(key, val)`          | System prompt text                            |

```go
options := genai.NewOptionsBuilder().
    SetMaxTokens(2048).
    SetTemperature(0.5).
    SetTopP(0.9).
    SetTopK(40).
    SetStopWords("STOP", "END").
    Add(genai.OptionSystemInstructions, "You are a coding assistant.").
    Build()
```

## Tool Use (Function Calling)

### Sending a Tool Call Result

```go
text := `{"temperature": "72°F", "condition": "sunny"}`
msg := &genai.Message{
    Role: genai.RoleUser,
    Parts: []genai.Part{
        {
            Name: "get_weather",
            FuncResponse: &genai.FuncResponsePart{
                Text: &text,
            },
        },
    },
}
```

### Receiving a Tool Call

```go
resp, _ := provider.Generate(ctx, model, msg, opts)

for _, candidate := range resp.Candidates {
    for _, part := range candidate.Message.Parts {
        if part.FuncCall != nil {
            fmt.Printf("Tool: %s, ID: %s, Args: %v\n",
                part.FuncCall.FunctionName,
                part.FuncCall.Id,
                part.FuncCall.Arguments,
            )
        }
    }
}
```

## Schema / Structured Output

Request JSON output conforming to a schema:

```go
schema := &data.Schema{
    Type: "object",
    Properties: map[string]*data.Schema{
        "name":  {Type: "string", Description: "Person's name"},
        "age":   {Type: "integer", Description: "Person's age"},
    },
    Required: []string{"name", "age"},
}

options := genai.NewOptionsBuilder().
    SetMaxTokens(256).
    Add(genai.OptionOutputMime, "application/json").
    Add(genai.OptionSchema, schema).
    Build()
```

## Streaming

The `GenerateStream` method returns two channels:

| Channel               | Type             | Description                                   |
| --------------------- | ---------------- | --------------------------------------------- |
| `<-chan *GenResponse` | response channel | Text deltas, stop events, and metadata events |
| `<-chan error`        | error channel    | API or stream errors (at most one)            |

The streaming implementation uses Go's `iter.Seq2` iterator from the GenAI SDK,
converting each streamed response chunk into a `GenResponse` on the response channel.

## Error Handling

The provider wraps all errors with descriptive context:

```go
resp, err := provider.Generate(ctx, model, msg, opts)
if err != nil {
    // Possible errors:
    // - "failed to create GenAI client: ..."
    // - "genai GenerateContent API call failed: ..."
    // - "genai streaming error: ..."
    log.Fatal(err)
}
```

## API Reference

### Types

| Type             | Description                                      |
| ---------------- | ------------------------------------------------ |
| `GCPProvider`    | Implements `genai.Provider` for Google GenAI     |
| `ProviderConfig` | Configuration for creating a provider instance   |
| `Backend`        | Type alias for selecting Vertex AI or Gemini API |

### Constants

| Constant           | Value          | Description        |
| ------------------ | -------------- | ------------------ |
| `ProviderName`     | `google-genai` | Provider name      |
| `ProviderVersion`  | `1.0.0`        | Provider version   |
| `DefaultMaxTokens` | `4096`         | Default max tokens |
| `BackendVertexAI`  | —              | Vertex AI backend  |
| `BackendGeminiAPI` | —              | Gemini API backend |

### Methods

| Method                                            | Description                       |
| ------------------------------------------------- | --------------------------------- |
| `NewGCPProvider(ctx, config) (*GCPProvider, err)` | Create a new provider             |
| `Name() string`                                   | Returns `"google-genai"`          |
| `Description() string`                            | Returns the provider description  |
| `Version() string`                                | Returns the provider version      |
| `Models() []string`                               | Returns configured model IDs      |
| `Generate(ctx, model, msg, opts) (*Resp, err)`    | Synchronous generation            |
| `GenerateStream(ctx, model, msg, opts)`           | Streaming generation              |
| `Close() error`                                   | No-op (no persistent connections) |

### Finish Reasons

| Google GenAI FinishReason | genai.FinishReason | Description             |
| ------------------------- | ------------------ | ----------------------- |
| `STOP`                    | `Stop`             | Natural end of response |
| `MAX_TOKENS`              | `Length`           | Token limit reached     |
| `SAFETY`                  | `ContentFilter`    | Safety filter triggered |
| `RECITATION`              | `ContentFilter`    | Recitation filter       |
| `BLOCKLIST`               | `ContentFilter`    | Blocked by blocklist    |
| `PROHIBITED_CONTENT`      | `ContentFilter`    | Prohibited content      |
| `SPII`                    | `ContentFilter`    | Sensitive PII detected  |
| `MALFORMED_FUNCTION_CALL` | `FunctionCall`     | Malformed function call |
| `OTHER`                   | `Unknown`          | Other reason            |

## Prerequisites

- Go 1.23+ (uses `iter.Seq2` for streaming)
- For **Vertex AI**: GCP project with Vertex AI API enabled, authentication configured (ADC, service account, etc.)
- For **Gemini API**: A valid API key from [Google AI Studio](https://aistudio.google.com/)

## Contributing

See the repository [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
