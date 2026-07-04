package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/blob"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/parse/law"
	"danny.vn/mise/pkg/parse/mylaw"
	"danny.vn/mise/pkg/parse/quality"
	"danny.vn/mise/pkg/parse/vnlaw"
	"danny.vn/mise/pkg/store"
)

// errNoMainContent marks a document whose detail carries neither an inline
// HTML body nor any downloadable file — a permanent data gap, not a transient
// fetch failure, so ProcessDoc records it as "failed" instead of retrying.
var errNoMainContent = errors.New("document has no main content (no inline HTML, no files)")

// embedBatchSize is how many section bodies go to the Embedder per call.
const embedBatchSize = 32

// ProcessDoc runs one document through the full ingest pipeline: fetch detail,
// select + download the main content (raw bytes preserved in the blob store),
// extract text, quality-gate, structure-parse, normalize, embed, and index
// into the corpus store. It returns one of "indexed", "skipped" (this
// discovery fingerprint was already indexed — an idempotent retry), or
// "failed" (a permanent data failure recorded in the ledger); transient
// failures return an error so Temporal retries. It heartbeats between stages.
func (a *Activities) ProcessDoc(ctx context.Context, ref DocRef) (string, error) {
	desc, err := descriptor(ref.Corpus)
	if err != nil {
		return "", fmt.Errorf("process doc: %w", err)
	}
	src, err := a.source(desc.ID, ref.SourceID)
	if err != nil {
		return "", fmt.Errorf("process doc: %w", err)
	}
	ledger := store.NewLedger(a.deps.Pool)

	// Idempotency: a retried activity whose previous attempt already indexed
	// this exact discovery fingerprint has nothing left to do.
	hash, state, found, err := ledger.Entry(ctx, desc.ID, ref.SourceID, ref.ExternalID)
	if err != nil {
		return "", fmt.Errorf("process doc %s/%s: %w", ref.SourceID, ref.ExternalID, err)
	}
	if found && state == stateIndexed && hash == ref.ContentHash {
		return outcomeSkipped, nil
	}

	outcome, docID, err := a.processStages(ctx, desc, src, ref)
	switch {
	case err != nil:
		return "", fmt.Errorf("process doc %s/%s: %w", ref.SourceID, ref.ExternalID, err)
	case outcome != outcomeIndexed:
		return outcome, nil
	}

	if err := ledger.SetState(ctx, desc.ID, ref.SourceID, ref.ExternalID, stateIndexed, ""); err != nil {
		return "", fmt.Errorf("process doc %s/%s: %w", ref.SourceID, ref.ExternalID, err)
	}
	if err := ledger.LinkDocument(ctx, desc.ID, ref.SourceID, ref.ExternalID, docID); err != nil {
		return "", fmt.Errorf("process doc %s/%s: %w", ref.SourceID, ref.ExternalID, err)
	}
	return outcomeIndexed, nil
}

// processStages runs the fetch→extract→gate→parse→index stages. It returns
// outcomeFailed (with the ledger updated) for permanent data failures, and an
// error for transient ones.
func (a *Activities) processStages(
	ctx context.Context, desc corpus.Descriptor, src ingest.Source, ref DocRef,
) (string, uuid.UUID, error) {
	heartbeat(ctx, "fetch "+ref.ExternalID)
	dref := ingest.DetailRef{ExternalID: ref.ExternalID, DetailURL: ref.DetailURL}
	detail, err := src.FetchDetail(ctx, dref)
	if err != nil {
		return "", uuid.UUID{}, fmt.Errorf("fetching detail: %w", err)
	}
	if detail == nil {
		return "", uuid.UUID{}, errors.New("fetching detail: source returned no document")
	}

	content, err := a.fetchMainContent(ctx, src, detail)
	if errors.Is(err, errNoMainContent) {
		return a.failDoc(ctx, desc.ID, ref, err)
	}
	if err != nil {
		return "", uuid.UUID{}, err
	}

	// Diagram corpora bypass text extraction: the raw image is captioned by
	// the Captioner seam, producing a textual description that becomes the
	// section body (what gets embedded). The image itself is referenced via
	// ImageRef on every section.
	if desc.Kind == corpus.KindDiagram {
		heartbeat(ctx, "caption "+ref.ExternalID)
		text, err := a.captionImage(ctx, content.data, content.contentType)
		if err != nil {
			return "", uuid.UUID{}, fmt.Errorf("captioning diagram: %w", err)
		}

		heartbeat(ctx, "index "+ref.ExternalID)
		docID, err := a.index(ctx, desc, *detail, nil, text, ref.RunID, content.blobKey)
		if err != nil {
			return "", uuid.UUID{}, err
		}
		return outcomeIndexed, docID, nil
	}

	heartbeat(ctx, "extract "+ref.ExternalID)
	stopExtract := heartbeatLoop(ctx, "extracting "+ref.ExternalID)
	text, err := a.deps.Extract.Text(ctx, content.data, content.contentType)
	stopExtract()
	if errors.Is(err, ingest.ErrUnsupportedContentType) {
		return a.failDoc(ctx, desc.ID, ref, err)
	}
	if err != nil {
		return "", uuid.UUID{}, fmt.Errorf("extracting text: %w", err)
	}
	if err := quality.Check(text); err != nil {
		return a.failDoc(ctx, desc.ID, ref, err)
	}

	heartbeat(ctx, "parse "+ref.ExternalID)
	tree, err := structureTree(ctx, desc.Jurisdiction, src, dref, text)
	if err != nil {
		return "", uuid.UUID{}, err
	}

	heartbeat(ctx, "index "+ref.ExternalID)
	docID, err := a.index(ctx, desc, *detail, tree, text, ref.RunID, "")
	if err != nil {
		return "", uuid.UUID{}, err
	}
	return outcomeIndexed, docID, nil
}

