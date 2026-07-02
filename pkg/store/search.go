package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/rag/embed"
)

// SearchOpts controls Search's scope, validity filter, and RLS role.
type SearchOpts struct {
	// Corpora limits the search to these corpus IDs; empty means every
	// registered corpus (corpus.All()).
	Corpora []corpus.ID
	// TopK caps the number of hits Search returns, applied globally after
	// merging every corpus's results (and, per corpus, as each RRF arm's
	// candidate-window LIMIT — see buildSearchSQL). <=0 defaults to
	// defaultTopK.
	TopK int
	// InForceOnly restricts sections to validity_status IN ('in_force',
	// 'amended') when true. false (the zero value) leaves every status
	// unfiltered, including repealed/superseded — see validityPredicate.
	InForceOnly bool
	// AsOf, when set, additionally requires each candidate's document to
	// have an effective/expiry window that contains this instant.
	AsOf *time.Time
	// Role is the RLS role the search runs as — one of mise_public/
	// mise_group/mise_local (migrations/004_rls_roles.sql); empty defaults
	// to mise_public. Must always come from the caller's resolved access
	// tier, never raw request input.
	Role string
}

// Hit is one ranked section result from Search.
type Hit struct {
	CorpusID       string
	DocumentID     uuid.UUID
	SectionID      uuid.UUID
	DocNumber      string
	Title          string
	CitationPath   string
	HeadingPath    string
	Text           string
	ValidityStatus string
	SourceURL      string
	Score          float64
}

// defaultTopK is Search's result cap when opts.TopK is unset (<=0).
const defaultTopK = 10

// searchRoles is the fixed RLS-role set opts.Role must belong to
// (migrations/004_rls_roles.sql) — mirrors the set corpus_read.go's
// GetDocument documents, but Search enforces it since one call fans a
// caller-supplied query out across every corpus.
var searchRoles = map[string]bool{"mise_public": true, "mise_group": true, "mise_local": true}

// queryEmbedCacheSize bounds the package-level query-embedding cache
// (Task 9 brief: 1024 entries, keyed by sha256(query+model+dims)).
const queryEmbedCacheSize = 1024

// queryEmbedCache caches query embeddings so repeated searches for the same
// query text under the same embedder/model don't re-embed. lru.New only
// errors when size<=0, which queryEmbedCacheSize never is, so that error is
// safe to discard here.
var queryEmbedCache, _ = lru.New[string, []float32](queryEmbedCacheSize)

// Search runs validity-aware hybrid search — vector cosine similarity fused
// with websearch_to_tsquery lexical ranking via reciprocal rank fusion
// (buildSearchSQL) — across opts.Corpora, scoped to opts.Role's row-level
// visibility. Each corpus is queried in its own SET LOCAL ROLE transaction;
// a corpus schema the role has no GRANT USAGE on at all (SQLSTATE 42501)
// silently contributes zero hits rather than failing the whole search
// (mirrors corpus_read.go's isNotFound classification — see
// isPermissionDenied). Results from every corpus are merged, sorted by
// score descending, and truncated to opts.TopK (default defaultTopK).
func Search(ctx context.Context, pool *pgxpool.Pool, emb embed.Embedder, query string, opts SearchOpts) ([]Hit, error) {
	role, err := resolveRole(opts.Role)
	if err != nil {
		return nil, err
	}
	topK := opts.TopK
	if topK <= 0 {
		topK = defaultTopK
	}

	qvec, err := cachedQueryEmbedding(ctx, emb, query)
	if err != nil {
		return nil, err
	}

	var hits []Hit
	for _, id := range searchCorpora(opts.Corpora) {
		desc, ok := corpus.Get(id)
		if !ok {
			return nil, fmt.Errorf("store: %q is not a registered corpus", id)
		}
		corpusHits, err := searchCorpus(ctx, pool, desc, role, qvec, query, topK, opts.InForceOnly, opts.AsOf)
		if err != nil {
			return nil, fmt.Errorf("searching corpus %s: %w", id, err)
		}
		hits = append(hits, corpusHits...)
	}

	sortHitsByScore(hits)
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

// searchCorpora returns ids, or every registered corpus ID when ids is
// empty (Search's "search everything" default).
func searchCorpora(ids []corpus.ID) []corpus.ID {
	if len(ids) > 0 {
		return ids
	}
	all := corpus.All()
	out := make([]corpus.ID, len(all))
	for i, d := range all {
		out[i] = d.ID
	}
	return out
}

// sortHitsByScore orders hits by score descending, breaking ties by corpus
// id then section id so Search's output order never depends on the
// registry's map iteration order or per-corpus query scheduling.
func sortHitsByScore(hits []Hit) {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].CorpusID != hits[j].CorpusID {
			return hits[i].CorpusID < hits[j].CorpusID
		}
		return hits[i].SectionID.String() < hits[j].SectionID.String()
	})
}

