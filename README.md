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
  <a href="https://github.com/nandlabs/golly-gcp/blob/main/LICENSING.md"><img src="https://img.shields.io/github/license/nandlabs/golly-gcp?color=blue" alt="License"></a>
</p>

<p align="center">
  <a href="#installation">Installation</a> •
  <a href="#packages">Packages</a> •
  <a href="#contributing">Contributing</a>
</p>

---

## Overview

Golly GCP provides Google Cloud service implementations for the full set of [Golly](https://github.com/nandlabs/golly) capability interfaces — VFS, Messaging, Cache, Secrets, GenAI, Auth, Authz, Vectorstore, and Chrono leader election. It uses the official Google Cloud client libraries and follows Golly's provider pattern: blank-import a package to auto-register it, then use standard Golly managers with `gs://` or `pubsub://` URLs.

## Compatibility

| golly-gcp | golly  | google.golang.org/api |
| --------- | ------ | --------------------- |
| v1.1.0    | v1.8.0 | v0.287.0              |
| v1.0.0    | v1.7.0 | v0.287.0              |
| v0.2.0    | v1.5.0 | v0.276.0              |
| v0.1.0    | v1.4.0 | v0.271.0              |

## Installation

```bash
go get oss.nandlabs.io/golly-gcp@v1.1.0
```

## Packages

### ⚙️ Configuration

| Package                    | Description                                                                                           |
| -------------------------- | ----------------------------------------------------------------------------------------------------- |
| [gcpsvc](gcpsvc/README.md) | Centralized GCP config management with named registry, multi-project/region, and URL-based resolution |

### 🔐 Auth & Authorization

| Package                                    | Description                                                                                                       |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- |
| [auth/firebase](auth/firebase/README.md)   | Firebase Auth JWT verifier (JWKS-cached RS256) + Firestore-backed `auth.SessionStore` with `ListByUser` helper    |
| [authz/gcpiam](authz/gcpiam/README.md)     | Cloud IAM `authz.Policy` — cached `TestIamPermissions` evaluator + `IAMResource{Ctx,Name}` live-check envelope    |

### 🤖 AI & Intelligence

| Package                                       | Description                                                                                                             |
| --------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| [genai](genai/README.md)                      | Google GenAI provider for Vertex AI, Gemini API, and Model Garden — streaming, tool use, structured output, Embedder, and a Gemini model catalog (`CapabilityProvider`) for the golly v1.8.0 model router |
| [vectorstore/vertex](vectorstore/vertex/README.md) | Vertex AI Vector Search backend — batched upserts via IndexServiceClient, FindNeighbors via MatchServiceClient       |
| [vectorstore/alloydb](vectorstore/alloydb/README.md) | AlloyDB / Cloud SQL pgvector backend on a caller-owned `*sql.DB` — HNSW / IVFFlat compatible, JSONB metadata filter |

### 🗃️ Storage

| Package            | Description                                                                                                                                          |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| [gs](gs/README.md) | Google Cloud Storage implementation of the golly VFS interface — read, write, copy, move, list, walk; VFileSystemCtx, Lister, RangeReader, and sentinel-error mapping |

### 🧠 Cache

| Package                                          | Description                                                                                                    |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| [cache/memorystore](cache/memorystore/README.md) | Memorystore Redis backend for `cache.Cache[K,V]` + `Sweeper` + `Loader` (single-flight); OAuth2 IAM auth       |

### 🕐 Scheduling & Leader Election

| Package                                            | Description                                                                                                     |
| -------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| [chrono/firestorelock](chrono/firestorelock/README.md) | `chrono.LeaderElector` on a Firestore doc — transactional acquire / renew / steal with expiring leases      |
| [chrono/gcslock](chrono/gcslock/README.md)             | `chrono.LeaderElector` on a GCS object — `DoesNotExist` acquire + `GenerationMatch` renew/steal              |

### 🔑 Secrets

| Package                      | Description                                                                                                          |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| [secrets](secrets/README.md) | GCP Secret Manager implementation of the golly secrets store — get, write, delete, list; `NewNamespacedGCPSecretStore` convenience constructor with Authorizer wiring |

### 📡 Messaging

| Package                    | Description                                                                                                                                                  |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [pubsub](pubsub/README.md) | Google Cloud Pub/Sub provider — publish, receive, listeners; `ListenerRemover`, `Producer/ReceiverCtx`, `Keyed → OrderingKey`, `ObservableProvider`, broker-targeted options |

> 📖 Full API documentation available at [pkg.go.dev](https://pkg.go.dev/oss.nandlabs.io/golly-gcp)

---

## Contributing

We welcome contributions to the project. If you find a bug or would like to
request a new feature, please open an issue on
[GitHub](https://github.com/nandlabs/golly-gcp/issues).

## License

Licensed under either of

- Apache License, Version 2.0 ([LICENSE-APACHE](LICENSE-APACHE) or <http://www.apache.org/licenses/LICENSE-2.0>)
- MIT license ([LICENSE-MIT](LICENSE-MIT) or <http://opensource.org/licenses/MIT>)

at your option.

### Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted
for inclusion in the work by you, as defined in the Apache-2.0 license, shall be
dual licensed as above, without any additional terms or conditions.
