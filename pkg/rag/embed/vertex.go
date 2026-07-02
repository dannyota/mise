package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	vertexModel = "gemini-embedding-001"
	vertexDims  = 1536

	// cloudPlatformScope is the OAuth2 scope Application Default Credentials
	// requests to call Vertex AI.
	cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

	// defaultBatchSize bounds how many texts go into one :predict call.
	defaultBatchSize = 4

	// vertexMaxAttempts bounds a single batch call: one initial try plus
	// three retries on throttling (429) and server errors (5xx).
	vertexMaxAttempts = 4

	// vertexMaxErrBody caps how much of an error response body is quoted.
	vertexMaxErrBody = 512

	taskTypeDocument = "RETRIEVAL_DOCUMENT"
	taskTypeQuery    = "RETRIEVAL_QUERY"
)

// Vertex calls the Vertex AI gemini-embedding-001 REST :predict endpoint. It
// implements Embedder (RETRIEVAL_DOCUMENT) and QueryEmbedder
// (RETRIEVAL_QUERY) — the locked embed space (DECISIONS 1, DEC 14).
type Vertex struct {
	endpoint  string        // full …/models/gemini-embedding-001:predict URL
	client    *http.Client  // OAuth2-wrapped HTTP client
	batchSize int           // texts per :predict call
	backoff   time.Duration // base retry delay, doubled per retry
}

// VertexOption configures a Vertex embedder.
type VertexOption func(*Vertex)

// WithBatchSize overrides the client-side chunk size (default 4 texts per
// :predict call). Non-positive values are ignored.
func WithBatchSize(n int) VertexOption {
	return func(v *Vertex) {
		if n > 0 {
			v.batchSize = n
		}
	}
}

// WithHTTPClient overrides the HTTP client, letting tests inject an
// httptest server without going through Application Default Credentials.
func WithHTTPClient(c *http.Client) VertexOption {
	return func(v *Vertex) { v.client = c }
}

// NewVertex returns an Embedder+QueryEmbedder backed by the Vertex AI
// gemini-embedding-001 REST :predict endpoint. Credentials come from
// Application Default Credentials unless WithHTTPClient overrides the
// client.
func NewVertex(ctx context.Context, project, region string, opts ...VertexOption) (*Vertex, error) {
	if project == "" || region == "" {
		return nil, errors.New("vertex embedder: project and region are required")
	}

	v := &Vertex{
		endpoint: fmt.Sprintf(
			"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
			region, project, region, vertexModel,
		),
		batchSize: defaultBatchSize,
		backoff:   time.Second,
	}
	for _, o := range opts {
		o(v)
	}
	if v.client == nil {
		ts, err := google.DefaultTokenSource(ctx, cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("creating vertex token source: %w", err)
		}
		v.client = oauth2.NewClient(ctx, ts)
	}
	return v, nil
}

// Model implements Embedder.
func (v *Vertex) Model() string { return vertexModel }

// Dims implements Embedder.
func (v *Vertex) Dims() int { return vertexDims }

// Embed embeds texts as retrieval documents (task type RETRIEVAL_DOCUMENT).
func (v *Vertex) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return v.embed(ctx, texts, taskTypeDocument)
}

// EmbedQueries embeds texts as retrieval queries (task type
// RETRIEVAL_QUERY); implements QueryEmbedder.
func (v *Vertex) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return v.embed(ctx, texts, taskTypeQuery)
}

// predictInstance is one :predict request instance.
type predictInstance struct {
	Content  string `json:"content"`
	TaskType string `json:"task_type"`
}

// predictParameters is the :predict request's shared parameters block.
type predictParameters struct {
	OutputDimensionality int  `json:"outputDimensionality"`
	AutoTruncate         bool `json:"autoTruncate"`
}

// predictRequest is the full :predict request body.
type predictRequest struct {
	Instances  []predictInstance `json:"instances"`
	Parameters predictParameters `json:"parameters"`
}

// predictResponse is the subset of the :predict response this adapter reads.
type predictResponse struct {
	Predictions []predictPrediction `json:"predictions"`
}

type predictPrediction struct {
	Embeddings predictEmbeddings `json:"embeddings"`
}

type predictEmbeddings struct {
	Values []float32 `json:"values"`
}

// embed chunks texts into batchSize-sized :predict calls and reassembles the
// vectors in the original order. Every returned vector must be exactly
// vertexDims long — a short or long vector fails the whole call closed
// rather than silently fragmenting the embed space.
func (v *Vertex) embed(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	batchSize := v.batchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := min(start+batchSize, len(texts))
		vecs, err := v.predictBatch(ctx, texts[start:end], taskType)
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// predictBatch issues one :predict call for a batch of texts and maps the
// response back to vectors, dimension-checked.
func (v *Vertex) predictBatch(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	instances := make([]predictInstance, len(texts))
	for i, t := range texts {
		instances[i] = predictInstance{Content: t, TaskType: taskType}
	}
	payload, err := json.Marshal(predictRequest{
		Instances: instances,
		Parameters: predictParameters{
			OutputDimensionality: vertexDims,
			AutoTruncate:         true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encoding vertex embed request: %w", err)
	}

	body, err := v.post(ctx, payload)
	if err != nil {
		return nil, err
	}

	var resp predictResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding vertex embed response: %w", err)
	}
	if len(resp.Predictions) != len(texts) {
		return nil, fmt.Errorf("vertex embed: got %d predictions for %d texts", len(resp.Predictions), len(texts))
	}

	vecs := make([][]float32, len(resp.Predictions))
	for i, p := range resp.Predictions {
		if len(p.Embeddings.Values) != vertexDims {
			return nil, fmt.Errorf(
				"vertex embed: prediction %d has %d dims, want %d", i, len(p.Embeddings.Values), vertexDims,
			)
		}
		vecs[i] = p.Embeddings.Values
	}
	return vecs, nil
}

// post sends payload to the :predict endpoint, retrying 429/5xx (and
// transport errors) with exponential backoff up to vertexMaxAttempts.
func (v *Vertex) post(ctx context.Context, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := range vertexMaxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("waiting to retry vertex embed call: %w", ctx.Err())
			case <-time.After(v.backoff << (attempt - 1)):
			}
		}
		body, retryable, err := v.postOnce(ctx, payload)
		if err == nil {
			return body, nil
		}
		if !retryable {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("vertex embed call failed after %d attempts: %w", vertexMaxAttempts, lastErr)
}

// postOnce performs a single :predict call. retryable reports whether the
// failure is transient (throttling, server error, transport error).
func (v *Vertex) postOnce(ctx context.Context, payload []byte) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("building vertex embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("calling vertex embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading vertex embed response: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, false, nil
	}
	retryable = resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= http.StatusInternalServerError
	return nil, retryable, fmt.Errorf("vertex embed returned %s: %s", resp.Status, truncate(body, vertexMaxErrBody))
}

// truncate clips b to at most n bytes for error messages.
func truncate(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}
