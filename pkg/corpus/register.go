package corpus

import (
	"errors"
	"fmt"
)

// ErrEmbedSpaceMismatch reports a descriptor whose embed config doesn't
// match the locked shared space — fail-closed (DECISIONS 1).
var ErrEmbedSpaceMismatch = errors.New("embed config does not match the locked shared space")

// ErrDuplicateCorpus reports an ID collision with an existing descriptor.
var ErrDuplicateCorpus = errors.New("corpus ID already registered")

// ErrInvalidMetadataConfig reports a structurally invalid metadata config.
var ErrInvalidMetadataConfig = errors.New("invalid metadata config")

// ValidateEmbedSpace rejects any EmbedConfig that doesn't exactly match
// the locked shared space (SharedEmbed). Native image vectors (task_type
// != RETRIEVAL_DOCUMENT) are barred — DECISIONS 1 LOCKED.
func ValidateEmbedSpace(e EmbedConfig) error {
	if e.Model != SharedEmbed.Model {
		return fmt.Errorf("model %q != %q: %w", e.Model, SharedEmbed.Model, ErrEmbedSpaceMismatch)
	}
	if e.Dims != SharedEmbed.Dims {
		return fmt.Errorf("dims %d != %d: %w", e.Dims, SharedEmbed.Dims, ErrEmbedSpaceMismatch)
	}
	if e.TaskType != SharedEmbed.TaskType {
		return fmt.Errorf("task_type %q != %q: %w", e.TaskType, SharedEmbed.TaskType, ErrEmbedSpaceMismatch)
	}
	return nil
}

// Register adds d to the in-process registry after validating embed space
// and id uniqueness. Returns an error if validation fails — no ingest
// starts on a rejected descriptor.
func Register(d Descriptor) error {
	if _, exists := registry[d.ID]; exists {
		return fmt.Errorf("corpus %s: %w", d.ID, ErrDuplicateCorpus)
	}
	if err := ValidateEmbedSpace(d.Embed); err != nil {
		return err
	}
	registry[d.ID] = d
	return nil
}

// Unregister removes a corpus from the registry. Used by tests for cleanup.
func Unregister(id ID) {
	delete(registry, id)
}
