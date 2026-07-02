package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/storage"
	"github.com/googleapis/gax-go/v2/apierror"
)

// GCS is a Store backed by a Google Cloud Storage bucket (GKE deployment;
// DEPLOYMENT.md). The caller owns client's lifecycle (ADC auth, Close).
type GCS struct {
	client *storage.Client
	bucket string
}

// NewGCS returns a Store backed by bucket, using client for all calls.
func NewGCS(client *storage.Client, bucket string) Store {
	return &GCS{client: client, bucket: bucket}
}

// Put implements Store. It relies on GCS's DoesNotExist precondition, so
// unlike the local FS backend the commit is atomic and race-safe across
// concurrent writers: GCS itself rejects the write if the key was created
// concurrently.
func (g *GCS) Put(ctx context.Context, key string, r io.Reader) (bool, error) {
	obj := g.client.Bucket(g.bucket).Object(key).If(storage.Conditions{DoesNotExist: true})
	w := obj.NewWriter(ctx)

	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return false, fmt.Errorf("writing %s: %w", key, err)
	}

	if err := w.Close(); err != nil {
		if apiErr, ok := apierror.FromError(err); ok && apiErr.HTTPCode() == http.StatusPreconditionFailed {
			return false, nil // already exists — immutable, put-once
		}
		return false, fmt.Errorf("committing %s: %w", key, err)
	}
	return true, nil
}

// Get implements Store.
func (g *GCS) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	r, err := g.client.Bucket(g.bucket).Object(key).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", key, err)
	}
	return r, nil
}

// Exists implements Store.
func (g *GCS) Exists(ctx context.Context, key string) (bool, error) {
	_, err := g.client.Bucket(g.bucket).Object(key).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("checking %s: %w", key, err)
}
