package cli

import (
	"context"
	"fmt"
	"html"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/sourcegraph/zoekt"
	"github.com/sourcegraph/zoekt/search"
	"github.com/sourcegraph/zoekt/web"
	"github.com/spf13/cobra"

	"github.com/sourcegraph/zoekt-simple/internal/config"
	"github.com/sourcegraph/zoekt-simple/internal/docs"
	"github.com/sourcegraph/zoekt-simple/internal/indexer"
	"github.com/sourcegraph/zoekt-simple/internal/server"
	"github.com/sourcegraph/zoekt-simple/internal/static"
	"github.com/sourcegraph/zoekt-simple/internal/ui"
)

func newServeCmd() *cobra.Command {
	var (
		configFile string
		listen     string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the zoekt-simple web server and indexer",
		Long:  "Start the zoekt-simple server that provides web UI, API, and MCP endpoints along with background indexing.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(configFile, listen)
		},
	}

	cmd.Flags().StringVar(&configFile, "config", envDefault("ZOEKT_CONFIG", ""), "path to YAML config file (env: ZOEKT_CONFIG)")
	cmd.Flags().StringVar(&listen, "listen", envDefault("ZOEKT_LISTEN", ""), "override listen address (env: ZOEKT_LISTEN)")

	return cmd
}

func runServe(configFile, listen string) error {
	if configFile == "" {
		return fmt.Errorf("required: --config <path> or ZOEKT_CONFIG env var")
	}

	yamlCfg, err := config.LoadYAMLConfig(configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if listen != "" {
		yamlCfg.Listen = listen
	}
	if yamlCfg.Listen == "" {
		yamlCfg.Listen = ":8000"
	}
	if yamlCfg.DataDir == "" {
		yamlCfg.DataDir = "/data"
	}

	dataDir := yamlCfg.DataDir
	reposDir := filepath.Join(dataDir, "repos")

	// Resolve global instructions (used as fallback for indexes without their own).
	globalInstr, err := resolveInstructions(yamlCfg.Instructions, yamlCfg.InstrFile)
	if err != nil {
		return fmt.Errorf("global instructions: %w", err)
	}

	// Netrc for git auth.
	netrcEntries, err := config.NetrcEntries(yamlCfg.Mirrors)
	if err != nil {
		return fmt.Errorf("netrc entries: %w", err)
	}
	if len(netrcEntries) > 0 {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = "/root"
		}
		if err := config.WriteNetrc(filepath.Join(home, ".netrc"), netrcEntries); err != nil {
			return fmt.Errorf("write netrc: %w", err)
		}
	}

	// Convert mirrors.
	mirrorEntries, credCleanup, err := config.ConvertMirrors(yamlCfg.Mirrors)
	if err != nil {
		return fmt.Errorf("convert mirrors: %w", err)
	}
	if credCleanup != nil {
		defer credCleanup()
	}

	// Resolve indexes.
	indexes := yamlCfg.ResolvedIndexes()
	defaultName := yamlCfg.ResolvedDefaultIndex()
	if _, ok := indexes[defaultName]; !ok {
		return fmt.Errorf("default_index %q not found in indexes", defaultName)
	}

	// Build per-index streamers, servers, web UIs, and indexer targets.
	var targets []indexer.IndexTarget
	var streamers []zoekt.Streamer
	var defaultTracker *server.TaskTracker
	var defaultQueue func(q *indexer.Queue)

	mux := http.NewServeMux()

	for name, idxCfg := range indexes {
		indexDir := filepath.Join(dataDir, "index", name)
		if err := os.MkdirAll(indexDir, 0o755); err != nil {
			return fmt.Errorf("MkdirAll(%s): %w", indexDir, err)
		}

		streamer, err := search.NewDirectorySearcherFast(indexDir)
		if err != nil {
			return fmt.Errorf("NewDirectorySearcherFast(%s): %w", indexDir, err)
		}
		streamers = append(streamers, streamer)

		// Resolve per-index instructions with fallback to global.
		instr, err := resolveInstructions(idxCfg.Instructions, idxCfg.InstrFile)
		if err != nil {
			return fmt.Errorf("instructions for index %q: %w", name, err)
		}
		if instr == "" {
			instr = globalInstr
		}

		srv := server.New(server.Config{
			Searcher:          streamer,
			ReposDir:          reposDir,
			ExtraInstructions: instr,
		})

		// Mount MCP at /index/<name>/mcp
		indexPrefix := fmt.Sprintf("/index/%s", name)
		indexMux := http.NewServeMux()
		srv.RegisterHandlers(indexMux)

		// Mount web UI at /index/<name>/
		webServer := &web.Server{
			Searcher: streamer,
			Top:      ui.Top,
			HTML:     true,
			RPC:      true,
			Print:    true,
		}
		webMux, err := web.NewMux(webServer)
		if err != nil {
			return fmt.Errorf("web.NewMux for index %q: %w", name, err)
		}
		indexMux.Handle("/", webMux)

		mux.Handle(indexPrefix+"/", http.StripPrefix(indexPrefix, indexMux))

		// Default index also serves at root.
		if name == defaultName {
			defaultTracker = srv.Tracker()
			defaultQueue = func(q *indexer.Queue) { srv.SetQueue(q) }

			rootMux := http.NewServeMux()
			srv.RegisterHandlers(rootMux)
			rootMux.Handle("/", webMux)

			mux.Handle("/mcp", rootMux)
			mux.Handle("/mcp/", rootMux)
			mux.Handle("/api/", rootMux)
			mux.Handle("/search", rootMux)
			mux.Handle("/", rootMux)
		}

		// Compile indexer target.
		target, err := compileTarget(name, indexDir, idxCfg)
		if err != nil {
			return fmt.Errorf("compile target %q: %w", name, err)
		}
		targets = append(targets, target)

		slog.Info("configured index", "name", name, "dir", indexDir, "default", name == defaultName)
	}

	// Landing page at /index/ listing all indexes.
	var indexNames []string
	for name := range indexes {
		indexNames = append(indexNames, name)
	}
	sort.Strings(indexNames)
	indexPageHTML := buildIndexPage(indexNames, defaultName)
	mux.HandleFunc("GET /index/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexPageHTML))
	})

	// Catch-all for undefined indexes.
	mux.HandleFunc("/index/{name}/", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if _, ok := indexes[name]; !ok {
			http.Error(w, fmt.Sprintf("index %q not found", name), http.StatusNotFound)
			return
		}
	})

	// Global routes.
	docs.RegisterHandlers(mux)
	static.RegisterHandlers(mux)

	// Create indexer with all targets.
	idx := indexer.New(indexer.Options{
		DataDir:        dataDir,
		Targets:        targets,
		FetchInterval:  yamlCfg.FetchInterval,
		MirrorInterval: yamlCfg.MirrorInterval,
		IndexTimeout:   yamlCfg.IndexTimeout,
		CPUFraction:    yamlCfg.CPUFraction,
		MaxLogAge:      yamlCfg.MaxLogAge,
		MirrorEntries:  mirrorEntries,
	}, defaultTracker)
	defaultQueue(idx.Queue())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go idx.Run(ctx)

	httpSrv := &http.Server{Addr: yamlCfg.Listen, Handler: mux}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		slog.Info("shutting down")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	// Close all streamers on exit.
	defer func() {
		for _, s := range streamers {
			s.Close()
		}
	}()

	slog.Info("listening", "addr", yamlCfg.Listen)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("ListenAndServe: %w", err)
	}
	return nil
}

