package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

type fakeReviewRepo struct {
	page       store.ReviewPage
	listErr    error
	promoteErr error
	rejectErr  error
	relinkErr  error

	gotRole       string
	gotOpts       store.ReviewListOpts
	gotEdgeID     uuid.UUID
	gotPromotedBy string
	gotNewTarget  uuid.UUID
}

func (f *fakeReviewRepo) ListReviewQueue(
	_ context.Context, role string, opts store.ReviewListOpts,
) (store.ReviewPage, error) {
	f.gotRole, f.gotOpts = role, opts
	return f.page, f.listErr
}

func (f *fakeReviewRepo) PromoteEdge(_ context.Context, edgeID uuid.UUID, promotedBy string) error {
	f.gotEdgeID, f.gotPromotedBy = edgeID, promotedBy
	return f.promoteErr
}

func (f *fakeReviewRepo) RejectEdge(_ context.Context, edgeID uuid.UUID) error {
	f.gotEdgeID = edgeID
	return f.rejectErr
}

func (f *fakeReviewRepo) RelinkEdge(_ context.Context, edgeID, newTarget uuid.UUID) error {
	f.gotEdgeID, f.gotNewTarget = edgeID, newTarget
	return f.relinkErr
}

type fakeFindingRepo struct {
	page      store.FindingPage
	listErr   error
	finding   store.Finding
	getErr    error
	resID     uuid.UUID
	createErr error

	gotRole string
	gotOpts store.FindingListOpts
	gotID   uuid.UUID
	gotRes  store.Resolution
}

func (f *fakeFindingRepo) ListFindings(
	_ context.Context, role string, opts store.FindingListOpts,
) (store.FindingPage, error) {
	f.gotRole, f.gotOpts = role, opts
	return f.page, f.listErr
}

func (f *fakeFindingRepo) GetFinding(
	_ context.Context, role string, id uuid.UUID,
) (store.Finding, error) {
	f.gotRole, f.gotID = role, id
	return f.finding, f.getErr
}

func (f *fakeFindingRepo) CreateResolution(
	_ context.Context, findingID uuid.UUID, res store.Resolution,
) (uuid.UUID, error) {
	f.gotID, f.gotRes = findingID, res
	return f.resID, f.createErr
}

func newReviewTestServer(
	t *testing.T, review ReviewRepoIface, finding FindingRepoIface, role string,
) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	Register(api, nil, review, finding, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, srv *httptest.Server, path string, body any) (int, string, []byte) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshaling request body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader([]byte("{}"))
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+path, reqBody)
	if err != nil {
		t.Fatalf("POST %s: creating request: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s error = %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("POST %s: reading body: %v", path, err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), respBody
}

func TestListReviewsReturnsItems(t *testing.T) {
	t.Parallel()
	edgeID := uuid.New()
	docID := uuid.New()
	createdAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	repo := &fakeReviewRepo{
		page: store.ReviewPage{
			Items: []store.ReviewItem{{
				Edge: graph.Edge{
					ID:         edgeID,
					From:       graph.NodeRef{CorpusID: "vn-reg", DocumentID: docID},
					ToRefID:    uuid.New(),
					ToCorpusID: "my-reg",
					EdgeType:   graph.EdgeSatisfies,
					Direction:  "up",
					Promoted:   false,
					AccessTier: graph.TierPublic,
					CreatedAt:  createdAt,
				},
				Confidence: 0.85,
				Grounding:  0.91,
			}},
			NextCursor: "next-abc",
		},
	}

	srv := newReviewTestServer(t, repo, &fakeFindingRepo{}, "mise_group")
	status, ct, body := getJSON(t, srv, "/reviews?limit=10&sort=confidence")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if repo.gotRole != "mise_group" {
		t.Errorf("repo received role = %q, want mise_group", repo.gotRole)
	}
	if repo.gotOpts.Limit != 10 {
		t.Errorf("repo received limit = %d, want 10", repo.gotOpts.Limit)
	}
	if repo.gotOpts.Sort != "confidence" {
		t.Errorf("repo received sort = %q, want confidence", repo.gotOpts.Sort)
	}

	var got ReviewListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling ReviewListBody: %v; body: %s", err, body)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(got.Items))
	}
	if got.Items[0].Edge.ID != edgeID.String() {
		t.Errorf("Items[0].Edge.ID = %q, want %s", got.Items[0].Edge.ID, edgeID)
	}
	if got.Items[0].Confidence != 0.85 {
		t.Errorf("Items[0].Confidence = %v, want 0.85", got.Items[0].Confidence)
	}
	if got.NextCursor != "next-abc" {
		t.Errorf("NextCursor = %q, want next-abc", got.NextCursor)
	}
}

func TestListReviewsEmptyReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	repo := &fakeReviewRepo{page: store.ReviewPage{}}
	srv := newReviewTestServer(t, repo, &fakeFindingRepo{}, "mise_public")

	status, _, body := getJSON(t, srv, "/reviews")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	var got ReviewListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}
	if got.Items == nil || len(got.Items) != 0 {
		t.Errorf("Items = %v, want non-nil empty slice", got.Items)
	}
}

func TestPromoteEdgeReturnsOK(t *testing.T) {
	t.Parallel()
	edgeID := uuid.New()
	repo := &fakeReviewRepo{}

	srv := newReviewTestServer(t, repo, &fakeFindingRepo{}, "mise_group")
	body := map[string]string{"promoted_by": "reviewer@example.com"}
	status, _, respBody := postJSON(t, srv, "/reviews/"+edgeID.String()+"/promote", body)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, respBody)
	}
	if repo.gotEdgeID != edgeID {
		t.Errorf("repo received edgeID = %v, want %v", repo.gotEdgeID, edgeID)
	}
	if repo.gotPromotedBy != "reviewer@example.com" {
		t.Errorf("repo received promotedBy = %q, want reviewer@example.com", repo.gotPromotedBy)
	}
}

func TestPromoteEdgeNotFoundReturns404(t *testing.T) {
	t.Parallel()
	repo := &fakeReviewRepo{promoteErr: store.ErrEdgeNotFound}
	srv := newReviewTestServer(t, repo, &fakeFindingRepo{}, "mise_public")

	status, ct, _ := postJSON(t, srv, "/reviews/"+uuid.NewString()+"/promote",
		map[string]string{"promoted_by": "x"})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestPromoteEdgeBadUUIDReturns400(t *testing.T) {
	t.Parallel()
	srv := newReviewTestServer(t, &fakeReviewRepo{}, &fakeFindingRepo{}, "mise_public")
	status, _, _ := postJSON(t, srv, "/reviews/not-a-uuid/promote",
		map[string]string{"promoted_by": "x"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestRejectEdgeReturnsOK(t *testing.T) {
	t.Parallel()
	edgeID := uuid.New()
	repo := &fakeReviewRepo{}
	srv := newReviewTestServer(t, repo, &fakeFindingRepo{}, "mise_public")

	status, _, respBody := postJSON(t, srv, "/reviews/"+edgeID.String()+"/reject", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, respBody)
	}
	if repo.gotEdgeID != edgeID {
		t.Errorf("repo received edgeID = %v, want %v", repo.gotEdgeID, edgeID)
	}
}

func TestRejectEdgeNotFoundReturns404(t *testing.T) {
	t.Parallel()
	repo := &fakeReviewRepo{rejectErr: store.ErrEdgeNotFound}
	srv := newReviewTestServer(t, repo, &fakeFindingRepo{}, "mise_public")

	status, _, _ := postJSON(t, srv, "/reviews/"+uuid.NewString()+"/reject", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestRelinkEdgeReturnsOK(t *testing.T) {
	t.Parallel()
	edgeID := uuid.New()
	newTarget := uuid.New()
	repo := &fakeReviewRepo{}
	srv := newReviewTestServer(t, repo, &fakeFindingRepo{}, "mise_public")

	body := map[string]string{"new_target": newTarget.String()}
	status, _, respBody := postJSON(t, srv, "/reviews/"+edgeID.String()+"/relink", body)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, respBody)
	}
	if repo.gotEdgeID != edgeID {
		t.Errorf("repo received edgeID = %v, want %v", repo.gotEdgeID, edgeID)
	}
	if repo.gotNewTarget != newTarget {
		t.Errorf("repo received newTarget = %v, want %v", repo.gotNewTarget, newTarget)
	}
}

func TestRelinkEdgeBadTargetReturns400(t *testing.T) {
	t.Parallel()
	srv := newReviewTestServer(t, &fakeReviewRepo{}, &fakeFindingRepo{}, "mise_public")
	body := map[string]string{"new_target": "not-a-uuid"}
	status, _, _ := postJSON(t, srv, "/reviews/"+uuid.NewString()+"/relink", body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestMapReviewItemsNonNil(t *testing.T) {
	t.Parallel()
	items := mapReviewItems(nil)
	if items == nil {
		t.Error("mapReviewItems(nil) = nil, want non-nil empty slice")
	}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshaling: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("json.Marshal(mapReviewItems(nil)) = %s, want []", data)
	}
}
