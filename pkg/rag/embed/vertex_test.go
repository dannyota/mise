package embed

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// testVertex returns a Vertex aimed at srv with a negligible backoff, for
// tests that don't go through NewVertex's Application Default Credentials
// path.
func testVertex(srv *httptest.Server) *Vertex {
	return &Vertex{
		endpoint:  srv.URL,
		client:    srv.Client(),
		batchSize: defaultBatchSize,
		backoff:   time.Millisecond,
	}
}

// cannedPredictions builds a :predict response body with n predictions of
// dims-length embedding vectors.
func cannedPredictions(t *testing.T, n, dims int) []byte {
	t.Helper()
	resp := predictResponse{Predictions: make([]predictPrediction, n)}
	for i := range resp.Predictions {
		resp.Predictions[i] = predictPrediction{Embeddings: predictEmbeddings{Values: make([]float32, dims)}}
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshaling canned predictions: %v", err)
	}
	return body
}

func TestNewVertexRejectsEmptyArgs(t *testing.T) {
	tests := []struct {
		name, project, region string
	}{
		{name: "empty project", region: "asia-southeast1"},
		{name: "empty region", project: "proj"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewVertex(context.Background(), tt.project, tt.region, WithHTTPClient(&http.Client{})); err == nil {
				t.Error("NewVertex() error = nil, want error")
			}
		})
	}
}

func TestNewVertexDefaultsAndEndpoint(t *testing.T) {
	v, err := NewVertex(context.Background(), "proj", "asia-southeast1", WithHTTPClient(&http.Client{}))
	if err != nil {
		t.Fatalf("NewVertex() error = %v", err)
	}
	wantEndpoint := "https://asia-southeast1-aiplatform.googleapis.com/v1/projects/proj/locations/asia-southeast1/" +
		"publishers/google/models/gemini-embedding-001:predict"
	if v.endpoint != wantEndpoint {
		t.Errorf("endpoint = %q, want %q", v.endpoint, wantEndpoint)
	}
	if v.batchSize != defaultBatchSize {
		t.Errorf("batchSize = %d, want default %d", v.batchSize, defaultBatchSize)
	}
}

func TestNewVertexAppliesOptions(t *testing.T) {
	hc := &http.Client{}
	v, err := NewVertex(context.Background(), "proj", "asia-southeast1", WithBatchSize(9), WithHTTPClient(hc))
	if err != nil {
		t.Fatalf("NewVertex() error = %v", err)
	}
	if v.batchSize != 9 {
		t.Errorf("batchSize = %d, want 9", v.batchSize)
	}
	if v.client != hc {
		t.Error("client was not overridden by WithHTTPClient")
	}
}

func TestNewVertexIgnoresNonPositiveBatchSize(t *testing.T) {
	v, err := NewVertex(context.Background(), "proj", "asia-southeast1", WithBatchSize(0), WithHTTPClient(&http.Client{}))
	if err != nil {
		t.Fatalf("NewVertex() error = %v", err)
	}
	if v.batchSize != defaultBatchSize {
		t.Errorf("batchSize = %d, want default %d after WithBatchSize(0)", v.batchSize, defaultBatchSize)
	}
}

func TestVertexModelAndDims(t *testing.T) {
	v := testVertex(httptest.NewServer(nil))
	if v.Model() != "gemini-embedding-001" {
		t.Errorf("Model() = %q, want gemini-embedding-001", v.Model())
	}
	if v.Dims() != 1536 {
		t.Errorf("Dims() = %d, want 1536", v.Dims())
	}
}