func resolveInstructions(text, filePath string) (string, error) {
	if text != "" {
		return text, nil
	}
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read instructions file %s: %w", filePath, err)
		}
		return string(data), nil
	}
	return "", nil
}

func compileTarget(name, indexDir string, cfg config.IndexConfig) (indexer.IndexTarget, error) {
	t := indexer.IndexTarget{
		Name:     name,
		IndexDir: indexDir,
	}

	for _, inc := range cfg.Include {
		re, err := regexp.Compile(inc.Repo)
		if err != nil {
			return t, fmt.Errorf("include repo pattern %q: %w", inc.Repo, err)
		}
		ci := indexer.CompiledInclude{Repo: re}
		if inc.Refs != "" {
			br, err := regexp.Compile(inc.Refs)
			if err != nil {
				return t, fmt.Errorf("include refs pattern %q: %w", inc.Refs, err)
			}
			ci.Refs = br
		}
		t.Include = append(t.Include, ci)
	}

	for _, ex := range cfg.Exclude {
		re, err := regexp.Compile(ex)
		if err != nil {
			return t, fmt.Errorf("exclude pattern %q: %w", ex, err)
		}
		t.Exclude = append(t.Exclude, re)
	}

	return t, nil
}

func buildIndexPage(names []string, defaultName string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html>`)
	b.WriteString(ui.TemplateText["head"])
	b.WriteString(`<body>`)
	b.WriteString(`<nav class="navbar navbar-default"><div class="container-fluid">`)
	b.WriteString(`<div class="navbar-header"><a class="navbar-brand" href="/">Zoekt</a></div>`)
	b.WriteString(`</div></nav>`)
	b.WriteString(`<div class="container" style="margin-top:1em">`)
	b.WriteString(`<h3>Indexes</h3>`)
	b.WriteString(`<table class="table table-hover table-condensed">`)
	b.WriteString(`<thead><tr><th>Name</th><th>Web UI</th><th>MCP Endpoint</th></tr></thead><tbody>`)
	for _, name := range names {
		esc := html.EscapeString(name)
		prefix := "/index/" + esc + "/"
		label := esc
		if name == defaultName {
			label += " (default)"
		}
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td>`, label))
		b.WriteString(fmt.Sprintf(`<td><a href="%s">%s</a></td>`, prefix, prefix))
		b.WriteString(fmt.Sprintf(`<td><code>%smcp</code></td></tr>`, prefix))
	}
	b.WriteString(`</tbody></table></div></body></html>`)
	return b.String()
}

func envDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// RunServe is exported so the legacy zoekt-server binary can delegate to it.
func RunServe(configFile, listen string) {
	if err := runServe(configFile, listen); err != nil {
		log.Fatal(err)
	}
}
