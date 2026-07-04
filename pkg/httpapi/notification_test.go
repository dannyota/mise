package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/store"
)

type fakeNotificationRepo struct {
	page       store.NotificationPage
	listErr    error
	markErr    error
	webhooks   []store.WebhookSubscription
	webhookErr error
	createID   uuid.UUID
	createErr  error
	deleteErr  error

	gotRole     string
	gotOpts     store.NotificationListOpts
	gotMarkID   uuid.UUID
	gotWebhook  store.WebhookSubscription
	gotDeleteID uuid.UUID
}

func (f *fakeNotificationRepo) ListNotifications(
	_ context.Context, role string, opts store.NotificationListOpts,
) (store.NotificationPage, error) {
	f.gotRole, f.gotOpts = role, opts
	return f.page, f.listErr
}

func (f *fakeNotificationRepo) MarkRead(_ context.Context, id uuid.UUID) error {
	f.gotMarkID = id
	return f.markErr
}

func (f *fakeNotificationRepo) ListWebhooks(_ context.Context) ([]store.WebhookSubscription, error) {
	return f.webhooks, f.webhookErr
}

func (f *fakeNotificationRepo) CreateWebhook(_ context.Context, sub store.WebhookSubscription) (uuid.UUID, error) {
	f.gotWebhook = sub
	return f.createID, f.createErr
}

func (f *fakeNotificationRepo) DeleteWebhook(_ context.Context, id uuid.UUID) error {
	f.gotDeleteID = id
	return f.deleteErr
}

func newNotificationTestServer(t *testing.T, repo NotificationRepoIface, role string) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterNotifications(api, repo, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestListNotificationsReturnsItems(t *testing.T) {
	t.Parallel()
	notifID := uuid.New()
	findingID := uuid.New()
	createdAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)

	repo := &fakeNotificationRepo{
		page: store.NotificationPage{
			Items: []store.Notification{{
				ID:         notifID,
				FindingRef: findingID,
				Kind:       "conflict",
				Title:      "Conflicting controls",
				CreatedAt:  createdAt,
			}},
			NextCursor: "next-page",
		},
	}

	srv := newNotificationTestServer(t, repo, "mise_group")
	status, ct, body := getJSON(t, srv, "/notifications?limit=5")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if repo.gotRole != "mise_group" {
		t.Errorf("repo received role = %q, want mise_group", repo.gotRole)
	}
	if repo.gotOpts.Limit != 5 {
		t.Errorf("repo received limit = %d, want 5", repo.gotOpts.Limit)
	}

	var got struct {
		Items      []NotificationWire `json:"items"`
		NextCursor string             `json:"next_cursor"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(got.Items))
	}
	if got.Items[0].ID != notifID.String() {
		t.Errorf("Items[0].ID = %q, want %s", got.Items[0].ID, notifID)
	}
	if got.Items[0].Type != "conflict" {
		t.Errorf("Items[0].Type = %q, want conflict", got.Items[0].Type)
	}
	if got.Items[0].FindingID != findingID.String() {
		t.Errorf("Items[0].FindingID = %q, want %s", got.Items[0].FindingID, findingID)
	}
	if got.Items[0].Read {
		t.Error("Items[0].Read = true, want false (no ReadAt)")
	}
	if got.NextCursor != "next-page" {
		t.Errorf("NextCursor = %q, want next-page", got.NextCursor)
	}
}

func TestListNotificationsEmptyNonNil(t *testing.T) {
	t.Parallel()
	repo := &fakeNotificationRepo{page: store.NotificationPage{}}
	srv := newNotificationTestServer(t, repo, "mise_public")

	status, _, body := getJSON(t, srv, "/notifications")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	var got struct {
		Items []NotificationWire `json:"items"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if got.Items == nil || len(got.Items) != 0 {
		t.Errorf("Items = %v, want non-nil empty slice", got.Items)
	}
}

func TestMarkNotificationReadReturnsOK(t *testing.T) {
	t.Parallel()
	notifID := uuid.New()
	repo := &fakeNotificationRepo{}
	srv := newNotificationTestServer(t, repo, "mise_public")

	status, _, respBody := postJSON(t, srv, "/notifications/"+notifID.String()+"/read", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, respBody)
	}
	if repo.gotMarkID != notifID {
		t.Errorf("repo received id = %v, want %v", repo.gotMarkID, notifID)
	}
}

