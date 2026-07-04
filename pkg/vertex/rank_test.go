package vertex

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Fake ranker
// ---------------------------------------------------------------------------

func TestFakeRankerDescendingScores(t *testing.T) {
	r := NewFakeRanker()
	docs := []string{"a", "b", "c", "d"}
	got, err := r.Rerank(context.Background(), "q", docs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("Rerank() returned %d results, want 4", len(got))
	}
	for i, rd := range got {
		wantScore := 1.0 - 0.1*float64(i)
		if rd.Index != i {
			t.Errorf("result[%d].Index = %d, want %d", i, rd.Index, i)
		}
		if rd.Score != wantScore {
			t.Errorf("result[%d].Score = %f, want %f", i, rd.Score, wantScore)
		}
	}
}

func TestFakeRankerTopK(t *testing.T) {
	r := NewFakeRanker()
	docs := []string{"a", "b", "c", "d", "e"}
	got, err := r.Rerank(context.Background(), "q", docs, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("Rerank() returned %d results, want 2", len(got))
	}
}

func TestFakeRankerClampsScoreToZero(t *testing.T) {
	r := NewFakeRanker()
	docs := make([]string, 15) // indices 10+ should have score 0
	for i := range docs {
		docs[i] = "doc"
	}
	got, err := r.Rerank(context.Background(), "q", docs, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 10; i < len(got); i++ {
		if got[i].Score != 0 {
			t.Errorf("result[%d].Score = %f, want 0 (clamped)", i, got[i].Score)
		}
	}
}

func TestFakeRankerEmpty(t *testing.T) {
	r := NewFakeRanker()
	got, err := r.Rerank(context.Background(), "q", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("Rerank(nil) = %d results, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// Vertex ranker (httptest)
// ---------------------------------------------------------------------------

// testVertexRanker returns a vertexRanker aimed at srv with negligible backoff.
func testVertexRanker(srv *httptest.Server) *vertexRanker {
	return &vertexRanker{
		endpoint: srv.URL,
		client:   srv.Client(),
		backoff:  time.Millisecond,
	}
}

// cannedRankOK is a minimal successful :rank response.
const cannedRankOK = `{"records":[
	{"id":"1","score":0.95,"content":"doc b"},
	{"id":"0","score":0.80,"content":"doc a"}
]}`

func TestVertexRankerMapsResponse(t *testing.T) {
	var gotReq rankRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cannedRankOK))
	}))
	defer srv.Close()

	r := testVertexRanker(srv)
	got, err := r.Rerank(context.Background(), "my query", []string{"doc a", "doc b"}, 2)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	// Verify request payload.
	if gotReq.Model != rankerModel {
		t.Errorf("request model = %q, want %q", gotReq.Model, rankerModel)
	}
	if gotReq.Query != "my query" {
		t.Errorf("request query = %q, want %q", gotReq.Query, "my query")
	}
	if len(gotReq.Records) != 2 {
		t.Errorf("request records = %d, want 2", len(gotReq.Records))
	}
	if gotReq.TopN != 2 {
		t.Errorf("request topN = %d, want 2", gotReq.TopN)
	}

	// Verify response is sorted by score desc.
	if len(got) != 2 {
		t.Fatalf("Rerank() returned %d results, want 2", len(got))
	}
	if got[0].Index != 1 || got[0].Score != 0.95 {
		t.Errorf("result[0] = %+v, want {Index:1 Score:0.95}", got[0])
	}
	if got[1].Index != 0 || got[1].Score != 0.80 {
		t.Errorf("result[1] = %+v, want {Index:0 Score:0.80}", got[1])
	}
}

func TestVertexRankerEmptyDocs(t *testing.T) {
	r := testVertexRanker(httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unexpected call with empty docs")
	})))
	got, err := r.Rerank(context.Background(), "q", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("Rerank(nil) = %d results, want 0", len(got))
	}
}

func TestVertexRankerRetriesThrottleAndServerErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 3:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(cannedRankOK))
		}
	}))
	defer srv.Close()

	got, err := testVertexRanker(srv).Rerank(context.Background(), "q", []string{"a", "b"}, 2)
	if err != nil {
		t.Fatalf("Rerank() error = %v, want success after retries", err)
	}
	if calls != 4 {
		t.Errorf("server calls = %d, want 4 (initial + 3 retries)", calls)
	}
	if len(got) != 2 {
		t.Errorf("Rerank() = %d results, want 2", len(got))
	}
}

func TestVertexRankerGivesUpAfterRetryBudget(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := testVertexRanker(srv).Rerank(context.Background(), "q", []string{"a"}, 1)
	if err == nil {
		t.Fatal("Rerank() error = nil, want error after exhausted retries")
	}
	if calls != 4 {
		t.Errorf("server calls = %d, want 4 (initial + 3 retries)", calls)
	}
}

func TestVertexRankerDoesNotRetryClientErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := testVertexRanker(srv).Rerank(context.Background(), "q", []string{"a"}, 1)
	if err == nil {
		t.Fatal("Rerank() error = nil, want error on 400")
	}
	if calls != 1 {
		t.Errorf("server calls = %d, want 1 (4xx is not retryable)", calls)
	}
}

func TestNewVertexRankerRejectsEmptyArgs(t *testing.T) {
	tests := []struct {
		name            string
		project, region string
	}{
		{name: "empty project", region: "us-central1"},
		{name: "empty region", project: "proj"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewVertexRanker(context.Background(), tt.project, tt.region,
				WithRankerHTTPClient(http.DefaultClient))
			if err == nil {
				t.Error("NewVertexRanker() error = nil, want error")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Live test — gated on GCP_PROJECT
// ---------------------------------------------------------------------------

func TestVertexRankerLive(t *testing.T) {
	project := os.Getenv("GCP_PROJECT")
	if project == "" {
		t.Skip("GCP_PROJECT not set; skipping live Ranking API test")
	}
	region := cmp.Or(os.Getenv("GCP_REGION"), "us-central1")

	r, err := NewVertexRanker(context.Background(), project, region)
	if err != nil {
		t.Fatalf("NewVertexRanker() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	docs := []string{
		"The capital of France is Paris.",
		"Go is a statically typed language.",
		"French cuisine is world-renowned.",
	}
	got, err := r.Rerank(ctx, "What is the capital of France?", docs, 2)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Rerank() returned %d results, want 2", len(got))
	}
	// The top result should be the Paris doc (index 0).
	if got[0].Index != 0 {
		t.Errorf("top result index = %d, want 0 (the Paris doc)", got[0].Index)
	}
}
