package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/ingest/scope"
	"danny.vn/mise/pkg/store"
)

// Discover reads every source of p.Corpus newest-first since its watermark and
// returns the DocRefs to process: in-scope documents whose discovery
// fingerprint changed since the last run. Each enqueued document gets a ledger
// row in state "discovered"; out-of-scope documents are recorded as
// "out_of_scope" (ledger-only bookkeeping — they are not enqueued and not
// counted in IngestResult). Per-source cursors advance to the max IssuedAt
// seen, so re-runs stay cheap; the ledger's fingerprint check keeps overlap
// re-sights idempotent.
//
// A failing source is logged and skipped — its cursor is not advanced, so the
// next run catches up — rather than failing the whole activity; Discover
// errors only when every source failed and nothing was discovered.
//
// Between sources (never after the last one) it waits Deps.PaceBetweenSources
// as a politeness delay; a context cancellation during that wait fails the
// activity immediately instead of hitting the next source.
func (a *Activities) Discover(ctx context.Context, p IngestParams) ([]DocRef, error) {
	desc, err := descriptor(p.Corpus)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	sources := a.deps.Sources[desc.ID]
	if len(sources) == 0 {
		return nil, fmt.Errorf("discover: no sources wired for corpus %q", p.Corpus)
	}

	d := &discovery{
		params:  p,
		desc:    desc,
		matcher: scope.For(desc.Jurisdiction),
		ledger:  store.NewLedger(a.deps.Pool),
		cursor:  store.Cursor(a.deps.Pool),
	}

	var srcErrs []error
	for i, src := range sources {
		if d.full() {
			break
		}
		if err := d.discoverSource(ctx, src); err != nil {
			logger(ctx).Warn("discover source failed; continuing",
				"corpus", p.Corpus, "source", src.ID(), "error", err)
			srcErrs = append(srcErrs, fmt.Errorf("source %s: %w", src.ID(), err))
		}
		heartbeat(ctx, "discovered "+src.ID())

		if i < len(sources)-1 {
			if err := Pace(ctx, a.deps.PaceBetweenSources); err != nil {
				return nil, fmt.Errorf("discover %s: pacing between sources: %w", p.Corpus, err)
			}
		}
	}
	if len(srcErrs) == len(sources) && len(d.refs) == 0 {
		return nil, fmt.Errorf("discover %s: every source failed: %w", p.Corpus, errors.Join(srcErrs...))
	}
	return d.refs, nil
}

// discovery threads one Discover call's state across its sources.
type discovery struct {
	params  IngestParams
	desc    corpus.Descriptor
	matcher *scope.Matcher
	ledger  *store.Ledger
	cursor  *store.CursorStore
	refs    []DocRef
}

// full reports whether the MaxDocs cap has been reached.
func (d *discovery) full() bool {
	return d.params.MaxDocs > 0 && len(d.refs) >= d.params.MaxDocs
}

// discoverSource drains one source's feed since its watermark, enqueues the
// in-scope changed documents, and advances the source's cursor. When MaxDocs
// truncates the walk mid-source the cursor is left untouched: the feed is
// newest-first, so advancing past unprocessed older documents would lose them.
func (d *discovery) discoverSource(ctx context.Context, src ingest.Source) error {
	stored, err := d.cursor.Get(ctx, d.desc.ID, src.ID(), d.params.Keyword)
	if err != nil {
		return err
	}
	since := d.params.Since // operator backfill override when non-zero
	if since.IsZero() {
		since = stored
	}

	docs, err := src.Discover(ctx, since, d.params.Keyword)
	if err != nil {
		return err
	}

	var maxIssued time.Time
	for _, doc := range docs {
		if d.full() {
			return nil // cap reached mid-source: keep the cursor for the tail
		}
		if strings.TrimSpace(doc.ExternalID) == "" {
			continue
		}
		if doc.IssuedAt.After(maxIssued) {
			maxIssued = doc.IssuedAt // advance over every doc seen, in scope or not
		}
		if err := d.record(ctx, src.ID(), doc); err != nil {
			return err
		}
	}

	// Advance only past the STORED watermark, so a Since-override backfill of
	// an old window can never regress the cursor.
	if maxIssued.After(stored) {
		return d.cursor.Set(ctx, d.desc.ID, src.ID(), d.params.Keyword, maxIssued)
	}
	return nil
}

// record applies the scope filter and the ledger fingerprint check to one
// discovered document, enqueueing a DocRef when it needs processing.
func (d *discovery) record(ctx context.Context, sourceID string, doc ingest.DiscoveredDoc) error {
	if in, _ := inScope(d.matcher, d.params.Keyword, doc); !in {
		// Ledger-only bookkeeping. The stored hash stays empty so a later
		// vocabulary change re-evaluates the document instead of matching its
		// fingerprint as unchanged.
		return d.ledger.Upsert(ctx, d.desc.ID, sourceID, doc.ExternalID, "", stateOutOfScope)
	}

	hash := discoveryHash(doc)
	unchanged, err := d.ledger.Unchanged(ctx, d.desc.ID, sourceID, doc.ExternalID, hash)
	if err != nil {
		return err
	}
	if unchanged {
		return nil // seen before with identical metadata — nothing to re-open
	}
	if err := d.ledger.Upsert(ctx, d.desc.ID, sourceID, doc.ExternalID, hash, stateDiscovered); err != nil {
		return err
	}
	d.refs = append(d.refs, DocRef{
		Corpus:      string(d.desc.ID),
		SourceID:    sourceID,
		ExternalID:  doc.ExternalID,
		DetailURL:   doc.DetailURL,
		ContentHash: hash,
	})
	return nil
}

// inScope decides whether a discovered document enters the pipeline. A
// non-empty keyword means the source already filtered server-side (the keyword
// is the scope), so every document is in scope with the keyword as its match
// provenance. Otherwise the jurisdiction matcher decides on the feed's
// number/title/abstract; an empty matcher fails open (scope.Matcher.Empty).
func inScope(m *scope.Matcher, keyword string, doc ingest.DiscoveredDoc) (bool, []string) {
	if keyword != "" {
		return true, []string{keyword}
	}
	if m == nil || m.Empty() {
		return true, nil
	}
	r := m.Match(doc.Number, doc.Title, doc.Abstract)
	return r.InScope, r.Matched
}

// discoveryHash fingerprints the discovery-time fields so re-discovery can
// detect a genuine source change — it never re-opens a completed document
// otherwise. Same recipe as banhmi's pipeline: sha256(Number|Title|DetailURL|
// DocType), lowercase hex.
func discoveryHash(d ingest.DiscoveredDoc) string {
	sum := sha256.Sum256([]byte(d.Number + "|" + d.Title + "|" + d.DetailURL + "|" + string(d.DocType)))
	return hex.EncodeToString(sum[:])
}
