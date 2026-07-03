package httpapi

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/store"
)

func TestListFindingsReturnsItems(t *testing.T) {
	t.Parallel()
	findingID := uuid.New()
	docID := uuid.New()
	detectedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	repo := &fakeFindingRepo{
		page: store.FindingPage{
			Items: []store.Finding{{
				ID:       findingID,
				Kind:     "gap",
				Severity: "high",
				Status:   "open",
				NodeRefs: []store.NodeRefJSON{{
					CorpusID:   "vn-reg",
					DocumentID: docID,
				}},
				Evidence:   json.RawMessage(`{"detail":"test"}`),
				AccessTier: "public",
				DetectedAt: detectedAt,
				DedupKey:   "gap:test",
			}},
			NextCursor: "cursor-xyz",
		},
	}

	srv := newReviewTestServer(t, &fakeReviewRepo{}, repo, "mise_public")
	status, _, body := getJSON(t, srv, "/findings?kind=gap&status=open&limit=5")

	if status != 200 {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if repo.gotRole != "mise_public" {
		t.Errorf("repo received role = %q, want mise_public", repo.gotRole)
	}
	if repo.gotOpts.Kind != "gap" || repo.gotOpts.Status != "open" || repo.gotOpts.Limit != 5 {
		t.Errorf("opts = %+v, want kind=gap status=open limit=5", repo.gotOpts)
	}

	var got FindingListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(got.Items))
	}
	if got.Items[0].ID != findingID.String() || got.Items[0].Kind != "gap" {
		t.Errorf("Items[0] = %+v, want id %s kind gap", got.Items[0], findingID)
	}
	if got.NextCursor != "cursor-xyz" {
		t.Errorf("NextCursor = %q, want cursor-xyz", got.NextCursor)
	}
}

func TestGetFindingReturns200(t *testing.T) {
	t.Parallel()
	findingID := uuid.New()
	docID := uuid.New()
	detectedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	repo := &fakeFindingRepo{
		finding: store.Finding{
			ID:       findingID,
			Kind:     "conflict",
			Severity: "medium",
			Status:   "open",
			NodeRefs: []store.NodeRefJSON{{
				CorpusID:   "my-reg",
				DocumentID: docID,
			}},
			Evidence:   json.RawMessage(`{}`),
			AccessTier: "public",
			DetectedAt: detectedAt,
			DedupKey:   "conflict:test",
		},
	}

	srv := newReviewTestServer(t, &fakeReviewRepo{}, repo, "mise_group")
	status, _, body := getJSON(t, srv, "/findings/"+findingID.String())

	if status != 200 {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if repo.gotID != findingID {
		t.Errorf("repo received id = %v, want %v", repo.gotID, findingID)
	}

	var got FindingWire
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if got.ID != findingID.String() || got.Kind != "conflict" {
		t.Errorf("got = %+v, want id %s kind conflict", got, findingID)
	}
}

func TestGetFindingNotFoundReturns404(t *testing.T) {
	t.Parallel()
	repo := &fakeFindingRepo{getErr: store.ErrFindingNotFound}
	srv := newReviewTestServer(t, &fakeReviewRepo{}, repo, "mise_public")

	status, ct, _ := getJSON(t, srv, "/findings/"+uuid.NewString())
	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestGetFindingBadUUIDReturns400(t *testing.T) {
	t.Parallel()
	srv := newReviewTestServer(t, &fakeReviewRepo{}, &fakeFindingRepo{}, "mise_public")
	status, _, _ := getJSON(t, srv, "/findings/not-a-uuid")
	if status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestCreateResolutionReturnsID(t *testing.T) {
	t.Parallel()
	findingID := uuid.New()
	resID := uuid.New()
	repo := &fakeFindingRepo{resID: resID}

	srv := newReviewTestServer(t, &fakeReviewRepo{}, repo, "mise_public")
	body := map[string]string{
		"disposition":      "accepted",
		"owner_department": "compliance",
		"owner_role":       "reviewer",
		"status":           "open",
		"rationale":        "reviewed and accepted",
		"due_date":         "2026-12-31T00:00:00Z",
	}
	status, _, respBody := postJSON(t, srv, "/findings/"+findingID.String()+"/resolution", body)

	if status != 200 {
		t.Fatalf("status = %d, want 200; body: %s", status, respBody)
	}
	if repo.gotID != findingID {
		t.Errorf("findingID = %v, want %v", repo.gotID, findingID)
	}
	if repo.gotRes.Disposition != "accepted" {
		t.Errorf("disposition = %q, want accepted", repo.gotRes.Disposition)
	}
	if repo.gotRes.DueDate == nil {
		t.Fatal("dueDate = nil, want non-nil")
	}

	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, respBody)
	}
	if got.ID != resID.String() {
		t.Errorf("response id = %q, want %s", got.ID, resID)
	}
}

func TestCreateResolutionBadFindingIDReturnsError(t *testing.T) {
	t.Parallel()
	srv := newReviewTestServer(t, &fakeReviewRepo{}, &fakeFindingRepo{}, "mise_public")
	status, _, _ := postJSON(t, srv, "/findings/not-a-uuid/resolution",
		map[string]string{"disposition": "x"})
	if status < 400 || status >= 500 {
		t.Fatalf("status = %d, want 4xx client error", status)
	}
}

func TestCreateResolutionStoreErrorReturnsError(t *testing.T) {
	t.Parallel()
	repo := &fakeFindingRepo{createErr: errors.New("db error")}
	srv := newReviewTestServer(t, &fakeReviewRepo{}, repo, "mise_public")

	status, _, _ := postJSON(t, srv, "/findings/"+uuid.NewString()+"/resolution",
		map[string]string{"disposition": "x"})
	if status < 400 {
		t.Fatalf("status = %d, want >= 400", status)
	}
}

func TestMapFindingsNonNil(t *testing.T) {
	t.Parallel()
	findings := mapFindings(nil)
	if findings == nil {
		t.Error("mapFindings(nil) = nil, want non-nil empty slice")
	}
	data, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("marshaling: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("json.Marshal(mapFindings(nil)) = %s, want []", data)
	}
}
