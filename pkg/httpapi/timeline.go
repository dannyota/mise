package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"danny.vn/mise/pkg/store"
)

// TimelineRepoIface is the timeline endpoint's dependency — satisfied by
// *store.TimelineStore — narrowed to the exact methods the handler needs.
type TimelineRepoIface interface {
	ListTimeline(ctx context.Context, role string, opts store.TimelineListOpts) (store.TimelinePage, error)
}

// TimelineEventWire is the wire form of store.TimelineEvent.
type TimelineEventWire struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	CorpusID    string  `json:"corpus_id"`
	DocumentID  *string `json:"document_id"`
	Description string  `json:"description"`
	Timestamp   string  `json:"timestamp"`
}

// TimelineListInput is GET /timeline's input.
type TimelineListInput struct {
	From   string `query:"from" doc:"Start of range (RFC 3339)" example:"2026-01-01T00:00:00Z"`
	To     string `query:"to" doc:"End of range (RFC 3339)" example:"2026-12-31T23:59:59Z"`
	Corpus string `query:"corpus" doc:"Filter by corpus ID" example:"vn-reg"`
	Cursor string `query:"cursor" doc:"Opaque pagination cursor" example:""`
	Limit  int    `query:"limit" doc:"Page size (1-100, default 20)" example:"20"`
}

// TimelineListOutput is GET /timeline's output.
type TimelineListOutput struct {
	Body struct {
		Items      []TimelineEventWire `json:"items"`
		NextCursor string              `json:"next_cursor,omitempty"`
	}
}

// RegisterTimeline mounts the timeline list endpoint.
func RegisterTimeline(api huma.API, repo TimelineRepoIface, role string) {
	huma.Register(api, huma.Operation{
		OperationID: "list-timeline",
		Method:      http.MethodGet,
		Path:        "/timeline",
		Summary:     "List timeline events (paginated, filterable by date/corpus)",
		Tags:        []string{"Timeline"},
		Errors:      []int{http.StatusBadRequest},
	}, newTimelineListHandler(repo, role))
}

func newTimelineListHandler(
	repo TimelineRepoIface, role string,
) func(context.Context, *TimelineListInput) (*TimelineListOutput, error) {
	return func(ctx context.Context, in *TimelineListInput) (*TimelineListOutput, error) {
		opts := store.TimelineListOpts{
			Corpus: in.Corpus,
			Cursor: in.Cursor,
			Limit:  in.Limit,
		}

		if in.From != "" {
			t, err := time.Parse(time.RFC3339, in.From)
			if err != nil {
				return nil, huma.Error400BadRequest("invalid from timestamp", err)
			}
			opts.From = &t
		}
		if in.To != "" {
			t, err := time.Parse(time.RFC3339, in.To)
			if err != nil {
				return nil, huma.Error400BadRequest("invalid to timestamp", err)
			}
			opts.To = &t
		}

		var page store.TimelinePage
		if repo != nil {
			var err error
			page, err = repo.ListTimeline(ctx, role, opts)
			if err != nil {
				return nil, fmt.Errorf("httpapi: listing timeline: %w", err)
			}
		}

		out := &TimelineListOutput{}
		out.Body.Items = mapTimelineEvents(page.Items)
		out.Body.NextCursor = page.NextCursor
		return out, nil
	}
}

// mapTimelineEvents maps store.TimelineEvent rows to their wire form. Always
// returns a non-nil slice.
func mapTimelineEvents(events []store.TimelineEvent) []TimelineEventWire {
	out := make([]TimelineEventWire, len(events))
	for i, e := range events {
		var docID *string
		if e.DocumentID != nil {
			s := e.DocumentID.String()
			docID = &s
		}
		out[i] = TimelineEventWire{
			ID:          e.ID.String(),
			Kind:        e.Kind,
			CorpusID:    e.CorpusID,
			DocumentID:  docID,
			Description: e.Description,
			Timestamp:   e.Timestamp.Format(time.RFC3339),
		}
	}
	return out
}
