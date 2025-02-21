# golly-gcp

[![report](https://img.shields.io/badge/go%20report-A+-brightgreen.svg?style=flat)](https://goreportcard.com/report/oss.nandlabs.io/golly-gcp)
[![testing](https://img.shields.io/github/actions/workflow/status/nandlabs/golly/go_ci.yml?branch=main&event=push&color=228B22)](https://github.com/nandlabs/golly-gcp/actions?query=event%3Apush+branch%3Amain+)
[![release](https://img.shields.io/github/v/release/nandlabs/golly?label=latest&color=228B22)](https://github.com/nandlabs/golly-gcp/releases/latest)
[![releaseDate](https://img.shields.io/github/release-date/nandlabs/golly-gcp?label=released&color=00ADD8)](https://github.com/nandlabs/golly-gcp/releases/latest)
[![godoc](https://godoc.org/oss.nandlabs.io/golly?status.svg)](https://pkg.go.dev/oss.nandlabs.io/golly-gcp)

`golly-gcp` is a Go module that provides a set of utilities to interact with GCP services. This is an extension of [golly](https://github.com/nandlabs/golly).

## Installation

```bash
go get oss.nandlabs.io/golly-gcp
```

## Core Packages

1. [storage](storage/README.md): Golly VFS implementation for Google Cloud Storage
2. [pubsub](pubsub/README.md): Golly Messaging Implementation for PubSub
3. [vertex](vertex/README.md): Golly GenAI Provider for Vertex AI

## Contributing

We welcome contributions to the project. If you find a bug or would like to
request a new feature, please open an issue on
[GitHub](https://github.com/nandlabs/golly-gcp/issues).

## License

This project is licensed under MIT License. See the [License](LICENSE) file for
details.
