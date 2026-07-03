// Package config provides typed, env-based configuration for all mise services.
package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"cloud.google.com/go/storage"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/blob"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/detect"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/ingest/agclom"
	"danny.vn/mise/pkg/ingest/bnm"
	"danny.vn/mise/pkg/ingest/congbao"
	"danny.vn/mise/pkg/ingest/sbv"
	"danny.vn/mise/pkg/ingest/sc"
	"danny.vn/mise/pkg/ingest/vanban"
	"danny.vn/mise/pkg/ingest/vbpl"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
	"danny.vn/mise/pkg/vertex"
)

// defaultRole is Role's fallback — the least-privileged RLS role
// (migrations/004_rls_roles.sql).
const defaultRole = "mise_public"

// DB returns AlloyDB connection parameters from the ALLOYDB_* environment
// variables (.env.example), defaulting to the reference local-dev stack
// (compose.yaml). An unparseable ALLOYDB_PORT falls back to the default
// port rather than erroring — DB has no error return.
func DB() store.Config {
	return store.Config{
		Host:     envOr("ALLOYDB_HOST", "localhost"),
		Port:     envIntOr("ALLOYDB_PORT", 5432),
		User:     envOr("ALLOYDB_USER", "mise"),
		Password: envOr("ALLOYDB_PASSWORD", "mise-dev"),
		Database: envOr("ALLOYDB_DATABASE", "mise"),
	}
}

// NewEmbedder returns the Embedder VERTEX selects: "fake" (the default) for
// the offline deterministic embedder (LOCAL-DEV §4 Mode B), "real" for
// Vertex AI gemini-embedding-001 via GCP_PROJECT/GCP_REGION. Any other
// VERTEX value is an error.
func NewEmbedder(ctx context.Context) (embed.Embedder, error) {
	switch v := envOr("VERTEX", "fake"); v {
	case "fake":
		return embed.NewFake(), nil
	case "real":
		e, err := embed.NewVertex(ctx, os.Getenv("GCP_PROJECT"), envOr("GCP_REGION", "us-central1"))
		if err != nil {
			return nil, fmt.Errorf("config: creating vertex embedder: %w", err)
		}
		return e, nil
	default:
		return nil, fmt.Errorf("config: unknown VERTEX value %q, want \"fake\" or \"real\"", v)
	}
}

// Role returns the RLS role serving assumes for MCP evidence calls
// (MISE_DB_ROLE), defaulting to mise_public — never derived from request
// input (migrations/004_rls_roles.sql).
func Role() string {
	return envOr("MISE_DB_ROLE", defaultRole)
}

// NewBlob returns the raw-artifact blob store: GCS when GCS_BUCKET is set
// (GKE; client auth via ADC), else the local filesystem rooted at BLOB_DIR
// (default ./data/raw — the compose worker mounts a named volume there).
func NewBlob(ctx context.Context) (blob.Store, error) {
	if bucket := os.Getenv("GCS_BUCKET"); bucket != "" {
		client, err := storage.NewClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("config: creating gcs client: %w", err)
		}
		return blob.NewGCS(client, bucket), nil
	}
	return blob.NewFS(envOr("BLOB_DIR", "./data/raw")), nil
}

// NewRanker returns the Ranker VERTEX selects: "fake" (the default) for the
// offline deterministic ranker (LOCAL-DEV §4 Mode B), "real" for the Discovery
// Engine Ranking API via GCP_PROJECT/GCP_REGION. Any other VERTEX value is an
// error.
func NewRanker(ctx context.Context) (vertex.Ranker, error) {
	switch v := envOr("VERTEX", "fake"); v {
	case "fake":
		return vertex.NewFakeRanker(), nil
	case "real":
		r, err := vertex.NewVertexRanker(ctx, os.Getenv("GCP_PROJECT"), envOr("GCP_REGION", "us-central1"))
		if err != nil {
			return nil, fmt.Errorf("config: creating vertex ranker: %w", err)
		}
		return r, nil
	default:
		return nil, fmt.Errorf("config: unknown VERTEX value %q, want \"fake\" or \"real\"", v)
	}
}

// NewJudge returns the Judge VERTEX selects: "fake" (the default) for the
// offline deterministic judge (LOCAL-DEV §4 Mode B), "real" for the Gemini
// :generateContent API via GCP_PROJECT/GCP_REGION. JUDGE_MODEL overrides
// the default model (gemini-3.5-flash). Any other VERTEX value is an error.
func NewJudge(ctx context.Context) (vertex.Judge, error) {
	switch v := envOr("VERTEX", "fake"); v {
	case "fake":
		return vertex.NewFakeJudge(), nil
	case "real":
		j, err := vertex.NewGeminiJudge(ctx,
			os.Getenv("GCP_PROJECT"),
			envOr("GCP_REGION", "us-central1"),
			vertex.WithJudgeModel(envOr("JUDGE_MODEL", "gemini-3.5-flash")),
		)
		if err != nil {
			return nil, fmt.Errorf("config: creating gemini judge: %w", err)
		}
		return j, nil
	default:
		return nil, fmt.Errorf("config: unknown VERTEX value %q, want \"fake\" or \"real\"", v)
	}
}

