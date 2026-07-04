package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotificationNotFound is returned when a notification id does not exist.
var ErrNotificationNotFound = errors.New("notification not found")

// ErrWebhookNotFound is returned when a webhook subscription id does not exist.
var ErrWebhookNotFound = errors.New("webhook subscription not found")

// Notification is one graph.notification row: a user-facing alert generated
// by a detector (conflict/staleness/overdue) against a finding.
type Notification struct {
	ID            uuid.UUID
	FindingRef    uuid.UUID
	Kind          string
	Severity      string
	RecipientRole string
	RecipientDept string
	Title         string
	ReadAt        *time.Time
	Channels      []string
	CreatedAt     time.Time
}

// NotificationListOpts controls ListNotifications' pagination.
type NotificationListOpts struct {
	Cursor string
	Limit  int
}

// NotificationPage is the paginated result of ListNotifications.
type NotificationPage struct {
	Items      []Notification
	NextCursor string
}

// WebhookSubscription is one graph.webhook_subscription row: an external
// endpoint registered to receive notification deliveries.
type WebhookSubscription struct {
	ID          uuid.UUID
	EndpointURL string
	SecretRef   string
	EventKinds  []string
	MinSeverity string
	AccessTier  string
	Active      bool
	CreatedBy   string
	CreatedAt   time.Time
}

// NotificationStore is the notifications + webhooks read/write path. Reads
// run inside a SET LOCAL ROLE transaction scoped to the caller's resolved tier.
type NotificationStore struct {
	pool *pgxpool.Pool
}

// NewNotificationStore returns a NotificationStore backed by pool.
func NewNotificationStore(pool *pgxpool.Pool) *NotificationStore {
	return &NotificationStore{pool: pool}
}

// ListNotifications returns notifications visible to role, paginated by
// cursor/limit, ordered by created_at DESC.
func (s *NotificationStore) ListNotifications(
	ctx context.Context, role string, opts NotificationListOpts,
) (NotificationPage, error) {
	validRole, err := resolveRole(role)
	if err != nil {
		return NotificationPage{}, err
	}

	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return NotificationPage{}, fmt.Errorf("beginning ListNotifications read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{validRole}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return NotificationPage{}, fmt.Errorf("setting local role %q: %w", validRole, err)
	}

	q, args := buildNotificationListQuery(opts, limit)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return NotificationPage{}, fmt.Errorf("querying notifications: %w", err)
	}
	defer rows.Close()

	var items []Notification
	for rows.Next() {
		var n Notification
		err := rows.Scan(&n.ID, &n.FindingRef, &n.Kind, &n.Severity,
			&n.RecipientRole, &n.RecipientDept, &n.Title, &n.ReadAt,
			&n.Channels, &n.CreatedAt)
		if err != nil {
			return NotificationPage{}, fmt.Errorf("scanning notification row: %w", err)
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return NotificationPage{}, fmt.Errorf("reading notification rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return NotificationPage{}, fmt.Errorf("committing ListNotifications read: %w", err)
	}

	var nextCursor string
	if len(items) > limit {
		nextCursor = items[limit-1].ID.String()
		items = items[:limit]
	}
	return NotificationPage{Items: items, NextCursor: nextCursor}, nil
}

func buildNotificationListQuery(opts NotificationListOpts, limit int) (string, []any) {
	args := []any{limit + 1}
	cursorFilter := ""

	if opts.Cursor != "" {
		cursorID, parseErr := uuid.Parse(opts.Cursor)
		if parseErr == nil {
			cursorFilter = " AND id < $2"
			args = append(args, cursorID)
		}
	}

	q := `SELECT id, finding_ref, kind, severity, recipient_role, recipient_dept,
		title, read_at, channels, created_at
	FROM graph.notification
	WHERE true` + cursorFilter + ` ORDER BY created_at DESC, id DESC LIMIT $1`

	return q, args
}

// MarkRead sets read_at = now() for the given notification. Returns
// ErrNotificationNotFound if no unread row matches.
func (s *NotificationStore) MarkRead(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE graph.notification SET read_at = now() WHERE id = $1 AND read_at IS NULL`
	tag, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("marking notification %s read: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("marking notification %s read: %w", id, ErrNotificationNotFound)
	}
	return nil
}

// ListWebhooks returns all active webhook subscriptions.
func (s *NotificationStore) ListWebhooks(ctx context.Context) ([]WebhookSubscription, error) {
	const q = `SELECT id, endpoint_url, secret_ref, event_kinds, min_severity,
		access_tier, active, created_by, created_at
	FROM graph.webhook_subscription
	WHERE active = true
	ORDER BY created_at DESC`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying webhook subscriptions: %w", err)
	}
	defer rows.Close()

	var out []WebhookSubscription
	for rows.Next() {
		var ws WebhookSubscription
		err := rows.Scan(&ws.ID, &ws.EndpointURL, &ws.SecretRef, &ws.EventKinds,
			&ws.MinSeverity, &ws.AccessTier, &ws.Active, &ws.CreatedBy, &ws.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning webhook subscription row: %w", err)
		}
		out = append(out, ws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading webhook subscription rows: %w", err)
	}
	return out, nil
}

// CreateWebhook inserts a webhook subscription and returns the new row's id.
func (s *NotificationStore) CreateWebhook(ctx context.Context, sub WebhookSubscription) (uuid.UUID, error) {
	const q = `
INSERT INTO graph.webhook_subscription
	(endpoint_url, secret_ref, event_kinds, min_severity, access_tier, active, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`

	var id uuid.UUID
	err := s.pool.QueryRow(ctx, q,
		sub.EndpointURL, sub.SecretRef, sub.EventKinds,
		sub.MinSeverity, sub.AccessTier, sub.Active, sub.CreatedBy,
	).Scan(&id)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("creating webhook subscription: %w", err)
	}
	return id, nil
}

// DeleteWebhook deletes a webhook subscription. Returns ErrWebhookNotFound
// if the id does not exist.
func (s *NotificationStore) DeleteWebhook(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM graph.webhook_subscription WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting webhook subscription %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting webhook subscription %s: %w", id, ErrWebhookNotFound)
	}
	return nil
}
