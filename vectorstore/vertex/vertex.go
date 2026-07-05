// Package vertex is a Vertex AI Vector Search (formerly Matching Engine)
// backend for oss.nandlabs.io/golly/vectorstore.
//
// It wraps two Vertex AI clients from
// cloud.google.com/go/aiplatform/apiv1beta1:
//
//   - IndexClient — Upsert / Remove datapoints against an Index resource.
//   - MatchClient — FindNeighbors against a DeployedIndex under an
//     IndexEndpoint (public or private).
//
// Target scale profile: managed ANN search over 10^6+ vectors with
// serving-side sharding, brute-force or tree-AH indexes, and per-query
// filtering via string / numeric restrictions.
//
// The Store is safe for concurrent use.
package vertex

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	aiplatform "cloud.google.com/go/aiplatform/apiv1beta1"
	"cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"

	"oss.nandlabs.io/golly/vectorstore"
)

// defaultUpsertBatchSize is the maximum number of datapoints sent in a single
// UpsertDatapoints RPC. Vertex accepts larger batches, but keeping requests
// modest bounds request size and simplifies retry semantics.
const defaultUpsertBatchSize = 1000

// indexAPI is the minimal surface of aiplatform.IndexClient the Store needs.
// Extracted as an interface so tests can inject fakes.
type indexAPI interface {
	UpsertDatapoints(ctx context.Context, req *aiplatformpb.UpsertDatapointsRequest, opts ...gax.CallOption) (*aiplatformpb.UpsertDatapointsResponse, error)
	RemoveDatapoints(ctx context.Context, req *aiplatformpb.RemoveDatapointsRequest, opts ...gax.CallOption) (*aiplatformpb.RemoveDatapointsResponse, error)
	Close() error
}

// matchAPI is the minimal surface of aiplatform.MatchClient the Store needs.
type matchAPI interface {
	FindNeighbors(ctx context.Context, req *aiplatformpb.FindNeighborsRequest, opts ...gax.CallOption) (*aiplatformpb.FindNeighborsResponse, error)
	Close() error
}

// Options configures a Vertex AI Vector Search Store.
type Options struct {
	// Project is the GCP project number or id (required).
	Project string
	// Location is the Vertex AI region, e.g. "us-central1" (required).
	Location string
	// IndexEndpoint is the endpoint resource id used for match calls. It
	// may be the short id ("1234567890") or a full resource name
	// ("projects/.../locations/.../indexEndpoints/...").
	IndexEndpoint string
	// DeployedIndex is the deployed-index id under IndexEndpoint that
	// FindNeighbors will route to.
	DeployedIndex string
	// Index is the Index resource id used for upsert / remove calls. It
	// may be the short id or a full resource name.
	Index string
	// UpsertBatchSize overrides the default batch size for UpsertDatapoints.
	// Zero uses the package default.
	UpsertBatchSize int
	// ClientOpts are passed through to both underlying aiplatform clients.
	ClientOpts []option.ClientOption
}

// Store implements vectorstore.Store on top of Vertex AI Vector Search.
type Store struct {
	opts   Options
	index  indexAPI
	match  matchAPI
	closed atomic.Bool
	// closeMu serialises Close so double-close returns nil.
	closeMu sync.Mutex
}

// New constructs a Store, opening one IndexClient and one MatchClient.
func New(ctx context.Context, opts Options) (*Store, error) {
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	ic, err := aiplatform.NewIndexClient(ctx, opts.ClientOpts...)
	if err != nil {
		return nil, fmt.Errorf("vectorstore/vertex: new IndexClient: %w", err)
	}
	mc, err := aiplatform.NewMatchClient(ctx, opts.ClientOpts...)
	if err != nil {
		_ = ic.Close()
		return nil, fmt.Errorf("vectorstore/vertex: new MatchClient: %w", err)
	}
	return newWithClients(opts, ic, mc), nil
}

// newWithClients is the internal constructor used by New and by tests.
func newWithClients(opts Options, ic indexAPI, mc matchAPI) *Store {
	if opts.UpsertBatchSize <= 0 {
		opts.UpsertBatchSize = defaultUpsertBatchSize
	}
	return &Store{opts: opts, index: ic, match: mc}
}

