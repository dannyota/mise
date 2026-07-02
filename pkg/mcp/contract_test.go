//go:build integration

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
)

// contractSearcher adapts store.Search to Searcher — the same shape
// cmd/serving's real wiring uses, rebuilt locally so this contract test
// doesn't need to import package main.
type contractSearcher struct {
	pool *pgxpool.Pool
	emb  embed.Embedder
}

func (s contractSearcher) Search(ctx context.Context, query string, opts store.SearchOpts) ([]store.Hit, error) {
	return store.Search(ctx, s.pool, s.emb, query, opts)
}

// contractDocGetter adapts a per-corpus store.Corpus.GetDocument to
// DocGetter, resolving corpusID against a pre-built map — the same shape
// cmd/serving's real wiring uses.
type contractDocGetter struct {
	corpora map[string]*store.Corpus
}

func (g contractDocGetter) GetDocument(
	ctx context.Context, role, corpusID string, docID uuid.UUID,
) (store.DocumentDetail, error) {
	c := g.corpora[corpusID]
	return c.GetDocument(ctx, role, docID)
}

// schemaPath resolves <repoRoot>/api/schemas/<name> from this source
// file's own path (mirrors internal/testdb.migrationsDir), so the test
// works regardless of the invoking directory.
func schemaPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolving contract_test.go source path via runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "api", "schemas", name)
}

