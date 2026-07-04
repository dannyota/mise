package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// ReviewRepoIface is the review endpoints' dependency — satisfied by
// *store.ReviewStore — narrowed to the exact methods the handlers need.
type ReviewRepoIface interface {
	ListReviewQueue(ctx context.Context, role string, opts store.ReviewListOpts) (store.ReviewPage, error)
	PromoteEdge(ctx context.Context, edgeID uuid.UUID, promotedBy string) error
	RejectEdge(ctx context.Context, edgeID uuid.UUID) error
	RelinkEdge(ctx context.Context, edgeID, newTarget uuid.UUID) error
}

// FindingRepoIface is the finding endpoints' dependency — satisfied by
// *store.FindingStore — narrowed to the exact methods the handlers need.
type FindingRepoIface interface {
	ListFindings(ctx context.Context, role string, opts store.FindingListOpts) (store.FindingPage, error)
	GetFinding(ctx context.Context, role string, id uuid.UUID) (store.Finding, error)
	CreateResolution(ctx context.Context, findingID uuid.UUID, res store.Resolution) (uuid.UUID, error)
}

// --- Wire types ---

// ReviewItemWire is the wire form of store.ReviewItem.
type ReviewItemWire struct {
	Edge       EdgeWire `json:"edge"`
	Confidence float64  `json:"confidence"`
	Grounding  float64  `json:"grounding_score"`
}

// ReviewListBody is GET /reviews's response body.
type ReviewListBody struct {
	Items      []ReviewItemWire `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// ReviewListInput is GET /reviews's input.
type ReviewListInput struct {
	Cursor string `query:"cursor" doc:"Opaque pagination cursor" example:""`
	Limit  int    `query:"limit" doc:"Page size (1-100, default 20)" example:"20"`
	Status string `query:"status" doc:"Filter by edge_type" example:""`
	Sort   string `query:"sort" doc:"Sort order: created_at (default) or confidence" example:"created_at"`
}

// ReviewListOutput is GET /reviews's output.
type ReviewListOutput struct {
	Body ReviewListBody
}

// PromoteInput is POST /reviews/{edge}/promote's input.
type PromoteInput struct {
	Edge           string `path:"edge" doc:"Edge UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	IdempotencyKey string `header:"Idempotency-Key"`
	Body           struct {
		PromotedBy string `json:"promoted_by" doc:"Attestation owner" example:"reviewer@example.com"`
	}
}

