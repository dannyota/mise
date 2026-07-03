// Package detect implements cross-corpus detection logic — thresholding,
// gating, and escalation for control-detection candidates.
package detect

import "danny.vn/mise/pkg/vertex"

// ThresholdConfig holds the configurable thresholds for the detection gate.
// A candidate passes the gate only when both the judge confidence and the
// grounding score meet their respective minimums.
type ThresholdConfig struct {
	ConfidenceMin   float64 // minimum judge confidence (default 0.7)
	GroundingMin    float64 // minimum grounding score (default 0.6)
	Model           string  // primary judge model (default "gemini-3.5-flash")
	EscalationModel string  // escalation model for low-confidence edges (default "")
}

// Gate returns true when both the judge result and ground result meet their
// configured thresholds, meaning the candidate should be written.
func (tc ThresholdConfig) Gate(jr vertex.JudgeResult, gr vertex.GroundResult) bool {
	return jr.Confidence >= tc.ConfidenceMin && gr.Score >= tc.GroundingMin
}
