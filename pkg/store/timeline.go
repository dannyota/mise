package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/corpus"
)

// TimelineEvent is one dated amendment occurrence mapped from a corpus's
// amendment_event table. Kind is always "amendment" for now (the timeline
// is derived, DATA-MODEL §10); Description carries the target document's
// title for display.
type TimelineEvent struct {
	ID          uuid.UUID
	Kind        string
	CorpusID    string
	DocumentID  *uuid.UUID
	Description string
	Timestamp   time.Time
}

// TimelineListOpts controls ListTimeline's pagination and filters.
type TimelineListOpts struct {
	From   *time.Time
	To     *time.Time
	Corpus string
	Cursor string
	Limit  int
}

// TimelinePage is the paginated result of ListTimeline.
type TimelinePage struct {
	Items      []TimelineEvent
	NextCursor string
}

// TimelineStore is the cross-corpus amendment timeline read path. Reads run
// inside a SET LOCAL ROLE transaction; RLS on each corpus's amendment_event
// table controls visibility.
type TimelineStore struct {
	pool *pgxpool.Pool
}

// NewTimelineStore returns a TimelineStore backed by pool.
func NewTimelineStore(pool *pgxpool.Pool) *TimelineStore {
	return &TimelineStore{pool: pool}
}

// ListTimeline returns amendment events across corpora visible to role,
// ordered by event_date DESC. Queries each corpus schema's amendment_event
// table via UNION ALL (schema names come from corpus.All(), trusted). Joins
// to the document table to resolve the target document's title.
func (s *TimelineStore) ListTimeline(
	ctx context.Context, role string, opts TimelineListOpts,
) (TimelinePage, error) {
	validRole, err := resolveRole(role)
	if err != nil {
		return TimelinePage{}, err
	}

	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	descs := timelineCorpora(validRole, opts.Corpus)
	if len(descs) == 0 {
		return TimelinePage{Items: []TimelineEvent{}}, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TimelinePage{}, fmt.Errorf("beginning ListTimeline read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{validRole}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return TimelinePage{}, fmt.Errorf("setting local role %q: %w", validRole, err)
	}

	q, args := buildTimelineQuery(descs, opts, limit)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return TimelinePage{}, fmt.Errorf("querying timeline events: %w", err)
	}
	defer rows.Close()

	var items []TimelineEvent
	for rows.Next() {
		var ev TimelineEvent
		var title *string
		err := rows.Scan(&ev.ID, &ev.CorpusID, &ev.DocumentID, &title, &ev.Kind, &ev.Timestamp)
		if err != nil {
			return TimelinePage{}, fmt.Errorf("scanning timeline event row: %w", err)
		}
		if title != nil {
			ev.Description = *title
		}
		items = append(items, ev)
	}
	if err := rows.Err(); err != nil {
		return TimelinePage{}, fmt.Errorf("reading timeline event rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return TimelinePage{}, fmt.Errorf("committing ListTimeline read: %w", err)
	}

	var nextCursor string
	if len(items) > limit {
		nextCursor = items[limit-1].ID.String()
		items = items[:limit]
	}
	return TimelinePage{Items: items, NextCursor: nextCursor}, nil
}

// roleVisibleTiers maps each RLS role to the corpus access tiers whose
// schemas it holds GRANT USAGE on (migrations/004 + 013). ListTimeline needs
// this upfront because its UNION ALL is one statement: a single arm touching
// a denied schema fails the whole query with SQLSTATE 42501 — there is no
// per-corpus transaction to fold the denial into zero rows the way
// store.Search does (isPermissionDenied). Filtering the corpus set to the
// role's visible tiers mirrors those grants exactly.
var roleVisibleTiers = map[string]map[corpus.AccessTier]bool{
	"mise_public": {corpus.TierPublic: true},
	"mise_group":  {corpus.TierPublic: true, corpus.TierGroupConfidential: true},
	"mise_local": {
		corpus.TierPublic: true, corpus.TierGroupConfidential: true, corpus.TierLocalConfidential: true,
	},
}

// timelineCorpora returns descriptors to query for role. When corpusFilter
// is set, only the matching corpus is returned (still tier-checked);
// otherwise all corpora that both have amendment_event tables
// (law/standard/policy/sop kinds) and sit in a tier role can see.
func timelineCorpora(role, corpusFilter string) []corpus.Descriptor {
	visible := roleVisibleTiers[role]
	if corpusFilter != "" {
		d, ok := corpus.Get(corpus.ID(corpusFilter))
		if !ok || !visible[d.AccessTier] {
			return nil
		}
		return []corpus.Descriptor{d}
	}
	var out []corpus.Descriptor
	for _, d := range corpus.All() {
		if !visible[d.AccessTier] {
			continue
		}
		// Only law/standard/policy/sop corpora have amendment_event tables
		// (reports/diagrams do not).
		switch d.Kind {
		case corpus.KindLaw, corpus.KindStandard, corpus.KindPolicy, corpus.KindSOP:
			out = append(out, d)
		}
	}
	return out
}

// buildTimelineQuery builds a UNION ALL across corpus schemas' amendment_event
// tables. Schema names are from corpus.All() — trusted, not user input.
func buildTimelineQuery(descs []corpus.Descriptor, opts TimelineListOpts, limit int) (string, []any) {
	var args []any
	argIdx := 1

	// Build date-range predicates referencing positional args.
	var predicates []string
	if opts.From != nil {
		predicates = append(predicates, fmt.Sprintf("ae.event_date >= $%d", argIdx))
		args = append(args, *opts.From)
		argIdx++
	}
	if opts.To != nil {
		predicates = append(predicates, fmt.Sprintf("ae.event_date <= $%d", argIdx))
		args = append(args, *opts.To)
		argIdx++
	}
	if opts.Cursor != "" {
		cursorID, parseErr := uuid.Parse(opts.Cursor)
		if parseErr == nil {
			predicates = append(predicates, fmt.Sprintf("ae.id < $%d", argIdx))
			args = append(args, cursorID)
			argIdx++
		}
	}

	where := "TRUE"
	if len(predicates) > 0 {
		where = strings.Join(predicates, " AND ")
	}

	limitArg := fmt.Sprintf("$%d", argIdx)
	args = append(args, limit+1)

	// Build UNION ALL of per-corpus sub-selects. Corpus IDs are injected as
	// single-quoted string constants; schema names are sanitized identifiers.
	parts := make([]string, 0, len(descs))
	for _, d := range descs {
		ae := pgx.Identifier{d.SchemaName, "amendment_event"}.Sanitize()
		doc := pgx.Identifier{d.SchemaName, "document"}.Sanitize()
		// Escape single quotes in corpus ID for safe embedding as a SQL literal.
		corpusLit := "'" + strings.ReplaceAll(string(d.ID), "'", "''") + "'"
		part := fmt.Sprintf(
			`SELECT ae.id, %s AS corpus_id, ae.target_doc_id, d.title, ae.kind, ae.event_date
FROM %s ae
JOIN %s d ON d.id = ae.target_doc_id
WHERE %s`,
			corpusLit, ae, doc, where)
		parts = append(parts, part)
	}

	q := strings.Join(parts, "\nUNION ALL\n") +
		"\nORDER BY event_date DESC, id DESC LIMIT " + limitArg

	return q, args
}
