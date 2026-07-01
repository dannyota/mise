// Command worker runs Temporal workers for the ingest pipeline and detectors.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	mise_temporal "danny.vn/mise/pkg/temporal"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

// run wires the Temporal client and worker and blocks until shutdown, so that
// deferred cleanup (closing the client) always runs before main exits —
// including on the worker.Run error path.
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

	w := mise_temporal.NewWorker(tc, cfg.TaskQueue)

	slog.Info("worker started", "task_queue", cfg.TaskQueue)
	if err := w.Run(interruptCh()); err != nil {
		return fmt.Errorf("worker run: %w", err)
	}
	return nil
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
