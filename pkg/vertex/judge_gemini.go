package vertex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	// judgeDefaultModel is the Gemini model used for edge classification.
	judgeDefaultModel = "gemini-3.5-flash"

	// judgeMaxAttempts bounds a single :generateContent call: one initial
	// try plus three retries on throttling (429) and server errors (5xx).
	judgeMaxAttempts = 4

	// judgeMaxErrBody caps how much of an error response body is quoted.
	judgeMaxErrBody = 512
)

// judgePromptTemplate is the versioned prompt contract. Changes to this
// string change the PromptHash, which triggers re-evaluation of existing
// edges (the hash is stored alongside each relation_evidence row).
var judgePromptTemplate = `You are a regulatory compliance analyst.

Given two regulatory texts — a control clause from an internal ` +
	`policy/standard and a law provision — determine if the control ` +
	`clause satisfies (implements or evidences compliance with) the ` +
	`law provision.

## Control clause

%s

## Law provision

%s

Respond with a JSON object containing:
- edge_type: "satisfies" if the control clause implements or ` +
	`evidences compliance with the law provision, "none" otherwise.
- confidence: a float 0.0–1.0 indicating classification confidence.
- rationale: a brief explanation of your reasoning.
- quoted_from_span: the most relevant verbatim excerpt from the control clause.
- quoted_to_span: the most relevant verbatim excerpt from the law provision.`

// judgeResponseSchema is the JSON Schema enforced via Gemini's structured
// output (responseMimeType: application/json + responseSchema). It is part
// of the versioned prompt contract — changes alter the PromptHash.
var judgeResponseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"edge_type": map[string]any{
			"type": "string",
			"enum": []string{"satisfies", "none"},
		},
		"confidence": map[string]any{
			"type": "number",
		},
		"rationale": map[string]any{
			"type": "string",
		},
		"quoted_from_span": map[string]any{
			"type": "string",
		},
		"quoted_to_span": map[string]any{
			"type": "string",
		},
	},
	"required": []any{
		"edge_type", "confidence", "rationale",
		"quoted_from_span", "quoted_to_span",
	},
}

// geminiJudge classifies candidate control–law pairs via the Gemini
// :generateContent REST API with structured JSON output.
type geminiJudge struct {
	endpoint      string        // full …/models/{model}:generateContent URL
	client        *http.Client  // OAuth2-wrapped or test-injected HTTP client
	backoff       time.Duration // base retry delay, doubled per retry
	promptHash    string        // sha256(promptTemplate + responseSchemaJSON)
	endpointModel string        // model override set by WithJudgeModel
}

// JudgeOption configures a geminiJudge.
type JudgeOption func(*geminiJudge)

// WithJudgeHTTPClient overrides the HTTP client, letting tests inject an
// httptest server without going through Application Default Credentials.
func WithJudgeHTTPClient(c *http.Client) JudgeOption {
	return func(j *geminiJudge) { j.client = c }
}

// WithJudgeModel overrides the Gemini model (default gemini-3.5-flash).
func WithJudgeModel(model string) JudgeOption {
	return func(j *geminiJudge) {
		if model != "" {
			j.endpoint = "" // signal to rebuild endpoint
			j.endpointModel = model
		}
	}
}

// NewGeminiJudge returns a Judge backed by the Gemini :generateContent
// REST endpoint. Credentials come from Application Default Credentials
// unless WithJudgeHTTPClient overrides the client.
func NewGeminiJudge(ctx context.Context, project, region string, opts ...JudgeOption) (Judge, error) {
	if project == "" || region == "" {
		return nil, errors.New("gemini judge: project and region are required")
	}

	model := judgeDefaultModel
	j := &geminiJudge{
		backoff: time.Second,
	}
	for _, o := range opts {
		o(j)
	}
	if j.endpointModel != "" {
		model = j.endpointModel
	}
	j.endpoint = fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		region, project, region, model,
	)

	// Compute the prompt hash: sha256(template + schema JSON).
	schemaJSON, err := json.Marshal(judgeResponseSchema)
	if err != nil {
		return nil, fmt.Errorf("gemini judge: marshaling response schema: %w", err)
	}
	h := sha256.New()
	h.Write([]byte(judgePromptTemplate))
	h.Write(schemaJSON)
	j.promptHash = hex.EncodeToString(h.Sum(nil))

	if j.client == nil {
		ts, err := google.DefaultTokenSource(ctx, cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("creating gemini judge token source: %w", err)
		}
		j.client = oauth2.NewClient(ctx, ts)
	}
	return j, nil
}

