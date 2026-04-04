package reload

import (
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sourcegraph/zoekt-simple/internal/config"
)

// ReloadFunc is called when the config changes. It receives the old and
// new configs. If it returns an error, the error is logged and the old config
// is retained.
type ReloadFunc func(old, new *config.YAMLConfig) error

// Reloader manages config reloads triggered by SIGHUP or explicit Reload calls.
type Reloader struct {
	stop     chan struct{}
	done     chan struct{}
	sighup   chan os.Signal
	reloadMu sync.Mutex       // serialises doReload calls
	mu       sync.RWMutex     // protects current
	current  *config.YAMLConfig
	path     string
	reloadFn ReloadFunc
}

// NewReloader creates a Reloader that listens for SIGHUP to reload the config
// file at path. It parses the initial config but does NOT call the reload
// function for the initial load.
func NewReloader(path string, fn ReloadFunc) (*Reloader, error) {
	initial, err := config.LoadYAMLConfig(path)
	if err != nil {
		return nil, err
	}

	r := &Reloader{
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		sighup:   make(chan os.Signal, 1),
		current:  initial,
		path:     path,
		reloadFn: fn,
	}

	signal.Notify(r.sighup, syscall.SIGHUP)
	go r.loop()
	return r, nil
}

// Stop stops the reloader and waits for the loop to exit.
func (r *Reloader) Stop() {
	signal.Stop(r.sighup)
	close(r.stop)
	<-r.done
}

// Current returns the most recently loaded config.
func (r *Reloader) Current() *config.YAMLConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}

// Reload re-reads the config file and applies changes. It is safe to call
// from any goroutine (e.g. an HTTP handler). Returns nil if the config was
// unchanged.
func (r *Reloader) Reload() error {
	return r.doReload()
}

func (r *Reloader) loop() {
	defer close(r.done)

	for {
		select {
		case <-r.stop:
			return
		case <-r.sighup:
			slog.Info("received SIGHUP, reloading config")
			r.doReload()
		}
	}
}

func (r *Reloader) doReload() error {
	// Serialise reloads so a concurrent SIGHUP + POST /api/reload
	// cannot double-apply (closing streamers twice, etc.).
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	newCfg, err := config.LoadYAMLConfig(r.path)
	if err != nil {
		slog.Error("config reload: parse failed, keeping previous config", "error", err)
		return err
	}

	r.mu.RLock()
	old := r.current
	r.mu.RUnlock()

	// Skip no-op reloads.
	if old.Equal(newCfg) {
		slog.Debug("config reload: no changes detected")
		return nil
	}

	// Warn on immutable field changes.
	if changed := config.ImmutableFieldsChanged(old, newCfg); len(changed) > 0 {
		slog.Warn("config reload: immutable fields changed (ignored, restart required)",
			"fields", changed)
	}

	slog.Info("config reload: applying changes")
	if err := r.reloadFn(old, newCfg); err != nil {
		slog.Error("config reload: reload failed, keeping previous config", "error", err)
		return err
	}

	r.mu.Lock()
	r.current = newCfg
	r.mu.Unlock()

	slog.Info("config reload: success")
	return nil
}
