package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/store"
)

// NotificationRepoIface is the notification and webhook endpoints' dependency
// — satisfied by *store.NotificationStore — narrowed to the exact methods the
// handlers need, consumer-defined per CODE_STYLE_GO.
type NotificationRepoIface interface {
	ListNotifications(ctx context.Context, role string, opts store.NotificationListOpts) (store.NotificationPage, error)
	MarkRead(ctx context.Context, id uuid.UUID) error
	ListWebhooks(ctx context.Context) ([]store.WebhookSubscription, error)
	CreateWebhook(ctx context.Context, sub store.WebhookSubscription) (uuid.UUID, error)
	DeleteWebhook(ctx context.Context, id uuid.UUID) error
}

// --- Wire types ---

// NotificationWire is the wire form of store.Notification.
type NotificationWire struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	FindingID string `json:"finding_id"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}

// WebhookWire is the wire form of store.WebhookSubscription.
type WebhookWire struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Active    bool     `json:"active"`
	CreatedAt string   `json:"created_at"`
}

// --- Input/Output types ---

// NotificationListInput is GET /notifications's input.
type NotificationListInput struct {
	Cursor string `query:"cursor" doc:"Opaque pagination cursor" example:""`
	Limit  int    `query:"limit" doc:"Page size (1-100, default 20)" example:"20"`
}

// NotificationListOutput is GET /notifications's output.
type NotificationListOutput struct {
	Body struct {
		Items      []NotificationWire `json:"items"`
		NextCursor string             `json:"next_cursor,omitempty"`
	}
}

// NotificationMarkReadInput is POST /notifications/{id}/read's input.
type NotificationMarkReadInput struct {
	ID string `path:"id" doc:"Notification UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// NotificationMarkReadOutput is POST /notifications/{id}/read's output.
type NotificationMarkReadOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// WebhookListOutput is GET /webhooks's output.
type WebhookListOutput struct {
	Body struct {
		Items []WebhookWire `json:"items"`
	}
}

// WebhookCreateInput is POST /webhooks's input.
type WebhookCreateInput struct {
	Body struct {
		URL    string   `json:"url" doc:"Webhook endpoint URL (HTTPS only)" example:"https://hooks.example.com/mise"`
		Events []string `json:"events" doc:"Event kinds to subscribe to" example:"[\"conflict\",\"staleness\"]"`
	}
}

// WebhookCreateOutput is POST /webhooks's output.
type WebhookCreateOutput struct {
	Body struct {
		ID string `json:"id"`
	}
}

