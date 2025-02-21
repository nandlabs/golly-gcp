package vertex

import (
	"oss.nandlabs.io/golly/genai"
	"oss.nandlabs.io/golly/l3"
)

const (
	VertexAI = "vertexai"
)

var logger = l3.Get()

func init() {
	logger.DebugF("vertex init")
	genai.Providers.Register(VertexAI, NewVertexAiProvider())

}
