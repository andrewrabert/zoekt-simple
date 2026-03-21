package indexer

import (
	"testing"
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