// failDoc records a permanent data failure in the ledger and returns the
// "failed" outcome. Only the ledger write itself can error (transient).
func (a *Activities) failDoc(
	ctx context.Context, corpusID corpus.ID, ref DocRef, cause error,
) (string, uuid.UUID, error) {
	logger(ctx).Warn("document failed a data gate",
		"corpus", ref.Corpus, "source", ref.SourceID, "external_id", ref.ExternalID, "error", cause)
	ledger := store.NewLedger(a.deps.Pool)
	if err := ledger.SetState(ctx, corpusID, ref.SourceID, ref.ExternalID, stateFailed, cause.Error()); err != nil {
		return "", uuid.UUID{}, fmt.Errorf("recording data failure: %w", err)
	}
	return outcomeFailed, uuid.UUID{}, nil
}

// source resolves a DocRef's source id against the wired sources for corpusID.
func (a *Activities) source(corpusID corpus.ID, sourceID string) (ingest.Source, error) {
	for _, s := range a.deps.Sources[corpusID] {
		if s.ID() == sourceID {
			return s, nil
		}
	}
	return nil, fmt.Errorf("unknown source %q for corpus %s", sourceID, corpusID)
}

// mainContent is the selected main artifact of a document: its raw bytes, the
// content type Extract dispatches on, the bytes' SHA-256 (lowercase hex), and
// the blob key when the bytes were downloaded (empty for an inline HTML body).
type mainContent struct {
	data        []byte
	contentType string
	sha         string
	blobKey     string
}

// fetchMainContent selects and materializes a document's main content: the
// inline HTML body when the source serves one, else the first "main" file
// (falling back to the source's first file — vbpl orders files best-first and
// a scanned PDF still extracts through Doc AI). Downloaded bytes are preserved
// content-addressed in the blob store before any parsing can fail.
func (a *Activities) fetchMainContent(
	ctx context.Context, src ingest.Source, doc *ingest.DiscoveredDoc,
) (mainContent, error) {
	if strings.TrimSpace(doc.HTML) != "" {
		data := []byte(doc.HTML)
		sum := sha256.Sum256(data)
		return mainContent{data: data, contentType: "text/html", sha: hex.EncodeToString(sum[:])}, nil
	}

	file, ok := pickMainFile(doc.Files)
	if !ok {
		return mainContent{}, errNoMainContent
	}
	var buf bytes.Buffer
	stopDownload := heartbeatLoop(ctx, "downloading "+file.Name)
	_, _, err := src.Download(ctx, file, &buf)
	stopDownload()
	if err != nil {
		return mainContent{}, fmt.Errorf("downloading %s: %w", file.Name, err)
	}
	sum := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(sum[:])

	key := blob.Key(sha, fileExt(file))
	if _, err := a.deps.Blob.Put(ctx, key, bytes.NewReader(buf.Bytes())); err != nil {
		return mainContent{}, fmt.Errorf("storing raw file %s: %w", key, err)
	}
	return mainContent{data: buf.Bytes(), contentType: contentTypeFor(file), sha: sha, blobKey: key}, nil
}