// TestVertexEmbedMapsRequestAndResponse checks the request body matches the
// locked predict schema (content, task_type, outputDimensionality,
// autoTruncate) and the response predictions[].embeddings.values map back
// into the returned vectors in order.
func TestVertexEmbedMapsRequestAndResponse(t *testing.T) {
	var gotReq predictRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := predictResponse{Predictions: []predictPrediction{
			{Embeddings: predictEmbeddings{Values: fill(1536, 0.5)}},
		}}
		body, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshaling response: %v", err)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	vecs, err := testVertex(srv).Embed(context.Background(), []string{"vốn điều lệ"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if len(gotReq.Instances) != 1 || gotReq.Instances[0].Content != "vốn điều lệ" {
		t.Errorf("request instances = %+v, want one instance with content %q", gotReq.Instances, "vốn điều lệ")
	}
	if gotReq.Instances[0].TaskType != "RETRIEVAL_DOCUMENT" {
		t.Errorf("request task_type = %q, want RETRIEVAL_DOCUMENT", gotReq.Instances[0].TaskType)
	}
	if gotReq.Parameters.OutputDimensionality != 1536 {
		t.Errorf("parameters.outputDimensionality = %d, want 1536", gotReq.Parameters.OutputDimensionality)
	}
	if !gotReq.Parameters.AutoTruncate {
		t.Error("parameters.autoTruncate = false, want true")
	}
	if len(vecs) != 1 {
		t.Fatalf("Embed() returned %d vectors, want 1", len(vecs))
	}
	if len(vecs[0]) != 1536 {
		t.Fatalf("Embed() vector has %d dims, want 1536", len(vecs[0]))
	}
	if vecs[0][0] != 0.5 {
		t.Errorf("Embed() vector[0] = %f, want 0.5", vecs[0][0])
	}
}

func TestVertexEmbedQueriesUsesRetrievalQueryTaskType(t *testing.T) {
	var gotTaskTypes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req predictRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		for _, inst := range req.Instances {
			gotTaskTypes = append(gotTaskTypes, inst.TaskType)
		}
		_, _ = w.Write(cannedPredictions(t, len(req.Instances), 1536))
	}))
	defer srv.Close()

	v := testVertex(srv)
	ctx := context.Background()
	if _, err := v.Embed(ctx, []string{"doc text"}); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if _, err := v.EmbedQueries(ctx, []string{"query text"}); err != nil {
		t.Fatalf("EmbedQueries() error = %v", err)
	}

	want := []string{"RETRIEVAL_DOCUMENT", "RETRIEVAL_QUERY"}
	if len(gotTaskTypes) != len(want) || gotTaskTypes[0] != want[0] || gotTaskTypes[1] != want[1] {
		t.Errorf("task types = %v, want %v", gotTaskTypes, want)
	}
}

// TestVertexEmbedFactUsesFactVerificationTaskType asserts EmbedFact sends
// task_type FACT_VERIFICATION (not RETRIEVAL_DOCUMENT or RETRIEVAL_QUERY).
func TestVertexEmbedFactUsesFactVerificationTaskType(t *testing.T) {
	var gotTaskTypes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req predictRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		for _, inst := range req.Instances {
			gotTaskTypes = append(gotTaskTypes, inst.TaskType)
		}
		_, _ = w.Write(cannedPredictions(t, len(req.Instances), 1536))
	}))
	defer srv.Close()

	v := testVertex(srv)
	ctx := context.Background()

	// First call Embed to confirm baseline task type.
	if _, err := v.Embed(ctx, []string{"doc text"}); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	// Then call EmbedFact.
	if _, err := v.EmbedFact(ctx, []string{"candidate pair"}); err != nil {
		t.Fatalf("EmbedFact() error = %v", err)
	}

	want := []string{"RETRIEVAL_DOCUMENT", "FACT_VERIFICATION"}
	if len(gotTaskTypes) != len(want) || gotTaskTypes[0] != want[0] || gotTaskTypes[1] != want[1] {
		t.Errorf("task types = %v, want %v", gotTaskTypes, want)
	}
}

// TestVertexBatchesRequests asserts client-side chunking splits texts into
// batchSize-sized :predict calls and reassembles the vectors in order.
func TestVertexBatchesRequests(t *testing.T) {
	var calls int
	var gotSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req predictRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		gotSizes = append(gotSizes, len(req.Instances))
		_, _ = w.Write(cannedPredictions(t, len(req.Instances), 1536))
	}))
	defer srv.Close()

	v := testVertex(srv)
	v.batchSize = 2
	texts := []string{"a", "b", "c", "d", "e"}
	vecs, err := v.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (batches of 2,2,1)", calls)
	}
	if len(gotSizes) != 3 || gotSizes[0] != 2 || gotSizes[1] != 2 || gotSizes[2] != 1 {
		t.Errorf("batch sizes = %v, want [2 2 1]", gotSizes)
	}
	if len(vecs) != len(texts) {
		t.Errorf("vecs = %d, want %d", len(vecs), len(texts))
	}
}