// resolveRole defaults an empty role to mise_public and rejects anything
// outside the fixed RLS-role set.
func resolveRole(role string) (string, error) {
	if role == "" {
		return "mise_public", nil
	}
	if !searchRoles[role] {
		return "", fmt.Errorf("store: %q is not a valid search role", role)
	}
	return role, nil
}

// searchCorpus runs the RRF hybrid-search query (buildSearchSQL) against
// one corpus, inside its own SET LOCAL ROLE transaction.
func searchCorpus(
	ctx context.Context, pool *pgxpool.Pool, desc corpus.Descriptor, role string,
	qvec []float32, query string, topK int, inForceOnly bool, asOf *time.Time,
) ([]Hit, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning search tx for corpus %s: %w", desc.ID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := pgxvec.RegisterTypes(ctx, tx.Conn()); err != nil {
		return nil, fmt.Errorf("registering pgvector types: %w", err)
	}

	// SET LOCAL ROLE can't take a query parameter for the role name; role is
	// validated against the fixed set in resolveRole, but it's quoted via
	// pgx.Identifier as defense in depth (mirrors corpus_read.go).
	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{role}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return nil, fmt.Errorf("setting local role %q: %w", role, err)
	}

	q := buildSearchSQL(desc.SchemaName, validityPredicate(inForceOnly, asOf))
	args := []any{pgvector.NewVector(qvec), topK, query}
	if asOf != nil {
		args = append(args, *asOf)
	}

	hits, err := scanHits(ctx, tx, desc.ID, q, args)
	if err != nil {
		if isPermissionDenied(err) {
			// The role has no GRANT USAGE on this schema at all — a denied
			// corpus contributes zero hits, not an error.
			return nil, nil
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing search tx for corpus %s: %w", desc.ID, err)
	}
	return hits, nil
}

// scanHits runs q/args and scans every row into a Hit, stamping CorpusID
// from id — buildSearchSQL's SELECT list never includes it, since the
// caller already knows which corpus it queried.
func scanHits(ctx context.Context, tx pgx.Tx, id corpus.ID, q string, args []any) ([]Hit, error) {
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("executing hybrid search on corpus %s: %w", id, err)
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		var h Hit
		var citationPath, headingPath, docNumber, sourceURL *string
		err := rows.Scan(&h.SectionID, &h.DocumentID, &citationPath, &headingPath, &h.Text,
			&h.ValidityStatus, &docNumber, &h.Title, &sourceURL, &h.Score)
		if err != nil {
			return nil, fmt.Errorf("scanning hybrid search row for corpus %s: %w", id, err)
		}
		h.CorpusID = string(id)
		h.CitationPath, h.HeadingPath = derefOr(citationPath), derefOr(headingPath)
		h.DocNumber, h.SourceURL = derefOr(docNumber), derefOr(sourceURL)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading hybrid search rows for corpus %s: %w", id, err)
	}
	return hits, nil
}

// isPermissionDenied reports whether err is Postgres SQLSTATE 42501
// (insufficient_privilege) — the role has no GRANT USAGE on this corpus's
// schema at all (migrations/004_rls_roles.sql), so it can't even reference
// the schema's tables to find out RLS would also hide the rows.
func isPermissionDenied(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "42501"
}

