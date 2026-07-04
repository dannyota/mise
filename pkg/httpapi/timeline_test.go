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

type fakeTimelineRepo struct {
	page    store.TimelinePage
	listErr error

	gotRole string
	gotOpts store.TimelineListOpts
}

func (f *fakeTimelineRepo) ListTimeline(
	_ context.Context, role string, opts store.TimelineListOpts,
) (store.TimelinePage, error) {
	f.gotRole, f.gotOpts = role, opts
	return f.page, f.listErr
}

func newTimelineTestServer(t *testing.T, repo TimelineRepoIface, role string) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterTimeline(api, repo, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestTimelineListReturnsItems(t *testing.T) {
	t.Parallel()
	eventID := uuid.New()
	docID := uuid.New()
	ts := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	repo := &fakeTimelineRepo{
		page: store.TimelinePage{
			Items: []store.TimelineEvent{{
				ID:          eventID,
				Kind:        "amendment",
				CorpusID:    "vn-reg",
				DocumentID:  &docID,
				Description: "Circular 09/2020 amended",
				Timestamp:   ts,
			}},
			NextCursor: "cursor-next",
		},
	}

	srv := newTimelineTestServer(t, repo, "mise_group")
	status, ct, body := getJSON(t, srv, "/timeline?corpus=vn-reg&limit=10")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if repo.gotRole != "mise_group" {
		t.Errorf("repo received role = %q, want mise_group", repo.gotRole)
	}
	if repo.gotOpts.Corpus != "vn-reg" {
		t.Errorf("repo received corpus = %q, want vn-reg", repo.gotOpts.Corpus)
	}
	if repo.gotOpts.Limit != 10 {
		t.Errorf("repo received limit = %d, want 10", repo.gotOpts.Limit)
	}

	var got struct {
		Items      []TimelineEventWire `json:"items"`
		NextCursor string              `json:"next_cursor"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(got.Items))
	}
	item := got.Items[0]
	if item.ID != eventID.String() {
		t.Errorf("Items[0].ID = %q, want %s", item.ID, eventID)
	}
	if item.Kind != "amendment" {
		t.Errorf("Items[0].Kind = %q, want amendment", item.Kind)
	}
	if item.CorpusID != "vn-reg" {
		t.Errorf("Items[0].CorpusID = %q, want vn-reg", item.CorpusID)
	}
	if item.DocumentID == nil || *item.DocumentID != docID.String() {
		t.Errorf("Items[0].DocumentID = %v, want %s", item.DocumentID, docID)
	}
	if item.Description != "Circular 09/2020 amended" {
		t.Errorf("Items[0].Description = %q, want 'Circular 09/2020 amended'", item.Description)
	}
	if got.NextCursor != "cursor-next" {
		t.Errorf("NextCursor = %q, want cursor-next", got.NextCursor)
	}
}

func TestTimelineListEmptyReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	repo := &fakeTimelineRepo{page: store.TimelinePage{}}
	srv := newTimelineTestServer(t, repo, "mise_public")

	status, _, body := getJSON(t, srv, "/timeline")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	var got struct {
		Items []TimelineEventWire `json:"items"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if got.Items == nil || len(got.Items) != 0 {
		t.Errorf("Items = %v, want non-nil empty slice", got.Items)
	}
}

func TestTimelineListParsesFromTo(t *testing.T) {
	t.Parallel()
	repo := &fakeTimelineRepo{page: store.TimelinePage{}}
	srv := newTimelineTestServer(t, repo, "mise_public")

	status, _, body := getJSON(t, srv, "/timeline?from=2026-01-01T00:00:00Z&to=2026-12-31T23:59:59Z")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	if repo.gotOpts.From == nil {
		t.Fatal("from = nil, want parsed time")
	}
	wantFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !repo.gotOpts.From.Equal(wantFrom) {
		t.Errorf("from = %v, want %v", *repo.gotOpts.From, wantFrom)
	}
	if repo.gotOpts.To == nil {
		t.Fatal("to = nil, want parsed time")
	}
	wantTo := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	if !repo.gotOpts.To.Equal(wantTo) {
		t.Errorf("to = %v, want %v", *repo.gotOpts.To, wantTo)
	}
}

func TestTimelineListBadFromReturns400(t *testing.T) {
	t.Parallel()
	repo := &fakeTimelineRepo{page: store.TimelinePage{}}
	srv := newTimelineTestServer(t, repo, "mise_public")

	status, ct, _ := getJSON(t, srv, "/timeline?from=not-a-date")
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestMapTimelineEventsNonNil(t *testing.T) {
	t.Parallel()
	events := mapTimelineEvents(nil)
	if events == nil {
		t.Error("mapTimelineEvents(nil) = nil, want non-nil empty slice")
	}
	data, _ := json.Marshal(events)
	if string(data) != "[]" {
		t.Errorf("json.Marshal(mapTimelineEvents(nil)) = %s, want []", data)
	}
}
