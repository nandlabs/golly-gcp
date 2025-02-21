# Vertex AI Provider

This package provides an implementation of the `genai.Provider` interface for Google Cloud's Vertex AI. It allows for generating and streaming content using Vertex AI models.

## Installation

To install this package, use the following command:

```sh
go get oss.nandlabs.io/golly-gcp/vertex
```

## Usage

### Creating a Vertex AI Provider

To create a new Vertex AI provider, use the `NewVertexAiProvider` function:

```go
import "oss.nandlabs.io/golly-gcp/vertex"

provider := genai.Providers.Get(vertex.VertexAI)
```

### Generating Content

To generate content using a specific model, use the `Generate` method:

```go
import (
    "oss.nandlabs.io/golly-gcp/gcpsvc"
	"oss.nandlabs.io/golly-gcp/vertex"
	"oss.nandlabs.io/golly/genai"
	"oss.nandlabs.io/golly/l3"
)
var Logger = l3.Get()


func main(){
    credentialsPath := "<path to credentials.json>"

	Logger.DebugF("Initializing GCP Service")
    // Initialize the GCP service with config
	config := &gcpsvc.Config{
		ProjectId: "golly-gcp-01",
		Location:  "us-central1",
	}
    // Set credentials to config if requried

    // Register the Vertex AI provider config. The key is the default vertex key.
    // If you want to use a different key, you can register it with the desired key and pass option[vertex.ProviderKey] to the provider
	gcpsvc.Manager.Register(vertex.DefaultVertexKey, config)



	provider := genai.Providers.Get(vertex.VertexAI)
	if provider == nil {
		Logger.ErrorF("Provider not found")
		return
	}
	exchange := genai.NewExchange("1D0001")
	exchange.AddTxtMsg("Tell Me a Joke", genai.UserActor)

	err := provider.Generate("gemini-1.5-pro-002", exchange, genai.NewOptionsBuilder().Build())
	if err != nil {
		Logger.ErrorF("Error generating: %v", err)
		return
	}
	msgs := exchange.MsgsByActors(genai.AIActor)
	for _, msg := range msgs {
		Logger.InfoF("Response from AI: \n%v", msg)
	}
}
```

### Streaming Content

To generate a stream of content, use the `GenerateStream` method:

```go
import (
    "fmt"
    "oss.nandlabs.io/golly-gcp/gcpsvc"
	"oss.nandlabs.io/golly-gcp/vertex"
	"oss.nandlabs.io/golly/genai"
	"oss.nandlabs.io/golly/l3"
)
var Logger = l3.Get()


func main(){
    credentialsPath := "<path to credentials.json>"

	Logger.DebugF("Initializing GCP Service")
    // Initialize the GCP service with config
	config := &gcpsvc.Config{
		ProjectId: "golly-gcp-01",
		Location:  "us-central1",
	}
    // Set credentials to config if requried

    // Register the Vertex AI provider config. The key is the default vertex key.
    // If you want to use a different key, you can register it with the desired key and pass option[vertex.ProviderKey] to the provider
	gcpsvc.Manager.Register(vertex.DefaultVertexKey, config)



	provider := genai.Providers.Get(vertex.VertexAI)
	if provider == nil {
		Logger.ErrorF("Provider not found")
		return
	}
	exchange := genai.NewExchange("1D0001")
	exchange.AddTxtMsg("Tell Me a Joke", genai.UserActor)

	var handler = func(last bool, messages ...*genai.Message) {
		for _, msg := range messages {
			fmt.Print(msg)
		}
		if last {
			Logger.InfoF("Completed Generation")
		}
	}
	err = provider.GenerateStream("gemini-2.0-flash-001", exchange, handler, genai.NewOptionsBuilder().Build())
	if err != nil {
		Logger.ErrorF("Error generating: %v", err)
		return
    }
}
```

## Key Functionalities

- **VertexAiProvider**: Implements the `oss.nandlabs.io/golly/genai.Provider` interface.
- **Generate**: Generates content using a specified model and exchange options.
- **GenerateStream**: Generates a stream of content and handles streaming responses.
- **Models**: Returns the models of the provider ( unsupported for Vertex AI).
