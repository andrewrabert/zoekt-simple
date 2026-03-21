package server

import (
	"encoding/json"
	"testing"

	"github.com/sourcegraph/zoekt"
)

func TestFormatSearchResultFilesMode(t *testing.T) {
	result := &zoekt.SearchResult{
		Files: []zoekt.FileMatch{
			{Repository: "repo", FileName: "a.go"},
			{Repository: "repo", FileName: "b.go"},
		},
	}
	result.FileCount = 2

	out := formatSearchResult(result, "files")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	files := parsed["results"].([]any)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestFormatSearchResultLinesMode(t *testing.T) {
	result := &zoekt.SearchResult{
		Files: []zoekt.FileMatch{
			{
				Repository: "repo",
				FileName:   "a.go",
				LineMatches: []zoekt.LineMatch{
					{LineNumber: 10, Line: []byte("func main()")},
				},
			},
		},
	}
	result.FileCount = 1

	out := formatSearchResult(result, "lines")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	results := parsed["results"].(map[string]any)
	if _, ok := results["repo:a.go"]; !ok {
		t.Fatal("expected repo:a.go in results")
	}
}

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
