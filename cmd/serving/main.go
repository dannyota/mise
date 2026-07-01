// Command serving is the evidence + graph API + MCP server.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"danny.vn/mise/pkg/mcp"
)

// readHeaderTimeout bounds the time spent reading request headers (gosec G112).
const readHeaderTimeout = 10 * time.Second

// shutdownTimeout bounds graceful shutdown after a stop signal.
const shutdownTimeout = 5 * time.Second

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	port := envOr("SERVING_PORT", "8080")

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/healthz", healthzHandler)

	mcpServer := mcp.New(mcp.WithLogger(log))
	r.Mount("/mcp", mcpServer.Handler())

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	go func() {
		log.Info("serving started", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown failed", "error", err)
	}
	log.Info("serving stopped")
}

// healthzHandler reports process liveness for the load balancer / readiness
// probe. It has no dependencies (DB, MCP) so it never fails once the process
// is up.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		slog.Error("healthz: write response", "error", err)
	}
}

// envOr returns the environment variable named key, or fallback if unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
