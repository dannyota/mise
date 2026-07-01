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
	mcp *mcp.Server
	log *slog.Logger
}

// Option configures the MCP server.
type Option func(*Server)

// WithLogger sets the server logger.
func WithLogger(log *slog.Logger) Option {
	return func(s *Server) { s.log = log }
}

// New creates an MCP server with zero tools registered.
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
	return s
}

// Handler returns an HTTP handler for the MCP streamable HTTP transport.
func (s *Server) Handler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, nil)
}
