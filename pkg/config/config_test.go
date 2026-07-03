package config_test

import (
	"context"
	"slices"
	"testing"

	"danny.vn/mise/pkg/blob"
	"danny.vn/mise/pkg/config"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

func TestDBDefaultsWhenUnset(t *testing.T) {
	t.Setenv("ALLOYDB_HOST", "")
	t.Setenv("ALLOYDB_PORT", "")
	t.Setenv("ALLOYDB_USER", "")
	t.Setenv("ALLOYDB_PASSWORD", "")
	t.Setenv("ALLOYDB_DATABASE", "")

	got := config.DB()
	want := store.Config{Host: "localhost", Port: 5432, User: "mise", Password: "mise-dev", Database: "mise"}
	if got != want {
		t.Errorf("DB() = %+v, want %+v", got, want)
	}
}

func TestDBUsesEnvOverrides(t *testing.T) {
	t.Setenv("ALLOYDB_HOST", "db.example")
	t.Setenv("ALLOYDB_PORT", "5555")
	t.Setenv("ALLOYDB_USER", "u")
	t.Setenv("ALLOYDB_PASSWORD", "p")
	t.Setenv("ALLOYDB_DATABASE", "d")

	got := config.DB()
	want := store.Config{Host: "db.example", Port: 5555, User: "u", Password: "p", Database: "d"}
	if got != want {
		t.Errorf("DB() = %+v, want %+v", got, want)
	}
}

func TestDBFallsBackOnUnparseablePort(t *testing.T) {
	t.Setenv("ALLOYDB_PORT", "not-a-number")
	if got := config.DB().Port; got != 5432 {
		t.Errorf("DB().Port = %d, want fallback 5432", got)
	}
}

func TestNewEmbedderFake(t *testing.T) {
	t.Setenv("VERTEX", "fake")
	e, err := config.NewEmbedder(context.Background())
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v, want nil", err)
	}
	if e.Model() != "gemini-embedding-001" || e.Dims() != 1536 {
		t.Errorf("NewEmbedder() = model %q dims %d, want gemini-embedding-001/1536", e.Model(), e.Dims())
	}
}

func TestNewEmbedderDefaultsToFakeWhenUnset(t *testing.T) {
	t.Setenv("VERTEX", "")
	e, err := config.NewEmbedder(context.Background())
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v, want nil", err)
	}
	if e == nil {
		t.Fatal("NewEmbedder() = nil, want the fake embedder")
	}
}

func TestNewEmbedderRejectsUnknownValue(t *testing.T) {
	t.Setenv("VERTEX", "bogus")
	if _, err := config.NewEmbedder(context.Background()); err == nil {
		t.Fatal("NewEmbedder() error = nil, want error for unknown VERTEX value")
	}
}

func TestNewEmbedderRealRequiresProject(t *testing.T) {
	t.Setenv("VERTEX", "real")
	t.Setenv("GCP_PROJECT", "")
	if _, err := config.NewEmbedder(context.Background()); err == nil {
		t.Fatal("NewEmbedder() error = nil, want error when GCP_PROJECT is unset")
	}
}

func TestNewBlobDefaultsToFS(t *testing.T) {
	t.Setenv("GCS_BUCKET", "")
	t.Setenv("BLOB_DIR", t.TempDir())
	b, err := config.NewBlob(context.Background())
	if err != nil {
		t.Fatalf("NewBlob() error = %v, want nil", err)
	}
	if _, ok := b.(*blob.FS); !ok {
		t.Errorf("NewBlob() = %T, want *blob.FS when GCS_BUCKET is unset", b)
	}
}

func TestNewRankerFake(t *testing.T) {
	t.Setenv("VERTEX", "fake")
	r, err := config.NewRanker(context.Background())
	if err != nil {
		t.Fatalf("NewRanker() error = %v, want nil", err)
	}
	if r == nil {
		t.Fatal("NewRanker() = nil, want the fake ranker")
	}
}

func TestNewRankerDefaultsToFakeWhenUnset(t *testing.T) {
	t.Setenv("VERTEX", "")
	r, err := config.NewRanker(context.Background())
	if err != nil {
		t.Fatalf("NewRanker() error = %v, want the fake default", err)
	}
	if r == nil {
		t.Fatal("NewRanker() = nil, want the fake ranker")
	}
}

