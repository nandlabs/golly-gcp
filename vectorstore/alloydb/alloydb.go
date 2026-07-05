// Package alloydb is a Postgres/pgvector backend for
// oss.nandlabs.io/golly/vectorstore, sized for AlloyDB for PostgreSQL and
// Cloud SQL for PostgreSQL with the pgvector extension.
//
// The Store expects a caller-owned *sql.DB whose driver understands the
// pgvector types (any Postgres driver works — pgvector-go supplies
// database/sql Scanner/Valuer implementations). New runs a small idempotent
// DDL to ensure the target table and optional ANN index exist.
//
// Target scale profile: OLTP-adjacent workloads with hundreds of thousands
// to a few tens of millions of vectors, HNSW or IVFFlat ANN, and rich
// SQL-side metadata filtering.
package alloydb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pgvector/pgvector-go"

	"oss.nandlabs.io/golly/vectorstore"
)

// Metric selects the distance operator used at search time and — where
// possible — the operator class used by the ANN index.
type Metric string

const (
	// MetricCosine uses the pgvector cosine distance operator (`<=>`).
	MetricCosine Metric = "cosine"
	// MetricL2 uses the pgvector Euclidean distance operator (`<->`).
	MetricL2 Metric = "l2"
	// MetricIP uses the pgvector negative inner-product operator (`<#>`).
	MetricIP Metric = "ip"
)

// IndexKind selects the ANN index type New will create alongside the table.
// Empty means no index — the caller is expected to manage it out of band.
type IndexKind string

const (
	IndexNone    IndexKind = ""
	IndexHNSW    IndexKind = "hnsw"
	IndexIVFFlat IndexKind = "ivfflat"
)

// Options configures a Store.
type Options struct {
	// DB is a caller-owned *sql.DB. The Store never calls DB.Close(); it
	// only takes a reference and stops touching it after its own Close.
	DB *sql.DB
	// Table is the fully-qualified target table (schema.name). Defaults
	// to "vectors" if empty.
	Table string
	// Dimension is the vector column dimension. Required.
	Dimension int
	// Metric selects the distance operator at search time. Defaults to
	// MetricCosine.
	Metric Metric
	// IndexKind selects the ANN index type to auto-create. Empty leaves
	// indexing to the caller.
	IndexKind IndexKind
	// SkipDDL disables the idempotent CREATE EXTENSION / CREATE TABLE /
	// CREATE INDEX run at New. Useful when the caller manages schema
	// migrations elsewhere.
	SkipDDL bool
}

// Store implements vectorstore.Store on top of pgvector.
type Store struct {
	db        *sql.DB
	table     string
	dim       int
	metric    Metric
	indexKind IndexKind

	closed   atomic.Bool
	closeMu  sync.Mutex
	quotedTb string
}