// buildSearchSQL renders the RRF hybrid-search query (Task 9 brief) for
// schema, splicing in schema-qualified table identifiers for the {S}
// placeholder and predicate for {validity}. Positional params: $1 the query
// vector, $2 topK (each CTE's candidate window, and the final LIMIT), $3
// the lexical query text, and — only when predicate references it (AsOf
// set) — $4 the AsOf instant.
//
// v ranks by vector cosine distance, l by websearch_to_tsquery lexical
// rank; fused combines them with reciprocal rank fusion (k=60, the standard
// constant — see banhmi's pkg/rag/retrieve/rrf.go for the equivalent Go-
// side computation this mirrors in SQL). schema is only ever one of
// corpus.All()'s registered SchemaName values (searchCorpus calls this via
// corpus.Get), never raw input.
func buildSearchSQL(schema, predicate string) string {
	section := pgx.Identifier{schema, "section"}.Sanitize()
	document := pgx.Identifier{schema, "document"}.Sanitize()
	return `
WITH v AS (
  SELECT s.id, row_number() OVER (ORDER BY s.embedding <=> $1::vector) AS r
  FROM ` + section + ` s JOIN ` + document + ` d ON d.id = s.document_id
  WHERE s.embedding IS NOT NULL AND ` + predicate + `
  ORDER BY s.embedding <=> $1::vector LIMIT $2
), l AS (
  SELECT s.id, row_number() OVER (ORDER BY ts_rank_cd(s.body_tsv, websearch_to_tsquery('simple',$3)) DESC) AS r
  FROM ` + section + ` s JOIN ` + document + ` d ON d.id = s.document_id
  WHERE s.body_tsv @@ websearch_to_tsquery('simple',$3) AND ` + predicate + `
  LIMIT $2
), fused AS (
  SELECT COALESCE(v.id,l.id) AS id,
         COALESCE(1.0/(60+v.r),0)+COALESCE(1.0/(60+l.r),0) AS score
  FROM v FULL OUTER JOIN l ON v.id = l.id
)
SELECT s.id, s.document_id, s.citation_path, s.heading_path, s.body, s.validity_status,
       d.doc_number, d.title, d.source_url, f.score
FROM fused f JOIN ` + section + ` s ON s.id = f.id JOIN ` + document + ` d ON d.id = s.document_id
ORDER BY f.score DESC LIMIT $2;`
}

// validityPredicate renders the {validity} fragment buildSearchSQL splices
// into both CTEs' WHERE clauses: unrestricted ("TRUE") unless inForceOnly,
// which limits sections to validity_status IN ('in_force','amended'); when
// asOf is set, it appends the document's effective/expiry date-window
// predicate (bound to $4) regardless of inForceOnly, since AsOf and the
// validity-status filter are independent controls.
func validityPredicate(inForceOnly bool, asOf *time.Time) string {
	pred := "TRUE"
	if inForceOnly {
		pred = "s.validity_status IN ('in_force','amended')"
	}
	if asOf != nil {
		pred += " AND (d.effective_date IS NULL OR d.effective_date <= $4) AND (d.expiry_date IS NULL OR d.expiry_date > $4)"
	}
	return pred
}

// cachedQueryEmbedding returns query's embedding vector, preferring emb's
// QueryEmbedder capability (falls back to Embed — see embed.QueryEmbedder),
// and caching the result in queryEmbedCache so repeated searches for the
// same query text under the same embedder/model don't re-embed.
func cachedQueryEmbedding(ctx context.Context, emb embed.Embedder, query string) ([]float32, error) {
	key := queryEmbedCacheKey(query, emb.Model(), emb.Dims())
	if v, ok := queryEmbedCache.Get(key); ok {
		return v, nil
	}

	vecs, err := embedQueryText(ctx, emb, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding search query: %w", err)
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("embedding search query: got %d vectors, want 1", len(vecs))
	}

	queryEmbedCache.Add(key, vecs[0])
	return vecs[0], nil
}

// embedQueryText embeds texts via emb's query-specific path when the
// adapter implements embed.QueryEmbedder, falling back to plain Embed.
func embedQueryText(ctx context.Context, emb embed.Embedder, texts []string) ([][]float32, error) {
	if qe, ok := emb.(embed.QueryEmbedder); ok {
		return qe.EmbedQueries(ctx, texts)
	}
	return emb.Embed(ctx, texts)
}

// queryEmbedCacheKey is queryEmbedCache's key: sha256 of query, model, and
// dims, NUL-joined so the three fields can't be ambiguously concatenated
// (e.g. query="ab",model="c" vs query="a",model="bc").
func queryEmbedCacheKey(query, model string, dims int) string {
	sum := sha256.Sum256([]byte(query + "\x00" + model + "\x00" + strconv.Itoa(dims)))
	return hex.EncodeToString(sum[:])
}
