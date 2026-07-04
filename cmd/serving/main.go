// Command serving is the evidence + graph API + MCP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/config"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/httpapi"
	"danny.vn/mise/pkg/mcp"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
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

	r, pool, err := newRouter(ctx, log)
	if err != nil {
		log.Error("wiring evidence store", "error", err)
		os.Exit(1)
	}
	if pool != nil {
		defer pool.Close()
	}

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

// readyzHandler reports whether pool can serve reads, pinging it on every
// call — unlike healthzHandler, readiness genuinely depends on AlloyDB.
func readyzHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, werr := w.Write([]byte("not ready")); werr != nil {
				slog.Error("readyz: write response", "error", werr)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, werr := w.Write([]byte("ok")); werr != nil {
			slog.Error("readyz: write response", "error", werr)
		}
	}
}

// newRouter builds the chi router — health/readiness endpoints, the REST
// graph API, and the MCP mount — and returns the AlloyDB pool wireEvidence
// opened (nil when ALLOYDB_HOST is unset), so the caller can close it on
// shutdown. Split out of main so tests can drive the real routing/wiring
// path directly, without starting a listener or blocking on OS signals.
func newRouter(ctx context.Context, log *slog.Logger) (*chi.Mux, *pgxpool.Pool, error) {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Get("/healthz", healthzHandler)

	pool, mcpOpts, err := wireEvidence(ctx, log)
	if err != nil {
		return nil, nil, err
	}
	if pool != nil {
		r.Get("/readyz", readyzHandler(pool))
		// /api/v1 needs a real pool (GraphRepo is a pure DB read), so it stays
		// gated behind the same ALLOYDB_HOST check as /readyz — without a pool,
		// serving stays healthz-only, unchanged from before this endpoint set.
		r.Route("/api/v1", func(v1 chi.Router) {
			api := httpapi.NewAPI(v1)
			graphRepo := store.NewGraphRepo(pool)
			httpapi.RegisterAll(api, httpapi.Deps{
				Graph:         graphRepo,
				Reviews:       store.NewReviewStore(pool),
				Findings:      store.NewFindingStore(pool),
				Dashboard:     store.NewDashboardStore(pool),
				GraphCanvas:   graphRepo,
				Timeline:      store.NewTimelineStore(pool),
				Notifications: store.NewNotificationStore(pool),
			}, config.Role())
		})
	}

	mcpServer := mcp.New(mcpOpts...)
	r.Mount("/mcp", mcpServer.Handler())
	return r, pool, nil
}

// wireEvidence builds the AlloyDB pool, embedder, and per-corpus store map
// and returns the mcp.Option slice to construct the MCP server with, plus
// the pool (nil when unused) so main can close it and mount /readyz. Without
// ALLOYDB_HOST set, serving stays healthz-only — the zero-dependency path
// mcp.New always supports — and pool is nil.
func wireEvidence(ctx context.Context, log *slog.Logger) (*pgxpool.Pool, []mcp.Option, error) {
	opts := make([]mcp.Option, 0, 3) // WithLogger, plus WithEvidence/WithGraph once wiring succeeds
	opts = append(opts, mcp.WithLogger(log))
	if os.Getenv("ALLOYDB_HOST") == "" {
		return nil, opts, nil
	}

	pool, err := store.Connect(ctx, config.DB())
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to alloydb: %w", err)
	}

	emb, err := config.NewEmbedder(ctx)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("creating embedder: %w", err)
	}

	corpora, err := newCorporaMap(pool)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("building corpus stores: %w", err)
	}

	searcher := storeSearcher{pool: pool, emb: emb}
	docGetter := storeDocGetter{corpora: corpora}
	graphRepo := storeGraphRepo{repo: store.NewGraphRepo(pool)}
	role := config.Role()
	opts = append(opts, mcp.WithEvidence(searcher, docGetter, role), mcp.WithGraph(graphRepo, role))
	return pool, opts, nil
}

// newCorporaMap builds one store.Corpus per registered corpus, keyed by
// corpus ID string — storeDocGetter's per-call corpus_id lookup.
func newCorporaMap(pool *pgxpool.Pool) (map[string]*store.Corpus, error) {
	all := corpus.All()
	out := make(map[string]*store.Corpus, len(all))
	for _, desc := range all {
		c, err := store.NewCorpus(pool, desc)
		if err != nil {
			return nil, fmt.Errorf("building corpus store for %s: %w", desc.ID, err)
		}
		out[string(desc.ID)] = c
	}
	return out, nil
}

// storeSearcher adapts store.Search to mcp.Searcher.
type storeSearcher struct {
	pool *pgxpool.Pool
	emb  embed.Embedder
}

// Search implements mcp.Searcher.
func (s storeSearcher) Search(ctx context.Context, query string, opts store.SearchOpts) ([]store.Hit, error) {
	return store.Search(ctx, s.pool, s.emb, query, opts)
}

// storeDocGetter adapts per-corpus store.Corpus.GetDocument to
// mcp.DocGetter, resolving corpusID against the pre-built corpora map.
type storeDocGetter struct {
	corpora map[string]*store.Corpus
}

// GetDocument implements mcp.DocGetter.
func (g storeDocGetter) GetDocument(
	ctx context.Context, role, corpusID string, docID uuid.UUID,
) (store.DocumentDetail, error) {
	c, ok := g.corpora[corpusID]
	if !ok {
		return store.DocumentDetail{}, fmt.Errorf("serving: %q is not a registered corpus", corpusID)
	}
	return c.GetDocument(ctx, role, docID)
}

// storeGraphRepo adapts *store.GraphRepo to mcp.GraphRepoIface. Both methods
// already match store.GraphRepo's own signatures exactly, so this is a pure
// pass-through — kept as an explicit adapter (like storeSearcher/
// storeDocGetter above) so the MCP-exposed surface is pinned to exactly
// GetNode+Chain, independent of whatever else store.GraphRepo might grow.
type storeGraphRepo struct {
	repo *store.GraphRepo
}

// GetNode implements mcp.GraphRepoIface.
func (g storeGraphRepo) GetNode(ctx context.Context, role string, ref graph.NodeRef) (store.NodeView, error) {
	return g.repo.GetNode(ctx, role, ref)
}

// Chain implements mcp.GraphRepoIface.
func (g storeGraphRepo) Chain(
	ctx context.Context, role string, start graph.NodeRef, maxDepth int,
) ([]store.Hop, error) {
	return g.repo.Chain(ctx, role, start, maxDepth)
}

// envOr returns the environment variable named key, or fallback if unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