func validateOptions(o Options) error {
	if o.Project == "" {
		return errors.New("vectorstore/vertex: Options.Project is required")
	}
	if o.Location == "" {
		return errors.New("vectorstore/vertex: Options.Location is required")
	}
	if o.IndexEndpoint == "" {
		return errors.New("vectorstore/vertex: Options.IndexEndpoint is required")
	}
	if o.DeployedIndex == "" {
		return errors.New("vectorstore/vertex: Options.DeployedIndex is required")
	}
	if o.Index == "" {
		return errors.New("vectorstore/vertex: Options.Index is required")
	}
	return nil
}

// Upsert insert-or-replaces datapoints in the configured Index. Batches larger
// than the configured limit are split into multiple RPCs; a failure on any
// batch aborts the sequence.
func (s *Store) Upsert(ctx context.Context, docs ...vectorstore.Doc) error {
	if s.closed.Load() {
		return errClosed
	}
	if len(docs) == 0 {
		return nil
	}
	dps := make([]*aiplatformpb.IndexDatapoint, 0, len(docs))
	for _, d := range docs {
		if err := vectorstore.ValidateDoc(d); err != nil {
			return err
		}
		dp := &aiplatformpb.IndexDatapoint{
			DatapointId:   d.ID,
			FeatureVector: []float32(d.Vector),
		}
		attachMetadataAsRestricts(dp, d.Metadata)
		dps = append(dps, dp)
	}
	batch := s.opts.UpsertBatchSize
	indexName := indexResourceName(s.opts.Project, s.opts.Location, s.opts.Index)
	for start := 0; start < len(dps); start += batch {
		end := start + batch
		if end > len(dps) {
			end = len(dps)
		}
		req := &aiplatformpb.UpsertDatapointsRequest{
			Index:      indexName,
			Datapoints: dps[start:end],
		}
		if _, err := s.index.UpsertDatapoints(ctx, req); err != nil {
			return fmt.Errorf("vectorstore/vertex: UpsertDatapoints [%d:%d]: %w", start, end, err)
		}
	}
	return nil
}

// Search runs an ANN query via MatchService.FindNeighbors and maps the
// response to []vectorstore.Hit. Distance is returned by Vertex; we surface
// it as Score unchanged (callers can invert if they need similarity).
func (s *Store) Search(ctx context.Context, q vectorstore.Query) ([]vectorstore.Hit, error) {
	if s.closed.Load() {
		return nil, errClosed
	}
	if len(q.Vector) == 0 {
		return nil, vectorstore.ErrEmptyVector
	}
	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}
	qdp := &aiplatformpb.IndexDatapoint{FeatureVector: []float32(q.Vector)}
	attachMetadataAsRestricts(qdp, q.Filter)
	req := &aiplatformpb.FindNeighborsRequest{
		IndexEndpoint:   endpointResourceName(s.opts.Project, s.opts.Location, s.opts.IndexEndpoint),
		DeployedIndexId: s.opts.DeployedIndex,
		Queries: []*aiplatformpb.FindNeighborsRequest_Query{
			{Datapoint: qdp, NeighborCount: int32(topK)},
		},
		ReturnFullDatapoint: false,
	}
	resp, err := s.match.FindNeighbors(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vectorstore/vertex: FindNeighbors: %w", err)
	}
	hits := make([]vectorstore.Hit, 0, topK)
	for _, nn := range resp.GetNearestNeighbors() {
		for _, n := range nn.GetNeighbors() {
			dp := n.GetDatapoint()
			if dp == nil {
				continue
			}
			score := float32(n.GetDistance())
			if q.MinScore != 0 && score < q.MinScore {
				continue
			}
			hits = append(hits, vectorstore.Hit{
				ID:    dp.GetDatapointId(),
				Score: score,
			})
		}
	}
	return hits, nil
}

