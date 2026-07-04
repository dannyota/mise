package vertex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const captionDefaultModel = "gemini-2.0-flash"

const captionPrompt = "Describe the content of this figure or table from a regulatory document. " +
	"Focus on the structure, labels, and relationships shown. " +
	"Be precise and factual — do not interpret or infer meaning beyond what is visually present."

type geminiCaptioner struct {
	endpoint string
	client   *http.Client
	backoff  time.Duration
	model    string
}

// CaptionerOption configures a geminiCaptioner.
type CaptionerOption func(*geminiCaptioner)

// WithCaptionerHTTPClient overrides the HTTP client for testing.
func WithCaptionerHTTPClient(c *http.Client) CaptionerOption {
	return func(gc *geminiCaptioner) { gc.client = c }
}

// WithCaptionerModel overrides the Gemini vision model.
func WithCaptionerModel(model string) CaptionerOption {
	return func(gc *geminiCaptioner) {
		if model != "" {
			gc.model = model
		}
	}
}

// NewGeminiCaptioner returns a Captioner backed by Gemini vision.
func NewGeminiCaptioner(ctx context.Context, project, region string, opts ...CaptionerOption) (Captioner, error) {
	if project == "" || region == "" {
		return nil, errors.New("gemini captioner: project and region are required")
	}
	gc := &geminiCaptioner{
		backoff: time.Second,
		model:   captionDefaultModel,
	}
	for _, o := range opts {
		o(gc)
	}
	gc.endpoint = fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		region, project, region, gc.model,
	)
	if gc.client == nil {
		ts, err := google.DefaultTokenSource(ctx, cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("creating captioner token source: %w", err)
		}
		gc.client = oauth2.NewClient(ctx, ts)
	}
	return gc, nil
}

type captionInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type captionPart struct {
	Text       string             `json:"text,omitempty"`
	InlineData *captionInlineData `json:"inlineData,omitempty"`
}

type captionContent struct {
	Role  string        `json:"role"`
	Parts []captionPart `json:"parts"`
}

type captionRequest struct {
	Contents         []captionContent `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

func (gc *geminiCaptioner) Caption(ctx context.Context, image []byte, contentType string) (CaptionResult, error) {
	payload, err := json.Marshal(captionRequest{
		Contents: []captionContent{{
			Role: "user",
			Parts: []captionPart{
				{InlineData: &captionInlineData{
					MIMEType: contentType,
					Data:     base64.StdEncoding.EncodeToString(image),
				}},
				{Text: captionPrompt},
			},
		}},
		GenerationConfig: generationConfig{
			Temperature: 0.0,
		},
	})
	if err != nil {
		return CaptionResult{}, fmt.Errorf("encoding caption request: %w", err)
	}

	body, err := gc.post(ctx, payload)
	if err != nil {
		return CaptionResult{}, err
	}

	var resp generateContentResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return CaptionResult{}, fmt.Errorf("decoding caption response: %w", err)
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return CaptionResult{}, errors.New("gemini captioner: empty response")
	}

	return CaptionResult{
		Text:      resp.Candidates[0].Content.Parts[0].Text,
		Model:     gc.model,
		Prompt:    captionPrompt,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (gc *geminiCaptioner) post(ctx context.Context, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := range judgeMaxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("waiting to retry captioner call: %w", ctx.Err())
			case <-time.After(gc.backoff << (attempt - 1)):
			}
		}
		body, retryable, err := gc.postOnce(ctx, payload)
		if err == nil {
			return body, nil
		}
		if !retryable {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("captioner call failed after %d attempts: %w", judgeMaxAttempts, lastErr)
}

func (gc *geminiCaptioner) postOnce(ctx context.Context, payload []byte) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gc.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("building captioner request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := gc.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("calling captioner: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading captioner response: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, false, nil
	}
	retryable = resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= http.StatusInternalServerError
	return nil, retryable, fmt.Errorf("captioner returned %s: %s", resp.Status, truncate(body, judgeMaxErrBody))
}
