package congbao

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"danny.vn/mise/pkg/ingest"
)

// Download streams a file reference into w, computing its SHA-256 in flight.
// The CDN requires a congbao Referer and a browser User-Agent. It returns the
// number of bytes written and the lowercase-hex digest. Transport errors
// (including the CDN's TLS failures in restricted networks) are returned so the
// caller can record the failure and continue. Callers should not log signed
// query strings from download URLs.
func (s *Source) Download(ctx context.Context, ref ingest.FileRef, w io.Writer) (int64, string, error) {
	if ref.URL == "" {
		return 0, "", errors.New("download: empty url")
	}
	// Body is closed by the deferred drainClose below; bodyclose can't trace
	// the close through the get/do retry wrapper.
	//nolint:bodyclose
	resp, err := s.get(ctx, ref.URL, browserUA, map[string]string{
		"Referer": refererURL,
		"Accept":  "*/*",
	})
	if err != nil {
		return 0, "", fmt.Errorf("download %s: %w", ref.Name, err)
	}
	defer drainClose(resp.Body)

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), resp.Body)
	if err != nil {
		return n, "", fmt.Errorf("download %s: copy body: %w", ref.Name, err)
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
