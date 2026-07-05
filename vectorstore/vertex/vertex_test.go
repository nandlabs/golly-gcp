package vertex

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/aiplatform/apiv1beta1/aiplatformpb"
	"github.com/googleapis/gax-go/v2"

	"oss.nandlabs.io/golly/vectorstore"
)

// --- fakes -------------------------------------------------------------

type fakeIndex struct {
	upserted []*aiplatformpb.UpsertDatapointsRequest
	removed  []*aiplatformpb.RemoveDatapointsRequest
	closes   int
	upErr    error
	rmErr    error
}

func (f *fakeIndex) UpsertDatapoints(_ context.Context, req *aiplatformpb.UpsertDatapointsRequest, _ ...gax.CallOption) (*aiplatformpb.UpsertDatapointsResponse, error) {
	// Deep-copy datapoint slice so batch splitting in caller can't alias us later.
	cp := &aiplatformpb.UpsertDatapointsRequest{
		Index:      req.Index,
		Datapoints: append([]*aiplatformpb.IndexDatapoint(nil), req.Datapoints...),
	}
	f.upserted = append(f.upserted, cp)
	if f.upErr != nil {
		return nil, f.upErr
	}
	return &aiplatformpb.UpsertDatapointsResponse{}, nil
}

func (f *fakeIndex) RemoveDatapoints(_ context.Context, req *aiplatformpb.RemoveDatapointsRequest, _ ...gax.CallOption) (*aiplatformpb.RemoveDatapointsResponse, error) {
	f.removed = append(f.removed, req)
	if f.rmErr != nil {
		return nil, f.rmErr
	}
	return &aiplatformpb.RemoveDatapointsResponse{}, nil
}

func (f *fakeIndex) Close() error { f.closes++; return nil }

type fakeMatch struct {
	lastReq *aiplatformpb.FindNeighborsRequest
	resp    *aiplatformpb.FindNeighborsResponse
	err     error
	closes  int
}