// NewParser returns the document Parser VERTEX selects: "fake" (the default)
// for the offline deterministic parser (LOCAL-DEV §4 Mode B), "real" for Doc
// AI Layout Parser via GCP_PROJECT / DOCAI_LOCATION (default "us") /
// DOCAI_PROCESSOR_ID. Any other VERTEX value is an error.
func NewParser(_ context.Context) (vertex.Parser, error) {
	switch v := envOr("VERTEX", "fake"); v {
	case "fake":
		return vertex.NewFakeParser(), nil
	case "real":
		//nolint:contextcheck // NewDocAIParser takes no context: it builds an ADC token source once at startup.
		p, err := vertex.NewDocAIParser(
			os.Getenv("GCP_PROJECT"),
			envOr("DOCAI_LOCATION", "us"),
			os.Getenv("DOCAI_PROCESSOR_ID"),
		)
		if err != nil {
			return nil, fmt.Errorf("config: creating doc ai parser: %w", err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("config: unknown VERTEX value %q, want \"fake\" or \"real\"", v)
	}
}

// NewGrounder returns the Grounder VERTEX selects: "fake" (the default) for
// the offline deterministic grounder (LOCAL-DEV §4 Mode B), "real" for the
// Discovery Engine Check Grounding API via GCP_PROJECT/GCP_REGION. Any other
// VERTEX value is an error.
func NewGrounder(ctx context.Context) (vertex.Grounder, error) {
	switch v := envOr("VERTEX", "fake"); v {
	case "fake":
		return vertex.NewFakeGrounder(), nil
	case "real":
		g, err := vertex.NewCheckGrounder(ctx, os.Getenv("GCP_PROJECT"), envOr("GCP_REGION", "us-central1"))
		if err != nil {
			return nil, fmt.Errorf("config: creating check grounder: %w", err)
		}
		return g, nil
	default:
		return nil, fmt.Errorf("config: unknown VERTEX value %q, want \"fake\" or \"real\"", v)
	}
}

// NewThresholdConfig returns a ThresholdConfig from environment variables,
// falling back to sensible defaults.
func NewThresholdConfig() detect.ThresholdConfig {
	return detect.ThresholdConfig{
		ConfidenceMin:   envFloatOr("JUDGE_CONFIDENCE_MIN", 0.7),
		GroundingMin:    envFloatOr("JUDGE_GROUNDING_MIN", 0.6),
		Model:           envOr("JUDGE_MODEL", "gemini-3.5-flash"),
		EscalationModel: os.Getenv("JUDGE_ESCALATION_MODEL"),
	}
}

// vbpl agency-id defaults, mirroring banhmi's config.issuer_code seed
// (deploy/seed/issuer_code.csv, source='vbpl'): the is_sbv set (62 current +
// 908 legacy "Ngân hàng quốc gia") drives the keyword-less State Bank sweep;
// the in-scope non-SBV set (Quốc hội, UBTVQH, Chính phủ, Thủ tướng, Bộ Công
// an, Bộ KH&CN, Bộ TT&TT, Bộ BCVT) is the target of keyword searches.
var (
	vbplSBVAgencyIDs    = []string{"62", "908"}
	vbplNonSBVAgencyIDs = []string{"55", "56", "1", "57", "3", "14", "169", "2"}
)

// NewSources returns the wired crawler set per law corpus — vn-reg: vbpl,
// vanban, congbao, sbv_hanoi; my-reg: agclom, bnm, sc — each with its default
// HTTP client and the process logger. vbpl additionally gets the reference
// agency-id sets and its built-in relation-type labels (nil map).
func NewSources(_ context.Context) (map[corpus.ID][]ingest.Source, error) {
	log := slog.Default()
	return map[corpus.ID][]ingest.Source{
		corpus.VNReg: {
			vbpl.New(nil, log, vbplSBVAgencyIDs, vbplNonSBVAgencyIDs, nil),
			vanban.New(nil, log),
			congbao.New(nil, log),
			sbv.New(nil, log),
		},
		corpus.MYReg: {
			agclom.New(nil, log),
			bnm.New(nil, log),
			sc.New(nil, log),
		},
	}, nil
}

// NewDetectDeps returns the detect pipeline's dependency set from env config.
func NewDetectDeps(ctx context.Context, pool *pgxpool.Pool) (detect.Deps, error) {
	emb, err := NewEmbedder(ctx)
	if err != nil {
		return detect.Deps{}, fmt.Errorf("config: detect embedder: %w", err)
	}
	factEmb, ok := emb.(embed.FactEmbedder)
	if !ok {
		return detect.Deps{}, errors.New("config: embedder does not implement FactEmbedder")
	}
	judge, err := NewJudge(ctx)
	if err != nil {
		return detect.Deps{}, fmt.Errorf("config: detect judge: %w", err)
	}
	grounder, err := NewGrounder(ctx)
	if err != nil {
		return detect.Deps{}, fmt.Errorf("config: detect grounder: %w", err)
	}
	ranker, err := NewRanker(ctx)
	if err != nil {
		return detect.Deps{}, fmt.Errorf("config: detect ranker: %w", err)
	}

	return detect.Deps{
		Pool:       pool,
		Embedder:   factEmb,
		Judge:      judge,
		Grounder:   grounder,
		Ranker:     ranker,
		Graph:      store.NewGraphStore(pool),
		Thresholds: NewThresholdConfig(),
	}, nil
}

// envOr returns the environment variable named key, or fallback if unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envIntOr returns the environment variable named key parsed as an int, or
// fallback if key is unset, empty, or not a valid integer.
func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// envFloatOr returns the environment variable named key parsed as a float64,
// or fallback if key is unset, empty, or not a valid float.
func envFloatOr(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
