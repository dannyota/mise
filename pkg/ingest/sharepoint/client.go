package sharepoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// errRetryable is a sentinel wrapped into retryable errors.
var errRetryable = errors.New("retryable")

// doGet performs an authenticated GET with bounded retries on 429/5xx.
// Auth failures (401/403) fail closed immediately.
func (s *Source) doGet(ctx context.Context, rawURL string) (string, error) {
	var lastErr error
	for attempt := range maxRetries + 1 {
		if attempt > 0 {
			if err := sleep(ctx, backoffDuration(attempt)); err != nil {
				return "", err
			}
		}
		body, err := s.doGetOnce(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !errors.Is(err, errRetryable) {
			return "", err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("exhausted retries")
	}
	return "", lastErr
}

func (s *Source) doGetOnce(
	ctx context.Context, rawURL string,
) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, rawURL, nil,
	)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json;odata=nometadata")
	if err := s.auth.Apply(req); err != nil {
		return "", fmt.Errorf("sharepoint: auth: %w", err)
	}

	resp, err := s.http.Do(req) //nolint:bodyclose // closed below
	if err != nil {
		return "", fmt.Errorf("%w: %w", errRetryable, err)
	}

	// Check auth BEFORE reading body: avoid reading potentially large
	// or broken error bodies on auth failure.
	if resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden {
		drainClose(resp.Body)
		return "", s.checkAuthStatus(resp.StatusCode)
	}

	body, closeErr := readAndClose(resp)
	if closeErr != nil {
		return "", fmt.Errorf("%w: %w", errRetryable, closeErr)
	}
	if resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= 500 {
		return "", fmt.Errorf(
			"%w: status %d", errRetryable, resp.StatusCode,
		)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	return body, nil
}

// checkAuthStatus returns a fail-closed error on 401/403.
func (s *Source) checkAuthStatus(statusCode int) error {
	if statusCode == http.StatusUnauthorized ||
		statusCode == http.StatusForbidden {
		return fmt.Errorf(
			"sharepoint: site %s returned %d — session or"+
				" credentials are invalid (fail-closed)",
			s.siteURL, statusCode,
		)
	}
	return nil
}

// odataEscape escapes a server-relative path for use inside a SharePoint
// OData string literal ('...'). SharePoint OData literals need slashes
// preserved (not percent-encoded) and apostrophes doubled (' -> ”).
// Spaces and other characters are left as-is: the path sits inside a
// single-quoted OData function parameter, not a URL path segment.
func odataEscape(serverRelPath string) string {
	return strings.ReplaceAll(serverRelPath, "'", "''")
}

// fileAPIURL builds the GetFileByServerRelativeUrl API path.
func (s *Source) fileAPIURL(serverRelPath string) string {
	return s.siteURL +
		"/_api/web/GetFileByServerRelativeUrl('" +
		odataEscape(serverRelPath) + "')"
}

// folderAPIURL builds the GetFolderByServerRelativeUrl API path.
func (s *Source) folderAPIURL(folderPath string) string {
	return s.siteURL +
		"/_api/web/GetFolderByServerRelativeUrl('" +
		odataEscape(folderPath) + "')"
}

// backoffDuration returns exponential backoff capped at maxBackoff.
func backoffDuration(attempt int) time.Duration {
	d := time.Duration(
		float64(baseBackoff) * math.Pow(2, float64(attempt-1)),
	)
	return min(d, maxBackoff)
}

func readAndClose(resp *http.Response) (string, error) {
	defer drainClose(resp.Body)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

func drainClose(r io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r, 512))
	_ = r.Close()
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// matchesKeyword does a case-insensitive substring match on title or number.
func matchesKeyword(kw, title, number string) bool {
	return strings.Contains(strings.ToLower(title), kw) ||
		strings.Contains(strings.ToLower(number), kw)
}

// extOf returns the lowercase extension without the dot.
func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return strings.ToLower(name[i+1:])
	}
	return ""
}

// titleFromFilename derives a human title from the filename stem.
func titleFromFilename(name string) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	return name
}

// mimeForExt returns the MIME type for supported extensions.
func mimeForExt(ext string) string {
	switch ext {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument." +
			"wordprocessingml.document"
	case "html":
		return "text/html"
	case "md":
		return "text/markdown"
	default:
		return ""
	}
}

// parseDate parses an ISO 8601 date or datetime string.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// normalizeETag strips surrounding quotes and braces from the SharePoint
// ETag.
func normalizeETag(etag string) string {
	etag = strings.Trim(etag, "\"")
	etag = strings.Trim(etag, "{}")
	return strings.ToLower(strings.TrimSpace(etag))
}

// getField reads a string field from ListItemAllFields (case-insensitive).
func getField(fields map[string]json.RawMessage, key string) string {
	// Try exact match first.
	if v, ok := fields[key]; ok {
		return unquote(v)
	}
	// Case-insensitive fallback.
	lower := strings.ToLower(key)
	for k, v := range fields {
		if strings.ToLower(k) == lower {
			return unquote(v)
		}
	}
	return ""
}

// unquote extracts a JSON string value; returns empty for null/non-string.
func unquote(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