func (f *fakeMatch) FindNeighbors(_ context.Context, req *aiplatformpb.FindNeighborsRequest, _ ...gax.CallOption) (*aiplatformpb.FindNeighborsResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func (f *fakeMatch) Close() error { f.closes++; return nil }

func newTestStore(t *testing.T) (*Store, *fakeIndex, *fakeMatch) {
	t.Helper()
	fi := &fakeIndex{}
	fm := &fakeMatch{}
	s := newWithClients(Options{
		Project:       "test-project",
		Location:      "us-central1",
		IndexEndpoint: "1234567890",
		DeployedIndex: "deploy-1",
		Index:         "9876543210",
	}, fi, fm)
	return s, fi, fm
}

// --- tests -------------------------------------------------------------

func TestUpsert_RoundTrip(t *testing.T) {
	s, fi, _ := newTestStore(t)
	docs := []vectorstore.Doc{
		{ID: "a", Vector: vectorstore.Vector{1, 2, 3}, Metadata: map[string]any{"source": "wiki"}},
		{ID: "b", Vector: vectorstore.Vector{4, 5, 6}, Metadata: map[string]any{"rank": 42}},
	}
	if err := s.Upsert(context.Background(), docs...); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if len(fi.upserted) != 1 {
		t.Fatalf("want 1 UpsertDatapoints call, got %d", len(fi.upserted))
	}
	req := fi.upserted[0]
	if req.Index != "projects/test-project/locations/us-central1/indexes/9876543210" {
		t.Errorf("index name = %q", req.Index)
	}
	if len(req.Datapoints) != 2 {
		t.Fatalf("want 2 datapoints, got %d", len(req.Datapoints))
	}
	if req.Datapoints[0].DatapointId != "a" || len(req.Datapoints[0].FeatureVector) != 3 {
		t.Errorf("dp0 mapped wrong: %+v", req.Datapoints[0])
	}
	// String metadata -> Restricts.
	if len(req.Datapoints[0].Restricts) != 1 || req.Datapoints[0].Restricts[0].Namespace != "source" {
		t.Errorf("expected string restrict, got %+v", req.Datapoints[0].Restricts)
	}
	// int metadata -> NumericRestricts.
	if len(req.Datapoints[1].NumericRestricts) != 1 || req.Datapoints[1].NumericRestricts[0].Namespace != "rank" {
		t.Errorf("expected numeric restrict, got %+v", req.Datapoints[1].NumericRestricts)
	}
}

func TestUpsert_BatchSplitting(t *testing.T) {
	s, fi, _ := newTestStore(t)
	s.opts.UpsertBatchSize = 2
	docs := []vectorstore.Doc{
		{ID: "a", Vector: vectorstore.Vector{1}},
		{ID: "b", Vector: vectorstore.Vector{2}},
		{ID: "c", Vector: vectorstore.Vector{3}},
	}
	if err := s.Upsert(context.Background(), docs...); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if len(fi.upserted) != 2 {
		t.Fatalf("want 2 batches, got %d", len(fi.upserted))
	}
	if len(fi.upserted[0].Datapoints) != 2 || len(fi.upserted[1].Datapoints) != 1 {
		t.Errorf("batch sizes: %d, %d", len(fi.upserted[0].Datapoints), len(fi.upserted[1].Datapoints))
	}
}

func TestUpsert_RejectsInvalidDoc(t *testing.T) {
	s, _, _ := newTestStore(t)
	err := s.Upsert(context.Background(), vectorstore.Doc{ID: "", Vector: vectorstore.Vector{1}})
	if !errors.Is(err, vectorstore.ErrEmptyID) {
		t.Fatalf("want ErrEmptyID, got %v", err)
	}
}

func TestSearch_ReturnsHits(t *testing.T) {
	s, _, fm := newTestStore(t)
	fm.resp = &aiplatformpb.FindNeighborsResponse{
		NearestNeighbors: []*aiplatformpb.FindNeighborsResponse_NearestNeighbors{
			{
				Id: "q0",
				Neighbors: []*aiplatformpb.FindNeighborsResponse_Neighbor{
					{Datapoint: &aiplatformpb.IndexDatapoint{DatapointId: "a"}, Distance: 0.12},
					{Datapoint: &aiplatformpb.IndexDatapoint{DatapointId: "b"}, Distance: 0.98},
				},
			},
		},
	}
	hits, err := s.Search(context.Background(), vectorstore.Query{
		Vector: vectorstore.Vector{0.1, 0.2},
		TopK:   5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 || hits[0].ID != "a" || hits[1].ID != "b" {
		t.Fatalf("hits = %+v", hits)
	}
	if fm.lastReq.IndexEndpoint != "projects/test-project/locations/us-central1/indexEndpoints/1234567890" {
		t.Errorf("endpoint = %q", fm.lastReq.IndexEndpoint)
	}
	if fm.lastReq.DeployedIndexId != "deploy-1" {
		t.Errorf("deployed_index_id = %q", fm.lastReq.DeployedIndexId)
	}
	if fm.lastReq.Queries[0].NeighborCount != 5 {
		t.Errorf("neighbor_count = %d", fm.lastReq.Queries[0].NeighborCount)
	}
}

func TestSearch_MetadataFilter(t *testing.T) {
	s, _, fm := newTestStore(t)
	fm.resp = &aiplatformpb.FindNeighborsResponse{}
	_, err := s.Search(context.Background(), vectorstore.Query{
		Vector: vectorstore.Vector{0.1},
		TopK:   3,
		Filter: map[string]any{"source": "wiki", "rank": 42},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	dp := fm.lastReq.Queries[0].Datapoint
	if len(dp.Restricts) != 1 || dp.Restricts[0].Namespace != "source" {
		t.Errorf("string filter not attached: %+v", dp.Restricts)
	}
	if len(dp.NumericRestricts) != 1 || dp.NumericRestricts[0].Namespace != "rank" {
		t.Errorf("numeric filter not attached: %+v", dp.NumericRestricts)
	}
}

func TestSearch_EmptyVector(t *testing.T) {
	s, _, _ := newTestStore(t)
	_, err := s.Search(context.Background(), vectorstore.Query{Vector: nil, TopK: 3})
	if !errors.Is(err, vectorstore.ErrEmptyVector) {
		t.Fatalf("want ErrEmptyVector, got %v", err)
	}
}

func TestDelete_RemovesById(t *testing.T) {
	s, fi, _ := newTestStore(t)
	if err := s.Delete(context.Background(), "a", "b"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(fi.removed) != 1 {
		t.Fatalf("want 1 RemoveDatapoints call, got %d", len(fi.removed))
	}
	got := fi.removed[0]
	if got.Index != "projects/test-project/locations/us-central1/indexes/9876543210" {
		t.Errorf("index name = %q", got.Index)
	}
	if len(got.DatapointIds) != 2 || got.DatapointIds[0] != "a" || got.DatapointIds[1] != "b" {
		t.Errorf("ids = %v", got.DatapointIds)
	}
}

func TestDelete_EmptyIsNoop(t *testing.T) {
	s, fi, _ := newTestStore(t)
	if err := s.Delete(context.Background()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(fi.removed) != 0 {
		t.Errorf("expected no RPC for empty delete, got %d", len(fi.removed))
	}
}

func TestClose_Idempotent(t *testing.T) {
	s, fi, fm := newTestStore(t)
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if fi.closes != 1 || fm.closes != 1 {
		t.Errorf("expected each client closed exactly once, got index=%d match=%d", fi.closes, fm.closes)
	}
	// After Close, operations should fail fast.
	if err := s.Upsert(context.Background(), vectorstore.Doc{ID: "x", Vector: vectorstore.Vector{1}}); err == nil {
		t.Error("Upsert after Close should fail")
	}
	if _, err := s.Search(context.Background(), vectorstore.Query{Vector: vectorstore.Vector{1}}); err == nil {
		t.Error("Search after Close should fail")
	}
	if err := s.Delete(context.Background(), "x"); err == nil {
		t.Error("Delete after Close should fail")
	}
}

func TestNew_ValidatesOptions(t *testing.T) {
	cases := []struct {
		name string
		o    Options
	}{
		{"no project", Options{Location: "us", IndexEndpoint: "e", DeployedIndex: "d", Index: "i"}},
		{"no location", Options{Project: "p", IndexEndpoint: "e", DeployedIndex: "d", Index: "i"}},
		{"no endpoint", Options{Project: "p", Location: "us", DeployedIndex: "d", Index: "i"}},
		{"no deployed", Options{Project: "p", Location: "us", IndexEndpoint: "e", Index: "i"}},
		{"no index", Options{Project: "p", Location: "us", IndexEndpoint: "e", DeployedIndex: "d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(context.Background(), tc.o); err == nil {
				t.Errorf("expected validation error, got nil")
			}
		})
	}
}

func TestFullResourceNamePassthrough(t *testing.T) {
	s, fi, _ := newTestStore(t)
	s.opts.Index = "projects/other/locations/eu-west1/indexes/xyz"
	if err := s.Delete(context.Background(), "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := fi.removed[0].Index; got != s.opts.Index {
		t.Errorf("full-name index not passed through: %q", got)
	}
}