// New validates options, applies defaults, optionally runs idempotent DDL,
// and returns a ready Store.
func New(opts Options) (*Store, error) {
	if opts.DB == nil {
		return nil, errors.New("vectorstore/alloydb: Options.DB is required")
	}
	if opts.Dimension <= 0 {
		return nil, errors.New("vectorstore/alloydb: Options.Dimension must be > 0")
	}
	table := opts.Table
	if table == "" {
		table = "vectors"
	}
	metric := opts.Metric
	if metric == "" {
		metric = MetricCosine
	}
	if _, ok := operatorFor(metric); !ok {
		return nil, fmt.Errorf("vectorstore/alloydb: unknown metric %q", metric)
	}
	if !validIndex(opts.IndexKind) {
		return nil, fmt.Errorf("vectorstore/alloydb: unknown index kind %q", opts.IndexKind)
	}
	s := &Store{
		db:        opts.DB,
		table:     table,
		dim:       opts.Dimension,
		metric:    metric,
		indexKind: opts.IndexKind,
		quotedTb:  quoteQualified(table),
	}
	if !opts.SkipDDL {
		if err := s.ensureSchema(context.Background()); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// ensureSchema runs the idempotent DDL that guarantees a matching table and
// (optionally) index exist. It is safe to run at every startup.
func (s *Store) ensureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s (id TEXT PRIMARY KEY, embedding vector(%d) NOT NULL, metadata JSONB)`,
			s.quotedTb, s.dim,
		),
	}
	if op, ok := indexOpClass(s.metric); ok && s.indexKind != IndexNone {
		stmts = append(stmts, fmt.Sprintf(
			`CREATE INDEX IF NOT EXISTS %s ON %s USING %s (embedding %s)`,
			quoteIdent(indexName(s.table, string(s.indexKind))),
			s.quotedTb, s.indexKind, op,
		))
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("vectorstore/alloydb: ensure schema: %w (stmt=%q)", err, stmt)
		}
	}
	return nil
}

// Upsert INSERTs docs and ON CONFLICT (id) DO UPDATEs the embedding and
// metadata. All rows go through a single multi-VALUES statement inside a
// transaction so the write is atomic.
func (s *Store) Upsert(ctx context.Context, docs ...vectorstore.Doc) error {
	if s.closed.Load() {
		return errClosed
	}
	if len(docs) == 0 {
		return nil
	}
	for _, d := range docs {
		if err := vectorstore.ValidateDoc(d); err != nil {
			return err
		}
		if len(d.Vector) != s.dim {
			return fmt.Errorf("%w: collection dim=%d, doc=%q dim=%d", vectorstore.ErrDimMismatch, s.dim, d.ID, len(d.Vector))
		}
	}
	var (
		placeholders = make([]string, 0, len(docs))
		args         = make([]any, 0, len(docs)*3)
	)
	for i, d := range docs {
		base := i * 3
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d)", base+1, base+2, base+3))
		md, err := marshalMetadata(d.Metadata)
		if err != nil {
			return fmt.Errorf("vectorstore/alloydb: marshal metadata for %q: %w", d.ID, err)
		}
		args = append(args, d.ID, pgvector.NewVector([]float32(d.Vector)), md)
	}
	stmt := fmt.Sprintf(
		"INSERT INTO %s (id, embedding, metadata) VALUES %s "+
			"ON CONFLICT (id) DO UPDATE SET embedding = EXCLUDED.embedding, metadata = EXCLUDED.metadata",
		s.quotedTb, strings.Join(placeholders, ", "),
	)
	if _, err := s.db.ExecContext(ctx, stmt, args...); err != nil {
		return fmt.Errorf("vectorstore/alloydb: upsert: %w", err)
	}
	return nil
}

// Search runs an ANN scan ordered by the metric's distance operator and
// returns the top-K rows. When q.Filter is non-empty it is JSONB-marshalled
// and applied as a `metadata @> $filter` containment predicate.
func (s *Store) Search(ctx context.Context, q vectorstore.Query) ([]vectorstore.Hit, error) {
	if s.closed.Load() {
		return nil, errClosed
	}
	if len(q.Vector) == 0 {
		return nil, vectorstore.ErrEmptyVector
	}
	if len(q.Vector) != s.dim {
		return nil, fmt.Errorf("%w: query dim=%d, collection dim=%d", vectorstore.ErrDimMismatch, len(q.Vector), s.dim)
	}
	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}
	op, _ := operatorFor(s.metric)
	var (
		where string
		args  = []any{pgvector.NewVector([]float32(q.Vector))}
	)
	if len(q.Filter) > 0 {
		md, err := marshalMetadata(q.Filter)
		if err != nil {
			return nil, fmt.Errorf("vectorstore/alloydb: marshal filter: %w", err)
		}
		args = append(args, md)
		where = fmt.Sprintf(" WHERE metadata @> $%d", len(args))
	}
	args = append(args, topK)
	stmt := fmt.Sprintf(
		"SELECT id, metadata, embedding %s $1 AS distance FROM %s%s ORDER BY embedding %s $1 LIMIT $%d",
		op, s.quotedTb, where, op, len(args),
	)
	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("vectorstore/alloydb: search: %w", err)
	}
	defer rows.Close()
	hits := make([]vectorstore.Hit, 0, topK)
	for rows.Next() {
		var (
			id       string
			mdRaw    []byte
			distance float64
		)
		if err := rows.Scan(&id, &mdRaw, &distance); err != nil {
			return nil, fmt.Errorf("vectorstore/alloydb: scan: %w", err)
		}
		score := float32(distance)
		if q.MinScore != 0 && score < q.MinScore {
			continue
		}
		md, err := unmarshalMetadata(mdRaw)
		if err != nil {
			return nil, fmt.Errorf("vectorstore/alloydb: metadata for %q: %w", id, err)
		}
		hits = append(hits, vectorstore.Hit{ID: id, Score: score, Metadata: md})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vectorstore/alloydb: rows: %w", err)
	}
	return hits, nil
}

// Delete removes rows by id. Unknown ids are silently skipped.
func (s *Store) Delete(ctx context.Context, ids ...string) error {
	if s.closed.Load() {
		return errClosed
	}
	if len(ids) == 0 {
		return nil
	}
	stmt := fmt.Sprintf("DELETE FROM %s WHERE id = ANY($1)", s.quotedTb)
	if _, err := s.db.ExecContext(ctx, stmt, textArray(ids)); err != nil {
		return fmt.Errorf("vectorstore/alloydb: delete: %w", err)
	}
	return nil
}

// textArray is a driver.Valuer that serialises a []string as a Postgres
// text-array literal. This keeps the Store driver-agnostic — lib/pq's
// pq.Array helper works too, but bringing it in as a hard dep would force
// every consumer to import lib/pq. pgx and any driver that accepts the
// canonical `{a,b,c}` literal (i.e. all mainstream Postgres drivers)
// consume this transparently.
type textArray []string

// Value implements driver.Valuer.
func (a textArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, s := range a {
		if i > 0 {
			b.WriteByte(',')
		}
		// Quote every element so commas, braces, or backslashes in ids can't
		// break the literal. Postgres text-array escape rules: doubled
		// backslashes and escaped double-quotes.
		b.WriteByte('"')
		for j := 0; j < len(s); j++ {
			c := s[j]
			if c == '\\' || c == '"' {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		}
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String(), nil
}

// Close marks the Store closed. It intentionally does NOT close the
// underlying *sql.DB — that handle is owned by the caller.
func (s *Store) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.closed.Store(true)
	return nil
}

// errClosed is returned when any method is called after Close.
var errClosed = errors.New("vectorstore/alloydb: store is closed")

// operatorFor returns the pgvector distance operator for the given metric,
// and true when the metric is known.
func operatorFor(m Metric) (string, bool) {
	switch m {
	case MetricCosine:
		return "<=>", true
	case MetricL2:
		return "<->", true
	case MetricIP:
		return "<#>", true
	default:
		return "", false
	}
}

// indexOpClass returns the operator-class name to feed into a
// CREATE INDEX ... USING <kind> (embedding <opclass>) statement.
func indexOpClass(m Metric) (string, bool) {
	switch m {
	case MetricCosine:
		return "vector_cosine_ops", true
	case MetricL2:
		return "vector_l2_ops", true
	case MetricIP:
		return "vector_ip_ops", true
	default:
		return "", false
	}
}

func validIndex(k IndexKind) bool {
	switch k {
	case IndexNone, IndexHNSW, IndexIVFFlat:
		return true
	default:
		return false
	}
}

// marshalMetadata returns a JSONB-friendly []byte, or nil for empty input.
func marshalMetadata(m map[string]any) (any, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func unmarshalMetadata(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// indexName derives a stable ANN-index name from the table + kind.
func indexName(table, kind string) string {
	// Strip an optional schema prefix for the index name.
	if i := strings.Index(table, "."); i >= 0 {
		table = table[i+1:]
	}
	return fmt.Sprintf("%s_embedding_%s_idx", table, kind)
}

// quoteQualified quotes a possibly schema-qualified identifier
// (schema.table -> "schema"."table"). Both halves are quoted independently.
func quoteQualified(name string) string {
	if i := strings.Index(name, "."); i >= 0 {
		return quoteIdent(name[:i]) + "." + quoteIdent(name[i+1:])
	}
	return quoteIdent(name)
}

// quoteIdent wraps a single Postgres identifier in double quotes and
// escapes any embedded quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
