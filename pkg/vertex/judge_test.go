package vertex

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Fake judge
// ---------------------------------------------------------------------------

func TestFakeJudgeReturnsSatisfies(t *testing.T) {
	j := NewFakeJudge()
	got, err := j.Judge(context.Background(), "control text", "law text")
	if err != nil {
		t.Fatal(err)
	}
	if got.EdgeType != "satisfies" {
		t.Errorf("EdgeType = %q, want %q", got.EdgeType, "satisfies")
	}
	if got.Confidence != 0.85 {
		t.Errorf("Confidence = %f, want 0.85", got.Confidence)
	}
}

// ---------------------------------------------------------------------------
// Gemini judge (httptest)
// ---------------------------------------------------------------------------

// testGeminiJudge returns a geminiJudge aimed at srv with negligible backoff.
func testGeminiJudge(srv *httptest.Server) *geminiJudge {
	schemaJSON, _ := json.Marshal(judgeResponseSchema)
	h := sha256.New()
	h.Write([]byte(judgePromptTemplate))
	h.Write(schemaJSON)
	return &geminiJudge{
		endpoint:   srv.URL,
		client:     srv.Client(),
		backoff:    time.Millisecond,
		promptHash: hex.EncodeToString(h.Sum(nil)),
	}
}

// cannedJudgeOK is a minimal successful :generateContent response with
// edge_type "satisfies".
func cannedJudgeOK() string {
	jr := judgeResponse{
		EdgeType:       "satisfies",
		Confidence:     0.92,
		Rationale:      "The control clause requires annual review, matching the law.",
		QuotedFromSpan: "annual review of all policies",
		QuotedToSpan:   "policies shall be reviewed at least annually",
	}
	inner, _ := json.Marshal(jr)
	resp := generateContentResponse{
		Candidates: []generateCandidate{{
			Content: generateCandidateContent{
				Parts: []generateCandidatePart{{Text: string(inner)}},
			},
		}},
	}
	out, _ := json.Marshal(resp)
	return string(out)
}

// cannedJudgeNone is a response with edge_type "none".
func cannedJudgeNone() string {
	jr := judgeResponse{
		EdgeType:       "none",
		Confidence:     0.15,
		Rationale:      "The control clause is about data retention, unrelated to audit.",
		QuotedFromSpan: "data retention schedule",
		QuotedToSpan:   "audit trails shall be maintained",
	}
	inner, _ := json.Marshal(jr)
	resp := generateContentResponse{
		Candidates: []generateCandidate{{
			Content: generateCandidateContent{
				Parts: []generateCandidatePart{{Text: string(inner)}},
			},
		}},
	}
	out, _ := json.Marshal(resp)
	return string(out)
}

func TestGeminiJudgeMapsResponse(t *testing.T) {
	var gotReq generateContentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cannedJudgeOK()))
	}))
	defer srv.Close()

	j := testGeminiJudge(srv)
	got, err := j.Judge(context.Background(), "control text here", "law text here")
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}

	// Verify result fields.
	if got.EdgeType != "satisfies" {
		t.Errorf("EdgeType = %q, want %q", got.EdgeType, "satisfies")
	}
	if got.Confidence != 0.92 {
		t.Errorf("Confidence = %f, want 0.92", got.Confidence)
	}
	if got.FromSpan != "annual review of all policies" {
		t.Errorf("FromSpan = %q, want %q", got.FromSpan, "annual review of all policies")
	}
	if got.ToSpan != "policies shall be reviewed at least annually" {
		t.Errorf("ToSpan = %q, want %q", got.ToSpan, "policies shall be reviewed at least annually")
	}
	if got.Rationale == "" {
		t.Error("Rationale is empty, want non-empty")
	}

	// Verify the request contains structured-output config.
	if gotReq.GenerationConfig.ResponseMIMEType != "application/json" {
		t.Errorf("responseMimeType = %q, want %q",
			gotReq.GenerationConfig.ResponseMIMEType, "application/json")
	}
	if gotReq.GenerationConfig.ResponseSchema == nil {
		t.Error("responseSchema is nil, want the judge schema")
	}
	if gotReq.GenerationConfig.Temperature != 0.0 {
		t.Errorf("temperature = %f, want 0.0", gotReq.GenerationConfig.Temperature)
	}

	// Verify the prompt contains both texts.
	if len(gotReq.Contents) == 0 || len(gotReq.Contents[0].Parts) == 0 {
		t.Fatal("request contents empty")
	}
	prompt := gotReq.Contents[0].Parts[0].Text
	if !strings.Contains(prompt, "control text here") {
		t.Error("prompt does not contain fromText")
	}
	if !strings.Contains(prompt, "law text here") {
		t.Error("prompt does not contain toText")
	}
}

