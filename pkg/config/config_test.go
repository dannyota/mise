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
	// The fake judge returns a deterministic result.
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
