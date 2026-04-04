package indexer

import (
	"testing"
	"time"
)

func TestOptionsValidate(t *testing.T) {
	opts := Options{
		CPUFraction:   0.25,
		FetchInterval: 0, // should get default
	}
	opts.validate()
	if opts.cpuCount < 1 {
		t.Fatalf("expected cpuCount >= 1, got %d", opts.cpuCount)
	}
}

func TestOptionsBadCPUFraction(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid cpu_fraction")
		}
	}()
	opts := Options{CPUFraction: -1}
	opts.validate()
}

func TestReconfigure(t *testing.T) {
	opts := Options{
		DataDir:       t.TempDir(),
		CPUFraction:   0.25,
		FetchInterval: 5 * time.Minute,
	}
	tracker := &noopTracker{}
	idx := New(opts, tracker)

	newOpts := Options{
		DataDir:       opts.DataDir,
		CPUFraction:   0.5,
		FetchInterval: 10 * time.Minute,
	}
	idx.Reconfigure(newOpts)

	got := idx.CurrentOptions()
	if got.CPUFraction != 0.5 {
		t.Fatalf("expected CPUFraction 0.5, got %f", got.CPUFraction)
	}
	if got.FetchInterval != 10*time.Minute {
		t.Fatalf("expected FetchInterval 10m, got %s", got.FetchInterval)
	}
}

type noopTracker struct{}

func (n *noopTracker) Update(id, status string, errMsg *string) {}
