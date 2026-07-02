// Package mcp implements the per-corpus MCP tool server.
package mcp

import (
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// name and version identify this server to connecting MCP clients.
const (
	name    = "mise"
	version = "0.1.0"
)

// Server wraps the MCP server with mise's evidence tools.
type Server struct {
	mcp       *mcp.Server
	log       *slog.Logger
	searcher  Searcher
	docGetter DocGetter
	role      string
}

// Option configures the MCP server.
type Option func(*Server)

// WithLogger sets the server logger.
func WithLogger(log *slog.Logger) Option {
	return func(s *Server) { s.log = log }
}

// WithEvidence registers the search and document tools, backed by s and g.
// Every store call the tools make is scoped to role — the RLS role the
// caller resolves once (pkg/config.Role) and hands down here, never derived
// per-request from client input (migrations/004_rls_roles.sql).
func WithEvidence(s Searcher, g DocGetter, role string) Option {
	return func(srv *Server) {
		srv.searcher = s
		srv.docGetter = g
		srv.role = role
	}
}

// New creates an MCP server. With no options — or without WithEvidence — it
// has zero tools registered, the dependency-free path healthz-only serving
// relies on. WithEvidence registers the search and document tools.
func New(opts ...Option) *Server {
	s := &Server{
		log: slog.Default(),
	}
	for _, o := range opts {
		o(s)
	}
	s.mcp = mcp.NewServer(
		&mcp.Implementation{Name: name, Version: version},
		&mcp.ServerOptions{Logger: s.log},
	)
	if s.searcher != nil && s.docGetter != nil {
		registerEvidenceTools(s.mcp, s.searcher, s.docGetter, s.role)
	}
	return s
}

// Handler returns an HTTP handler for the MCP streamable HTTP transport.
func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, nil)
}
