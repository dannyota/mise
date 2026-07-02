// Command worker runs Temporal workers for the ingest pipeline and detectors.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/worker"

	"danny.vn/mise/pkg/config"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/pipeline"
	"danny.vn/mise/pkg/store"
	mise_temporal "danny.vn/mise/pkg/temporal"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

// run wires the Temporal client and worker and blocks until shutdown, so that
// deferred cleanup (closing the client and pool) always runs before main
// exits — including on the worker.Run error path.
func run() error {
	cfg := mise_temporal.Config{
		Host:      envOr("TEMPORAL_HOST", "localhost:7233"),
		Namespace: envOr("TEMPORAL_NAMESPACE", "default"),
		TaskQueue: envOr("TEMPORAL_TASK_QUEUE", "mise-ingest"),
	}

	ctx := context.Background()
	tc, err := mise_temporal.Connect(ctx, cfg)
	if err != nil {
		return fmt.Errorf("temporal connect: %w", err)
	}
	defer tc.Close()

	pool, err := store.Connect(ctx, config.DB())
	if err != nil {
		return fmt.Errorf("store connect: %w", err)
	}
	defer pool.Close()

	acts, err := buildActivities(ctx, pool)
	if err != nil {
		return err
	}
	w := mise_temporal.NewWorkerWith(tc, cfg.TaskQueue, func(w worker.Worker) {
		w.RegisterWorkflow(pipeline.IngestCorpusWorkflow)
		w.RegisterActivity(acts)
	})

	slog.Info("worker started", "task_queue", cfg.TaskQueue)
	if err := w.Run(interruptCh()); err != nil {
		return fmt.Errorf("worker run: %w", err)
	}
	return nil
}

// buildActivities assembles the ingest pipeline dependencies from config: the
// blob store, embedder, Doc AI parser seam, and the per-corpus crawler set.
func buildActivities(ctx context.Context, pool *pgxpool.Pool) (*pipeline.Activities, error) {
	blobStore, err := config.NewBlob(ctx)
	if err != nil {
		return nil, fmt.Errorf("blob store: %w", err)
	}
	embedder, err := config.NewEmbedder(ctx)
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}
	parser, err := config.NewParser(ctx)
	if err != nil {
		return nil, fmt.Errorf("parser: %w", err)
	}
	sources, err := config.NewSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("sources: %w", err)
	}
	return pipeline.NewActivities(pipeline.Deps{
		Pool:     pool,
		Blob:     blobStore,
		Embedder: embedder,
		Extract:  ingest.NewExtractor(parser),
		Sources:  sources,
	}), nil
}

// interruptCh signals on SIGINT/SIGTERM so worker.Run can shut down gracefully.
func interruptCh() <-chan any {
	ch := make(chan any, 1)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		close(ch)
	}()
	return ch
}

// envOr returns the environment variable named key, or fallback if unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
