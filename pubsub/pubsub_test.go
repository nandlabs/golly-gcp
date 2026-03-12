package pubsub

import (
	"io"
	"net/url"
	"testing"

	"oss.nandlabs.io/golly/messaging"
)

// --- Provider basic tests ---

func TestProvider_Id(t *testing.T) {
	p := &Provider{}
	if p.Id() != PubSubProviderID {
		t.Errorf("expected %q, got %q", PubSubProviderID, p.Id())
	}
}

func TestProvider_Schemes(t *testing.T) {
	p := &Provider{}
	schemes := p.Schemes()
	if len(schemes) != 1 || schemes[0] != PubSubScheme {
		t.Errorf("expected ['pubsub'], got %v", schemes)
	}
}

func TestProvider_Setup(t *testing.T) {
	p := &Provider{}
	if err := p.Setup(); err != nil {
		t.Errorf("expected nil error from Setup, got %v", err)
	}
}

func TestProvider_NewMessage(t *testing.T) {
	p := &Provider{}
	msg, err := p.NewMessage(PubSubScheme)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	psMsg, ok := msg.(*MessagePubSub)
	if !ok {
		t.Fatal("expected *MessagePubSub")
	}
	if psMsg.provider != p {
		t.Error("expected provider reference to match")
	}
	if psMsg.stringHeaders == nil {
		t.Error("expected stringHeaders to be initialized")
	}
}

func TestProvider_Close(t *testing.T) {
	p := &Provider{}
	if err := p.Close(); err != nil {
		t.Errorf("expected nil error from Close, got %v", err)
	}
	if !p.closed.Load() {
		t.Error("expected closed to be true after Close()")
	}
}

func TestProvider_Close_StopsListeners(t *testing.T) {
	p := &Provider{}
	cancelled := false
	p.stopFns = append(p.stopFns, func() { cancelled = true })
	if err := p.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cancelled {
		t.Error("expected cancel function to be called")
	}
	if p.stopFns != nil {
		t.Error("expected stopFns to be nil after Close")
	}
}

func TestProvider_SendBatch_Empty(t *testing.T) {
	p := &Provider{}
	u, _ := url.Parse("pubsub://my-topic")
	err := p.SendBatch(u, nil)
	if err != nil {
		t.Errorf("expected nil error for empty batch, got %v", err)
	}
}

// --- MessagePubSub tests ---

func TestMessagePubSub_Headers(t *testing.T) {
	p := &Provider{}
	msg, _ := p.NewMessage(PubSubScheme)
	psMsg := msg.(*MessagePubSub)

	psMsg.SetStrHeader("key1", "value1")
	psMsg.SetIntHeader("count", 42)
	psMsg.SetBoolHeader("flag", true)
	psMsg.SetFloatHeader("rate", 3.14)
	psMsg.SetFloat64Header("precise", 2.71828)
	psMsg.SetInt8Header("small", 7)
	psMsg.SetInt16Header("medium", 256)
	psMsg.SetInt32Header("large", 100000)
	psMsg.SetInt64Header("huge", 9999999999)
	psMsg.SetHeader("raw", []byte("bytes"))

	expected := map[string]string{
		"key1":    "value1",
		"count":   "42",
		"flag":    "true",
		"rate":    "3.14",
		"precise": "2.71828",
		"small":   "7",
		"medium":  "256",
		"large":   "100000",
		"huge":    "9999999999",
		"raw":     "bytes",
	}

	for k, want := range expected {
		got, ok := psMsg.stringHeaders[k]
		if !ok {
			t.Errorf("missing header %q", k)
			continue
		}
		if got != want {
			t.Errorf("header %q: expected %q, got %q", k, want, got)
		}
	}
}

func TestMessagePubSub_Rsvp_NilPubsubMessage(t *testing.T) {
	p := &Provider{}
	msg, _ := p.NewMessage(PubSubScheme)
	psMsg := msg.(*MessagePubSub)

	// Rsvp on an outbound message (no underlying pubsub message) should be no-op
	if err := psMsg.Rsvp(true); err != nil {
		t.Errorf("expected nil error for Rsvp on outbound message, got %v", err)
	}
	if err := psMsg.Rsvp(false); err != nil {
		t.Errorf("expected nil error for Rsvp on outbound message, got %v", err)
	}
}

// --- buildAttributes tests ---

func TestBuildAttributes_PubSubMessage(t *testing.T) {
	p := &Provider{}
	msg, _ := p.NewMessage(PubSubScheme)
	psMsg := msg.(*MessagePubSub)

	psMsg.SetStrHeader("env", "prod")
	psMsg.SetStrHeader("version", "1.0")

	attrs := buildAttributes(psMsg)
	if attrs == nil {
		t.Fatal("expected non-nil attributes")
	}
	if attrs["env"] != "prod" {
		t.Errorf("expected env='prod', got %q", attrs["env"])
	}
	if attrs["version"] != "1.0" {
		t.Errorf("expected version='1.0', got %q", attrs["version"])
	}
}

