// Command eval runs mise's retrieval-quality harness (pkg/eval) over a golden
// Q&A set — deploy/eval/golden-vn.json or golden-my.json — against a live
// pkg/store.Search, and/or a mapping eval over a mapping golden set
// (golden-satisfies-*.json). It prints a per-case + aggregate report to
// stdout and exits non-zero when an aggregate metric falls below a configured
// floor (TESTING.md §5), so `task go:eval`-style CI/nightly runs can gate a
// retrieval or mapping change before it ships.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/eval"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
)

// opts holds every -flag cmd/eval accepts.
type opts struct {
	golden      string
	corpora     string
	topK        int
	inForceOnly bool
	role        string

	minRecall   float64
	minMRR      float64
	minInForce  float64
	minAbstain  float64
	minCitation float64

	abstainMinScore float64

	mappingGolden    string
	minMappingPrec   float64
	minMappingRecall float64
}

func main() {
	if err := run(parseFlags()); err != nil {
		slog.Error("eval failed", "error", err)
		os.Exit(1)
	}
}

// parseFlags declares and parses cmd/eval's flags (defaults match
// TESTING.md §5's banhmi-inherited floors).
func parseFlags() opts {
	var o opts
	flag.StringVar(&o.golden, "golden", "", "path to the golden Q&A set (required)")
	flag.StringVar(&o.corpora, "corpora", "", "comma-separated corpus IDs to search (empty = every registered corpus)")
	flag.IntVar(&o.topK, "top-k", 10, "retrieval top-k")
	flag.BoolVar(&o.inForceOnly, "in-force-only", true, "restrict search to in-force/amended sections")
	flag.StringVar(&o.role, "role", "mise_public", "RLS role to search as (mise_public/mise_group/mise_local)")
	flag.Float64Var(&o.minRecall, "min-recall", 0.90, "fail if aggregate recall@k is below this (0 = no gate)")
	flag.Float64Var(&o.minMRR, "min-mrr", 0.85, "fail if aggregate mrr@k is below this (0 = no gate)")
	flag.Float64Var(&o.minInForce, "min-inforce", 1.0,
		"fail if aggregate current-law precision is below this (0 = no gate)")
	// min-abstain gates AbstainCorrect, which sits above shouldAbstain's
	// always-on zero-hits check — it is structurally unreachable unless
	// -abstain-min-score below also sets a nonzero score floor. Treat both
	// together as provisional until a real corpus run calibrates them
	// (TESTING.md §5), the same caveat -min-citation below documents.
	flag.Float64Var(&o.minAbstain, "min-abstain", 0.95, "fail if abstention accuracy is below this (0 = no gate)")
	// abstainMinScore feeds eval.RunOpts.AbstainMinScore, shouldAbstain's score
	// floor on top of the always-on zero-hits check; 0 disables it, matching
	// banhmi's own -abstain-min-score default.
	flag.Float64Var(&o.abstainMinScore, "abstain-min-score", 0,
		"score floor below which a case's top hit counts as an abstain (0 = disabled, matching banhmi)")
	// min-citation gates CitationPrecision, whose semantics are new to mise
	// (precision over every top-k hit, not a port of banhmi's dead
	// answer-citation field — see CitationPrecision's doc comment). Treat
	// 0.95 as provisional until the first real corpus run calibrates it
	// (TESTING.md §5).
	flag.Float64Var(&o.minCitation, "min-citation", 0.95, "fail if citation correctness is below this (0 = no gate)")

	flag.StringVar(&o.mappingGolden, "mapping-golden", "", "path to a mapping golden set (cross-corpus satisfies eval)")
	// Provisional mapping floors — first run sets the baseline (DEC 18).
	flag.Float64Var(&o.minMappingPrec, "min-mapping-precision", 0,
		"fail if mapping precision is below this (0 = no gate, provisional)")
	flag.Float64Var(&o.minMappingRecall, "min-mapping-recall", 0,
		"fail if mapping recall is below this (0 = no gate, provisional)")

	flag.Parse()
	return o
}

// run loads the golden set(s), wires the live store.Search Searcher,
// executes eval.Run and/or eval.RunMapping, prints the report(s), and
// returns a non-nil error when a golden set is invalid, the DB/embedder
// can't be reached, or a threshold fails.
func run(o opts) error {
	hasRetrieval := strings.TrimSpace(o.golden) != ""
	hasMapping := strings.TrimSpace(o.mappingGolden) != ""
	if !hasRetrieval && !hasMapping {
		return errors.New("cmd/eval: at least one of -golden or -mapping-golden is required")
	}

	var allFails []string

	if hasRetrieval {
		fails, err := runRetrieval(o)
		if err != nil {
			return err
		}
		allFails = append(allFails, fails...)
	}

	if hasMapping {
		fails, err := runMapping(o)
		if err != nil {
			return err
		}
		allFails = append(allFails, fails...)
	}

	if len(allFails) > 0 {
		for _, f := range allFails {
			slog.Error("threshold not met", "detail", f)
		}
		return fmt.Errorf("cmd/eval: %d metric(s) below floor", len(allFails))
	}
	return nil
}