func TestNewRankerRejectsUnknownValue(t *testing.T) {
	t.Setenv("VERTEX", "bogus")
	if _, err := config.NewRanker(context.Background()); err == nil {
		t.Fatal("NewRanker() error = nil, want error for unknown VERTEX value")
	}
}

func TestNewRankerRealRequiresProject(t *testing.T) {
	t.Setenv("VERTEX", "real")
	t.Setenv("GCP_PROJECT", "")
	if _, err := config.NewRanker(context.Background()); err == nil {
		t.Fatal("NewRanker() error = nil, want error when GCP_PROJECT is unset")
	}
}

func TestNewParserFake(t *testing.T) {
	t.Setenv("VERTEX", "fake")
	p, err := config.NewParser(context.Background())
	if err != nil {
		t.Fatalf("NewParser() error = %v, want nil", err)
	}
	if p == nil {
		t.Fatal("NewParser() = nil, want the fake parser")
	}
}

func TestNewParserDefaultsToFakeWhenUnset(t *testing.T) {
	t.Setenv("VERTEX", "")
	if _, err := config.NewParser(context.Background()); err != nil {
		t.Fatalf("NewParser() error = %v, want the fake default", err)
	}
}

func TestNewParserRejectsUnknownValue(t *testing.T) {
	t.Setenv("VERTEX", "bogus")
	if _, err := config.NewParser(context.Background()); err == nil {
		t.Fatal("NewParser() error = nil, want error for unknown VERTEX value")
	}
}

func TestNewParserRealRequiresProcessorConfig(t *testing.T) {
	t.Setenv("VERTEX", "real")
	t.Setenv("GCP_PROJECT", "p")
	t.Setenv("DOCAI_PROCESSOR_ID", "")
	if _, err := config.NewParser(context.Background()); err == nil {
		t.Fatal("NewParser() error = nil, want error when DOCAI_PROCESSOR_ID is unset")
	}
}

func TestNewSourcesWiresBothLawCorpora(t *testing.T) {
	sources, err := config.NewSources(context.Background())
	if err != nil {
		t.Fatalf("NewSources() error = %v, want nil", err)
	}
	want := map[corpus.ID][]string{
		corpus.VNReg: {"vbpl", "vanban", "congbao", "sbv_hanoi"},
		corpus.MYReg: {"agclom", "bnm", "sc"},
	}
	if len(sources) != len(want) {
		t.Fatalf("NewSources() wired %d corpora, want %d", len(sources), len(want))
	}
	for id, wantIDs := range want {
		got := make([]string, 0, len(sources[id]))
		for _, s := range sources[id] {
			got = append(got, s.ID())
		}
		if !slices.Equal(got, wantIDs) {
			t.Errorf("NewSources()[%s] = %v, want %v", id, got, wantIDs)
		}
	}
}

func TestNewJudgeFake(t *testing.T) {
	t.Setenv("VERTEX", "fake")
	j, err := config.NewJudge(context.Background())
	if err != nil {
		t.Fatalf("NewJudge() error = %v, want nil", err)
	}
	if j == nil {
		t.Fatal("NewJudge() = nil, want the fake judge")
	}
	got, err := j.Judge(context.Background(), "a", "b")
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if got.EdgeType != "satisfies" {
		t.Errorf("EdgeType = %q, want %q", got.EdgeType, "satisfies")
	}
}

func TestNewJudgeDefaultsToFakeWhenUnset(t *testing.T) {
	t.Setenv("VERTEX", "")
	j, err := config.NewJudge(context.Background())
	if err != nil {
		t.Fatalf("NewJudge() error = %v, want the fake default", err)
	}
	if j == nil {
		t.Fatal("NewJudge() = nil, want the fake judge")
	}
}

func TestNewJudgeRejectsUnknownValue(t *testing.T) {
	t.Setenv("VERTEX", "bogus")
	if _, err := config.NewJudge(context.Background()); err == nil {
		t.Fatal("NewJudge() error = nil, want error for unknown VERTEX value")
	}
}

func TestNewJudgeRealRequiresProject(t *testing.T) {
	t.Setenv("VERTEX", "real")
	t.Setenv("GCP_PROJECT", "")
	if _, err := config.NewJudge(context.Background()); err == nil {
		t.Fatal("NewJudge() error = nil, want error when GCP_PROJECT is unset")
	}
}

func TestNewGrounderFake(t *testing.T) {
	t.Setenv("VERTEX", "fake")
	g, err := config.NewGrounder(context.Background())
	if err != nil {
		t.Fatalf("NewGrounder() error = %v, want nil", err)
	}
	if g == nil {
		t.Fatal("NewGrounder() = nil, want the fake grounder")
	}
}