// WebhookDeleteInput is DELETE /webhooks/{id}'s input.
type WebhookDeleteInput struct {
	ID string `path:"id" doc:"Webhook UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// WebhookDeleteOutput is DELETE /webhooks/{id}'s output.
type WebhookDeleteOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// RegisterNotifications mounts the notification and webhook REST operations.
func RegisterNotifications(api huma.API, repo NotificationRepoIface, role string) {
	huma.Register(api, huma.Operation{
		OperationID: "list-notifications",
		Method:      http.MethodGet,
		Path:        "/notifications",
		Summary:     "List notifications (paginated)",
		Tags:        []string{"Notifications"},
	}, newListNotificationsHandler(repo, role))

	huma.Register(api, huma.Operation{
		OperationID: "mark-notification-read",
		Method:      http.MethodPost,
		Path:        "/notifications/{id}/read",
		Summary:     "Mark a notification as read",
		Tags:        []string{"Notifications"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newMarkReadHandler(repo))

	huma.Register(api, huma.Operation{
		OperationID: "list-webhooks",
		Method:      http.MethodGet,
		Path:        "/webhooks",
		Summary:     "List webhook subscriptions",
		Tags:        []string{"Webhooks"},
	}, newListWebhooksHandler(repo))

	huma.Register(api, huma.Operation{
		OperationID: "create-webhook",
		Method:      http.MethodPost,
		Path:        "/webhooks",
		Summary:     "Create a webhook subscription",
		Tags:        []string{"Webhooks"},
		Errors:      []int{http.StatusBadRequest},
	}, newCreateWebhookHandler(repo))

	huma.Register(api, huma.Operation{
		OperationID: "delete-webhook",
		Method:      http.MethodDelete,
		Path:        "/webhooks/{id}",
		Summary:     "Delete a webhook subscription",
		Tags:        []string{"Webhooks"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newDeleteWebhookHandler(repo))
}

func newListNotificationsHandler(
	repo NotificationRepoIface, role string,
) func(context.Context, *NotificationListInput) (*NotificationListOutput, error) {
	return func(ctx context.Context, in *NotificationListInput) (*NotificationListOutput, error) {
		var page store.NotificationPage
		if repo != nil {
			var err error
			page, err = repo.ListNotifications(ctx, role, store.NotificationListOpts{
				Cursor: in.Cursor,
				Limit:  in.Limit,
			})
			if err != nil {
				return nil, fmt.Errorf("httpapi: listing notifications: %w", err)
			}
		}

		out := &NotificationListOutput{}
		out.Body.Items = mapNotifications(page.Items)
		out.Body.NextCursor = page.NextCursor
		return out, nil
	}
}

func newMarkReadHandler(
	repo NotificationRepoIface,
) func(context.Context, *NotificationMarkReadInput) (*NotificationMarkReadOutput, error) {
	return func(ctx context.Context, in *NotificationMarkReadInput) (*NotificationMarkReadOutput, error) {
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid notification UUID", err)
		}
		if err := repo.MarkRead(ctx, id); err != nil {
			if errors.Is(err, store.ErrNotificationNotFound) {
				return nil, huma.Error404NotFound("notification not found")
			}
			return nil, fmt.Errorf("httpapi: marking notification read: %w", err)
		}
		out := &NotificationMarkReadOutput{}
		out.Body.OK = true
		return out, nil
	}
}

func newListWebhooksHandler(
	repo NotificationRepoIface,
) func(context.Context, *struct{}) (*WebhookListOutput, error) {
	return func(ctx context.Context, _ *struct{}) (*WebhookListOutput, error) {
		var subs []store.WebhookSubscription
		if repo != nil {
			var err error
			subs, err = repo.ListWebhooks(ctx)
			if err != nil {
				return nil, fmt.Errorf("httpapi: listing webhooks: %w", err)
			}
		}

		out := &WebhookListOutput{}
		out.Body.Items = mapWebhooks(subs)
		return out, nil
	}
}

func newCreateWebhookHandler(
	repo NotificationRepoIface,
) func(context.Context, *WebhookCreateInput) (*WebhookCreateOutput, error) {
	return func(ctx context.Context, in *WebhookCreateInput) (*WebhookCreateOutput, error) {
		if err := ValidateWebhookURL(in.Body.URL); err != nil {
			return nil, huma.Error400BadRequest("invalid webhook URL", err)
		}

		events := in.Body.Events
		if events == nil {
			events = []string{}
		}

		id, err := repo.CreateWebhook(ctx, store.WebhookSubscription{
			EndpointURL: in.Body.URL,
			EventKinds:  events,
			Active:      true,
		})
		if err != nil {
			return nil, fmt.Errorf("httpapi: creating webhook: %w", err)
		}

		out := &WebhookCreateOutput{}
		out.Body.ID = id.String()
		return out, nil
	}
}

func newDeleteWebhookHandler(
	repo NotificationRepoIface,
) func(context.Context, *WebhookDeleteInput) (*WebhookDeleteOutput, error) {
	return func(ctx context.Context, in *WebhookDeleteInput) (*WebhookDeleteOutput, error) {
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid webhook UUID", err)
		}
		if err := repo.DeleteWebhook(ctx, id); err != nil {
			if errors.Is(err, store.ErrWebhookNotFound) {
				return nil, huma.Error404NotFound("webhook not found")
			}
			return nil, fmt.Errorf("httpapi: deleting webhook: %w", err)
		}
		out := &WebhookDeleteOutput{}
		out.Body.OK = true
		return out, nil
	}
}

// mapNotifications maps store.Notification rows to their wire form. Always
// returns a non-nil slice.
func mapNotifications(notifications []store.Notification) []NotificationWire {
	out := make([]NotificationWire, len(notifications))
	for i, n := range notifications {
		out[i] = NotificationWire{
			ID:        n.ID.String(),
			Type:      n.Kind,
			Title:     n.Title,
			FindingID: n.FindingRef.String(),
			Read:      n.ReadAt != nil,
			CreatedAt: n.CreatedAt.Format(time.RFC3339),
		}
	}
	return out
}

// mapWebhooks maps store.WebhookSubscription rows to their wire form. Always
// returns a non-nil slice.
func mapWebhooks(subs []store.WebhookSubscription) []WebhookWire {
	out := make([]WebhookWire, len(subs))
	for i, s := range subs {
		events := s.EventKinds
		if events == nil {
			events = []string{}
		}
		out[i] = WebhookWire{
			ID:        s.ID.String(),
			URL:       s.EndpointURL,
			Events:    events,
			Active:    s.Active,
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
		}
	}
	return out
}
