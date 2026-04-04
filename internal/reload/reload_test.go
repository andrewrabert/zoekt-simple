package reload

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/zoekt-simple/internal/config"
)

func TestReloadCallsReloadFunc(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initial := []byte("listen: \":8000\"\ndata_dir: /data\nfetch_interval: 5m\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	var reloadCount atomic.Int32
	var lastCfg atomic.Value

	r, err := NewReloader(cfgPath, func(oldCfg, newCfg *config.YAMLConfig) error {
		reloadCount.Add(1)
		lastCfg.Store(newCfg)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	// Write a change.
	updated := []byte("listen: \":8000\"\ndata_dir: /data\nfetch_interval: 10m\n")
	if err := os.WriteFile(cfgPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	// Trigger explicit reload.
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}

	if reloadCount.Load() != 1 {
		t.Fatalf("expected 1 reload, got %d", reloadCount.Load())
	}

	got := lastCfg.Load().(*config.YAMLConfig)
	if got.FetchInterval != 10*time.Minute {
		t.Fatalf("expected 10m, got %s", got.FetchInterval)
	}

	// Verify Current() was updated.
	cur := r.Current()
	if cur.FetchInterval != 10*time.Minute {
		t.Fatalf("Current() not updated: expected 10m, got %s", cur.FetchInterval)
	}
}

func TestReloadSkipsNoOp(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initial := []byte("listen: \":8000\"\ndata_dir: /data\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	var reloadCount atomic.Int32

	r, err := NewReloader(cfgPath, func(oldCfg, newCfg *config.YAMLConfig) error {
		reloadCount.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	// Reload without changing the file.
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}

	if reloadCount.Load() != 0 {
		t.Fatalf("expected 0 reloads for identical config, got %d", reloadCount.Load())
	}
}

func TestReloadWarnsOnImmutableChange(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initial := []byte("listen: \":8000\"\ndata_dir: /data\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	var reloadCount atomic.Int32

	r, err := NewReloader(cfgPath, func(oldCfg, newCfg *config.YAMLConfig) error {
		reloadCount.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	// Change an immutable field + a mutable field.
	updated := []byte("listen: \":9999\"\ndata_dir: /data\nfetch_interval: 10m\n")
	if err := os.WriteFile(cfgPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}

	if reloadCount.Load() != 1 {
		t.Fatalf("expected 1 reload even when immutable fields changed, got %d", reloadCount.Load())
	}
}

func TestReloadInvalidConfigKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initial := []byte("listen: \":8000\"\ndata_dir: /data\nfetch_interval: 5m\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	var reloadCount atomic.Int32

	r, err := NewReloader(cfgPath, func(oldCfg, newCfg *config.YAMLConfig) error {
		reloadCount.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	// Write invalid YAML.
	if err := os.WriteFile(cfgPath, []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = r.Reload()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}

	if reloadCount.Load() != 0 {
		t.Fatalf("expected 0 reload calls for invalid YAML, got %d", reloadCount.Load())
	}

	// Current config should still be the initial one.
	cur := r.Current()
	if cur.FetchInterval != 5*time.Minute {
		t.Fatalf("expected 5m (previous config), got %s", cur.FetchInterval)
	}
}

func TestReloadEndToEnd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initial := []byte("listen: \":8000\"\ndata_dir: /data\nfetch_interval: 5m\nmirrors:\n  - github:\n      org: testorg\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	var reloadCalls atomic.Int32
	var lastNew atomic.Value

	r, err := NewReloader(cfgPath, func(old, new *config.YAMLConfig) error {
		reloadCalls.Add(1)
		lastNew.Store(new)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	// Verify initial config parsed correctly.
	cur := r.Current()
	if cur.FetchInterval != 5*time.Minute {
		t.Fatalf("initial FetchInterval: expected 5m, got %s", cur.FetchInterval)
	}

	// Modify: change fetch_interval and add a mirror.
	updated := []byte("listen: \":8000\"\ndata_dir: /data\nfetch_interval: 10m\nmirrors:\n  - github:\n      org: testorg\n  - github:\n      org: neworg\n")
	if err := os.WriteFile(cfgPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}

	if reloadCalls.Load() != 1 {
		t.Fatalf("expected 1 reload, got %d", reloadCalls.Load())
	}

	got := lastNew.Load().(*config.YAMLConfig)
	if got.FetchInterval != 10*time.Minute {
		t.Fatalf("expected 10m, got %s", got.FetchInterval)
	}
	if len(got.Mirrors) != 2 {
		t.Fatalf("expected 2 mirrors, got %d", len(got.Mirrors))
	}

	cur = r.Current()
	if cur.FetchInterval != 10*time.Minute {
		t.Fatalf("Current() not updated: expected 10m, got %s", cur.FetchInterval)
	}
}