func TestNewGrounderDefaultsToFakeWhenUnset(t *testing.T) {
	t.Setenv("VERTEX", "")
	g, err := config.NewGrounder(context.Background())
	if err != nil {
		t.Fatalf("NewGrounder() error = %v, want the fake default", err)
	}
	if g == nil {
		t.Fatal("NewGrounder() = nil, want the fake grounder")
	}
}

func TestNewGrounderRejectsUnknownValue(t *testing.T) {
	t.Setenv("VERTEX", "bogus")
	if _, err := config.NewGrounder(context.Background()); err == nil {
		t.Fatal("NewGrounder() error = nil, want error for unknown VERTEX value")
	}
}

func TestNewGrounderRealRequiresProject(t *testing.T) {
	t.Setenv("VERTEX", "real")
	t.Setenv("GCP_PROJECT", "")
	if _, err := config.NewGrounder(context.Background()); err == nil {
		t.Fatal("NewGrounder() error = nil, want error when GCP_PROJECT is unset")
	}
}

func TestNewThresholdConfigDefaults(t *testing.T) {
	t.Setenv("JUDGE_CONFIDENCE_MIN", "")
	t.Setenv("JUDGE_GROUNDING_MIN", "")
	t.Setenv("JUDGE_MODEL", "")
	t.Setenv("JUDGE_ESCALATION_MODEL", "")

	tc := config.NewThresholdConfig()
	if tc.ConfidenceMin != 0.7 {
		t.Errorf("ConfidenceMin = %f, want 0.7", tc.ConfidenceMin)
	}
	if tc.GroundingMin != 0.6 {
		t.Errorf("GroundingMin = %f, want 0.6", tc.GroundingMin)
	}
	if tc.Model != "gemini-3.5-flash" {
		t.Errorf("Model = %q, want %q", tc.Model, "gemini-3.5-flash")
	}
	if tc.EscalationModel != "" {
		t.Errorf("EscalationModel = %q, want empty", tc.EscalationModel)
	}
}

func TestNewThresholdConfigUsesEnvOverrides(t *testing.T) {
	t.Setenv("JUDGE_CONFIDENCE_MIN", "0.85")
	t.Setenv("JUDGE_GROUNDING_MIN", "0.75")
	t.Setenv("JUDGE_MODEL", "gemini-2.5-pro")
	t.Setenv("JUDGE_ESCALATION_MODEL", "gemini-2.5-pro")

	tc := config.NewThresholdConfig()
	if tc.ConfidenceMin != 0.85 {
		t.Errorf("ConfidenceMin = %f, want 0.85", tc.ConfidenceMin)
	}
	if tc.GroundingMin != 0.75 {
		t.Errorf("GroundingMin = %f, want 0.75", tc.GroundingMin)
	}
	if tc.Model != "gemini-2.5-pro" {
		t.Errorf("Model = %q, want %q", tc.Model, "gemini-2.5-pro")
	}
	if tc.EscalationModel != "gemini-2.5-pro" {
		t.Errorf("EscalationModel = %q, want %q", tc.EscalationModel, "gemini-2.5-pro")
	}
}

func TestNewThresholdConfigFallsBackOnUnparseableFloat(t *testing.T) {
	t.Setenv("JUDGE_CONFIDENCE_MIN", "not-a-float")
	t.Setenv("JUDGE_GROUNDING_MIN", "also-bad")

	tc := config.NewThresholdConfig()
	if tc.ConfidenceMin != 0.7 {
		t.Errorf("ConfidenceMin = %f, want fallback 0.7", tc.ConfidenceMin)
	}
	if tc.GroundingMin != 0.6 {
		t.Errorf("GroundingMin = %f, want fallback 0.6", tc.GroundingMin)
	}
}

func TestRoleDefaultsToMisePublic(t *testing.T) {
	t.Setenv("MISE_DB_ROLE", "")
	if got := config.Role(); got != "mise_public" {
		t.Errorf("Role() = %q, want %q", got, "mise_public")
	}
}

func TestRoleUsesEnvOverride(t *testing.T) {
	t.Setenv("MISE_DB_ROLE", "mise_group")
	if got := config.Role(); got != "mise_group" {
		t.Errorf("Role() = %q, want %q", got, "mise_group")
	}
}