// PromoteOutput is POST /reviews/{edge}/promote's output.
type PromoteOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// RejectInput is POST /reviews/{edge}/reject's input.
type RejectInput struct {
	Edge           string `path:"edge" doc:"Edge UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	IdempotencyKey string `header:"Idempotency-Key"`
}

// RejectOutput is POST /reviews/{edge}/reject's output.
type RejectOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// RelinkInput is POST /reviews/{edge}/relink's input.
type RelinkInput struct {
	Edge           string `path:"edge" doc:"Edge UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	IdempotencyKey string `header:"Idempotency-Key"`
	Body           struct {
		NewTarget string `json:"new_target" doc:"New target doc_ref UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}
}

// RelinkOutput is POST /reviews/{edge}/relink's output.
type RelinkOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// FindingWire is the wire form of store.Finding.
type FindingWire struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Severity   string          `json:"severity"`
	Status     string          `json:"status"`
	NodeRefs   []NodeRefWire   `json:"node_refs"`
	Evidence   json.RawMessage `json:"evidence"`
	AccessTier string          `json:"access_tier"`
	DetectedAt string          `json:"detected_at"`
	DedupKey   string          `json:"dedup_key"`
}

// FindingListBody is GET /findings's response body.
type FindingListBody struct {
	Items      []FindingWire `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// FindingListInput is GET /findings's input.
type FindingListInput struct {
	Cursor string `query:"cursor" doc:"Opaque pagination cursor" example:""`
	Limit  int    `query:"limit" doc:"Page size (1-100, default 20)" example:"20"`
	Kind   string `query:"kind" doc:"Filter by finding kind (gap, conflict, stale)" example:""`
	Status string `query:"status" doc:"Filter by finding status" example:""`
}

// FindingListOutput is GET /findings's output.
type FindingListOutput struct {
	Body FindingListBody
}

// FindingGetInput is GET /findings/{id}'s input.
type FindingGetInput struct {
	ID string `path:"id" doc:"Finding UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// FindingGetOutput is GET /findings/{id}'s output.
type FindingGetOutput struct {
	Body FindingWire
}

// ResolutionInput is POST /findings/{id}/resolution's input.
type ResolutionInput struct {
	ID             string `path:"id" doc:"Finding UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	IdempotencyKey string `header:"Idempotency-Key"`
	Body           struct {
		Disposition string  `json:"disposition" doc:"Resolution disposition" example:"accepted"`
		OwnerDept   string  `json:"owner_department" doc:"Owning department" example:"compliance"`
		OwnerRole   string  `json:"owner_role" doc:"Owning role" example:"reviewer"`
		Status      string  `json:"status" doc:"Resolution status" example:"open"`
		Rationale   string  `json:"rationale" doc:"Resolution rationale" example:"Reviewed and accepted"`
		DueDate     *string `json:"due_date,omitempty" doc:"Due date (RFC 3339)" example:"2026-12-31T00:00:00Z"`
	}
}

// ResolutionOutput is POST /findings/{id}/resolution's output.
type ResolutionOutput struct {
	Body struct {
		ID string `json:"id" doc:"New resolution UUID"`
	}
}

// RegisterReviews wires the review and finding REST operations onto api.
func RegisterReviews(
	api huma.API, reviewRepo ReviewRepoIface, findingRepo FindingRepoIface, role string,
) {
	huma.Register(api, huma.Operation{
		OperationID: "list-reviews",
		Method:      http.MethodGet,
		Path:        "/reviews",
		Summary:     "List unpromoted edges in the review queue",
		Tags:        []string{"Reviews"},
		Errors:      []int{http.StatusBadRequest},
	}, newListReviewsHandler(reviewRepo, role))

	huma.Register(api, huma.Operation{
		OperationID: "promote-edge",
		Method:      http.MethodPost,
		Path:        "/reviews/{edge}/promote",
		Summary:     "Promote a candidate edge with human attestation",
		Tags:        []string{"Reviews"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newPromoteHandler(reviewRepo, role))

	huma.Register(api, huma.Operation{
		OperationID: "reject-edge",
		Method:      http.MethodPost,
		Path:        "/reviews/{edge}/reject",
		Summary:     "Reject a candidate edge",
		Tags:        []string{"Reviews"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newRejectHandler(reviewRepo))

	huma.Register(api, huma.Operation{
		OperationID: "relink-edge",
		Method:      http.MethodPost,
		Path:        "/reviews/{edge}/relink",
		Summary:     "Update an edge's target (re-detection deferred to M5)",
		Tags:        []string{"Reviews"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newRelinkHandler(reviewRepo))

	huma.Register(api, huma.Operation{
		OperationID: "list-findings",
		Method:      http.MethodGet,
		Path:        "/findings",
		Summary:     "List findings (paginated, filterable by kind/status)",
		Tags:        []string{"Findings"},
		Errors:      []int{http.StatusBadRequest},
	}, newListFindingsHandler(findingRepo, role))

	huma.Register(api, huma.Operation{
		OperationID: "get-finding",
		Method:      http.MethodGet,
		Path:        "/findings/{id}",
		Summary:     "Get a single finding by UUID",
		Tags:        []string{"Findings"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newGetFindingHandler(findingRepo, role))

	huma.Register(api, huma.Operation{
		OperationID: "create-resolution",
		Method:      http.MethodPost,
		Path:        "/findings/{id}/resolution",
		Summary:     "Create a resolution for a finding",
		Tags:        []string{"Findings"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newCreateResolutionHandler(findingRepo))
}

func newListReviewsHandler(
	repo ReviewRepoIface, role string,
) func(context.Context, *ReviewListInput) (*ReviewListOutput, error) {
	return func(ctx context.Context, in *ReviewListInput) (*ReviewListOutput, error) {
		page, err := repo.ListReviewQueue(ctx, role, store.ReviewListOpts{
			Cursor: in.Cursor,
			Limit:  in.Limit,
			Status: in.Status,
			Sort:   in.Sort,
		})
		if err != nil {
			return nil, fmt.Errorf("httpapi: listing review queue: %w", err)
		}

		out := &ReviewListOutput{}
		out.Body.Items = mapReviewItems(page.Items)
		out.Body.NextCursor = page.NextCursor
		return out, nil
	}
}

func newPromoteHandler(
	repo ReviewRepoIface, role string,
) func(context.Context, *PromoteInput) (*PromoteOutput, error) {
	return func(ctx context.Context, in *PromoteInput) (*PromoteOutput, error) {
		edgeID, err := uuid.Parse(in.Edge)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid edge UUID", err)
		}
		promotedBy := in.Body.PromotedBy
		if promotedBy == "" {
			promotedBy = role
		}
		if err := repo.PromoteEdge(ctx, edgeID, promotedBy); err != nil {
			if errors.Is(err, store.ErrEdgeNotFound) {
				return nil, huma.Error404NotFound("edge not found")
			}
			return nil, fmt.Errorf("httpapi: promoting edge: %w", err)
		}
		out := &PromoteOutput{}
		out.Body.OK = true
		return out, nil
	}
}

func newRejectHandler(repo ReviewRepoIface) func(context.Context, *RejectInput) (*RejectOutput, error) {
	return func(ctx context.Context, in *RejectInput) (*RejectOutput, error) {
		edgeID, err := uuid.Parse(in.Edge)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid edge UUID", err)
		}
		if err := repo.RejectEdge(ctx, edgeID); err != nil {
			if errors.Is(err, store.ErrEdgeNotFound) {
				return nil, huma.Error404NotFound("edge not found")
			}
			return nil, fmt.Errorf("httpapi: rejecting edge: %w", err)
		}
		out := &RejectOutput{}
		out.Body.OK = true
		return out, nil
	}
}

func newRelinkHandler(repo ReviewRepoIface) func(context.Context, *RelinkInput) (*RelinkOutput, error) {
	return func(ctx context.Context, in *RelinkInput) (*RelinkOutput, error) {
		edgeID, err := uuid.Parse(in.Edge)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid edge UUID", err)
		}
		newTarget, err := uuid.Parse(in.Body.NewTarget)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid new_target UUID", err)
		}
		// TODO(M5): re-trigger detection against the new target after relinking.
		if err := repo.RelinkEdge(ctx, edgeID, newTarget); err != nil {
			if errors.Is(err, store.ErrEdgeNotFound) {
				return nil, huma.Error404NotFound("edge not found")
			}
			return nil, fmt.Errorf("httpapi: relinking edge: %w", err)
		}
		out := &RelinkOutput{}
		out.Body.OK = true
		return out, nil
	}
}

func newListFindingsHandler(
	repo FindingRepoIface, role string,
) func(context.Context, *FindingListInput) (*FindingListOutput, error) {
	return func(ctx context.Context, in *FindingListInput) (*FindingListOutput, error) {
		page, err := repo.ListFindings(ctx, role, store.FindingListOpts{
			Cursor: in.Cursor,
			Limit:  in.Limit,
			Kind:   in.Kind,
			Status: in.Status,
		})
		if err != nil {
			return nil, fmt.Errorf("httpapi: listing findings: %w", err)
		}

		out := &FindingListOutput{}
		out.Body.Items = mapFindings(page.Items)
		out.Body.NextCursor = page.NextCursor
		return out, nil
	}
}

func newGetFindingHandler(
	repo FindingRepoIface, role string,
) func(context.Context, *FindingGetInput) (*FindingGetOutput, error) {
	return func(ctx context.Context, in *FindingGetInput) (*FindingGetOutput, error) {
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid finding UUID", err)
		}
		f, err := repo.GetFinding(ctx, role, id)
		if err != nil {
			if errors.Is(err, store.ErrFindingNotFound) {
				return nil, huma.Error404NotFound("finding not found")
			}
			return nil, fmt.Errorf("httpapi: getting finding: %w", err)
		}
		out := &FindingGetOutput{}
		out.Body = findingToWire(f)
		return out, nil
	}
}

func newCreateResolutionHandler(
	repo FindingRepoIface,
) func(context.Context, *ResolutionInput) (*ResolutionOutput, error) {
	return func(ctx context.Context, in *ResolutionInput) (*ResolutionOutput, error) {
		findingID, err := uuid.Parse(in.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid finding UUID", err)
		}

		var dueDate *time.Time
		if in.Body.DueDate != nil {
			t, parseErr := time.Parse(time.RFC3339, *in.Body.DueDate)
			if parseErr != nil {
				return nil, huma.Error400BadRequest("invalid due_date", parseErr)
			}
			dueDate = &t
		}

		resID, err := repo.CreateResolution(ctx, findingID, store.Resolution{
			Disposition: in.Body.Disposition,
			OwnerDept:   in.Body.OwnerDept,
			OwnerRole:   in.Body.OwnerRole,
			Status:      in.Body.Status,
			Rationale:   in.Body.Rationale,
			DueDate:     dueDate,
		})
		if err != nil {
			return nil, fmt.Errorf("httpapi: creating resolution: %w", err)
		}
		out := &ResolutionOutput{}
		out.Body.ID = resID.String()
		return out, nil
	}
}

// mapReviewItems maps store.ReviewItem rows to their wire form. Always
// returns a non-nil slice.
func mapReviewItems(items []store.ReviewItem) []ReviewItemWire {
	out := make([]ReviewItemWire, len(items))
	for i, item := range items {
		out[i] = ReviewItemWire{
			Edge:       edgeToWire(item.Edge),
			Confidence: item.Confidence,
			Grounding:  item.Grounding,
		}
	}
	return out
}

// edgeToWire maps a graph.Edge to its wire form without evidence
// (review items carry top-level confidence/grounding instead).
func edgeToWire(e graph.Edge) EdgeWire {
	return EdgeWire{
		ID:         e.ID.String(),
		From:       nodeRefToWire(e.From),
		ToRefID:    e.ToRefID.String(),
		ToCorpusID: e.ToCorpusID,
		EdgeType:   string(e.EdgeType),
		Direction:  e.Direction,
		Promoted:   e.Promoted,
		AccessTier: string(e.AccessTier),
		CreatedAt:  e.CreatedAt.Format(time.RFC3339),
		Evidence:   make([]EvidenceWire, 0),
	}
}

// mapFindings maps store.Finding rows to their wire form. Always returns a
// non-nil slice.
func mapFindings(findings []store.Finding) []FindingWire {
	out := make([]FindingWire, len(findings))
	for i, f := range findings {
		out[i] = findingToWire(f)
	}
	return out
}

func findingToWire(f store.Finding) FindingWire {
	refs := make([]NodeRefWire, len(f.NodeRefs))
	for i, r := range f.NodeRefs {
		refs[i] = NodeRefWire{
			CorpusID:   r.CorpusID,
			DocumentID: r.DocumentID.String(),
			SectionID:  uuidPtrToWire(r.SectionID),
		}
	}
	ev := f.Evidence
	if ev == nil {
		ev = json.RawMessage(`{}`)
	}
	return FindingWire{
		ID:         f.ID.String(),
		Kind:       f.Kind,
		Severity:   f.Severity,
		Status:     f.Status,
		NodeRefs:   refs,
		Evidence:   ev,
		AccessTier: f.AccessTier,
		DetectedAt: f.DetectedAt.Format(time.RFC3339),
		DedupKey:   f.DedupKey,
	}
}