// validateAgainstSchema asserts data (a json.Marshal'd tool output) matches
// the JSON Schema at schemaFile.
func validateAgainstSchema(t *testing.T, schemaFile string, data []byte) {
	t.Helper()
	c := jsonschema.NewCompiler()
	sch, err := c.Compile(schemaFile)
	if err != nil {
		t.Fatalf("compiling schema %s: %v", schemaFile, err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unmarshalling tool output for schema validation: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Errorf("output does not validate against %s: %v\noutput: %s", schemaFile, err, data)
	}
}

// contractCorpus returns a store.Corpus bound to id's registered schema.
func contractCorpus(t *testing.T, pool *pgxpool.Pool, id corpus.ID) *store.Corpus {
	t.Helper()
	desc, ok := corpus.Get(id)
	if !ok {
		t.Fatalf("corpus.Get(%s): not registered", id)
	}
	c, err := store.NewCorpus(pool, desc)
	if err != nil {
		t.Fatalf("NewCorpus(%s): %v", id, err)
	}
	return c
}

// TestSearchOutputMatchesSchema seeds a real document/section via store
// writes, calls the search tool handler directly against a store-backed
// Searcher, and validates the marshaled SearchOutput against
// api/schemas/search_output.schema.json.
func TestSearchOutputMatchesSchema(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	emb := embed.NewFake()
	c := contractCorpus(t, pool, corpus.VNReg)

	marker := "contractsearch" + strippedUUID()
	docID, err := c.UpsertDocument(ctx, store.Document{
		CorpusID: string(corpus.VNReg), Title: "Contract Fixture " + marker,
		DocNumber: "doc-" + marker, Language: "vi", ValidityStatus: "in_force",
		AccessTier: string(corpus.TierPublic), ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertDocument() error = %v", err)
	}

	body := marker + " incident reporting deadline obligations"
	vecs, err := emb.Embed(ctx, []string{body})
	if err != nil {
		t.Fatalf("embedding fixture section: %v", err)
	}
	err = c.ReplaceSections(ctx, docID, []store.Section{{
		CorpusID: string(corpus.VNReg), CitationPath: "Điều 7", Body: body,
		ValidityStatus: "in_force", AccessTier: string(corpus.TierPublic), Embedding: vecs[0],
	}})
	if err != nil {
		t.Fatalf("ReplaceSections() error = %v", err)
	}

	h := newSearchHandler(contractSearcher{pool: pool, emb: emb}, "mise_public")
	_, out, err := h(ctx, nil, SearchInput{Query: body, Corpora: []string{string(corpus.VNReg)}})
	if err != nil {
		t.Fatalf("search handler error = %v", err)
	}
	if len(out.Sections) == 0 {
		t.Fatal("search handler returned zero sections, want the seeded fixture")
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshaling SearchOutput: %v", err)
	}
	validateAgainstSchema(t, schemaPath(t, "search_output.schema.json"), data)
}

// TestDocumentOutputMatchesSchema seeds a document with a section and both
// an attributed and an unattributed amendment event, calls the document
// tool handler directly against a store-backed DocGetter, and validates the
// marshaled DocumentOutput against api/schemas/document_output.schema.json.
func TestDocumentOutputMatchesSchema(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := contractCorpus(t, pool, corpus.VNReg)

	marker := "contractdoc" + strippedUUID()
	issued := time.Now().UTC().Truncate(time.Second)
	docID, err := c.UpsertDocument(ctx, store.Document{
		CorpusID: string(corpus.VNReg), Title: "Contract Document Fixture " + marker,
		DocNumber: "doc-" + marker, CitationScheme: "dieu-khoan-diem", Language: "vi",
		ValidityStatus: "amended", IssuingAuthority: "SBV", SignerName: "Governor",
		Version: "1", SourceURL: "https://vbpl.vn/" + marker, SourceSystem: "vbpl",
		ContentType: "text/html", AccessTier: string(corpus.TierPublic),
		IssuedDate: &issued, ObservedAt: issued,
	})
	if err != nil {
		t.Fatalf("UpsertDocument() error = %v", err)
	}

	err = c.ReplaceSections(ctx, docID, []store.Section{{
		CorpusID: string(corpus.VNReg), CitationPath: "Điều 1", HeadingPath: "Chương I ▸ Điều 1",
		Body: "first section text " + marker, ValidityStatus: "in_force", AccessTier: string(corpus.TierPublic),
	}})
	if err != nil {
		t.Fatalf("ReplaceSections() error = %v", err)
	}

	amendingID, err := c.UpsertDocument(ctx, store.Document{
		CorpusID: string(corpus.VNReg), Title: "Amending Fixture " + marker,
		DocNumber: "amend-" + marker, Language: "vi", ValidityStatus: "in_force",
		AccessTier: string(corpus.TierPublic), ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertDocument() amending fixture error = %v", err)
	}

	events := []store.AmendmentEvent{
		{TargetDocID: docID, AmendingDocID: &amendingID, Kind: "amended", Clause: "Điều 5", EventDate: issued},
		// unattributed: amending_doc_id omitted
		{TargetDocID: docID, Kind: "superseded", Clause: "Điều 6", EventDate: issued},
	}
	if err := c.InsertAmendmentEvents(ctx, events); err != nil {
		t.Fatalf("InsertAmendmentEvents() error = %v", err)
	}

	corpora := map[string]*store.Corpus{string(corpus.VNReg): c}
	h := newDocumentHandler(contractDocGetter{corpora: corpora}, "mise_public")
	_, out, err := h(ctx, nil, DocumentInput{CorpusID: string(corpus.VNReg), DocumentID: docID.String()})
	if err != nil {
		t.Fatalf("document handler error = %v", err)
	}
	if len(out.Sections) == 0 {
		t.Fatal("document handler returned zero sections, want the seeded fixture")
	}
	if len(out.Amendments) != 2 {
		t.Fatalf("document handler returned %d amendments, want 2", len(out.Amendments))
	}
	for _, a := range out.Amendments {
		if a.Kind != "amended" && a.Kind != "superseded" {
			t.Errorf("Amendments[_].Kind = %q, want it to round-trip from the seeded event", a.Kind)
		}
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshaling DocumentOutput: %v", err)
	}
	validateAgainstSchema(t, schemaPath(t, "document_output.schema.json"), data)
}

// strippedUUID returns a hyphen-free UUID string — a short, unique,
// alphanumeric marker safe to embed in fixture text/doc numbers so
// concurrent tests sharing testdb.New's one container never collide.
func strippedUUID() string {
	id := uuid.NewString()
	out := make([]byte, 0, len(id))
	for _, r := range id {
		if r != '-' {
			out = append(out, byte(r))
		}
	}
	return string(out)
}