// PromptHash returns the deterministic hash of the prompt template and
// response schema. Callers store this in relation_evidence.prompt_hash to
// detect when re-evaluation is needed.
func (j *geminiJudge) PromptHash() string { return j.promptHash }

// generateContentRequest is the :generateContent request body.
type generateContentRequest struct {
	Contents         []generateContentPart `json:"contents"`
	GenerationConfig generationConfig      `json:"generationConfig"`
}

type generateContentPart struct {
	Role  string             `json:"role"`
	Parts []generateTextPart `json:"parts"`
}

type generateTextPart struct {
	Text string `json:"text"`
}

type generationConfig struct {
	ResponseMIMEType string  `json:"responseMimeType"`
	ResponseSchema   any     `json:"responseSchema"`
	Temperature      float64 `json:"temperature"`
}

// generateContentResponse is the subset of the :generateContent response
// this adapter reads.
type generateContentResponse struct {
	Candidates []generateCandidate `json:"candidates"`
}

type generateCandidate struct {
	Content generateCandidateContent `json:"content"`
}

type generateCandidateContent struct {
	Parts []generateCandidatePart `json:"parts"`
}

type generateCandidatePart struct {
	Text string `json:"text"`
}

// judgeResponse is the structured JSON output the model returns.
type judgeResponse struct {
	EdgeType       string  `json:"edge_type"`
	Confidence     float64 `json:"confidence"`
	Rationale      string  `json:"rationale"`
	QuotedFromSpan string  `json:"quoted_from_span"`
	QuotedToSpan   string  `json:"quoted_to_span"`
}

// Judge classifies whether fromText (control) satisfies toText (law).
func (j *geminiJudge) Judge(ctx context.Context, fromText, toText string) (JudgeResult, error) {
	prompt := fmt.Sprintf(judgePromptTemplate, fromText, toText)
	payload, err := json.Marshal(generateContentRequest{
		Contents: []generateContentPart{{
			Role:  "user",
			Parts: []generateTextPart{{Text: prompt}},
		}},
		GenerationConfig: generationConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   judgeResponseSchema,
			Temperature:      0.0,
		},
	})
	if err != nil {
		return JudgeResult{}, fmt.Errorf("encoding gemini judge request: %w", err)
	}

	body, err := j.post(ctx, payload)
	if err != nil {
		return JudgeResult{}, err
	}

	var resp generateContentResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return JudgeResult{}, fmt.Errorf("decoding gemini judge response: %w", err)
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return JudgeResult{}, errors.New("gemini judge: empty response")
	}

	var jr judgeResponse
	if err := json.Unmarshal([]byte(resp.Candidates[0].Content.Parts[0].Text), &jr); err != nil {
		return JudgeResult{}, fmt.Errorf("decoding gemini judge structured output: %w", err)
	}

	result := JudgeResult{
		EdgeType:   jr.EdgeType,
		Confidence: jr.Confidence,
		FromSpan:   jr.QuotedFromSpan,
		ToSpan:     jr.QuotedToSpan,
		Rationale:  jr.Rationale,
	}
	// Map "none" to zero confidence so the threshold gate filters it.
	if jr.EdgeType == "none" {
		result.Confidence = 0
	}
	return result, nil
}

// post sends payload to the :generateContent endpoint, retrying 429/5xx
// (and transport errors) with exponential backoff up to judgeMaxAttempts.
func (j *geminiJudge) post(ctx context.Context, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := range judgeMaxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("waiting to retry gemini judge call: %w", ctx.Err())
			case <-time.After(j.backoff << (attempt - 1)):
			}
		}
		body, retryable, err := j.postOnce(ctx, payload)
		if err == nil {
			return body, nil
		}
		if !retryable {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("gemini judge call failed after %d attempts: %w", judgeMaxAttempts, lastErr)
}

// postOnce performs a single :generateContent call. retryable reports
// whether the failure is transient (throttling, server error, transport
// error).
func (j *geminiJudge) postOnce(ctx context.Context, payload []byte) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("building gemini judge request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("calling gemini judge: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading gemini judge response: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, false, nil
	}
	retryable = resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= http.StatusInternalServerError
	return nil, retryable, fmt.Errorf("gemini judge returned %s: %s", resp.Status, truncate(body, judgeMaxErrBody))
}
