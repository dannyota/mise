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

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

type tierFilterReviewRepo struct {
	items []store.ReviewItem
}

func (r *tierFilterReviewRepo) ListReviewQueue(
	_ context.Context, role string, _ store.ReviewListOpts,
) (store.ReviewPage, error) {
	return store.ReviewPage{Items: filterReviewByRole(r.items, role)}, nil
}

func (*tierFilterReviewRepo) PromoteEdge(context.Context, uuid.UUID, string) error { return nil }
func (*tierFilterReviewRepo) RejectEdge(context.Context, uuid.UUID) error          { return nil }
func (*tierFilterReviewRepo) RelinkEdge(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

type tierFilterFindingRepo struct {
	items []store.Finding
}

func (r *tierFilterFindingRepo) ListFindings(
	_ context.Context, role string, _ store.FindingListOpts,
) (store.FindingPage, error) {
	return store.FindingPage{Items: filterFindingByRole(r.items, role)}, nil
}

func (r *tierFilterFindingRepo) GetFinding(
	_ context.Context, role string, id uuid.UUID,
) (store.Finding, error) {
	for _, f := range r.items {
		if f.ID == id && canSee(f.AccessTier, role) {
			return f, nil
		}
	}
	return store.Finding{}, store.ErrFindingNotFound
}

func (*tierFilterFindingRepo) CreateResolution(
	_ context.Context, _ uuid.UUID, _ store.Resolution,
) (uuid.UUID, error) {
	return uuid.New(), nil
}

func canSee(tier, role string) bool {
	switch tier {
	case "local-confidential":
		return role == "mise_local"
	case "group-confidential":
		return role == "mise_local" || role == "mise_group"
	default:
		return true
	}
}

func filterReviewByRole(items []store.ReviewItem, role string) []store.ReviewItem {
	var out []store.ReviewItem
	for _, it := range items {
		if canSee(string(it.Edge.AccessTier), role) {
			out = append(out, it)
		}
	}
	return out
}

func filterFindingByRole(items []store.Finding, role string) []store.Finding {
	var out []store.Finding
	for _, f := range items {
		if canSee(f.AccessTier, role) {
			out = append(out, f)
		}
	}
	return out
}

func localConfidentialEdge() store.ReviewItem {
	return store.ReviewItem{
		Edge: graph.Edge{
			ID:         uuid.New(),
			From:       graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()},
			ToRefID:    uuid.New(),
			ToCorpusID: "local-policy",
			EdgeType:   "satisfies",
			Direction:  "up",
			AccessTier: "local-confidential",
			CreatedAt:  time.Now(),
		},
		Confidence: 0.9,
		Grounding:  0.85,
	}
}

func localConfidentialFinding() store.Finding {
	return store.Finding{
		ID:         uuid.New(),
		Kind:       "conflict",
		Severity:   "critical",
		Status:     "open",
		AccessTier: "local-confidential",
		DetectedAt: time.Now(),
		DedupKey:   "deny-test",
	}
}

func denyTestServer(
	t *testing.T, review ReviewRepoIface, findings FindingRepoIface, role string,
) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterReviews(api, review, findings, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestReviewQueueDenyPublic(t *testing.T) {
	t.Parallel()
	review := &tierFilterReviewRepo{items: []store.ReviewItem{localConfidentialEdge()}}
	findings := &tierFilterFindingRepo{}
	srv := denyTestServer(t, review, findings, "mise_public")

	resp, err := http.Get(srv.URL + "/reviews") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /reviews: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body ReviewListBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 0 {
		t.Errorf("mise_public sees %d items, want 0", len(body.Items))
	}
}

func TestReviewQueueDenyGroup(t *testing.T) {
	t.Parallel()
	review := &tierFilterReviewRepo{items: []store.ReviewItem{localConfidentialEdge()}}
	findings := &tierFilterFindingRepo{}
	srv := denyTestServer(t, review, findings, "mise_group")

	resp, err := http.Get(srv.URL + "/reviews") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /reviews: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body ReviewListBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 0 {
		t.Errorf("mise_group sees %d items, want 0", len(body.Items))
	}
}

func TestReviewQueueAllowLocal(t *testing.T) {
	t.Parallel()
	review := &tierFilterReviewRepo{items: []store.ReviewItem{localConfidentialEdge()}}
	findings := &tierFilterFindingRepo{}
	srv := denyTestServer(t, review, findings, "mise_local")

	resp, err := http.Get(srv.URL + "/reviews") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /reviews: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body ReviewListBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("mise_local sees %d items, want 1", len(body.Items))
	}
}

func TestFindingsDenyPublic(t *testing.T) {
	t.Parallel()
	findings := &tierFilterFindingRepo{items: []store.Finding{localConfidentialFinding()}}
	srv := denyTestServer(t, &tierFilterReviewRepo{}, findings, "mise_public")

	resp, err := http.Get(srv.URL + "/findings") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /findings: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body FindingListBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 0 {
		t.Errorf("mise_public sees %d findings, want 0", len(body.Items))
	}
}

func TestFindingsDenyGroup(t *testing.T) {
	t.Parallel()
	findings := &tierFilterFindingRepo{items: []store.Finding{localConfidentialFinding()}}
	srv := denyTestServer(t, &tierFilterReviewRepo{}, findings, "mise_group")

	resp, err := http.Get(srv.URL + "/findings") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /findings: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body FindingListBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 0 {
		t.Errorf("mise_group sees %d findings, want 0", len(body.Items))
	}
}

func TestFindingsAllowLocal(t *testing.T) {
	t.Parallel()
	findings := &tierFilterFindingRepo{items: []store.Finding{localConfidentialFinding()}}
	srv := denyTestServer(t, &tierFilterReviewRepo{}, findings, "mise_local")

	resp, err := http.Get(srv.URL + "/findings") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /findings: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body FindingListBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("mise_local sees %d findings, want 1", len(body.Items))
	}
	if body.Items[0].Kind != "conflict" {
		t.Errorf("kind = %q, want conflict", body.Items[0].Kind)
	}
}
