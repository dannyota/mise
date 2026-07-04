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

// testCheckGrounder returns a checkGrounder aimed at srv with a negligible
// backoff, for tests that don't go through NewCheckGrounder's Application
// Default Credentials path.
func testCheckGrounder(srv *httptest.Server) *checkGrounder {
	return &checkGrounder{
		endpoint:          srv.URL,
		client:            srv.Client(),
		backoff:           time.Millisecond,
		citationThreshold: defaultCitationThreshold,
	}
}

// cannedCheckResponse builds a Check Grounding response with the given support score.
func cannedCheckResponse(score float64) []byte {
	resp := checkResponse{SupportScore: score}
	b, _ := json.Marshal(resp) //nolint:errcheck // test helper
	return b
}

func TestNewCheckGrounderRejectsEmptyArgs(t *testing.T) {
	tests := []struct {
		name, project, region string
	}{
		{name: "empty project", region: "us-central1"},
		{name: "empty region", project: "proj"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCheckGrounder(
				context.Background(), tt.project, tt.region,
				WithGrounderHTTPClient(&http.Client{}),
			)
			if err == nil {
				t.Error("NewCheckGrounder() error = nil, want error")
			}
		})
	}
}

func TestNewCheckGrounderEndpoint(t *testing.T) {
	g, err := NewCheckGrounder(
		context.Background(), "proj", "us-central1",
		WithGrounderHTTPClient(&http.Client{}),
	)
	if err != nil {
		t.Fatalf("NewCheckGrounder() error = %v", err)
	}
	cg := g.(*checkGrounder)
	want := "https://us-central1-discoveryengine.googleapis.com/" +
		"v1alpha/projects/proj/locations/us-central1/" +
		"groundingConfigs/default_grounding_config:check"
	if cg.endpoint != want {
		t.Errorf("endpoint = %q, want %q", cg.endpoint, want)
	}
}

func TestCheckGrounderMapsRequestAndResponse(t *testing.T) {
	var gotReq checkRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cannedCheckResponse(0.85))
	}))
	defer srv.Close()

	result, err := testCheckGrounder(srv).Ground(context.Background(), "the claim", "the source")
	if err != nil {
		t.Fatalf("Ground() error = %v", err)
	}

	if gotReq.AnswerCandidate != "the claim" {
		t.Errorf("request answerCandidate = %q, want %q", gotReq.AnswerCandidate, "the claim")
	}
	if len(gotReq.Facts) != 1 || gotReq.Facts[0].FactText != "the source" {
		t.Errorf("request facts = %+v, want one fact with %q", gotReq.Facts, "the source")
	}
	if gotReq.GroundingSpec.CitationThreshold != defaultCitationThreshold {
		t.Errorf("request citationThreshold = %f, want %f", gotReq.GroundingSpec.CitationThreshold, defaultCitationThreshold)
	}
	if !result.Grounded {
		t.Error("Ground() Grounded = false, want true for score 0.85")
	}
	if result.Score != 0.85 {
		t.Errorf("Ground() Score = %f, want 0.85", result.Score)
	}
}

func TestCheckGrounderGroundedBelowThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(cannedCheckResponse(0.3))
	}))
	defer srv.Close()

	result, err := testCheckGrounder(srv).Ground(context.Background(), "claim", "source")
	if err != nil {
		t.Fatalf("Ground() error = %v", err)
	}
	if result.Grounded {
		t.Error("Ground() Grounded = true, want false for score below threshold")
	}
	if result.Score != 0.3 {
		t.Errorf("Ground() Score = %f, want 0.3", result.Score)
	}
}

func TestCheckGrounderZeroScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	result, err := testCheckGrounder(srv).Ground(context.Background(), "claim", "source")
	if err != nil {
		t.Fatalf("Ground() error = %v", err)
	}
	if result.Grounded {
		t.Error("Ground() Grounded = true, want false for zero/absent supportScore")
	}
	if result.Score != 0 {
		t.Errorf("Ground() Score = %f, want 0", result.Score)
	}
}

func TestCheckGrounderExactThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(cannedCheckResponse(defaultCitationThreshold))
	}))
	defer srv.Close()

	result, err := testCheckGrounder(srv).Ground(context.Background(), "claim", "source")
	if err != nil {
		t.Fatalf("Ground() error = %v", err)
	}
	if !result.Grounded {
		t.Error("Ground() Grounded = false, want true at exact threshold")
	}
}

func TestCheckGrounderRetriesThrottleAndServerErrors(t *testing.T) {
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
			_, _ = w.Write(cannedCheckResponse(0.9))
		}
	}))
	defer srv.Close()

	result, err := testCheckGrounder(srv).Ground(context.Background(), "claim", "source")
	if err != nil {
		t.Fatalf("Ground() error = %v, want success after retries", err)
	}
	if calls != 4 {
		t.Errorf("server calls = %d, want 4 (initial + 3 retries)", calls)
	}
	if !result.Grounded {
		t.Error("Ground() Grounded = false, want true")
	}
}

func TestCheckGrounderGivesUpAfterRetryBudget(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := testCheckGrounder(srv).Ground(context.Background(), "claim", "source")
	if err == nil {
		t.Fatal("Ground() error = nil, want error after exhausted retries")
	}
	if calls != 4 {
		t.Errorf("server calls = %d, want 4 (initial + 3 retries)", calls)
	}
}

func TestCheckGrounderDoesNotRetryClientErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := testCheckGrounder(srv).Ground(context.Background(), "claim", "source")
	if err == nil {
		t.Fatal("Ground() error = nil, want error on 400")
	}
	if calls != 1 {
		t.Errorf("server calls = %d, want 1 (4xx is not retryable)", calls)
	}
}

// TestCheckGrounderLive hits the real Check Grounding API; it is gated on
// GCP_PROJECT so CI and offline runs skip it.
func TestCheckGrounderLive(t *testing.T) {
	project := os.Getenv("GCP_PROJECT")
	if project == "" {
		t.Skip("GCP_PROJECT not set; skipping live Check Grounding test")
	}
	region := cmp.Or(os.Getenv("GCP_REGION"), "us-central1")

	g, err := NewCheckGrounder(context.Background(), project, region)
	if err != nil {
		t.Fatalf("NewCheckGrounder() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := g.Ground(ctx,
		"Banks must maintain a minimum capital adequacy ratio of 8%.",
		"Article 6. Capital adequacy ratio: Credit institutions must maintain a minimum capital adequacy ratio of 8%.",
	)
	if err != nil {
		t.Fatalf("Ground() error = %v", err)
	}
	if result.Score == 0 {
		t.Error("Ground() Score = 0, expected non-zero for supported claim")
	}
}
