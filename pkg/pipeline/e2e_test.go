//go:build integration

// e2e_test.go drives the real IngestCorpusWorkflow end to end against
// testdb: a static, in-memory two-document VN fixture source (the second
// document amends the first), through Discover -> ProcessDoc -> index with
// every activity real, then re-verifies the ledger-level idempotent-retry
// contract and the read surfaces (store.Search, the MCP search tool) over
// the result.
package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/blob"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/mcp"
	"danny.vn/mise/pkg/pipeline"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
	"danny.vn/mise/pkg/vertex"
)

// --- fixture doc/source ---------------------------------------------------

const (
	fixtureSourceID = "fixture"
	doc1ExternalID  = "doc-1"
	doc2ExternalID  = "doc-2"
	doc1Number      = "15/2024/TT-NHNN"
	doc2Number      = "20/2025/TT-NHNN"
)

// seededPhrase is a distinctive substring of doc1's Điều 3 body. The fake
// embedder's vectors aren't semantically meaningful (pkg/rag/embed), so
// finding it back through store.Search/the MCP tool exercises the hybrid
// search's lexical (FTS) arm, not the vector one.
const seededPhrase = "giám sát an toàn hệ thống thông tin ngân hàng điện tử"

// doc1HTML and doc2HTML reuse vnlaw's Chương/Điều/Khoản shape
// (pkg/parse/vnlaw's viSnippet fixture) as HTML paragraphs: htmltext.Text
// turns each <p> into its own line, the one-line-per-block discipline
// vnlaw.Parse expects. Both titles carry vnStrong scope terms directly
// (pkg/ingest/scope/vocab.go: "an toàn hệ thống thông tin", "ngân hàng điện
// tử") so Discover's scope matcher admits them regardless of issuer/signal.
const doc1HTML = `<p>Chương I</p>
<p>QUY ĐỊNH CHUNG</p>
<p>Điều 1. Phạm vi điều chỉnh</p>
<p>Thông tư này quy định về an toàn hệ thống thông tin trong hoạt động ngân hàng điện tử của tổ chức tín dụng.</p>
<p>Điều 2. Đối tượng áp dụng</p>
<p>Thông tư này áp dụng đối với tổ chức tín dụng, chi nhánh ngân hàng nước ngoài cung cấp dịch vụ ngân hàng điện tử.</p>
<p>Chương II</p>
<p>QUY ĐỊNH CỤ THỂ</p>
<p>Điều 3. Biện pháp bảo đảm an toàn</p>
<p>Tổ chức tín dụng phải triển khai biện pháp giám sát an toàn hệ thống thông tin ngân hàng điện tử</p>
<p>theo tiêu chuẩn kỹ thuật của Ngân hàng Nhà nước.</p>`

const doc2HTML = `<p>Điều 1. Sửa đổi, bổ sung Điều 3</p>
<p>Sửa đổi, bổ sung Điều 3 Thông tư số 15/2024/TT-NHNN quy định về biện pháp giám sát</p>
<p>an toàn hệ thống thông tin ngân hàng điện tử.</p>
<p>Điều 2. Hiệu lực thi hành</p>
<p>Thông tư này có hiệu lực thi hành kể từ ngày 01 tháng 02 năm 2025.</p>`

// fixtureDocs returns the two-document fixture feed: doc2 amends doc1
// (Relation.Type "amends" maps to ingest.StatusAmended — pkg/ingest/
// validity.go's relationEventKinds), both dated in the past so
// ingest.TransitionAt applies the amendment immediately rather than
// recording a future-dated event.
func fixtureDocs() []ingest.DiscoveredDoc {
	return []ingest.DiscoveredDoc{
		{
			SourceID: fixtureSourceID, ExternalID: doc1ExternalID, Number: doc1Number,
			Title:       "Thông tư quy định về an toàn hệ thống thông tin trong hoạt động ngân hàng điện tử",
			DocType:     "Thông tư",
			DetailURL:   "https://fixture.test/" + doc1ExternalID,
			IssuedAt:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			EffectiveAt: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
			HTML:        doc1HTML,
		},
		{
			SourceID: fixtureSourceID, ExternalID: doc2ExternalID, Number: doc2Number,
			Title: "Thông tư sửa đổi, bổ sung một số điều của Thông tư quy định về an toàn hệ thống " +
				"thông tin trong hoạt động ngân hàng điện tử",
			DocType:     "Thông tư",
			DetailURL:   "https://fixture.test/" + doc2ExternalID,
			IssuedAt:    time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			EffectiveAt: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			HTML:        doc2HTML,
			Relations:   []ingest.Relation{{Type: "amends", TargetNumber: doc1Number}},
		},
	}
}