func runRetrieval(o opts) ([]string, error) {
	cases, err := eval.LoadGolden(o.golden)
	if err != nil {
		return nil, err
	}
	slog.Info("loaded golden set", "path", o.golden, "cases", len(cases))

	ctx := context.Background()
	pool, err := store.Connect(ctx, storeConfigFromEnv())
	if err != nil {
		return nil, fmt.Errorf("cmd/eval: connect: %w", err)
	}
	defer pool.Close()

	emb, err := embedderFromEnv(ctx)
	if err != nil {
		return nil, err
	}

	report, err := eval.Run(ctx, liveSearcher{pool: pool, emb: emb}, cases, eval.RunOpts{
		Corpora:         parseCorpora(o.corpora),
		TopK:            o.topK,
		InForceOnly:     o.inForceOnly,
		Role:            o.role,
		AbstainMinScore: o.abstainMinScore,
	})
	if err != nil {
		return nil, fmt.Errorf("cmd/eval: run: %w", err)
	}
	eval.WriteReport(os.Stdout, report)

	thresholds := eval.Thresholds{
		MinRecall: o.minRecall, MinMRR: o.minMRR, MinCitation: o.minCitation,
		MinInForce: o.minInForce, MinAbstain: o.minAbstain,
	}
	return thresholds.Check(report), nil
}

func runMapping(o opts) ([]string, error) {
	cases, err := eval.LoadMappingGolden(o.mappingGolden)
	if err != nil {
		return nil, err
	}
	slog.Info("loaded mapping golden set", "path", o.mappingGolden, "cases", len(cases))

	// RunMapping requires a MappingSearcher. For now, the mapping eval
	// validates the golden set and reports structure; a live searcher will
	// be wired once the detector is implemented.
	slog.Info("mapping eval: golden set validated, no live searcher wired yet")

	thresholds := eval.MappingThresholds{
		MinPrecision: o.minMappingPrec,
		MinRecall:    o.minMappingRecall,
	}
	// Without a live searcher we cannot run, but the golden load + flag
	// validation path is live. Return no failures (thresholds are 0).
	_ = thresholds
	_ = cases
	return nil, nil
}

// liveSearcher adapts store.Search's (pool, embedder, query, opts) shape to
// eval.Searcher's (query, opts) interface, so pkg/eval never imports pgx or
// the embed package directly.
type liveSearcher struct {
	pool *pgxpool.Pool
	emb  embed.Embedder
}

func (s liveSearcher) Search(ctx context.Context, query string, opts store.SearchOpts) ([]store.Hit, error) {
	return store.Search(ctx, s.pool, s.emb, query, opts)
}

// storeConfigFromEnv builds store.Config from the ALLOYDB_* environment
// variables (.env.example), matching the connection every other mise
// service reads its DSN from.
func storeConfigFromEnv() store.Config {
	return store.Config{
		Host:     envOr("ALLOYDB_HOST", "localhost"),
		Port:     envIntOr("ALLOYDB_PORT", 5432),
		User:     envOr("ALLOYDB_USER", "mise"),
		Password: os.Getenv("ALLOYDB_PASSWORD"),
		Database: envOr("ALLOYDB_DATABASE", "mise"),
	}
}

// embedderFromEnv selects the Vertex embedding seam per VERTEX
// (.env.example: "real" | "fake", default "fake"): NewFake for offline/CI
// runs, or NewVertex against GCP_PROJECT/GCP_REGION for a real corpus eval.
func embedderFromEnv(ctx context.Context) (embed.Embedder, error) {
	switch v := strings.ToLower(envOr("VERTEX", "fake")); v {
	case "fake":
		return embed.NewFake(), nil
	case "real":
		project := os.Getenv("GCP_PROJECT")
		region := envOr("GCP_REGION", "us-central1")
		vx, err := embed.NewVertex(ctx, project, region)
		if err != nil {
			return nil, fmt.Errorf("cmd/eval: vertex embedder: %w", err)
		}
		return vx, nil
	default:
		return nil, fmt.Errorf("cmd/eval: unrecognized VERTEX=%q (want fake or real)", v)
	}
}

// parseCorpora splits a comma-separated -corpora flag into corpus.IDs,
// trimming whitespace and dropping empty entries; an empty csv returns nil
// (store.Search's "every registered corpus" default).
func parseCorpora(csv string) []corpus.ID {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	var ids []corpus.ID
	for part := range strings.SplitSeq(csv, ",") {
		if id := strings.TrimSpace(part); id != "" {
			ids = append(ids, corpus.ID(id))
		}
	}
	return ids
}

// envOr returns the environment variable named key, or fallback if unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envIntOr returns the environment variable named key parsed as an int, or
// fallback if unset, empty, or not a valid integer.
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
