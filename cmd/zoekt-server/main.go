package main

import (
	"context"
	"flag"
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

	"github.com/sourcegraph/zoekt-simple/internal/config"
	"github.com/sourcegraph/zoekt-simple/internal/docs"
	"github.com/sourcegraph/zoekt-simple/internal/indexer"
	"github.com/sourcegraph/zoekt-simple/internal/server"
	"github.com/sourcegraph/zoekt-simple/internal/static"
	"github.com/sourcegraph/zoekt-simple/internal/ui"
)

func envDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func main() {
	configFile := flag.String("config", envDefault("ZOEKT_CONFIG", ""), "path to YAML config file (env: ZOEKT_CONFIG)")
	listen := flag.String("listen", envDefault("ZOEKT_LISTEN", ""), "override listen address (env: ZOEKT_LISTEN)")
	flag.Parse()

	if *configFile == "" {
		log.Fatal("required: -config <path> or ZOEKT_CONFIG env var")
	}

	yamlCfg, err := config.LoadYAMLConfig(*configFile)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if *listen != "" {
		yamlCfg.Listen = *listen
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
		log.Fatalf("global instructions: %v", err)
	}

	// Netrc for git auth.
	netrcEntries, err := config.NetrcEntries(yamlCfg.Mirrors)
	if err != nil {
		log.Fatalf("netrc entries: %v", err)
	}
	if len(netrcEntries) > 0 {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = "/root"
		}
		if err := config.WriteNetrc(filepath.Join(home, ".netrc"), netrcEntries); err != nil {
			log.Fatalf("write netrc: %v", err)
		}
	}

	// Convert mirrors.
	mirrorEntries, credCleanup, err := config.ConvertMirrors(yamlCfg.Mirrors)
	if err != nil {
		log.Fatalf("convert mirrors: %v", err)
	}
	if credCleanup != nil {
		defer credCleanup()
	}

	// Resolve indexes.
	indexes := yamlCfg.ResolvedIndexes()
	defaultName := yamlCfg.ResolvedDefaultIndex()
	if _, ok := indexes[defaultName]; !ok {
		log.Fatalf("default_index %q not found in indexes", defaultName)
	}

	// Build per-index streamers, servers, web UIs, and indexer targets.
	var targets []indexer.IndexTarget
	var streamers []zoekt.Streamer
	// We need the first server's tracker for the indexer. Use the default index's server.
	var defaultTracker *server.TaskTracker
	var defaultQueue func(q *indexer.Queue)

	mux := http.NewServeMux()

	for name, idxCfg := range indexes {
		indexDir := filepath.Join(dataDir, "index", name)
		if err := os.MkdirAll(indexDir, 0o755); err != nil {
			log.Fatalf("MkdirAll(%s): %v", indexDir, err)
		}

		streamer, err := search.NewDirectorySearcherFast(indexDir)
		if err != nil {
			log.Fatalf("NewDirectorySearcherFast(%s): %v", indexDir, err)
		}
		streamers = append(streamers, streamer)

		// Resolve per-index instructions with fallback to global.
		instr, err := resolveInstructions(idxCfg.Instructions, idxCfg.InstrFile)
		if err != nil {
			log.Fatalf("instructions for index %q: %v", name, err)
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
			log.Fatalf("web.NewMux for index %q: %v", name, err)
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

			// Mount root-level routes. These go after /index/ routes
			// since ServeMux picks the most specific match.
			mux.Handle("/mcp", rootMux)
			mux.Handle("/mcp/", rootMux)
			mux.Handle("/api/", rootMux)
			mux.Handle("/search", rootMux)
			mux.Handle("/", rootMux)
		}

		// Compile indexer target.
		target, err := compileTarget(name, indexDir, idxCfg)
		if err != nil {
			log.Fatalf("compile target %q: %v", name, err)
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
		log.Fatalf("ListenAndServe: %v", err)
	}
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

// compileTarget converts config types into a compiled IndexTarget with regexes.
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
