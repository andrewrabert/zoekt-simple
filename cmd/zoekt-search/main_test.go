package main

import (
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/zoekt"
)

func TestFormatRepoDetailsPlainOneRowPerBranch(t *testing.T) {
	indexed := time.Date(2026, 4, 2, 20, 32, 30, 0, time.UTC)
	committed := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	repoList := &zoekt.RepoList{
		Repos: []*zoekt.RepoListEntry{
			{
				Repository: zoekt.Repository{
					Name:             "host/org/zebra",
					LatestCommitDate: committed,
					Branches: []zoekt.RepositoryBranch{
						{Name: "main", Version: "aaaa"},
						{Name: "release", Version: "bbbb"},
					},
				},
				IndexMetadata: zoekt.IndexMetadata{IndexTime: indexed},
			},
			{Repository: zoekt.Repository{Name: "host/org/apple"}},
		},
	}

	rows := strings.Split(formatRepoDetailsPlain(repoList), "\n")
	want := []string{
		"host/org/apple\t\t\t0001-01-01T00:00:00Z\t0001-01-01T00:00:00Z",
		"host/org/zebra\tmain\taaaa\t2026-04-01T09:00:00Z\t2026-04-02T20:32:30Z",
		"host/org/zebra\trelease\tbbbb\t2026-04-01T09:00:00Z\t2026-04-02T20:32:30Z",
	}
	if len(rows) != len(want) {
		t.Fatalf("expected %d rows, got %d:\n%s", len(want), len(rows), strings.Join(rows, "\n"))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d:\n got %q\nwant %q", i, rows[i], want[i])
		}
	}
}