// captionImage produces a text caption for a diagram image using the wired
// Captioner. When no Captioner is configured (nil deps.Captioner — a
// programming error for the diagram corpus but harmless), it returns a
// placeholder so the pipeline never crashes on a missing optional dep.
func (a *Activities) captionImage(ctx context.Context, data []byte, contentType string) (string, error) {
	if a.deps.Captioner == nil {
		return fmt.Sprintf("[no captioner configured for %d-byte image]", len(data)), nil
	}
	result, err := a.deps.Captioner.Caption(ctx, data, contentType)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// pickMainFile returns the first file whose Kind is "main", falling back to
// the first file at all (sources order files best-first).
func pickMainFile(files []ingest.FileRef) (ingest.FileRef, bool) {
	for _, f := range files {
		if f.Kind == "main" {
			return f, true
		}
	}
	if len(files) > 0 {
		return files[0], true
	}
	return ingest.FileRef{}, false
}

// fileExt renders a FileRef's extension in blob.Key's leading-dot form.
func fileExt(f ingest.FileRef) string {
	if f.Ext == "" {
		return ""
	}
	return "." + f.Ext
}

// contentTypeFor returns the content type Extract should dispatch on for a
// file: its scraped MIMEType when present, else a best-effort mapping from the
// extension. An unknown extension returns "" and fails the extract gate as
// unsupported.
func contentTypeFor(f ingest.FileRef) string {
	if f.MIMEType != "" {
		return f.MIMEType
	}
	switch f.Ext {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "doc":
		return "application/msword"
	case "html", "htm":
		return "text/html"
	case "txt":
		return "text/plain"
	default:
		return ""
	}
}

// structureTree parses extracted text (plus the source's first-party provision
// tree, when it has one) into the shared law.Node hierarchy for jurisdiction.
// An unknown jurisdiction returns a nil tree: Normalize then falls back to one
// whole-document section, so content is never dropped.
func structureTree(
	ctx context.Context, jurisdiction string, src ingest.Source, dref ingest.DetailRef, text string,
) ([]*law.Node, error) {
	switch jurisdiction {
	case "vn":
		return vnStructure(ctx, src, dref, text)
	case "my":
		return mylaw.Parse(text)
	default:
		return nil, nil
	}
}

// vnStructure prefers the source's first-party provision tree (vbpl) and falls
// back to the vnlaw text parser whenever the tree is unavailable, undecodable,
// or — the ParseTree contract's trap — decodes to nodes that carry no content
// at all (law.Flatten returns nothing), which would otherwise silently drop
// the document's text. A hard FetchTree error is transient and propagates so
// the activity retries.
func vnStructure(
	ctx context.Context, src ingest.Source, dref ingest.DetailRef, text string,
) ([]*law.Node, error) {
	tp, ok := src.(ingest.TreeProvider)
	if !ok {
		return vnlaw.Parse(text)
	}
	payload, ok, err := tp.FetchTree(ctx, dref)
	if err != nil {
		return nil, fmt.Errorf("fetching provision tree: %w", err)
	}
	if !ok {
		return vnlaw.Parse(text)
	}
	tree, err := vnlaw.ParseTree(payload)
	if err != nil {
		// ErrInvalidProvisionTree / ErrEmptyProvisionTree: the source never
		// delivered a usable tree — fall back per the ParseTree contract.
		return vnlaw.Parse(text)
	}
	if len(law.Flatten(tree)) == 0 {
		// A contentless tree returns (tree, nil), NOT an error — using it
		// would index the document with zero section bodies.
		return vnlaw.Parse(text)
	}
	return tree, nil
}

// parseRunID parses runID — a DocRef.RunID, the ingest.run id the workflow
// stamps onto every ref after Discover returns (see IngestCorpusWorkflow) —
// into a uuid.UUID for Document.IngestRunID. An empty runID means ProcessDoc
// was invoked without going through the workflow's stamp, which is always a
// bug: it fails loudly here instead of minting a throwaway uuid.New() the way
// the old CurrentRun heuristic did for its "no open run" case.
func parseRunID(runID string) (uuid.UUID, error) {
	if runID == "" {
		return uuid.UUID{}, errors.New("missing run id")
	}
	id, err := uuid.Parse(runID)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("parsing run id %q: %w", runID, err)
	}
	return id, nil
}

// index normalizes the document, embeds its section bodies, and writes the
// document, sections, and resolved amendment events to the corpus store.
// runID is the DocRef.RunID the workflow stamped after Discover returned.
// imageRef, when non-empty, is stamped onto every section (diagram corpora).
func (a *Activities) index(
	ctx context.Context, desc corpus.Descriptor, doc ingest.DiscoveredDoc, tree []*law.Node, text, runID, imageRef string,
) (uuid.UUID, error) {
	parsedRunID, err := parseRunID(runID)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("index: %w", err)
	}
	now := time.Now().UTC()

	norm, err := ingest.Normalize(desc, doc, tree, text, parsedRunID, now)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("normalizing: %w", err)
	}
	if imageRef != "" {
		for i := range norm.Sections {
			norm.Sections[i].ImageRef = imageRef
		}
	}
	if err := a.embedSections(ctx, norm.Sections); err != nil {
		return uuid.UUID{}, err
	}

	c, err := store.NewCorpus(a.deps.Pool, desc)
	if err != nil {
		return uuid.UUID{}, err
	}
	docID, err := c.UpsertDocument(ctx, norm.Doc)
	if err != nil {
		return uuid.UUID{}, err
	}
	if err := c.ReplaceSections(ctx, docID, norm.Sections); err != nil {
		return uuid.UUID{}, err
	}
	if err := reapplyIncomingEvents(ctx, c, docID, now); err != nil {
		return uuid.UUID{}, err
	}
	if err := applyRelations(ctx, c, docID, norm.RelationEvents, now); err != nil {
		return uuid.UUID{}, err
	}
	return docID, nil
}

