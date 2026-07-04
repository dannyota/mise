package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// --- Input/Output types ---

// TranslateInput is POST /translate's input.
type TranslateInput struct {
	Body struct {
		Text       string `json:"text" doc:"Text to translate" example:"Requirement for IT system safety"`
		SourceLang string `json:"source_lang" doc:"BCP-47 source language code" example:"vi"`
		TargetLang string `json:"target_lang" doc:"BCP-47 target language code" example:"en"`
	}
}

// TranslateOutput is POST /translate's output (501 stub).
type TranslateOutput struct {
	Body struct {
		TranslatedText string `json:"translated_text"`
		SourceLang     string `json:"source_lang"`
		TargetLang     string `json:"target_lang"`
	}
}

// RegisterTranslate mounts the cross-lingual translation endpoint.
//
// Implementation is gated on AI-GOVERNANCE section 7: confidential-tier text is
// refused unless the AI gate permits it; public-corpus text is always allowed.
// The endpoint delegates to the Google Cloud Translation API (a managed GCP
// service, not the reasoning LLM). Until Translation API credentials are
// provisioned and the tier-gating logic is implemented, this returns 501.
func RegisterTranslate(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "translate",
		Method:      http.MethodPost,
		Path:        "/translate",
		Summary:     "Translate evidence text via Google Cloud Translation API (gated per AI-GOVERNANCE section 7)",
		Tags:        []string{"Translation"},
		Errors:      []int{http.StatusNotImplemented},
	}, newTranslateHandler())
}

func newTranslateHandler() func(context.Context, *TranslateInput) (*TranslateOutput, error) {
	return func(_ context.Context, _ *TranslateInput) (*TranslateOutput, error) {
		// Needs Google Cloud Translation API credentials and the
		// confidential-tier gating logic (AI-GOVERNANCE §7).
		return nil, huma.Error501NotImplemented(
			"translation is not yet implemented — gated on AI-GOVERNANCE §7",
		)
	}
}