// TestVertexRejectsWrongDimensions asserts every returned vector must be
// exactly 1536-d or the call fails closed, and the error names the expected
// dimensionality.
func TestVertexRejectsWrongDimensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(cannedPredictions(t, 1, 42))
	}))
	defer srv.Close()

	_, err := testVertex(srv).Embed(context.Background(), []string{"vốn điều lệ"})
	if err == nil {
		t.Fatal("Embed() error = nil, want error for wrong-dimension embedding")
	}
	if !strings.Contains(err.Error(), "1536") {
		t.Errorf("error = %q, want it to mention 1536", err.Error())
	}
}

func TestVertexRetriesThrottleAndServerErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 3:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write(cannedPredictions(t, 1, 1536))
		}
	}))
	defer srv.Close()

	vecs, err := testVertex(srv).Embed(context.Background(), []string{"text"})
	if err != nil {
		t.Fatalf("Embed() error = %v, want success after retries", err)
	}
	if calls != 4 {
		t.Errorf("calls = %d, want 4 (initial + 3 retries)", calls)
	}
	if len(vecs) != 1 {
		t.Errorf("vecs = %d, want 1", len(vecs))
	}
}

func TestVertexGivesUpAfterRetryBudget(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := testVertex(srv).Embed(context.Background(), []string{"text"})
	if err == nil {
		t.Fatal("Embed() error = nil, want error after exhausted retries")
	}
	if calls != 4 {
		t.Errorf("calls = %d, want 4 (initial + 3 retries)", calls)
	}
}

func TestVertexDoesNotRetryClientErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := testVertex(srv).Embed(context.Background(), []string{"text"})
	if err == nil {
		t.Fatal("Embed() error = nil, want error on 400")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (4xx is not retryable)", calls)
	}
}

func TestVertexEmbedEmptyTexts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called for zero texts")
	}))
	defer srv.Close()

	vecs, err := testVertex(srv).Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vecs) != 0 {
		t.Errorf("vecs = %d, want 0", len(vecs))
	}
}

// hostRewriteTransport redirects every request to base's scheme+host,
// preserving path/query — letting a client built for a fixed googleapis.com
// endpoint reach a local httptest.Server.
type hostRewriteTransport struct {
	base *url.URL
}

func (t *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.base.Scheme
	req.URL.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

// TestNewVertexEndToEndViaHTTPClient exercises the full public path —
// NewVertex + WithHTTPClient — with no ADC available, confirming
// WithHTTPClient is sufficient to route real Embed() calls to a fake server.
func TestNewVertexEndToEndViaHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(cannedPredictions(t, 1, 1536))
	}))
	defer srv.Close()

	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parsing test server URL: %v", err)
	}
	client := &http.Client{Transport: &hostRewriteTransport{base: base}}

	v, err := NewVertex(context.Background(), "proj", "asia-southeast1", WithHTTPClient(client))
	if err != nil {
		t.Fatalf("NewVertex() error = %v", err)
	}
	vecs, err := v.Embed(context.Background(), []string{"vốn điều lệ"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 1536 {
		t.Fatalf("Embed() vecs = %d (dims %d), want 1 vector of 1536 dims", len(vecs), len(vecs[0]))
	}
}

// TestVertexLive hits the real Vertex AI gemini-embedding-001 endpoint; it
// is gated on GCP_PROJECT so CI and offline runs skip it.
func TestVertexLive(t *testing.T) {
	project := os.Getenv("GCP_PROJECT")
	if project == "" {
		t.Skip("GCP_PROJECT not set; skipping live Vertex embed test")
	}
	region := cmp.Or(os.Getenv("GCP_REGION"), "asia-southeast1")

	v, err := NewVertex(context.Background(), project, region)
	if err != nil {
		t.Fatalf("NewVertex() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	vecs, err := v.Embed(ctx, []string{"vốn điều lệ ngân hàng", "kiểm toán nội bộ"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("Embed() returned %d vectors, want 2", len(vecs))
	}
	for i, v := range vecs {
		if len(v) != 1536 {
			t.Errorf("vecs[%d] has %d dims, want 1536", i, len(v))
		}
	}

	qvecs, err := v.EmbedQueries(ctx, []string{"vốn điều lệ ngân hàng"})
	if err != nil {
		t.Fatalf("EmbedQueries() error = %v", err)
	}
	if len(qvecs) != 1 || len(qvecs[0]) != 1536 {
		t.Fatalf("EmbedQueries() returned %d vectors of dims %d, want 1 of 1536", len(qvecs), len(qvecs[0]))
	}
}

// fill returns a length-n slice with every element set to v.
func fill(n int, v float32) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = v
	}
	return out
}
