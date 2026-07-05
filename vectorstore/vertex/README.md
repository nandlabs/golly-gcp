# vectorstore/vertex

Vertex AI Vector Search (formerly Matching Engine) backend for the
[`oss.nandlabs.io/golly/vectorstore`](https://pkg.go.dev/oss.nandlabs.io/golly/vectorstore)
`Store` interface.

Uses `cloud.google.com/go/aiplatform/apiv1beta1` — `IndexServiceClient`
for upsert / remove and `MatchServiceClient` for FindNeighbors.

## Install

```bash
go get oss.nandlabs.io/golly-gcp/vectorstore/vertex
```

## Quick start

```go
import (
    "context"

    "oss.nandlabs.io/golly-gcp/vectorstore/vertex"
    "oss.nandlabs.io/golly/vectorstore"
)

store, err := vertex.New(ctx, vertex.Options{
    Project:       "my-project",
    Location:      "us-central1",
    IndexEndpoint: "1234567890",   // or full resource name
    DeployedIndex: "my_deploy",
    Index:         "9876543210",
})
if err != nil { return err }
defer store.Close()

// Upsert
_ = store.Upsert(ctx, vectorstore.Doc{
    ID:       "doc-1",
    Vector:   embedding,                                // []float32
    Metadata: map[string]any{"source": "wiki"},
})

// Search
hits, _ := store.Search(ctx, vectorstore.Query{Vector: qEmb, TopK: 10})
```

## Notes

- `Search` returns Vertex-native distance in `Hit.Score` (lower is closer).
- String metadata becomes categorical `Restricts`; int/float becomes
  `NumericRestricts` with the `EQUAL` op. Unsupported types are ignored.
- `Upsert` batches larger than `UpsertBatchSize` (default 1000) into
  multiple RPCs.