// fixtureSource is a static, in-memory ingest.Source: an HTML-only feed
// (neither TreeProvider nor NumberSearcher), so ProcessDoc's vnStructure
// falls back to vnlaw.Parse directly — the extractor's vertex.NewFakeParser
// is wired only because Deps.Extract is mandatory; it is never reached
// since fetchMainContent always finds an inline HTML body here.
type fixtureSource struct{ docs []ingest.DiscoveredDoc }

func newFixtureSource() *fixtureSource { return &fixtureSource{docs: fixtureDocs()} }

func (s *fixtureSource) ID() string { return fixtureSourceID }

// Discover returns the feed-level projection of every fixture doc,
// regardless of since/keyword: this fixture is a fixed, two-document feed.
func (s *fixtureSource) Discover(context.Context, time.Time, string) ([]ingest.DiscoveredDoc, error) {
	out := make([]ingest.DiscoveredDoc, len(s.docs))
	for i, d := range s.docs {
		out[i] = ingest.DiscoveredDoc{
			SourceID: d.SourceID, ExternalID: d.ExternalID, Number: d.Number,
			Title: d.Title, DocType: d.DocType, DetailURL: d.DetailURL, IssuedAt: d.IssuedAt,
		}
	}
	return out, nil
}

func (s *fixtureSource) FetchDetail(_ context.Context, ref ingest.DetailRef) (*ingest.DiscoveredDoc, error) {
	for _, d := range s.docs {
		if d.ExternalID == ref.ExternalID {
			cp := d
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("fixture: unknown external id %q", ref.ExternalID)
}

// Download is never called: both fixture documents serve an inline HTML
// body, so ProcessDoc's fetchMainContent never reaches the file path.
func (s *fixtureSource) Download(context.Context, ingest.FileRef, io.Writer) (int64, string, error) {
	return 0, "", errors.New("fixture: inline HTML only, no files to download")
}

func newFixtureDeps(t *testing.T, pool *pgxpool.Pool) pipeline.Deps {
	t.Helper()
	return pipeline.Deps{
		Pool:     pool,
		Blob:     blob.NewFS(t.TempDir()),
		Embedder: embed.NewFake(),
		Extract:  ingest.NewExtractor(vertex.NewFakeParser()),
		Sources:  map[corpus.ID][]ingest.Source{corpus.VNReg: {newFixtureSource()}},
	}
}

// --- the end-to-end test ---------------------------------------------------

// TestIngestCorpusWorkflowEndToEnd drives IngestCorpusWorkflow with a real
// Discover call up front (captured once, since a fixture-content-unchanged
// re-Discover is itself idempotent and would return nothing to replay —
// see Ledger.Unchanged), then two single-document workflow runs (doc1,
// then doc2 — deterministic order, since doc2's ProcessDoc resolves its
// amendment against doc1 by doc_number and must find it already indexed),
// then a re-run workflow replaying both refs together to prove ProcessDoc's
// ledger-level idempotent-retry contract.
func TestIngestCorpusWorkflowEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	deps := newFixtureDeps(t, pool)
	a := pipeline.NewActivities(deps)
	params := pipeline.IngestParams{Corpus: string(corpus.VNReg)}

	refs, err := a.Discover(ctx, params)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("Discover() returned %d refs, want 2", len(refs))
	}
	ref1, ref2 := findRef(t, refs, doc1ExternalID), findRef(t, refs, doc2ExternalID)

	assertIngestResult(t, runIngestWorkflow(t, a, params, []pipeline.DocRef{ref1}),
		pipeline.IngestResult{Discovered: 1, Processed: 1})
	assertIngestResult(t, runIngestWorkflow(t, a, params, []pipeline.DocRef{ref2}),
		pipeline.IngestResult{Discovered: 1, Processed: 1})

	c := newVNCorpus(t, pool)
	doc1ID, secs1 := assertDocumentSections(t, ctx, c, doc1Number)
	doc2ID, secs2 := assertDocumentSections(t, ctx, c, doc2Number)
	assertAmendmentApplied(t, ctx, c, doc1ID, doc2ID)
	assertLedgerIndexed(t, ctx, pool, doc1ExternalID)
	assertLedgerIndexed(t, ctx, pool, doc2ExternalID)

	// Re-discover: call the REAL Discover again with the same params, now
	// that both fixture docs are ledger-indexed. This is the FIRST of two
	// idempotency layers — Discover's own Ledger.Unchanged fingerprint
	// check (discover.go's record): the fixture source still reports the
	// same two documents, but their discovery hash (Number|Title|
	// DetailURL|DocType) is unchanged, so record() short-circuits before
	// enqueueing any DocRef.
	refs2, err := a.Discover(ctx, params)
	if err != nil {
		t.Fatalf("Discover() re-run error = %v", err)
	}
	if len(refs2) != 0 {
		t.Fatalf("Discover() re-run returned %d refs, want 0 (ledger-deduped)", len(refs2))
	}

	// Re-run: replay the SAME two refs (same ContentHash Discover already
	// recorded) through a fresh workflow execution. This is the SECOND
	// idempotency layer — ProcessDoc's own idempotency check, distinct from
	// Discover's above: both refs are already "indexed" in the ledger, so
	// ProcessDoc short-circuits before touching the store — see
	// process.go's ProcessDoc doc comment ("an idempotent retry").
	rerun := runIngestWorkflow(t, a, params, []pipeline.DocRef{ref1, ref2})
	assertIngestResult(t, rerun, pipeline.IngestResult{Discovered: 2, Skipped: 2})

	if _, n := assertDocumentSections(t, ctx, c, doc1Number); n != secs1 {
		t.Errorf("doc1 section count after re-run = %d, want unchanged %d", n, secs1)
	}
	if _, n := assertDocumentSections(t, ctx, c, doc2Number); n != secs2 {
		t.Errorf("doc2 section count after re-run = %d, want unchanged %d", n, secs2)
	}
	assertAmendmentApplied(t, ctx, c, doc1ID, doc2ID) // still exactly 1 event, not duplicated

	assertSearchFindsSeededPhrase(t, ctx, pool, deps.Embedder, doc1ID)
	assertMCPSearchMatchesSchema(t, ctx, pool, deps.Embedder, c)
}

