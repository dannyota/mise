package detect_test

import (
	"testing"

	"danny.vn/mise/pkg/detect"
	"danny.vn/mise/pkg/vertex"
)

func TestGatePassesBothAboveThreshold(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}
	jr := vertex.JudgeResult{Confidence: 0.9}
	gr := vertex.GroundResult{Score: 0.85}
	if !tc.Gate(jr, gr) {
		t.Error("Gate() = false, want true when both above threshold")
	}
}

func TestGateRejectsLowConfidence(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}
	jr := vertex.JudgeResult{Confidence: 0.5}
	gr := vertex.GroundResult{Score: 0.85}
	if tc.Gate(jr, gr) {
		t.Error("Gate() = true, want false when confidence below threshold")
	}
}

func TestGateRejectsLowGrounding(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}
	jr := vertex.JudgeResult{Confidence: 0.9}
	gr := vertex.GroundResult{Score: 0.3}
	if tc.Gate(jr, gr) {
		t.Error("Gate() = true, want false when grounding below threshold")
	}
}

func TestGateRejectsBothBelowThreshold(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}
	jr := vertex.JudgeResult{Confidence: 0.4}
	gr := vertex.GroundResult{Score: 0.3}
	if tc.Gate(jr, gr) {
		t.Error("Gate() = true, want false when both below threshold")
	}
}

func TestGatePassesAtExactThresholds(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}
	jr := vertex.JudgeResult{Confidence: 0.7}
	gr := vertex.GroundResult{Score: 0.6}
	if !tc.Gate(jr, gr) {
		t.Error("Gate() = false, want true at exact thresholds")
	}
}

func TestGateRejectsConfidenceJustBelowThreshold(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}
	jr := vertex.JudgeResult{Confidence: 0.6999}
	gr := vertex.GroundResult{Score: 0.6}
	if tc.Gate(jr, gr) {
		t.Error("Gate() = true, want false when confidence is 0.6999 < 0.7")
	}
}

func TestGateRejectsGroundingJustBelowThreshold(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}
	jr := vertex.JudgeResult{Confidence: 0.7}
	gr := vertex.GroundResult{Score: 0.5999}
	if tc.Gate(jr, gr) {
		t.Error("Gate() = true, want false when grounding is 0.5999 < 0.6")
	}
}

func TestGateZeroThresholdsPassEverything(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0, GroundingMin: 0}
	jr := vertex.JudgeResult{Confidence: 0}
	gr := vertex.GroundResult{Score: 0}
	if !tc.Gate(jr, gr) {
		t.Error("Gate() = false, want true with zero thresholds and zero scores")
	}
}
