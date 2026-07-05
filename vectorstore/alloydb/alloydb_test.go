package alloydb

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"oss.nandlabs.io/golly/vectorstore"
)

// newTestStore returns an alloydb Store wired to a sqlmock DB, with the
// bootstrap DDL expectations already satisfied.
func newTestStore(t *testing.T, opts Options) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	// Expected DDL from ensureSchema, unless SkipDDL is set.
	if !opts.SkipDDL {
		mock.ExpectExec(`CREATE EXTENSION IF NOT EXISTS vector`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS ".*" \(id TEXT PRIMARY KEY`).WillReturnResult(sqlmock.NewResult(0, 0))
		if opts.IndexKind != "" && opts.IndexKind != IndexNone {
			mock.ExpectExec(`CREATE INDEX IF NOT EXISTS`).WillReturnResult(sqlmock.NewResult(0, 0))
		}
	}
	opts.DB = db
	if opts.Dimension == 0 {
		opts.Dimension = 3
	}
	s, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, mock, db
}

func TestNew_DDL_Cosine_HNSW(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`CREATE EXTENSION IF NOT EXISTS vector`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS "docs" \(id TEXT PRIMARY KEY, embedding vector\(4\) NOT NULL, metadata JSONB\)`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CREATE INDEX IF NOT EXISTS "docs_embedding_hnsw_idx" ON "docs" USING hnsw \(embedding vector_cosine_ops\)`).WillReturnResult(sqlmock.NewResult(0, 0))

	if _, err := New(Options{
		DB:        db,
		Table:     "docs",
		Dimension: 4,
		Metric:    MetricCosine,
		IndexKind: IndexHNSW,
	}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestUpsert_RoundTrip(t *testing.T) {
	s, mock, db := newTestStore(t, Options{Table: "docs", Dimension: 3, SkipDDL: true})
	defer db.Close()

	// Two rows -> multi-row VALUES + ON CONFLICT ... DO UPDATE.
	mock.ExpectExec(`INSERT INTO "docs" \(id, embedding, metadata\) VALUES \(\$1, \$2, \$3\), \(\$4, \$5, \$6\) ON CONFLICT \(id\) DO UPDATE SET embedding = EXCLUDED\.embedding, metadata = EXCLUDED\.metadata`).
		WithArgs(
			"a", sqlmock.AnyArg(), sqlmock.AnyArg(),
			"b", sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 2))

	err := s.Upsert(context.Background(),
		vectorstore.Doc{ID: "a", Vector: vectorstore.Vector{1, 2, 3}, Metadata: map[string]any{"source": "wiki"}},
		vectorstore.Doc{ID: "b", Vector: vectorstore.Vector{4, 5, 6}},
	)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestUpsert_RejectsDimMismatch(t *testing.T) {
	s, _, db := newTestStore(t, Options{Dimension: 3, SkipDDL: true})
	defer db.Close()

	err := s.Upsert(context.Background(),
		vectorstore.Doc{ID: "a", Vector: vectorstore.Vector{1, 2}},
	)
	if !errors.Is(err, vectorstore.ErrDimMismatch) {
		t.Fatalf("want ErrDimMismatch, got %v", err)
	}
}

func TestUpsert_RejectsInvalidDoc(t *testing.T) {
	s, _, db := newTestStore(t, Options{Dimension: 3, SkipDDL: true})
	defer db.Close()

	err := s.Upsert(context.Background(),
		vectorstore.Doc{ID: "", Vector: vectorstore.Vector{1, 2, 3}},
	)
	if !errors.Is(err, vectorstore.ErrEmptyID) {
		t.Fatalf("want ErrEmptyID, got %v", err)
	}
}

func TestSearch_ReturnsHits(t *testing.T) {
	s, mock, db := newTestStore(t, Options{Table: "docs", Dimension: 3, SkipDDL: true})
	defer db.Close()

	// Cosine uses <=>; no filter WHERE clause.
	pattern := regexp.QuoteMeta(`SELECT id, metadata, embedding <=> $1 AS distance FROM "docs" ORDER BY embedding <=> $1 LIMIT $2`)
	rows := sqlmock.NewRows([]string{"id", "metadata", "distance"}).
		AddRow("a", []byte(`{"source":"wiki"}`), 0.12).
		AddRow("b", []byte(`{"source":"blog"}`), 0.34)
	mock.ExpectQuery(pattern).WithArgs(sqlmock.AnyArg(), 5).WillReturnRows(rows)

	hits, err := s.Search(context.Background(), vectorstore.Query{
		Vector: vectorstore.Vector{0.1, 0.2, 0.3},
		TopK:   5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 || hits[0].ID != "a" || hits[1].ID != "b" {
		t.Fatalf("hits = %+v", hits)
	}
	if hits[0].Metadata["source"] != "wiki" {
		t.Errorf("metadata not decoded: %+v", hits[0].Metadata)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestSearch_UsesConfiguredOperator(t *testing.T) {
	cases := []struct {
		metric Metric
		op     string
	}{
		{MetricCosine, "<=>"},
		{MetricL2, "<->"},
		{MetricIP, "<#>"},
	}
	for _, tc := range cases {
		t.Run(string(tc.metric), func(t *testing.T) {
			s, mock, db := newTestStore(t, Options{Dimension: 2, SkipDDL: true, Metric: tc.metric})
			defer db.Close()

			mock.ExpectQuery(regexp.QuoteMeta(tc.op)).WithArgs(sqlmock.AnyArg(), 10).WillReturnRows(sqlmock.NewRows([]string{"id", "metadata", "distance"}))
			if _, err := s.Search(context.Background(), vectorstore.Query{Vector: vectorstore.Vector{1, 2}}); err != nil {
				t.Fatalf("Search: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("expectations: %v", err)
			}
		})
	}
}

func TestSearch_MetadataFilter(t *testing.T) {
	s, mock, db := newTestStore(t, Options{Table: "docs", Dimension: 3, SkipDDL: true})
	defer db.Close()

	// Filter path adds `WHERE metadata @> $2` and shifts the LIMIT placeholder.
	pattern := regexp.QuoteMeta(`SELECT id, metadata, embedding <=> $1 AS distance FROM "docs" WHERE metadata @> $2 ORDER BY embedding <=> $1 LIMIT $3`)
	mock.ExpectQuery(pattern).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 3).
		WillReturnRows(sqlmock.NewRows([]string{"id", "metadata", "distance"}).AddRow("a", []byte(`{"source":"wiki"}`), 0.1))

	hits, err := s.Search(context.Background(), vectorstore.Query{
		Vector: vectorstore.Vector{0.1, 0.2, 0.3},
		TopK:   3,
		Filter: map[string]any{"source": "wiki"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("hits = %+v", hits)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestSearch_EmptyVector(t *testing.T) {
	s, _, db := newTestStore(t, Options{Dimension: 3, SkipDDL: true})
	defer db.Close()

	_, err := s.Search(context.Background(), vectorstore.Query{Vector: nil, TopK: 5})
	if !errors.Is(err, vectorstore.ErrEmptyVector) {
		t.Fatalf("want ErrEmptyVector, got %v", err)
	}
}

func TestSearch_DimMismatch(t *testing.T) {
	s, _, db := newTestStore(t, Options{Dimension: 3, SkipDDL: true})
	defer db.Close()

	_, err := s.Search(context.Background(), vectorstore.Query{Vector: vectorstore.Vector{1, 2}})
	if !errors.Is(err, vectorstore.ErrDimMismatch) {
		t.Fatalf("want ErrDimMismatch, got %v", err)
	}
}

func TestDelete_RemovesById(t *testing.T) {
	s, mock, db := newTestStore(t, Options{Table: "docs", Dimension: 3, SkipDDL: true})
	defer db.Close()

	pattern := regexp.QuoteMeta(`DELETE FROM "docs" WHERE id = ANY($1)`)
	mock.ExpectExec(pattern).
		WithArgs(sqlmock.AnyArg()). // driver receives the []string slice
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := s.Delete(context.Background(), "a", "b"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestDelete_EmptyNoop(t *testing.T) {
	s, mock, db := newTestStore(t, Options{Dimension: 3, SkipDDL: true})
	defer db.Close()

	if err := s.Delete(context.Background()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// No expectation registered — sqlmock will complain if any DB call fired.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	s, _, db := newTestStore(t, Options{Dimension: 3, SkipDDL: true})
	defer db.Close()

	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	// DB must remain usable — Store must not have closed it.
	if err := db.Ping(); err != nil {
		t.Errorf("Store.Close closed the caller-owned DB: %v", err)
	}
	// Operations after Close should fail fast, no DB traffic.
	if err := s.Upsert(context.Background(), vectorstore.Doc{ID: "x", Vector: vectorstore.Vector{1, 2, 3}}); err == nil {
		t.Error("Upsert after Close should fail")
	}
	if _, err := s.Search(context.Background(), vectorstore.Query{Vector: vectorstore.Vector{1, 2, 3}}); err == nil {
		t.Error("Search after Close should fail")
	}
	if err := s.Delete(context.Background(), "x"); err == nil {
		t.Error("Delete after Close should fail")
	}
}

func TestNew_ValidatesOptions(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	if _, err := New(Options{DB: nil, Dimension: 3, SkipDDL: true}); err == nil {
		t.Error("want error for nil DB")
	}
	if _, err := New(Options{DB: db, Dimension: 0, SkipDDL: true}); err == nil {
		t.Error("want error for zero dimension")
	}
	if _, err := New(Options{DB: db, Dimension: 3, Metric: "unknown", SkipDDL: true}); err == nil {
		t.Error("want error for unknown metric")
	}
	if _, err := New(Options{DB: db, Dimension: 3, IndexKind: "bogus", SkipDDL: true}); err == nil {
		t.Error("want error for unknown index kind")
	}
}

func TestQuoteQualified(t *testing.T) {
	cases := map[string]string{
		"vectors":         `"vectors"`,
		"public.vectors":  `"public"."vectors"`,
		`weird"name`:      `"weird""name"`,
		"schema.tab.name": `"schema"."tab.name"`, // only the first dot splits; rest stays.
	}
	for in, want := range cases {
		if got := quoteQualified(in); got != want {
			t.Errorf("quoteQualified(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIndexName(t *testing.T) {
	// schema is stripped from the derived index name to avoid an
	// unquotable dot inside the identifier.
	if got := indexName("public.docs", "hnsw"); strings.Contains(got, ".") {
		t.Errorf("indexName should strip schema: got %q", got)
	}
}
