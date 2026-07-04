package vertex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// rankerModel is the Discovery Engine semantic ranker model.
	rankerModel = "semantic-ranker-512@latest"

	// rankerMaxAttempts bounds a single :rank call: one initial try plus
	// three retries on throttling (429) and server errors (5xx).
	rankerMaxAttempts = 4

	// rankerMaxErrBody caps how much of an error response body is quoted.
	rankerMaxErrBody = 512
)

// vertexRanker calls the Discovery Engine Ranking API to rerank documents.
type vertexRanker struct {
	endpoint string        // full …/rankingConfigs/default_ranking_config:rank URL
	client   *http.Client  // OAuth2-wrapped HTTP client
	backoff  time.Duration // base retry delay, doubled per retry
}

// RankerOption configures a vertexRanker.
type RankerOption func(*vertexRanker)

// WithRankerHTTPClient overrides the HTTP client, letting tests inject an
// httptest server without going through Application Default Credentials.
func WithRankerHTTPClient(c *http.Client) RankerOption {
	return func(r *vertexRanker) { r.client = c }
}

// NewVertexRanker returns a Ranker backed by the Discovery Engine Ranking API.
// Credentials come from Application Default Credentials unless
// WithRankerHTTPClient overrides the client.
func NewVertexRanker(ctx context.Context, project, region string, opts ...RankerOption) (Ranker, error) {
	if project == "" || region == "" {
		return nil, errors.New("vertex ranker: project and region are required")
	}

	r := &vertexRanker{
		endpoint: fmt.Sprintf(
			"https://%s-discoveryengine.googleapis.com/v1/projects/%s/locations/%s/rankingConfigs/default_ranking_config:rank",
			region, project, region,
		),
		backoff: time.Second,
	}
	for _, o := range opts {
		o(r)
	}
	if r.client == nil {
		ts, err := google.DefaultTokenSource(ctx, cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("creating vertex ranker token source: %w", err)
		}
		r.client = oauth2.NewClient(ctx, ts)
	}
	return r, nil
}

// rankRequest is the :rank request body.
type rankRequest struct {
	Model   string       `json:"model"`
	Query   string       `json:"query"`
	Records []rankRecord `json:"records"`
	TopN    int          `json:"topN"`
}

// rankRecord is one input document in a :rank request.
type rankRecord struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// rankResponse is the subset of the :rank response this adapter reads.
type rankResponse struct {
	Records []rankResponseRecord `json:"records"`
}

// rankResponseRecord is one reranked document in the :rank response.
type rankResponseRecord struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Content string  `json:"content"`
}

// Rerank implements Ranker via the Discovery Engine Ranking API.
func (r *vertexRanker) Rerank(ctx context.Context, query string, docs []string, topK int) ([]RankedDoc, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	records := make([]rankRecord, len(docs))
	for i, d := range docs {
		records[i] = rankRecord{ID: strconv.Itoa(i), Content: d}
	}

	n := topK
	if n <= 0 || n > len(docs) {
		n = len(docs)
	}

	payload, err := json.Marshal(rankRequest{
		Model:   rankerModel,
		Query:   query,
		Records: records,
		TopN:    n,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding rank request: %w", err)
	}

	body, err := r.post(ctx, payload)
	if err != nil {
		return nil, err
	}

	var resp rankResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding rank response: %w", err)
	}

	out := make([]RankedDoc, len(resp.Records))
	for i, rec := range resp.Records {
		idx, err := strconv.Atoi(rec.ID)
		if err != nil {
			return nil, fmt.Errorf("rank response: invalid record id %q: %w", rec.ID, err)
		}
		out[i] = RankedDoc{Index: idx, Score: rec.Score}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })

	if topK > 0 && topK < len(out) {
		out = out[:topK]
	}
	return out, nil
}

// post sends payload to the :rank endpoint, retrying 429/5xx (and transport
// errors) with exponential backoff up to rankerMaxAttempts.
func (r *vertexRanker) post(ctx context.Context, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := range rankerMaxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("waiting to retry rank call: %w", ctx.Err())
			case <-time.After(r.backoff << (attempt - 1)):
			}
		}
		body, retryable, err := r.postOnce(ctx, payload)
		if err == nil {
			return body, nil
		}
		if !retryable {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("rank call failed after %d attempts: %w", rankerMaxAttempts, lastErr)
}

// postOnce performs a single :rank call. retryable reports whether the failure
// is transient (throttling, server error, transport error).
func (r *vertexRanker) postOnce(ctx context.Context, payload []byte) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("building rank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("calling rank api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading rank response: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, false, nil
	}
	retryable = resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= http.StatusInternalServerError
	return nil, retryable, fmt.Errorf("rank api returned %s: %s", resp.Status, truncate(body, rankerMaxErrBody))
}