// findRef returns refs' entry for externalID, failing the test if absent.
func findRef(t *testing.T, refs []pipeline.DocRef, externalID string) pipeline.DocRef {
	t.Helper()
	for _, r := range refs {
		if r.ExternalID == externalID {
			return r
		}
	}
	t.Fatalf("Discover() did not return a ref for external id %q", externalID)
	return pipeline.DocRef{}
}

// runIngestWorkflow drives IngestCorpusWorkflow through a fresh
// TestWorkflowEnvironment with a's real StartRun/ProcessDoc/FinishRun
// activities; Discover is mocked to replay refs (captured from one
// authentic a.Discover call — see TestIngestCorpusWorkflowEndToEnd), so the
// caller controls exactly which document(s) one run processes and in what
// order, without racing ProcessDoc activities that resolve relations
// against each other.
func runIngestWorkflow(
	t *testing.T, a *pipeline.Activities, params pipeline.IngestParams, refs []pipeline.DocRef,
) pipeline.IngestResult {
	t.Helper()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(pipeline.IngestCorpusWorkflow)
	env.RegisterActivity(a)
	env.OnActivity(a.Discover, mock.Anything, mock.Anything).Return(refs, nil).Once()

	env.ExecuteWorkflow(pipeline.IngestCorpusWorkflow, params)
	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var res pipeline.IngestResult
	if err := env.GetWorkflowResult(&res); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	return res
}

func assertIngestResult(t *testing.T, got, want pipeline.IngestResult) {
	t.Helper()
	if got != want {
		t.Errorf("IngestResult = %+v, want %+v", got, want)
	}
}

// --- store-level assertions -------------------------------------------------

func newVNCorpus(t *testing.T, pool *pgxpool.Pool) *store.Corpus {
	t.Helper()
	desc, ok := corpus.Get(corpus.VNReg)
	if !ok {
		t.Fatal("corpus.Get(vn-reg): not registered")
	}
	c, err := store.NewCorpus(pool, desc)
	if err != nil {
		t.Fatalf("NewCorpus(vn-reg): %v", err)
	}
	return c
}

// assertDocumentSections resolves docNumber to its store row and asserts it
// has at least one section, each with a 1536-d embedding, the public access
// tier, and positions 0..n matching their slice index. It returns the
// document's id and section count for before/after re-run comparisons.
func assertDocumentSections(
	t *testing.T, ctx context.Context, c *store.Corpus, docNumber string,
) (uuid.UUID, int) {
	t.Helper()
	docID, found, err := c.FindDocIDByNumber(ctx, docNumber)
	if err != nil || !found {
		t.Fatalf("FindDocIDByNumber(%q) = _, %v, %v, want found", docNumber, found, err)
	}
	detail, err := c.GetDocument(ctx, "", docID)
	if err != nil {
		t.Fatalf("GetDocument(%s) error = %v", docNumber, err)
	}
	if len(detail.Sections) == 0 {
		t.Fatalf("document %q has zero sections", docNumber)
	}
	for i, s := range detail.Sections {
		if len(s.Embedding) != 1536 {
			t.Errorf("document %q section %d embedding len = %d, want 1536", docNumber, i, len(s.Embedding))
		}
		if s.AccessTier != string(corpus.TierPublic) {
			t.Errorf("document %q section %d access_tier = %q, want %q", docNumber, i, s.AccessTier, corpus.TierPublic)
		}
		if s.Position != i {
			t.Errorf("document %q section %d position = %d, want %d", docNumber, i, s.Position, i)
		}
	}
	return docID, len(detail.Sections)
}

