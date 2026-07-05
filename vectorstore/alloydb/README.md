# vectorstore/alloydb

Postgres/pgvector backend for the
[`oss.nandlabs.io/golly/vectorstore`](https://pkg.go.dev/oss.nandlabs.io/golly/vectorstore)
`Store` interface. Sized for AlloyDB for PostgreSQL and Cloud SQL for
PostgreSQL with the `pgvector` extension.

## Install

```bash
go get oss.nandlabs.io/golly-gcp/vectorstore/alloydb
```

## Quick start

```go
import (
    "database/sql"

    _ "github.com/jackc/pgx/v5/stdlib"           // or any Postgres driver
    "oss.nandlabs.io/golly-gcp/vectorstore/alloydb"
    "oss.nandlabs.io/golly/vectorstore"
)

db, _ := sql.Open("pgx", dsn)                    // caller owns *sql.DB
defer db.Close()

store, err := alloydb.New(alloydb.Options{
    DB:        db,
    Table:     "public.vectors",                 // schema.name, quoted safely
    Dimension: 768,
    Metric:    alloydb.MetricCosine,             // "" defaults to cosine
    IndexKind: alloydb.IndexHNSW,                // or IVFFlat, or IndexNone
})
if err != nil { return err }
defer store.Close()                              // does NOT close db

_ = store.Upsert(ctx, vectorstore.Doc{ID: "d1", Vector: emb})
hits, _ := store.Search(ctx, vectorstore.Query{Vector: qEmb, TopK: 10})
```

## Notes

- `New` runs `CREATE EXTENSION IF NOT EXISTS vector`, `CREATE TABLE IF NOT
  EXISTS`, and an optional ANN index. Pass `SkipDDL: true` to opt out.
- `Store.Close()` does **not** close the underlying `*sql.DB` — the caller
  owns that handle.
- Metric selects operator: cosine `<=>`, l2 `<->`, ip `<#>`.
- `Query.Filter` is applied as `metadata @> $filter` JSONB containment.
