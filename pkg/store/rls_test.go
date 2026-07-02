//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
)

// rlsFixture is one group_std (group-confidential) + one local_policy
// (local-confidential) document/section pair, inserted as the pool's
// connecting owner role — which bypasses RLS on write, exactly like
// corpus_test.go's TestGetDocumentMasksConfidentialRowFromPublicRole
// fixture. Both sections carry the exact same distinctive marker text, so
// an exact-text-match query would hit both if RLS did not gate them.
type rlsFixture struct {
	query      string
	groupDocID uuid.UUID
	localDocID uuid.UUID
	groupPath  string
	localPath  string
}

func newRLSFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, emb embed.Embedder) rlsFixture {
	t.Helper()
	m := searchMarker("zzrls")
	query := m + " confidential internal control text"

	cGroup := newCorpus(t, pool, corpus.GroupStd)
	groupDocID := upsertSearchDoc(t, ctx, cGroup, corpus.GroupStd, corpus.TierGroupConfidential, "rls-group-"+m, nil)
	groupPath := "rls-group-section-" + m
	writeSearchSection(t, ctx, cGroup, corpus.GroupStd, corpus.TierGroupConfidential, groupDocID, groupPath,
		query, "in_force", emb)

	cLocal := newCorpus(t, pool, corpus.LocalPolicy)
	localDocID := upsertSearchDoc(t, ctx, cLocal, corpus.LocalPolicy, corpus.TierLocalConfidential, "rls-local-"+m, nil)
	localPath := "rls-local-section-" + m
	writeSearchSection(t, ctx, cLocal, corpus.LocalPolicy, corpus.TierLocalConfidential, localDocID, localPath,
		query, "in_force", emb)

	return rlsFixture{
		query: query, groupDocID: groupDocID, localDocID: localDocID,
		groupPath: groupPath, localPath: localPath,
	}
}

// TestSearchRLSDenySuite is the M1a RLS gate. mise_public must never see
// group-confidential or local-confidential rows — even on an exact-text
// match against their own marker; mise_group sees group- but not
// local-confidential; mise_local sees both. store.Search's own results and
// a raw SELECT count(*) run under SET LOCAL ROLE (bypassing Search
// entirely — an independent oracle) must agree on every case. Each
// assertion group is its own top-level helper (not an inline closure) so
// this driver stays under the cognitive-complexity lint budget.
func TestSearchRLSDenySuite(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	emb := embed.NewFake()
	fx := newRLSFixture(t, ctx, pool, emb)

	t.Run("mise_public sees neither group nor local confidential row", func(t *testing.T) {
		assertPublicSeesNoConfidentialRow(t, ctx, pool, emb, fx)
	})
	t.Run("mise_group sees group but not local confidential row", func(t *testing.T) {
		assertGroupSeesGroupNotLocal(t, ctx, pool, emb, fx)
	})
	t.Run("mise_local sees both group and local confidential rows", func(t *testing.T) {
		assertLocalSeesBothTiers(t, ctx, pool, emb, fx)
	})
	t.Run("raw SELECT count(*) per role confirms the same visibility", func(t *testing.T) {
		assertRawCountsMatchVisibility(t, ctx, pool, fx)
	})
}

// assertPublicSeesNoConfidentialRow is TestSearchRLSDenySuite's mise_public
// case: neither fixture section, nor any hit from a confidential corpus,
// may appear — even though the query is an exact match on their body text.
func assertPublicSeesNoConfidentialRow(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, emb embed.Embedder, fx rlsFixture,
) {
	t.Helper()
	hits, err := store.Search(ctx, pool, emb, fx.query, store.SearchOpts{
		Role: "mise_public", TopK: 50, InForceOnly: false,
	})
	if err != nil {
		t.Fatalf("Search(mise_public) error = %v", err)
	}
	if containsCitationPath(hits, fx.groupPath) {
		t.Errorf("Search(mise_public) hits = %v, want group-confidential section %q ABSENT",
			hitCitationPaths(hits), fx.groupPath)
	}
	if containsCitationPath(hits, fx.localPath) {
		t.Errorf("Search(mise_public) hits = %v, want local-confidential section %q ABSENT",
			hitCitationPaths(hits), fx.localPath)
	}
	for _, h := range hits {
		if isConfidentialCorpus(h.CorpusID) {
			t.Errorf("Search(mise_public) returned a hit from confidential corpus %q: %+v", h.CorpusID, h)
		}
	}
}