// assertAmendmentApplied asserts targetID carries exactly one amendment
// event attributed to amendingID and has transitioned to StatusAmended.
func assertAmendmentApplied(t *testing.T, ctx context.Context, c *store.Corpus, targetID, amendingID uuid.UUID) {
	t.Helper()
	detail, err := c.GetDocument(ctx, "", targetID)
	if err != nil {
		t.Fatalf("GetDocument(target) error = %v", err)
	}
	if len(detail.Events) != 1 {
		t.Fatalf("target document amendment events = %d, want 1", len(detail.Events))
	}
	if detail.Events[0].AmendingDocID == nil || *detail.Events[0].AmendingDocID != amendingID {
		t.Errorf("amendment event AmendingDocID = %v, want %v", detail.Events[0].AmendingDocID, amendingID)
	}
	if detail.Doc.ValidityStatus != ingest.StatusAmended {
		t.Errorf("target document validity_status = %q, want %q", detail.Doc.ValidityStatus, ingest.StatusAmended)
	}
}

func assertLedgerIndexed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, externalID string) {
	t.Helper()
	_, state, found, err := store.NewLedger(pool).Entry(ctx, corpus.VNReg, fixtureSourceID, externalID)
	if err != nil || !found || state != "indexed" {
		t.Errorf("ledger entry(%q) = state %q found %v err %v, want indexed/true/nil", externalID, state, found, err)
	}
}

func assertSearchFindsSeededPhrase(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, emb embed.Embedder, docID uuid.UUID,
) {
	t.Helper()
	hits, err := store.Search(ctx, pool, emb, seededPhrase, store.SearchOpts{
		Corpora: []corpus.ID{corpus.VNReg}, Role: "mise_public",
	})
	if err != nil {
		t.Fatalf("store.Search() error = %v", err)
	}
	for _, h := range hits {
		if h.DocumentID == docID {
			return
		}
	}
	t.Errorf("store.Search(%q) = %d hits, none from document %s", seededPhrase, len(hits), docID)
}

// --- MCP contract check ------------------------------------------------

// e2eSearcher/e2eDocGetter adapt store.Search/store.Corpus.GetDocument to
// mcp.Searcher/mcp.DocGetter — the same shape cmd/serving's real wiring
// uses, rebuilt locally since pkg/mcp's own unexported handler constructors
// aren't reachable from this package (see pkg/mcp/contract_test.go for the
// same pattern from inside package mcp).
type e2eSearcher struct {
	pool *pgxpool.Pool
	emb  embed.Embedder
}

func (s e2eSearcher) Search(ctx context.Context, query string, opts store.SearchOpts) ([]store.Hit, error) {
	return store.Search(ctx, s.pool, s.emb, query, opts)
}

type e2eDocGetter struct{ c *store.Corpus }

func (g e2eDocGetter) GetDocument(
	ctx context.Context, role, _ string, docID uuid.UUID,
) (store.DocumentDetail, error) {
	return g.c.GetDocument(ctx, role, docID)
}

// assertMCPSearchMatchesSchema mounts a real mcp.Server (WithEvidence) on
// an httptest server, calls the "search" tool over the streamable-HTTP
// transport with a real MCP client, and validates the marshaled structured
// output against api/schemas/search_output.schema.json (mirrors
// pkg/mcp/contract_test.go's pattern, over the wire instead of a direct
// handler call, since that package's handler constructors are unexported).
func assertMCPSearchMatchesSchema(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, emb embed.Embedder, c *store.Corpus,
) {
	t.Helper()
	srv := mcp.New(mcp.WithEvidence(e2eSearcher{pool, emb}, e2eDocGetter{c}, "mise_public"))
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "e2e-test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: httpSrv.URL}, nil)
	if err != nil {
		t.Fatalf("connecting mcp client: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "search",
		Arguments: mcp.SearchInput{Query: seededPhrase, Corpora: []string{string(corpus.VNReg)}},
	})
	if err != nil {
		t.Fatalf("CallTool(search) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(search) tool error: %+v", res.Content)
	}

	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshaling structured content: %v", err)
	}
	validateAgainstSchema(t, schemaPath(t, "search_output.schema.json"), data)
}

// schemaPath resolves <repoRoot>/api/schemas/<name> from this source file's
// own path (mirrors internal/testdb.migrationsDir).
func schemaPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolving e2e_test.go source path via runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "api", "schemas", name)
}

func validateAgainstSchema(t *testing.T, schemaFile string, data []byte) {
	t.Helper()
	comp := jsonschema.NewCompiler()
	sch, err := comp.Compile(schemaFile)
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