func TestMarkNotificationReadNotFoundReturns404(t *testing.T) {
	t.Parallel()
	repo := &fakeNotificationRepo{markErr: store.ErrNotificationNotFound}
	srv := newNotificationTestServer(t, repo, "mise_public")

	status, ct, _ := postJSON(t, srv, "/notifications/"+uuid.NewString()+"/read", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestMarkNotificationReadBadUUIDReturns400(t *testing.T) {
	t.Parallel()
	srv := newNotificationTestServer(t, &fakeNotificationRepo{}, "mise_public")
	status, _, _ := postJSON(t, srv, "/notifications/not-a-uuid/read", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestListWebhooksReturnsItems(t *testing.T) {
	t.Parallel()
	whID := uuid.New()
	createdAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	repo := &fakeNotificationRepo{
		webhooks: []store.WebhookSubscription{{
			ID:          whID,
			EndpointURL: "https://hooks.example.com/mise",
			EventKinds:  []string{"conflict", "staleness"},
			Active:      true,
			CreatedAt:   createdAt,
		}},
	}

	srv := newNotificationTestServer(t, repo, "mise_public")
	status, _, body := getJSON(t, srv, "/webhooks")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	var got struct {
		Items []WebhookWire `json:"items"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(got.Items))
	}
	if got.Items[0].ID != whID.String() {
		t.Errorf("Items[0].ID = %q, want %s", got.Items[0].ID, whID)
	}
	if got.Items[0].URL != "https://hooks.example.com/mise" {
		t.Errorf("Items[0].URL = %q, want https://hooks.example.com/mise", got.Items[0].URL)
	}
	if len(got.Items[0].Events) != 2 {
		t.Errorf("Items[0].Events = %v, want [conflict staleness]", got.Items[0].Events)
	}
}

func TestCreateWebhookReturnsID(t *testing.T) {
	t.Parallel()
	whID := uuid.New()
	repo := &fakeNotificationRepo{createID: whID}

	srv := newNotificationTestServer(t, repo, "mise_public")
	body := map[string]any{
		"url":    "https://hooks.example.com/mise",
		"events": []string{"conflict"},
	}
	status, _, respBody := postJSON(t, srv, "/webhooks", body)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, respBody)
	}

	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, respBody)
	}
	if got.ID != whID.String() {
		t.Errorf("id = %q, want %s", got.ID, whID)
	}
	if repo.gotWebhook.EndpointURL != "https://hooks.example.com/mise" {
		t.Errorf("repo received url = %q, want https://hooks.example.com/mise", repo.gotWebhook.EndpointURL)
	}
}

func TestCreateWebhookRejectsHTTP(t *testing.T) {
	t.Parallel()
	srv := newNotificationTestServer(t, &fakeNotificationRepo{createID: uuid.New()}, "mise_public")

	body := map[string]any{
		"url":    "http://hooks.example.com/mise",
		"events": []string{"conflict"},
	}
	status, ct, _ := postJSON(t, srv, "/webhooks", body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (HTTP URL rejected)", status)
	}
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestCreateWebhookRejectsLocalhost(t *testing.T) {
	t.Parallel()
	srv := newNotificationTestServer(t, &fakeNotificationRepo{createID: uuid.New()}, "mise_public")

	body := map[string]any{
		"url":    "https://localhost/hook",
		"events": []string{},
	}
	status, _, _ := postJSON(t, srv, "/webhooks", body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (localhost rejected)", status)
	}
}

func TestDeleteWebhookReturnsOK(t *testing.T) {
	t.Parallel()
	whID := uuid.New()
	repo := &fakeNotificationRepo{}
	srv := newNotificationTestServer(t, repo, "mise_public")

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/webhooks/"+whID.String(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /webhooks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if repo.gotDeleteID != whID {
		t.Errorf("repo received id = %v, want %v", repo.gotDeleteID, whID)
	}
}

func TestDeleteWebhookNotFoundReturns404(t *testing.T) {
	t.Parallel()
	repo := &fakeNotificationRepo{deleteErr: store.ErrWebhookNotFound}
	srv := newNotificationTestServer(t, repo, "mise_public")

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/webhooks/"+uuid.NewString(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /webhooks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDeleteWebhookBadUUIDReturns400(t *testing.T) {
	t.Parallel()
	srv := newNotificationTestServer(t, &fakeNotificationRepo{}, "mise_public")

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/webhooks/not-a-uuid", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /webhooks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestMapNotificationsNonNil(t *testing.T) {
	t.Parallel()
	items := mapNotifications(nil)
	if items == nil {
		t.Error("mapNotifications(nil) = nil, want non-nil empty slice")
	}
	data, _ := json.Marshal(items)
	if string(data) != "[]" {
		t.Errorf("json.Marshal(mapNotifications(nil)) = %s, want []", data)
	}
}

func TestMapWebhooksNonNil(t *testing.T) {
	t.Parallel()
	items := mapWebhooks(nil)
	if items == nil {
		t.Error("mapWebhooks(nil) = nil, want non-nil empty slice")
	}
	data, _ := json.Marshal(items)
	if string(data) != "[]" {
		t.Errorf("json.Marshal(mapWebhooks(nil)) = %s, want []", data)
	}
}
