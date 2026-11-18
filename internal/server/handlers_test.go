package server

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sourcegraph/zoekt"
	"github.com/sourcegraph/zoekt/index"
	"github.com/sourcegraph/zoekt/search"
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

func indexedSearcher(t *testing.T, content string) zoekt.Streamer {
	t.Helper()
	dir := t.TempDir()
	builder, err := index.NewBuilder(index.Options{
		IndexDir:              dir,
		RepositoryDescription: zoekt.Repository{Name: "repo"},
		DisableCTags:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.AddFile("a.txt", []byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := builder.Finish(); err != nil {
		t.Fatal(err)
	}
	searcher, err := search.NewDirectorySearcher(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(searcher.Close)
	return searcher
}

func searchLines(t *testing.T, content string, contextLines int) map[string]string {
	t.Helper()
	a := &app{searcher: indexedSearcher(t, content)}
	result, _, err := a.handleSearch(context.Background(), nil, searchArgs{Query: "needle", ContextLines: contextLines})
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Results map[string]map[string]string `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed.Results["repo:a.txt"]
}

func assertLines(t *testing.T, got, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSearchWithoutContextLinesReturnsOnlyMatches(t *testing.T) {
	got := searchLines(t, "one\ntwo\nneedle three\nfour\nfive\n", 0)
	assertLines(t, got, map[string]string{"3": "needle three"})
}

func TestSearchContextLinesSurroundMatch(t *testing.T) {
	got := searchLines(t, "one\ntwo\nneedle three\nfour\nfive\n", 1)
	assertLines(t, got, map[string]string{"2-4": "two\nneedle three\nfour"})
}

func TestSearchContextLinesMergeOverlappingMatches(t *testing.T) {
	got := searchLines(t, "one\ntwo\nneedle three\nfour\nneedle five\nsix\nseven\neight\n", 2)
	assertLines(t, got, map[string]string{"1-7": "one\ntwo\nneedle three\nfour\nneedle five\nsix\nseven"})
}

func TestSearchContextLinesMergeAdjacentContext(t *testing.T) {
	got := searchLines(t, "one\nneedle two\nthree\nfour\nneedle five\nsix\n", 1)
	assertLines(t, got, map[string]string{"1-6": "one\nneedle two\nthree\nfour\nneedle five\nsix"})
}

func TestSearchContextLinesKeepDistantMatchesApart(t *testing.T) {
	got := searchLines(t, "one\nneedle two\nthree\nfour\nfive\nneedle six\nseven\n", 1)
	assertLines(t, got, map[string]string{
		"1-3": "one\nneedle two\nthree",
		"5-7": "five\nneedle six\nseven",
	})
}

func TestSearchContextLinesStopAtFileBoundaries(t *testing.T) {
	got := searchLines(t, "needle one\ntwo\nthree\nfour\nfive\nsix\nneedle seven", 2)
	assertLines(t, got, map[string]string{
		"1-3": "needle one\ntwo\nthree",
		"5-7": "five\nsix\nneedle seven",
	})
}

func TestSearchContextLinesKeepBlankLines(t *testing.T) {
	got := searchLines(t, "one\n\nneedle three\n\nfive\n", 1)
	assertLines(t, got, map[string]string{"2-4": "\nneedle three\n"})
}

func repoList() *zoekt.RepoList {
	indexed := time.Date(2026, 4, 2, 20, 32, 30, 0, time.UTC)
	committed := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	return &zoekt.RepoList{
		Repos: []*zoekt.RepoListEntry{
			{
				Repository: zoekt.Repository{
					Name:             "host/org/zebra",
					LatestCommitDate: committed,
					Branches: []zoekt.RepositoryBranch{
						{Name: "main", Version: "2fb345a6c2e58e54fe090028d0bd58bcd9288ea6"},
					},
				},
				IndexMetadata: zoekt.IndexMetadata{IndexTime: indexed},
			},
			{
				Repository: zoekt.Repository{Name: "host/org/apple"},
			},
		},
	}
}

func TestFormatRepoListNamesOnly(t *testing.T) {
	// The default repos mode must keep returning a sorted []string.
	// Callers parse it as one, so widening it in place would break them.
	var parsed struct {
		Results      []string `json:"results"`
		TotalMatches int      `json:"total_matches"`
	}
	if err := json.Unmarshal([]byte(formatRepoList(repoList(), false)), &parsed); err != nil {
		t.Fatal(err)
	}
	if got := parsed.Results; len(got) != 2 || got[0] != "host/org/apple" || got[1] != "host/org/zebra" {
		t.Fatalf("expected sorted names, got %v", got)
	}
	if parsed.TotalMatches != 2 {
		t.Fatalf("expected total_matches 2, got %d", parsed.TotalMatches)
	}
}

func TestFormatRepoListDetailCarriesIndexedCommit(t *testing.T) {
	// The indexed commit SHA is the point of this mode: it lets a caller
	// tell whether anything it cached from a repo could have changed,
	// without re-fetching a single file.
	var parsed struct {
		Results []repoDetail `json:"results"`
	}
	if err := json.Unmarshal([]byte(formatRepoList(repoList(), true)), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Results) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(parsed.Results))
	}
	// Sorted by name, so zebra is second.
	zebra := parsed.Results[1]
	if zebra.Name != "host/org/zebra" {
		t.Fatalf("expected sorting by name, got %q first", parsed.Results[0].Name)
	}
	if len(zebra.Branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(zebra.Branches))
	}
	if zebra.Branches[0].Version != "2fb345a6c2e58e54fe090028d0bd58bcd9288ea6" {
		t.Fatalf("expected the indexed commit SHA, got %q", zebra.Branches[0].Version)
	}
	if zebra.Branches[0].Name != "main" {
		t.Fatalf("expected branch name main, got %q", zebra.Branches[0].Name)
	}
	if !zebra.IndexTime.Equal(time.Date(2026, 4, 2, 20, 32, 30, 0, time.UTC)) {
		t.Fatalf("expected the index time, got %v", zebra.IndexTime)
	}
}

func TestFormatRepoListDetailToleratesUnindexedRepo(t *testing.T) {
	// A repo with no indexed branches must still be listed rather than
	// dropped or panicking on an empty slice.
	var parsed struct {
		Results []repoDetail `json:"results"`
	}
	if err := json.Unmarshal([]byte(formatRepoList(repoList(), true)), &parsed); err != nil {
		t.Fatal(err)
	}
	apple := parsed.Results[0]
	if apple.Name != "host/org/apple" {
		t.Fatalf("expected host/org/apple, got %q", apple.Name)
	}
	if len(apple.Branches) != 0 {
		t.Fatalf("expected no branches, got %v", apple.Branches)
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
