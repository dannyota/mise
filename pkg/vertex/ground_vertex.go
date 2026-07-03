package vertex

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
	// groundMaxAttempts bounds the :check call: one initial try plus three
	// retries on throttling (429) and server errors (5xx).
	groundMaxAttempts = 4

	// groundMaxErrBody caps how much of an error response body is quoted.
	groundMaxErrBody = 512

	// defaultCitationThreshold is the Check Grounding API's citation threshold.
	defaultCitationThreshold = 0.6
)

// checkGrounder calls the Discovery Engine Check Grounding REST API to
// verify a claim against source text.
type checkGrounder struct {
	endpoint          string        // full …/groundingConfigs/default_grounding_config:check URL
	client            *http.Client  // OAuth2-wrapped HTTP client
	backoff           time.Duration // base retry delay, doubled per retry
	citationThreshold float64
}

// GrounderOption configures a checkGrounder.
type GrounderOption func(*checkGrounder)

// WithGrounderHTTPClient overrides the HTTP client, letting tests inject an
// httptest server without going through Application Default Credentials.
func WithGrounderHTTPClient(c *http.Client) GrounderOption {
	return func(g *checkGrounder) { g.client = c }
}

// NewCheckGrounder returns a Grounder backed by the Discovery Engine Check
// Grounding REST API. Credentials come from Application Default Credentials
// unless WithGrounderHTTPClient overrides the client.
func NewCheckGrounder(ctx context.Context, project, region string, opts ...GrounderOption) (Grounder, error) {
	if project == "" || region == "" {
		return nil, errors.New("check grounder: project and region are required")
	}

	const urlFmt = "https://%s-discoveryengine.googleapis.com/" +
		"v1alpha/projects/%s/locations/%s/" +
		"groundingConfigs/default_grounding_config:check"
	g := &checkGrounder{
		endpoint:          fmt.Sprintf(urlFmt, region, project, region),
		backoff:           time.Second,
		citationThreshold: defaultCitationThreshold,
	}
	for _, o := range opts {
		o(g)
	}
	if g.client == nil {
		ts, err := google.DefaultTokenSource(ctx, cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("creating check grounding token source: %w", err)
		}
		g.client = oauth2.NewClient(ctx, ts)
	}
	return g, nil
}

// checkRequest is the Check Grounding API request body.
type checkRequest struct {
	AnswerCandidate string        `json:"answerCandidate"`
	Facts           []checkFact   `json:"facts"`
	GroundingSpec   groundingSpec `json:"groundingSpec"`
}

type checkFact struct {
	FactText string `json:"factText"`
}

type groundingSpec struct {
	CitationThreshold float64 `json:"citationThreshold"`
}

// checkResponse is the subset of the Check Grounding response this adapter reads.
type checkResponse struct {
	SupportScore float64 `json:"supportScore"`
}

// Ground implements Grounder via the Check Grounding REST API.
func (g *checkGrounder) Ground(ctx context.Context, claim, source string) (GroundResult, error) {
	payload, err := json.Marshal(checkRequest{
		AnswerCandidate: claim,
		Facts:           []checkFact{{FactText: source}},
		GroundingSpec:   groundingSpec{CitationThreshold: g.citationThreshold},
	})
	if err != nil {
		return GroundResult{}, fmt.Errorf("encoding check grounding request: %w", err)
	}

	body, err := g.post(ctx, payload)
	if err != nil {
		return GroundResult{}, err
	}

	var resp checkResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return GroundResult{}, fmt.Errorf("decoding check grounding response: %w", err)
	}

	return GroundResult{
		Grounded: resp.SupportScore >= g.citationThreshold,
		Score:    resp.SupportScore,
	}, nil
}

// post sends payload to the :check endpoint, retrying 429/5xx (and transport
// errors) with exponential backoff up to groundMaxAttempts.
func (g *checkGrounder) post(ctx context.Context, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := range groundMaxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("waiting to retry check grounding call: %w", ctx.Err())
			case <-time.After(g.backoff << (attempt - 1)):
			}
		}
		body, retryable, err := g.postOnce(ctx, payload)
		if err == nil {
			return body, nil
		}
		if !retryable {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("check grounding call failed after %d attempts: %w", groundMaxAttempts, lastErr)
}

// postOnce performs a single :check call. retryable reports whether the
// failure is transient (throttling, server error, transport error).
func (g *checkGrounder) postOnce(ctx context.Context, payload []byte) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("building check grounding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("calling check grounding: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading check grounding response: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, false, nil
	}
	retryable = resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= http.StatusInternalServerError
	return nil, retryable, fmt.Errorf("check grounding returned %s: %s", resp.Status, truncate(body, groundMaxErrBody))
}
