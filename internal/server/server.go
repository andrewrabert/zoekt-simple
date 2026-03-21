// Package server provides an MCP server exposing Zoekt code search tools.
package server

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sourcegraph/zoekt"

	"github.com/sourcegraph/zoekt-simple/internal/indexer"
)

const baseInstructions = `Code search across indexed repositories. Use search for finding code, get_file to retrieve file contents. Use output_mode="repos" to list repositories.

IMPORTANT: Search returns {total_matches, returned, truncated, results}. ALWAYS check if truncated=true - this means there are more results than returned. Options:
1. Refine query with filters (r:, f:, lang:)
2. Increase max_results
3. For exhaustive analysis: use output_mode="files" to get paths, then get_file each
4. For very large result sets: chunk by repo (r:orgname/) or file type (f:\.py$) and iterate

Query syntax:
- Multiple terms are AND'd: "class needle" finds files with both
- Use "or" for alternatives: "thread or needle"
- Use quotes for phrases: "class Needle"
- Use - to negate: "needle -hay", "-file:java"
- r: or repo: - filter by repo (e.g. r:myrepo)
- f: or file: - filter by path (e.g. f:README.md, f:\.py$, f:^src/.*\.ts$)
- lang: - filter by language (e.g. lang:python)
- sym: - search symbol definitions (e.g. sym:MyClass)
- case:yes - case-sensitive search`

// Config holds configuration for the MCP server.
type Config struct {
	// Searcher is the zoekt searcher/streamer to use for searches.
	Searcher zoekt.Streamer

	// ReposDir is the directory containing bare git repositories.
	ReposDir string

	// ExtraInstructions is appended to the base MCP instructions.
	ExtraInstructions string
}

// Server wraps the MCP server and provides HTTP handlers.
type Server struct {
	mcp         *mcp.Server
	app         *app
	httpHandler *mcp.StreamableHTTPHandler
}

// New creates a new MCP server with zoekt tools registered.
func New(cfg Config) *Server {
	a := &app{
		searcher: cfg.Searcher,
		reposDir: cfg.ReposDir,
		tracker:  &TaskTracker{tasks: make(map[string]*Task)},
	}

	instr := baseInstructions
	if cfg.ExtraInstructions != "" {
		instr = instr + "\n\n" + cfg.ExtraInstructions
	}

	s := mcp.NewServer(
		&mcp.Implementation{Name: "zoekt", Version: "1.0.0"},
		&mcp.ServerOptions{Instructions: instr},
	)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search",
		Description: "Search code in indexed repositories.",
	}, a.handleSearch)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file",
		Description: "Get contents of a specific file.",
	}, a.handleGetFile)

	return &Server{mcp: s, app: a}
}

// Tracker returns the task tracker used by the server.
func (s *Server) Tracker() *TaskTracker { return s.app.tracker }

// SetQueue sets the index queue on the server's app.
func (s *Server) SetQueue(q *indexer.Queue) { s.app.queue = q }

// ServeStdio runs the MCP server over stdin/stdout.
func (s *Server) ServeStdio() error {
	return s.mcp.Run(context.Background(), &mcp.StdioTransport{})
}

// RegisterHandlers registers MCP and reindex HTTP handlers on the given mux.
// MCP is served at /mcp via streamable-http (stateless).
// Reindex API is served at /api/reindex.
func (s *Server) RegisterHandlers(mux *http.ServeMux) {
	if s.httpHandler == nil {
		s.httpHandler = mcp.NewStreamableHTTPHandler(
			func(r *http.Request) *mcp.Server { return s.mcp },
			&mcp.StreamableHTTPOptions{Stateless: true},
		)
	}
	mux.Handle("/mcp", s.httpHandler)
	mux.Handle("/mcp/", s.httpHandler)
	mux.HandleFunc("POST /api/reindex", s.app.postReindex)
	mux.HandleFunc("GET /api/reindex/{taskID}", s.app.getReindex)
	mux.HandleFunc("GET /api/file", s.app.getFile)
}

// --- internal app ---

type app struct {
	searcher zoekt.Streamer
	reposDir string
	tracker  *TaskTracker
	queue    *indexer.Queue
}
