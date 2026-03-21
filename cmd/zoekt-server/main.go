package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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
	indexDir := yamlCfg.IndexDir
	if indexDir == "" {
		indexDir = filepath.Join(dataDir, "index")
	}

	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		log.Fatalf("create index dir: %v", err)
	}

	streamer, err := search.NewDirectorySearcherFast(indexDir)
	if err != nil {
		log.Fatalf("NewDirectorySearcherFast: %v", err)
	}
	defer streamer.Close()

	instrText := yamlCfg.Instructions
	if instrText == "" && yamlCfg.InstrFile != "" {
		data, err := os.ReadFile(yamlCfg.InstrFile)
		if err != nil {
			log.Fatalf("read instructions file: %v", err)
		}
		instrText = string(data)
	}

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

	mirrorEntries, credCleanup, err := config.ConvertMirrors(yamlCfg.Mirrors)
	if err != nil {
		log.Fatalf("convert mirrors: %v", err)
	}
	if credCleanup != nil {
		defer credCleanup()
	}

	srv := server.New(server.Config{
		Searcher:          streamer,
		ReposDir:          filepath.Join(dataDir, "repos"),
		ExtraInstructions: instrText,
	})

	idx := indexer.New(indexer.Options{
		DataDir:        dataDir,
		IndexDir:       indexDir,
		FetchInterval:  yamlCfg.FetchInterval,
		MirrorInterval: yamlCfg.MirrorInterval,
		IndexTimeout:   yamlCfg.IndexTimeout,
		CPUFraction:    yamlCfg.CPUFraction,
		MaxLogAge:      yamlCfg.MaxLogAge,
		MirrorEntries:  mirrorEntries,
	}, srv.Tracker())
	srv.SetQueue(idx.Queue())

	mux := http.NewServeMux()
	srv.RegisterHandlers(mux)
	docs.RegisterHandlers(mux)
	static.RegisterHandlers(mux)

	// Forked upstream web UI with dark mode and vendored assets
	webServer := &web.Server{
		Searcher: streamer,
		Top:      ui.Top,
		HTML:     true,
		RPC:      true,
		Print:    true,
	}
	webMux, err := web.NewMux(webServer)
	if err != nil {
		log.Fatalf("web.NewMux: %v", err)
	}
	mux.Handle("/", webMux)

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

	slog.Info("listening", "addr", yamlCfg.Listen)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
