// Package format renders zoekt results as JSON shared by the MCP server and CLI.
package format

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sourcegraph/zoekt"
)

// RepoBranch is one indexed branch and the commit it was indexed at.
type RepoBranch struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// RepoDetail is a repository with its indexed branches and timestamps.
type RepoDetail struct {
	Name             string       `json:"name"`
	Branches         []RepoBranch `json:"branches"`
	LatestCommitDate time.Time    `json:"latest_commit_date"`
	IndexTime        time.Time    `json:"index_time"`
}

// RepoList renders a List response as RepoDetail entries, or as a sorted
// list of names when detail is false.
func RepoList(repoList *zoekt.RepoList, detail bool) string {
	meta := map[string]any{
		"total_matches": len(repoList.Repos),
		"returned":      len(repoList.Repos),
		"truncated":     false,
	}

	if !detail {
		repos := make([]string, 0, len(repoList.Repos))
		for _, r := range repoList.Repos {
			repos = append(repos, r.Repository.Name)
		}
		sort.Strings(repos)
		meta["results"] = repos
		b, _ := json.Marshal(meta)
		return string(b)
	}

	meta["results"] = RepoDetails(repoList)
	b, _ := json.Marshal(meta)
	return string(b)
}

// RepoDetails converts a List response into RepoDetail entries sorted by name.
func RepoDetails(repoList *zoekt.RepoList) []RepoDetail {
	details := make([]RepoDetail, 0, len(repoList.Repos))
	for _, r := range repoList.Repos {
		branches := make([]RepoBranch, 0, len(r.Repository.Branches))
		for _, b := range r.Repository.Branches {
			branches = append(branches, RepoBranch{Name: b.Name, Version: b.Version})
		}
		details = append(details, RepoDetail{
			Name:             r.Repository.Name,
			Branches:         branches,
			LatestCommitDate: r.Repository.LatestCommitDate,
			IndexTime:        r.IndexMetadata.IndexTime,
		})
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Name < details[j].Name })
	return details
}

// SearchResult renders a Search response. Mode "files" lists repo:path
// names; any other mode groups matched lines into ranges per file.
func SearchResult(result *zoekt.SearchResult, outputMode string) string {
	files := result.Files
	total := result.FileCount
	if total == 0 {
		total = len(files)
	}

	meta := map[string]any{
		"total_matches": total,
		"returned":      len(files),
		"truncated":     total > len(files),
	}

	if outputMode == "files" {
		seen := make(map[string]bool)
		for _, f := range files {
			seen[f.Repository+":"+f.FileName] = true
		}
		names := make([]string, 0, len(seen))
		for p := range seen {
			names = append(names, p)
		}
		sort.Strings(names)
		meta["results"] = names
		b, _ := json.Marshal(meta)
		return string(b)
	}

	type ld struct {
		num  int
		text string
	}
	type pathEntry struct {
		path  string
		lines []ld
	}

	var order []string
	pm := make(map[string]*pathEntry)
	for _, f := range files {
		path := f.Repository + ":" + f.FileName
		pe, ok := pm[path]
		if !ok {
			pe = &pathEntry{path: path}
			pm[path] = pe
			order = append(order, path)
		}
		for _, m := range f.LineMatches {
			pe.lines = append(pe.lines, ld{m.LineNumber, strings.TrimRight(string(m.Line), "\n")})
		}
	}

	results := make(map[string]map[string]string)
	for _, path := range order {
		pe := pm[path]
		if len(pe.lines) == 0 {
			continue
		}
		sort.Slice(pe.lines, func(i, j int) bool { return pe.lines[i].num < pe.lines[j].num })

		ranges := make(map[string]string)
		start, end := pe.lines[0].num, pe.lines[0].num
		texts := []string{pe.lines[0].text}
		for _, l := range pe.lines[1:] {
			if l.num == end+1 {
				end = l.num
				texts = append(texts, l.text)
			} else {
				key := fmt.Sprintf("%d", start)
				if start != end {
					key = fmt.Sprintf("%d-%d", start, end)
				}
				ranges[key] = strings.Join(texts, "\n")
				start, end = l.num, l.num
				texts = []string{l.text}
			}
		}
		key := fmt.Sprintf("%d", start)
		if start != end {
			key = fmt.Sprintf("%d-%d", start, end)
		}
		ranges[key] = strings.Join(texts, "\n")
		results[path] = ranges
	}

	meta["results"] = results
	b, _ := json.Marshal(meta)
	return string(b)
}