// Delete removes datapoints by id. Unknown ids are silently ignored by
// Vertex, matching the vectorstore.Store idempotence contract.
func (s *Store) Delete(ctx context.Context, ids ...string) error {
	if s.closed.Load() {
		return errClosed
	}
	if len(ids) == 0 {
		return nil
	}
	req := &aiplatformpb.RemoveDatapointsRequest{
		Index:        indexResourceName(s.opts.Project, s.opts.Location, s.opts.Index),
		DatapointIds: ids,
	}
	if _, err := s.index.RemoveDatapoints(ctx, req); err != nil {
		return fmt.Errorf("vectorstore/vertex: RemoveDatapoints: %w", err)
	}
	return nil
}

// Close releases both underlying aiplatform clients. Safe to call multiple
// times.
func (s *Store) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed.Swap(true) {
		return nil
	}
	var errs []error
	if err := s.index.Close(); err != nil {
		errs = append(errs, fmt.Errorf("index close: %w", err))
	}
	if err := s.match.Close(); err != nil {
		errs = append(errs, fmt.Errorf("match close: %w", err))
	}
	return errors.Join(errs...)
}

// errClosed is returned when any method is called after Close.
var errClosed = errors.New("vectorstore/vertex: store is closed")

// indexResourceName expands a possibly-short index id to the full Vertex
// resource name required by the API. If the caller already passed a full
// name it is returned untouched.
func indexResourceName(project, location, index string) string {
	if isFullResourceName(index) {
		return index
	}
	return fmt.Sprintf("projects/%s/locations/%s/indexes/%s", project, location, index)
}

// endpointResourceName is the equivalent helper for indexEndpoints.
func endpointResourceName(project, location, endpoint string) string {
	if isFullResourceName(endpoint) {
		return endpoint
	}
	return fmt.Sprintf("projects/%s/locations/%s/indexEndpoints/%s", project, location, endpoint)
}

// isFullResourceName is a cheap check: Vertex resource names always start
// with "projects/". Short ids never contain a slash.
func isFullResourceName(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

// attachMetadataAsRestricts maps a golly filter/metadata map onto Vertex
// datapoint restricts. String values become categorical Restrictions
// (namespace=key, allow_list=[value]); numeric values become
// NumericRestrictions with the EQUAL operator. Unsupported types are
// silently skipped — Vertex has no notion of arbitrary JSON filters.
func attachMetadataAsRestricts(dp *aiplatformpb.IndexDatapoint, md map[string]any) {
	if len(md) == 0 {
		return
	}
	for k, v := range md {
		switch val := v.(type) {
		case string:
			dp.Restricts = append(dp.Restricts, &aiplatformpb.IndexDatapoint_Restriction{
				Namespace: k,
				AllowList: []string{val},
			})
		case []string:
			dp.Restricts = append(dp.Restricts, &aiplatformpb.IndexDatapoint_Restriction{
				Namespace: k,
				AllowList: val,
			})
		case int:
			dp.NumericRestricts = append(dp.NumericRestricts, &aiplatformpb.IndexDatapoint_NumericRestriction{
				Namespace: k,
				Op:        aiplatformpb.IndexDatapoint_NumericRestriction_EQUAL,
				Value:     &aiplatformpb.IndexDatapoint_NumericRestriction_ValueInt{ValueInt: int64(val)},
			})
		case int64:
			dp.NumericRestricts = append(dp.NumericRestricts, &aiplatformpb.IndexDatapoint_NumericRestriction{
				Namespace: k,
				Op:        aiplatformpb.IndexDatapoint_NumericRestriction_EQUAL,
				Value:     &aiplatformpb.IndexDatapoint_NumericRestriction_ValueInt{ValueInt: val},
			})
		case float32:
			dp.NumericRestricts = append(dp.NumericRestricts, &aiplatformpb.IndexDatapoint_NumericRestriction{
				Namespace: k,
				Op:        aiplatformpb.IndexDatapoint_NumericRestriction_EQUAL,
				Value:     &aiplatformpb.IndexDatapoint_NumericRestriction_ValueFloat{ValueFloat: val},
			})
		case float64:
			dp.NumericRestricts = append(dp.NumericRestricts, &aiplatformpb.IndexDatapoint_NumericRestriction{
				Namespace: k,
				Op:        aiplatformpb.IndexDatapoint_NumericRestriction_EQUAL,
				Value:     &aiplatformpb.IndexDatapoint_NumericRestriction_ValueDouble{ValueDouble: val},
			})
		}
	}
}
