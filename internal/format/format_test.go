package format

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sourcegraph/zoekt"
)

func TestSearchResultFilesMode(t *testing.T) {
	result := &zoekt.SearchResult{
		Files: []zoekt.FileMatch{
			{Repository: "repo", FileName: "a.go"},
			{Repository: "repo", FileName: "b.go"},
		},
	}
	result.FileCount = 2

	out := SearchResult(result, "files")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	files := parsed["results"].([]any)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestSearchResultLinesMode(t *testing.T) {
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

	out := SearchResult(result, "lines")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	results := parsed["results"].(map[string]any)
	if _, ok := results["repo:a.go"]; !ok {
		t.Fatal("expected repo:a.go in results")
	}
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

func TestRepoListNamesOnly(t *testing.T) {
	var parsed struct {
		Results      []string `json:"results"`
		TotalMatches int      `json:"total_matches"`
	}
	if err := json.Unmarshal([]byte(RepoList(repoList(), false)), &parsed); err != nil {
		t.Fatal(err)
	}
	if got := parsed.Results; len(got) != 2 || got[0] != "host/org/apple" || got[1] != "host/org/zebra" {
		t.Fatalf("expected sorted names, got %v", got)
	}
	if parsed.TotalMatches != 2 {
		t.Fatalf("expected total_matches 2, got %d", parsed.TotalMatches)
	}
}

func TestRepoListDetailCarriesIndexedCommit(t *testing.T) {
	var parsed struct {
		Results []RepoDetail `json:"results"`
	}
	if err := json.Unmarshal([]byte(RepoList(repoList(), true)), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Results) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(parsed.Results))
	}
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

func TestRepoListDetailToleratesUnindexedRepo(t *testing.T) {
	var parsed struct {
		Results []RepoDetail `json:"results"`
	}
	if err := json.Unmarshal([]byte(RepoList(repoList(), true)), &parsed); err != nil {
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
