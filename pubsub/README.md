# pubsub

Google Cloud Pub/Sub (v2) implementation of the [golly messaging](https://pkg.go.dev/oss.nandlabs.io/golly/messaging) `Provider` interface.

This package uses [`cloud.google.com/go/pubsub/v2`](https://pkg.go.dev/cloud.google.com/go/pubsub/v2), the latest major version of the Google Cloud Pub/Sub client library.

---

- [Installation](#installation)
- [Features](#features)
- [Architecture](#architecture)
- [Auto-Registration](#auto-registration)
- [URL Format](#url-format)
- [Configuration](#configuration)
- [Usage](#usage)
- [Message Acknowledgement](#message-acknowledgement)
- [Headers & Attributes](#headers--attributes)
- [Options](#options)
- [Ordered Delivery](#ordered-delivery)
- [Error Handling](#error-handling)
- [Thread Safety & Graceful Shutdown](#thread-safety--graceful-shutdown)
- [API Reference](#api-reference)
- [Prerequisites](#prerequisites)
- [Contributing](#contributing)

---

## Installation

```bash
go get oss.nandlabs.io/golly-gcp/pubsub
```

## Features

- **Send** — publish a single message to a Pub/Sub topic
- **SendBatch** — publish multiple messages asynchronously with automatic batching by the Pub/Sub client library
- **Receive** — receive a single message from a subscription with configurable timeout
- **ReceiveBatch** — receive up to N messages from a subscription
- **AddListener** — continuously receive messages in a background goroutine with graceful shutdown
- **Rsvp** — acknowledge (Ack) or reject (Nack) received messages for at-least-once delivery
- **Ordered delivery** — ordering key support for FIFO-like message ordering
- **Auto-registration** — blank import registers the Pub/Sub provider with the golly messaging manager
- **Config resolution** — leverages `gcpsvc` for per-topic/subscription or global GCP configuration
- **Thread-safe** — all listener management is protected with mutexes and atomic flags

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  Application                                                     │
│                                                                  │
│  import _ "oss.nandlabs.io/golly-gcp/pubsub"                   │
│                                                                  │
│  mgr := messaging.GetManager()                                  │
│  mgr.Send(url, msg, opts...)                                    │
│  mgr.Receive(url, opts...)                                      │
│  mgr.AddListener(url, fn, opts...)                              │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│  golly/messaging.Manager                                         │
│                                                                  │
│  Routes to provider by URL scheme ("pubsub")                     │
│  Calls provider.Send / Receive / AddListener                    │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│  pubsub.Provider                                                 │
│                                                                  │
│  1. getPubSubClient(u)       → gcpsvc.GetConfig(u, "pubsub")   │
│  2. resolvePublisher/Sub(c,u)→ client.Publisher / Subscriber    │
│  3. Pub/Sub v2 API call      → Publish / Receive                │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│  gcpsvc.Manager                                                  │
│                                                                  │
│  Config resolution chain:                                        │
│  url.Host → url.Host+"/"+url.Path → fallback name ("pubsub")   │
│                                                                  │
│  Returns *gcpsvc.Config with ProjectId + ClientOptions          │
└──────────────────────────────────────────────────────────────────┘
```

## Auto-Registration

On package import, the `init()` function in `pkg.go` creates a `Provider` and registers it with `messaging.GetManager()`:

```go
func init() {
    provider := &Provider{}
    messagingManager := messaging.GetManager()
    messagingManager.Register(provider)
}
```

This means a **blank import** is all you need to make the Pub/Sub provider available:

```go
import _ "oss.nandlabs.io/golly-gcp/pubsub"
```

After this import, any call to `messaging.GetManager().Send(u, ...)` with a `pubsub://` scheme URL will automatically route to this provider.

## URL Format

```
pubsub://topic-name          (for publishing)
pubsub://subscription-name   (for receiving)
```

| Component | Maps To                                                             |
| --------- | ------------------------------------------------------------------- |
| Scheme    | `pubsub` — used to route to this provider via the messaging manager |
| Host      | Topic name (for Send/SendBatch) or Subscription name (for Receive)  |

**Examples:**

| URL                        | Operation | Target                |
| -------------------------- | --------- | --------------------- |
| `pubsub://my-topic`        | Send      | Topic `my-topic`      |
| `pubsub://my-subscription` | Receive   | Sub `my-subscription` |
| `pubsub://orders-topic`    | Send      | Topic `orders-topic`  |
| `pubsub://orders-sub`      | Receive   | Sub `orders-sub`      |

> **Note:** Google Cloud Pub/Sub uses separate topic and subscription resources. Publishing goes to topics; receiving is done from subscriptions. Topics and subscriptions must be created in advance (via Console, Terraform, `gcloud`, etc.).

## Configuration

Configuration is resolved via the [`gcpsvc`](../gcpsvc/) package.

### How Config Resolution Works

When `getPubSubClient` is called with a URL like `pubsub://my-topic`, the provider calls `gcpsvc.GetConfig(u, "pubsub")` which tries the following resolution chain:

1. **`url.Host`** — look up `"my-topic"` in `gcpsvc.Manager`
2. **`url.Host + "/" + url.Path`** — look up `"my-topic/"` (if path is present)
3. **Fallback name** — look up `"pubsub"` in `gcpsvc.Manager`

If no config is found or the config has no `ProjectId`, the provider returns an error.

### Basic Setup

```go
import (
    "oss.nandlabs.io/golly-gcp/gcpsvc"
    _ "oss.nandlabs.io/golly-gcp/pubsub"
    "oss.nandlabs.io/golly/messaging"
)

func main() {
    // Register a default config for all Pub/Sub operations
    cfg := &gcpsvc.Config{ProjectId: "my-gcp-project"}
    gcpsvc.Manager.Register("pubsub", cfg)

    mgr := messaging.GetManager()
    // mgr.Send, mgr.Receive, etc.
}
```

### With Service Account Credentials

```go
cfg := &gcpsvc.Config{ProjectId: "my-gcp-project"}
cfg.SetCredentialFile("/path/to/service-account.json")
gcpsvc.Manager.Register("pubsub", cfg)
```

### With Custom Endpoint (Emulator)

```go
cfg := &gcpsvc.Config{ProjectId: "my-gcp-project"}
cfg.SetEndpoint("localhost:8085")
gcpsvc.Manager.Register("pubsub", cfg)
```

The [Pub/Sub emulator](https://cloud.google.com/pubsub/docs/emulator) can be started with:

```bash
gcloud beta emulators pubsub start --project=my-gcp-project
```

### Per-Topic/Subscription Configuration

```go
// Orders topic uses a specific project
ordersCfg := &gcpsvc.Config{ProjectId: "orders-project"}
ordersCfg.SetCredentialFile("/path/to/orders-sa.json")
gcpsvc.Manager.Register("orders-topic", ordersCfg)

// Default for all other Pub/Sub operations
defaultCfg := &gcpsvc.Config{ProjectId: "default-project"}
gcpsvc.Manager.Register("pubsub", defaultCfg)
```

**Resolution table for this setup:**

| URL                        | Resolved Config Key |
| -------------------------- | ------------------- |
| `pubsub://orders-topic`    | `orders-topic`      |
| `pubsub://any-other-topic` | `pubsub` (fallback) |

## Usage

### Publishing a Message

```go
import (
    "net/url"

    _ "oss.nandlabs.io/golly-gcp/pubsub"
    "oss.nandlabs.io/golly-gcp/gcpsvc"
    "oss.nandlabs.io/golly/messaging"
)

func main() {
    cfg := &gcpsvc.Config{ProjectId: "my-project"}
    gcpsvc.Manager.Register("pubsub", cfg)

    mgr := messaging.GetManager()

    msg, _ := mgr.NewMessage("pubsub")
    msg.SetBodyStr(`{"event": "order.created", "id": "12345"}`)

    u, _ := url.Parse("pubsub://my-topic")
    err := mgr.Send(u, msg)
    if err != nil {
        panic(err)
    }
}
```

### Publishing a Batch

```go
msgs := make([]messaging.Message, 5)
for i := range msgs {
    m, _ := mgr.NewMessage("pubsub")
    m.SetBodyStr(fmt.Sprintf("message-%d", i))
    msgs[i] = m
}

u, _ := url.Parse("pubsub://my-topic")
err := mgr.SendBatch(u, msgs)
```

### Receiving a Single Message

```go
u, _ := url.Parse("pubsub://my-subscription")

opts := messaging.NewOptionsBuilder().
    Add("Timeout", 10). // 10-second timeout
    Build()

msg, err := mgr.Receive(u, opts...)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Received:", msg.ReadAsStr())

// Acknowledge the message
msg.Rsvp(true)
```

### Receiving a Batch

```go
u, _ := url.Parse("pubsub://my-subscription")

opts := messaging.NewOptionsBuilder().
    Add("BatchSize", 5).
    Add("Timeout", 15).
    Build()

msgs, err := mgr.ReceiveBatch(u, opts...)
if err != nil {
    log.Fatal(err)
}
for _, msg := range msgs {
    fmt.Println("Received:", msg.ReadAsStr())
    msg.Rsvp(true) // Acknowledge
}
```

### Adding a Listener

```go
u, _ := url.Parse("pubsub://my-subscription")

opts := messaging.NewOptionsBuilder().
    Add("MaxOutstandingMessages", 100).
    Build()

err := mgr.AddListener(u, func(msg messaging.Message) {
    fmt.Println("Received:", msg.ReadAsStr())
    // Process the message...
    msg.Rsvp(true)  // Acknowledge on success
    // msg.Rsvp(false) // Nack to redeliver on failure
}, opts...)
if err != nil {
    log.Fatal(err)
}

// Wait for signal to shut down
mgr.Wait()
```

### Listener with Timeout

```go
opts := messaging.NewOptionsBuilder().
    Add("Timeout", 60). // Stop listening after 60 seconds
    Build()

mgr.AddListener(u, handler, opts...)
```

## Message Acknowledgement

Unlike the old implementation which auto-acknowledged messages, the reimplemented provider delegates acknowledgement to the caller via `Rsvp`:

| Call              | Effect                                                                                   |
| ----------------- | ---------------------------------------------------------------------------------------- |
| `msg.Rsvp(true)`  | **Ack** — Removes the message from the subscription. It will not be redelivered.         |
| `msg.Rsvp(false)` | **Nack** — Makes the message immediately available for redelivery to another subscriber. |

For outbound (sent) messages, `Rsvp` is a no-op.

> **Important:** If you don't call `Rsvp`, the message's ack deadline will expire, and Pub/Sub will redeliver it. Always ack or nack in your listener callbacks.

## Headers & Attributes

Google Cloud Pub/Sub message attributes are **string key-value pairs only**. The golly messaging `Header` interface supports multiple types (string, bool, int, float, etc.), so this provider automatically converts all header values to their string representation when publishing.

### How It Works

- **Outbound (Send):** When you set headers on a `MessagePubSub` using any `Set*Header` method, the provider tracks a string copy of each value. These string headers are used as Pub/Sub message attributes when publishing.
- **Inbound (Receive):** Pub/Sub message attributes are mapped to string headers via `SetStrHeader`. Use `GetStrHeader` to retrieve them.

### Type Conversion

| Header Method       | Example Value | Attribute Value |
| ------------------- | ------------- | --------------- |
| `SetStrHeader`      | `"v1"`        | `"v1"`          |
| `SetHeader` (bytes) | `[]byte("x")` | `"x"`           |
| `SetBoolHeader`     | `true`        | `"true"`        |
| `SetIntHeader`      | `42`          | `"42"`          |
| `SetInt64Header`    | `100`         | `"100"`         |
| `SetFloatHeader`    | `3.14`        | `"3.14"`        |
| `SetFloat64Header`  | `2.718`       | `"2.718"`       |

### Example

```go
msg, _ := mgr.NewMessage("pubsub")
msg.SetBodyStr(`{"event": "order.created"}`)

// All header types are converted to string attributes
msg.SetStrHeader("source", "order-service")
msg.SetIntHeader("version", 2)
msg.SetBoolHeader("retry", false)

// Published with attributes: {"source": "order-service", "version": "2", "retry": "false"}
u, _ := url.Parse("pubsub://my-topic")
mgr.Send(u, msg)
```

```go
// On the receiving side, all attributes come back as strings
received, _ := mgr.Receive(subURL)
source, _ := received.GetStrHeader("source")   // "order-service"
version, _ := received.GetStrHeader("version")  // "2"
retry, _ := received.GetStrHeader("retry")      // "false"
```

> **Note:** Since Pub/Sub attributes only support strings, non-string header values are converted at send time and arrive as strings on the receiving side. If you need the original typed values, you must parse them from the string representation in your application code.

## Options

### Send Options

| Key           | Type   | Description                                                    |
| ------------- | ------ | -------------------------------------------------------------- |
| `OrderingKey` | string | Ordering key for messages requiring FIFO ordering within a key |

### Receive Options

| Key         | Type | Description                                               |
| ----------- | ---- | --------------------------------------------------------- |
| `Timeout`   | int  | Timeout in seconds for the receive operation. Default: 30 |
| `BatchSize` | int  | Maximum number of messages for ReceiveBatch. Default: 10  |

### Listener Options

| Key                      | Type | Description                                        |
| ------------------------ | ---- | -------------------------------------------------- |
| `Timeout`                | int  | Total listener duration in seconds. 0 = indefinite |
| `MaxOutstandingMessages` | int  | Max unprocessed messages before pausing pulls      |
| `MaxExtension`           | int  | Maximum ack deadline extension in seconds          |

## Ordered Delivery

Google Cloud Pub/Sub supports [message ordering](https://cloud.google.com/pubsub/docs/ordering) with ordering keys. When messages share the same ordering key, they are delivered in the order they were published.

```go
opts := messaging.NewOptionsBuilder().
    Add("OrderingKey", "order-123").
    Build()

msg, _ := mgr.NewMessage("pubsub")
msg.SetBodyStr("first event")
mgr.Send(u, msg, opts...)

msg2, _ := mgr.NewMessage("pubsub")
msg2.SetBodyStr("second event")
mgr.Send(u, msg2, opts...)
```

> **Note:** The subscription must have message ordering enabled. See the [Pub/Sub documentation](https://cloud.google.com/pubsub/docs/ordering) for setup details.

## Error Handling

All methods return standard Go errors with the `pubsub:` prefix for easy identification:

```go
msg, err := mgr.Receive(u, opts...)
if err != nil {
    // err: "pubsub: no messages available within timeout"
    // err: "pubsub: receive failed: ..."
    // err: "pubsub: no GCP config with ProjectId registered for ..."
}
```

Common error scenarios:

| Error                                              | Cause                                                |
| -------------------------------------------------- | ---------------------------------------------------- |
| `pubsub: no GCP config with ProjectId registered`  | No `gcpsvc.Config` registered or missing `ProjectId` |
| `pubsub: topic name (URL host) is required`        | Empty host in URL                                    |
| `pubsub: subscription name (URL host) is required` | Empty host in URL                                    |
| `pubsub: publish failed: ...`                      | GCP API error during publish                         |
| `pubsub: no messages available within timeout`     | No messages in subscription within timeout           |

## Thread Safety & Graceful Shutdown

The `Provider` is thread-safe. The `AddListener` method runs the Pub/Sub subscription receiver in a background goroutine. Multiple listeners can be active simultaneously.

To shut down all listeners:

```go
mgr.Close() // Cancels all listener contexts
```

The provider uses:

- `sync/atomic.Bool` — to track closed state without locks on the hot path
- `sync.Mutex` — to protect the list of cancel functions
- `context.WithCancel` / `context.WithTimeout` — for listener lifecycle management

When `Close()` is called:

1. The `closed` flag is set atomically
2. All cancel functions for active listeners are invoked
3. Each listener goroutine detects the context cancellation and exits
4. Pub/Sub clients are closed in the deferred cleanup of each goroutine

## API Reference

### Types

| Type            | Description                                                 |
| --------------- | ----------------------------------------------------------- |
| `Provider`      | Implements `messaging.Provider` for Google Cloud Pub/Sub    |
| `MessagePubSub` | Wraps `messaging.BaseMessage` with Pub/Sub Ack/Nack support |

### Provider Methods

| Method           | Description                                             |
| ---------------- | ------------------------------------------------------- |
| `Id()`           | Returns `"pubsub-provider"`                             |
| `Schemes()`      | Returns `["pubsub"]`                                    |
| `Setup()`        | No-op initialization                                    |
| `NewMessage()`   | Creates a new `MessagePubSub`                           |
| `Send()`         | Publishes a single message to a topic                   |
| `SendBatch()`    | Publishes multiple messages (async with result waiting) |
| `Receive()`      | Receives a single message from a subscription           |
| `ReceiveBatch()` | Receives up to N messages from a subscription           |
| `AddListener()`  | Starts a background subscription receiver goroutine     |
| `Close()`        | Stops all active listeners and releases resources       |

### MessagePubSub Methods

| Method   | Description                                                 |
| -------- | ----------------------------------------------------------- |
| `Rsvp()` | Ack (accept=true) or Nack (accept=false) a received message |

All `BaseMessage` methods (SetBodyStr, ReadAsStr, SetStrHeader, etc.) are also available.

### Constants

| Constant           | Value               | Description         |
| ------------------ | ------------------- | ------------------- |
| `PubSubScheme`     | `"pubsub"`          | URL scheme          |
| `PubSubProviderID` | `"pubsub-provider"` | Provider identifier |

## Prerequisites

1. **Go 1.21+**
2. **Google Cloud SDK** (for credential setup)
3. **A GCP project** with Pub/Sub API enabled
4. **Topics & subscriptions** created beforehand
5. **Authentication** — one of:
   - Application Default Credentials (`gcloud auth application-default login`)
   - Service account key file (via `cfg.SetCredentialFile(...)`)
   - Service account JSON bytes (via `cfg.SetCredentialJSON(...)`)
   - Workload Identity (on GKE)

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.