// reapplyIncomingEvents re-derives docID's validity_status from every
// amendment event already recorded against it as a TARGET — necessary
// because UpsertDocument's update path just overwrote validity_status with
// norm.Doc's source-derived value (Normalize's MapValidity), which knows
// nothing about amendment events some OTHER, already-indexed document
// recorded against docID in an earlier run (applyRelations, below). Without
// this, a changed TARGET document re-indexed by the pipeline would silently
// regress from "amended"/"superseded"/"repealed" back to whatever the
// source itself currently reports (typically "in_force").
//
// Replaying every stored event in event_date order — not just the latest —
// reproduces Transition's order-sensitive last-non-repeal-wins/
// repeal-is-absorbing semantics exactly as if each had been applied live;
// TransitionAt's own future-date gate makes this safe to call unconditionally
// (a not-yet-due event is a no-op), and TransitionValidity's no-op-when-
// unchanged write makes it cheap to call on every index, not just a genuine
// regression.
func reapplyIncomingEvents(ctx context.Context, c *store.Corpus, docID uuid.UUID, now time.Time) error {
	events, err := c.EventsForTarget(ctx, docID)
	if err != nil {
		return fmt.Errorf("loading incoming amendment events for %s: %w", docID, err)
	}
	for _, ev := range events {
		if _, err := c.TransitionValidity(ctx, docID, func(current string) string {
			return ingest.TransitionAt(current, ev.Kind, ev.EventDate, now)
		}); err != nil {
			return fmt.Errorf("re-applying amendment event onto %s: %w", docID, err)
		}
	}
	return nil
}

// embedSections fills each section's Embedding, batching bodies through the
// Embedder embedBatchSize at a time, heartbeating both during a single slow
// batch call (heartbeatLoop) and once after each batch completes.
func (a *Activities) embedSections(ctx context.Context, secs []store.Section) error {
	for start := 0; start < len(secs); start += embedBatchSize {
		end := min(start+embedBatchSize, len(secs))
		texts := make([]string, 0, end-start)
		for _, s := range secs[start:end] {
			texts = append(texts, s.Body)
		}
		stopEmbed := heartbeatLoop(ctx, fmt.Sprintf("embedding %d/%d", end, len(secs)))
		vecs, err := a.deps.Embedder.Embed(ctx, texts)
		stopEmbed()
		if err != nil {
			return fmt.Errorf("embedding sections %d-%d: %w", start, end, err)
		}
		if len(vecs) != len(texts) {
			return fmt.Errorf("embedding sections %d-%d: got %d vectors for %d texts", start, end, len(vecs), len(texts))
		}
		for i, v := range vecs {
			secs[start+i].Embedding = v
		}
		heartbeat(ctx, fmt.Sprintf("embedded %d/%d", end, len(secs)))
	}
	return nil
}

// applyRelations resolves the document's pending relation events against the
// store: each target found by citation number gets an amendment_event row
// attributed to docID, and its validity transitions atomically per
// TransitionAt (future-dated events are recorded but do not change current
// validity). Targets not in the store yet are skipped — the event
// re-materializes when the source re-publishes, and a later backfill pass can
// sweep the remainder.
//
// The validity read-and-write is one TransitionValidity call, not a
// GetValidity/SetValidity pair: two ProcessDoc activities racing an amendment
// event onto the same target document must never interleave a plain
// read-modify-write, or one's write silently overwrites the other's.
// TransitionValidity's row lock (SELECT ... FOR UPDATE) serializes them.
func applyRelations(
	ctx context.Context, c *store.Corpus, docID uuid.UUID, evs []ingest.RelationEvent, now time.Time,
) error {
	var rows []store.AmendmentEvent
	for _, ev := range evs {
		targetID, found, err := c.FindDocIDByNumber(ctx, ev.TargetDocNumber)
		if err != nil {
			return err
		}
		if !found || targetID == docID {
			continue
		}
		rows = append(rows, store.AmendmentEvent{
			TargetDocID:   targetID,
			AmendingDocID: new(docID),
			Kind:          ev.Kind,
			Clause:        ev.Clause,
			EventDate:     ev.Date,
		})
		if _, err := c.TransitionValidity(ctx, targetID, func(current string) string {
			return ingest.TransitionAt(current, ev.Kind, ev.Date, now)
		}); err != nil {
			return err
		}
	}
	return c.InsertAmendmentEvents(ctx, rows)
}