func TestBuildAttributes_NonPubSubMessage(t *testing.T) {
	// Use a mock that satisfies messaging.Message but is not *MessagePubSub
	attrs := buildAttributes(&mockMessage{})
	if attrs != nil {
		t.Errorf("expected nil for non-PubSub message, got %v", attrs)
	}
}

// --- toMessage tests ---

func TestProvider_ToMessage(t *testing.T) {
	// We can't easily create a real gpubsub.Message, but we can test
	// the constants and option keys
	if PubSubScheme != "pubsub" {
		t.Errorf("expected PubSubScheme 'pubsub', got %q", PubSubScheme)
	}
	if PubSubProviderID != "pubsub-provider" {
		t.Errorf("expected PubSubProviderID 'pubsub-provider', got %q", PubSubProviderID)
	}
}

// --- Option constants tests ---

func TestOptionConstants(t *testing.T) {
	constants := map[string]string{
		"OptTimeout":                OptTimeout,
		"OptBatchSize":              OptBatchSize,
		"OptOrderingKey":            OptOrderingKey,
		"OptMaxExtension":           OptMaxExtension,
		"OptMaxOutstandingMessages": OptMaxOutstandingMessages,
	}
	for name, val := range constants {
		if val == "" {
			t.Errorf("option constant %s should not be empty", name)
		}
	}
}

// --- getPubSubClient tests ---

func TestGetPubSubClient_NoConfig(t *testing.T) {
	u, _ := url.Parse("pubsub://nonexistent-topic")
	_, err := getPubSubClient(u)
	if err == nil {
		t.Fatal("expected error when no config registered")
	}
}

// --- resolvePublisher tests ---

func TestResolvePublisher_EmptyHost(t *testing.T) {
	u := &url.URL{Scheme: "pubsub"}
	// We need a client but can't create one without credentials.
	// Test just the URL validation by calling with nil client — expect the host check.
	_, err := resolvePublisher(nil, u)
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

// --- resolveSubscriber tests ---

func TestResolveSubscriber_EmptyHost(t *testing.T) {
	u := &url.URL{Scheme: "pubsub"}
	_, err := resolveSubscriber(nil, u)
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

// --- mockMessage ---

// mockMessage is a minimal implementation of messaging.Message for testing buildAttributes.
type mockMessage struct{}

var _ messaging.Message = (*mockMessage)(nil)

// --- Header interface ---
func (m *mockMessage) Id() string                                  { return "" }
func (m *mockMessage) SetHeader(key string, value []byte)          {}
func (m *mockMessage) SetStrHeader(key string, value string)       {}
func (m *mockMessage) SetBoolHeader(key string, value bool)        {}
func (m *mockMessage) SetIntHeader(key string, value int)          {}
func (m *mockMessage) SetInt8Header(key string, value int8)        {}
func (m *mockMessage) SetInt16Header(key string, value int16)      {}
func (m *mockMessage) SetInt32Header(key string, value int32)      {}
func (m *mockMessage) SetInt64Header(key string, value int64)      {}
func (m *mockMessage) SetFloatHeader(key string, value float32)    {}
func (m *mockMessage) SetFloat64Header(key string, value float64)  {}
func (m *mockMessage) GetHeader(key string) ([]byte, bool)         { return nil, false }
func (m *mockMessage) GetStrHeader(key string) (string, bool)      { return "", false }
func (m *mockMessage) GetBoolHeader(key string) (bool, bool)       { return false, false }
func (m *mockMessage) GetIntHeader(key string) (int, bool)         { return 0, false }
func (m *mockMessage) GetInt8Header(key string) (int8, bool)       { return 0, false }
func (m *mockMessage) GetInt16Header(key string) (int16, bool)     { return 0, false }
func (m *mockMessage) GetInt32Header(key string) (int32, bool)     { return 0, false }
func (m *mockMessage) GetInt64Header(key string) (int64, bool)     { return 0, false }
func (m *mockMessage) GetFloatHeader(key string) (float32, bool)   { return 0, false }
func (m *mockMessage) GetFloat64Header(key string) (float64, bool) { return 0, false }

// --- Body interface ---
func (m *mockMessage) SetBodyStr(in string) (int, error)                     { return 0, nil }
func (m *mockMessage) SetBodyBytes(data []byte) (int, error)                 { return 0, nil }
func (m *mockMessage) SetFrom(content io.Reader) (int64, error)              { return 0, nil }
func (m *mockMessage) WriteJSON(in interface{}) error                        { return nil }
func (m *mockMessage) WriteXML(in interface{}) error                         { return nil }
func (m *mockMessage) WriteContent(in interface{}, contentType string) error { return nil }
func (m *mockMessage) ReadBody() io.Reader                                   { return nil }
func (m *mockMessage) ReadBytes() []byte                                     { return nil }
func (m *mockMessage) ReadAsStr() string                                     { return "" }
func (m *mockMessage) ReadJSON(out interface{}) error                        { return nil }
func (m *mockMessage) ReadXML(out interface{}) error                         { return nil }
func (m *mockMessage) ReadContent(out interface{}, contentType string) error { return nil }

// --- Rsvp ---
func (m *mockMessage) Rsvp(accept bool, options ...messaging.Option) error { return nil }
