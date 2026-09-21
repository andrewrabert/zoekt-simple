package server

import "testing"

func TestSliceLines(t *testing.T) {
	content := "line0\nline1\nline2\nline3\n"
	got := sliceLines(content, 1, 2)
	if got != "line1\nline2\n" {
		t.Fatalf("expected 'line1\\nline2\\n', got %q", got)
	}
}

func TestSliceLinesOffsetBeyondEnd(t *testing.T) {
	got := sliceLines("one\n", 10, 0)
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