func TestGeminiJudgeNoneMapsToZeroConfidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(cannedJudgeNone()))
	}))
	defer srv.Close()

	got, err := testGeminiJudge(srv).Judge(context.Background(), "a", "b")
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.EdgeType != "none" {
		t.Errorf("EdgeType = %q, want %q", got.EdgeType, "none")
	}
	if got.Confidence != 0 {
		t.Errorf("Confidence = %f, want 0 (none maps to zero)", got.Confidence)
	}
}

func TestGeminiJudgePromptHashDeterministic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(cannedJudgeOK()))
	}))
	defer srv.Close()

	j1 := testGeminiJudge(srv)
	j2 := testGeminiJudge(srv)
	if j1.PromptHash() != j2.PromptHash() {
		t.Errorf("PromptHash() not deterministic: %q != %q", j1.PromptHash(), j2.PromptHash())
	}
	if j1.PromptHash() == "" {
		t.Error("PromptHash() is empty")
	}
	// Verify it's a 64-char hex string (sha256).
	if len(j1.PromptHash()) != 64 {
		t.Errorf("PromptHash() length = %d, want 64", len(j1.PromptHash()))
	}
}

func TestGeminiJudgeRetries429ThenSucceeds(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(cannedJudgeOK()))
	}))
	defer srv.Close()

	got, err := testGeminiJudge(srv).Judge(context.Background(), "a", "b")
	if err != nil {
		t.Fatalf("Judge() error = %v, want success after retry", err)
	}
	if calls != 2 {
		t.Errorf("server calls = %d, want 2 (429 then success)", calls)
	}
	if got.EdgeType != "satisfies" {
		t.Errorf("EdgeType = %q, want %q", got.EdgeType, "satisfies")
	}
}

func TestGeminiJudgeRetries5xxAndGivesUp(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := testGeminiJudge(srv).Judge(context.Background(), "a", "b")
	if err == nil {
		t.Fatal("Judge() error = nil, want error after exhausted retries")
	}
	if calls != judgeMaxAttempts {
		t.Errorf("server calls = %d, want %d", calls, judgeMaxAttempts)
	}
}

func TestGeminiJudgeDoesNotRetryClientErrors(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := testGeminiJudge(srv).Judge(context.Background(), "a", "b")
	if err == nil {
		t.Fatal("Judge() error = nil, want error on 400")
	}
	if calls != 1 {
		t.Errorf("server calls = %d, want 1 (4xx is not retryable)", calls)
	}
}

func TestGeminiJudgeEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer srv.Close()

	_, err := testGeminiJudge(srv).Judge(context.Background(), "a", "b")
	if err == nil {
		t.Fatal("Judge() error = nil, want error on empty response")
	}
}

func TestNewGeminiJudgeRejectsEmptyArgs(t *testing.T) {
	tests := []struct {
		name            string
		project, region string
	}{
		{name: "empty project", region: "asia-southeast1"},
		{name: "empty region", project: "proj"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGeminiJudge(context.Background(), tt.project, tt.region,
				WithJudgeHTTPClient(http.DefaultClient))
			if err == nil {
				t.Error("NewGeminiJudge() error = nil, want error")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Live test — gated on GCP_PROJECT
// ---------------------------------------------------------------------------

func TestGeminiJudgeLive(t *testing.T) {
	project := os.Getenv("GCP_PROJECT")
	if project == "" {
		t.Skip("GCP_PROJECT not set; skipping live Gemini judge test")
	}
	region := cmp.Or(os.Getenv("GCP_REGION"), "asia-southeast1")

	j, err := NewGeminiJudge(context.Background(), project, region)
	if err != nil {
		t.Fatalf("NewGeminiJudge() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	got, err := j.Judge(ctx,
		"The bank shall conduct annual reviews of its IT risk management framework.",
		"Financial institutions must review their technology risk management policies at least once every twelve months.",
	)
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.EdgeType != "satisfies" && got.EdgeType != "none" {
		t.Errorf("EdgeType = %q, want satisfies or none", got.EdgeType)
	}
	if got.Rationale == "" {
		t.Error("Rationale is empty, want non-empty")
	}
	t.Logf("live result: edge_type=%s confidence=%.2f rationale=%s",
		got.EdgeType, got.Confidence, got.Rationale)
}
