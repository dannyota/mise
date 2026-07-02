// Command eval runs mise's retrieval-quality harness (pkg/eval) over a golden
// Q&A set — deploy/eval/golden-vn.json or golden-my.json — against a live
// pkg/store.Search. It prints a per-case + aggregate report to stdout and
// exits non-zero when an aggregate metric falls below a configured floor
// (TESTING.md §5), so `task go:eval`-style CI/nightly runs can gate a
// retrieval change before it ships.
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
	flag.Float64Var(&o.minAbstain, "min-abstain", 0.95, "fail if abstention accuracy is below this (0 = no gate)")
	flag.Float64Var(&o.minCitation, "min-citation", 0.95, "fail if citation correctness is below this (0 = no gate)")
	flag.Parse()
	return o
}

// run loads the golden set, wires the live store.Search Searcher, executes
// eval.Run, prints the report, and returns a non-nil error when the golden
// set is invalid, the DB/embedder can't be reached, or a threshold fails.
func run(o opts) error {
	if strings.TrimSpace(o.golden) == "" {
		return errors.New("cmd/eval: -golden is required")
	}

	cases, err := eval.LoadGolden(o.golden)
	if err != nil {
		return err
	}
	slog.Info("loaded golden set", "path", o.golden, "cases", len(cases))

	ctx := context.Background()
	pool, err := store.Connect(ctx, storeConfigFromEnv())
	if err != nil {
		return fmt.Errorf("cmd/eval: connect: %w", err)
	}
	defer pool.Close()

	emb, err := embedderFromEnv(ctx)
	if err != nil {
		return err
	}

	report, err := eval.Run(ctx, liveSearcher{pool: pool, emb: emb}, cases, eval.RunOpts{
		Corpora:     parseCorpora(o.corpora),
		TopK:        o.topK,
		InForceOnly: o.inForceOnly,
		Role:        o.role,
	})
	if err != nil {
		return fmt.Errorf("cmd/eval: run: %w", err)
	}
	eval.WriteReport(os.Stdout, report)

	thresholds := eval.Thresholds{
		MinRecall: o.minRecall, MinMRR: o.minMRR, MinCitation: o.minCitation,
		MinInForce: o.minInForce, MinAbstain: o.minAbstain,
	}
	if fails := thresholds.Check(report); len(fails) > 0 {
		for _, f := range fails {
			slog.Error("threshold not met", "detail", f)
		}
		return fmt.Errorf("cmd/eval: %d metric(s) below floor", len(fails))
	}
	return nil
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
