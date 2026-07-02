// Package blob provides content-addressed, immutable storage for raw ingest
// artifacts (PDF/DOCX/DOC bytes and any other fetched file), backed by either
// the local filesystem (dev) or GCS (GKE). Every object is written once at a
// deterministic key derived from its SHA-256; a second Put for the same key is
// a no-op, so re-running Discover/Fetch never re-uploads or mutates raw bytes.
package blob

import (
	"context"
	"io"
)

// Store is a put-once, content-addressed object store. Implementations never
// overwrite an existing key.
type Store interface {
	// Put writes r to key if key does not already exist. It returns false
	// (with a nil error) when the key already exists, so callers can
	// distinguish "already have it" from a fresh write without a separate
	// Exists call.
	Put(ctx context.Context, key string, r io.Reader) (bool, error)

	// Get opens key for reading. Callers must Close the returned reader.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Exists reports whether key has already been written.
	Exists(ctx context.Context, key string) (bool, error)
}

// Key returns the content-addressed storage key for a downloaded file: the raw
// bytes' lowercase-hex SHA-256 (as returned by ingest.Source.Download),
// sharded two hex chars deep so any one directory can't grow unbounded, plus
// the file's extension (with leading dot, e.g. ".pdf"; may be empty).
func Key(sha256Hex, ext string) string {
	return "raw/" + sha256Hex[:2] + "/" + sha256Hex + ext
}