// assertGroupSeesGroupNotLocal is TestSearchRLSDenySuite's mise_group case.
func assertGroupSeesGroupNotLocal(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, emb embed.Embedder, fx rlsFixture,
) {
	t.Helper()
	hits, err := store.Search(ctx, pool, emb, fx.query, store.SearchOpts{
		Role: "mise_group", TopK: 50, InForceOnly: false,
	})
	if err != nil {
		t.Fatalf("Search(mise_group) error = %v", err)
	}
	if !containsCitationPath(hits, fx.groupPath) {
		t.Errorf("Search(mise_group) hits = %v, want group-confidential section %q PRESENT",
			hitCitationPaths(hits), fx.groupPath)
	}
	if containsCitationPath(hits, fx.localPath) {
		t.Errorf("Search(mise_group) hits = %v, want local-confidential section %q ABSENT",
			hitCitationPaths(hits), fx.localPath)
	}
}

// assertLocalSeesBothTiers is TestSearchRLSDenySuite's mise_local case.
func assertLocalSeesBothTiers(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, emb embed.Embedder, fx rlsFixture,
) {
	t.Helper()
	hits, err := store.Search(ctx, pool, emb, fx.query, store.SearchOpts{
		Role: "mise_local", TopK: 50, InForceOnly: false,
	})
	if err != nil {
		t.Fatalf("Search(mise_local) error = %v", err)
	}
	if !containsCitationPath(hits, fx.groupPath) {
		t.Errorf("Search(mise_local) hits = %v, want group-confidential section %q PRESENT",
			hitCitationPaths(hits), fx.groupPath)
	}
	if !containsCitationPath(hits, fx.localPath) {
		t.Errorf("Search(mise_local) hits = %v, want local-confidential section %q PRESENT",
			hitCitationPaths(hits), fx.localPath)
	}
}

// assertRawCountsMatchVisibility is TestSearchRLSDenySuite's independent
// oracle: a raw SELECT count(*) under SET LOCAL ROLE, per role/schema pair,
// bypassing store.Search entirely.
func assertRawCountsMatchVisibility(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx rlsFixture) {
	t.Helper()
	cases := []struct {
		role        string
		schema      string
		docID       uuid.UUID
		wantVisible bool
	}{
		{role: "mise_public", schema: "group_std", docID: fx.groupDocID, wantVisible: false},
		{role: "mise_public", schema: "local_policy", docID: fx.localDocID, wantVisible: false},
		{role: "mise_group", schema: "group_std", docID: fx.groupDocID, wantVisible: true},
		{role: "mise_group", schema: "local_policy", docID: fx.localDocID, wantVisible: false},
		{role: "mise_local", schema: "group_std", docID: fx.groupDocID, wantVisible: true},
		{role: "mise_local", schema: "local_policy", docID: fx.localDocID, wantVisible: true},
	}
	for _, tc := range cases {
		got := rawVisibleCount(t, ctx, pool, tc.role, tc.schema, tc.docID)
		want := 0
		if tc.wantVisible {
			want = 1
		}
		if got != want {
			t.Errorf("raw count on %s.section as %s = %d, want %d", tc.schema, tc.role, got, want)
		}
	}
}

// isConfidentialCorpus reports whether id is one of the two internal-tier
// corpora (group-confidential/local-confidential) — vn-reg/my-reg are
// public and visible to every role.
func isConfidentialCorpus(id string) bool {
	switch corpus.ID(id) {
	case corpus.GroupStd, corpus.LocalPolicy, corpus.LocalSOP:
		return true
	default:
		return false
	}
}

// rawVisibleCount runs SET LOCAL ROLE role; SELECT count(*) FROM
// schema.section WHERE document_id = docID directly against pool,
// bypassing store.Search/Corpus entirely — the RLS suite's independent
// oracle. A role with no GRANT USAGE on schema at all (SQLSTATE 42501) is
// folded to 0, matching Search's own "denied schema = zero hits, not an
// error" contract (search.go's isPermissionDenied).
func rawVisibleCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, role, schema string, docID uuid.UUID) int {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning raw count tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// role always comes from this file's own fixed case table above, never
	// external input.
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+role); err != nil {
		t.Fatalf("SET LOCAL ROLE %s: %v", role, err)
	}

	var n int
	q := `SELECT count(*) FROM ` + schema + `.section WHERE document_id = $1`
	err = tx.QueryRow(ctx, q, docID).Scan(&n)
	switch {
	case err == nil:
		return n
	case isSchemaPermissionDenied(err):
		return 0
	default:
		t.Fatalf("raw count on %s.section as %s: %v", schema, role, err)
		return 0
	}
}

// isSchemaPermissionDenied reports whether err is Postgres SQLSTATE 42501
// (insufficient_privilege). Mirrors search.go's isPermissionDenied, kept as
// its own small copy here since that helper is unexported and this test
// lives in the external store_test package.
func isSchemaPermissionDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42501"
}
