// Package detect implements cross-corpus coverage detectors: gap scans
// (missing downstream coverage) and stale scans (law amendments outdating
// downstream controls). Both are pure graph queries with no model calls.
package detect

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// FindingCreator abstracts store.FindingStore.CreateFinding so detectors can
// be unit-tested with a fake that doesn't require a real database.
type FindingCreator interface {
	CreateFinding(ctx context.Context, f store.Finding) (uuid.UUID, error)
}

// nodeRefKey returns a stable string key for a NodeRef, used in dedup keys.
func nodeRefKey(ref graph.NodeRef) string {
	key := ref.CorpusID + ":" + ref.DocumentID.String()
	if ref.SectionID != nil {
		key += ":" + ref.SectionID.String()
	}
	return key
}

// mustMarshalJSON marshals v to JSON, panicking on error. Only used for
// simple map[string]string literals that are guaranteed to marshal.
func mustMarshalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// Unreachable for the simple maps we pass.
		panic(fmt.Sprintf("detect: json.Marshal: %v", err))
	}
	return b
}
