<h1 align="center">Golly GCP</h1>

<p align="center">
  <strong>GCP service integrations for the <a href="https://github.com/nandlabs/golly">Golly</a> ecosystem</strong>
</p>

<p align="center">
  <a href="https://goreportcard.com/report/oss.nandlabs.io/golly-gcp"><img src="https://img.shields.io/badge/go%20report-A+-brightgreen.svg?style=flat" alt="Go Report"></a>
  <a href="https://github.com/nandlabs/golly-gcp/actions?query=event%3Apush+branch%3Amain+"><img src="https://img.shields.io/github/actions/workflow/status/nandlabs/golly-gcp/go_ci.yml?branch=main&event=push&color=228B22" alt="Build Status"></a>
  <a href="https://github.com/nandlabs/golly-gcp/releases/latest"><img src="https://img.shields.io/github/v/release/nandlabs/golly-gcp?label=latest&color=228B22" alt="Release"></a>
  <a href="https://github.com/nandlabs/golly-gcp/releases/latest"><img src="https://img.shields.io/github/release-date/nandlabs/golly-gcp?label=released&color=00ADD8" alt="Release Date"></a>
  <a href="https://pkg.go.dev/oss.nandlabs.io/golly-gcp"><img src="https://godoc.org/oss.nandlabs.io/golly-gcp?status.svg" alt="GoDoc"></a>
  <a href="https://github.com/nandlabs/golly-gcp/blob/main/LICENSE"><img src="https://img.shields.io/github/license/nandlabs/golly-gcp?color=blue" alt="License"></a>
</p>

<p align="center">
  <a href="#installation">Installation</a> •
  <a href="#packages">Packages</a> •
  <a href="#contributing">Contributing</a>
</p>

---

## Overview

Golly GCP provides Google Cloud service implementations for core [Golly](https://github.com/nandlabs/golly) interfaces — VFS, Messaging, and GenAI. It uses the official Google Cloud client libraries and follows Golly's provider pattern: blank-import a package to auto-register it, then use standard Golly managers with `gs://` or `pubsub://` URLs.

## Installation

```bash
go get oss.nandlabs.io/golly-gcp
```

## Packages

### ⚙️ Configuration

| Package                    | Description                                                                                           |
| -------------------------- | ----------------------------------------------------------------------------------------------------- |
| [gcpsvc](gcpsvc/README.md) | Centralized GCP config management with named registry, multi-project/region, and URL-based resolution |

### 🤖 AI & Intelligence

| Package                  | Description                                                                                                             |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| [genai](genai/README.md) | Google GenAI provider for Vertex AI, Gemini API, and Model Garden — supports streaming, tool use, and structured output |

### 🗃️ Storage

| Package            | Description                                                                                              |
| ------------------ | -------------------------------------------------------------------------------------------------------- |
| [gs](gs/README.md) | Google Cloud Storage implementation of the golly VFS interface — read, write, copy, move, list, and walk |

### 📡 Messaging

| Package                    | Description                                                                                                         |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| [pubsub](pubsub/README.md) | Google Cloud Pub/Sub implementation of the golly messaging provider — publish, receive, listeners, ordered delivery |

> 📖 Full API documentation available at [pkg.go.dev](https://pkg.go.dev/oss.nandlabs.io/golly-gcp)

---

## Contributing

We welcome contributions to the project. If you find a bug or would like to
request a new feature, please open an issue on
[GitHub](https://github.com/nandlabs/golly-gcp/issues).

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
