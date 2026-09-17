package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sourcegraph/zoekt"
	"github.com/sourcegraph/zoekt/query"
)

// --- tool handlers ---

type searchArgs struct {
	Query        string `json:"query" jsonschema:"Zoekt query string with optional filters (r: f: lang: sym: case:yes)"`
	MaxResults   int    `json:"max_results,omitempty" jsonschema:"Maximum number of file matches to return (default: 50)"`
	OutputMode   string `json:"output_mode,omitempty" jsonschema:"Output format: lines (default), files, repos, or repos_detail"`
	ContextLines int    `json:"context_lines,omitempty" jsonschema:"Number of context lines around matches (default: 0)"`
}

func (a *app) handleSearch(ctx context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, any, error) {
	maxResults := args.MaxResults
	if maxResults == 0 {
		maxResults = 50
	}
	outputMode := args.OutputMode
	if outputMode == "" {
		outputMode = "lines"
	}

	slog.Info("search", "query", args.Query, "max_results", maxResults, "output_mode", outputMode, "context_lines", args.ContextLines)

	q, err := query.Parse(args.Query)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid query: %v", err)
	}

	// Auto-detect repo-only queries (matching web UI behavior)
	if outputMode == "lines" {
		repoOnly := true
		query.VisitAtoms(q, func(q query.Q) {
			if _, ok := q.(*query.Repo); !ok {
				repoOnly = false
			}
		})
		if repoOnly {
			outputMode = "repos"
		}
	}

	// Use the List API for repos mode — Search results are capped by
	// MaxDocDisplayCount and can silently omit repos with fewer file hits.
	if outputMode == "repos" || outputMode == "repos_detail" {
		repoList, err := a.searcher.List(ctx, q, nil)
		if err != nil {
			slog.Error("list failed", "query", args.Query, "error", err)
			return nil, nil, fmt.Errorf("list failed: %v", err)
		}
		slog.Info("list complete", "query", args.Query, "repos", len(repoList.Repos), "output_mode", outputMode)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: formatRepoList(repoList, outputMode == "repos_detail"),
			}},
		}, nil, nil
	}

	result, err := a.searcher.Search(ctx, q, &zoekt.SearchOptions{
		MaxDocDisplayCount: maxResults,
		NumContextLines:    args.ContextLines,
	})
	if err != nil {
		slog.Error("search failed", "query", args.Query, "error", err)
		return nil, nil, fmt.Errorf("search failed: %v", err)
	}

	total := result.FileCount
	returned := len(result.Files)
	slog.Info("search complete", "query", args.Query, "total", total, "returned", returned)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: formatSearchResult(result, outputMode)}},
	}, nil, nil
}

type getFileArgs struct {
	Hostname string `json:"hostname" jsonschema:"Git host"`
	Repo     string `json:"repo" jsonschema:"Org and repo (e.g. myorg/myrepo)"`
	Path     string `json:"path" jsonschema:"File path within the repository"`
	Offset   int    `json:"offset,omitempty" jsonschema:"Skip first N lines (default: 0)"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Return only N lines, 0 = all (default: 0)"`
}

func (a *app) handleGetFile(ctx context.Context, _ *mcp.CallToolRequest, args getFileArgs) (*mcp.CallToolResult, any, error) {
	slog.Info("get_file", "hostname", args.Hostname, "repo", args.Repo, "path", args.Path, "offset", args.Offset, "limit", args.Limit)

	barePath := filepath.Join(a.reposDir, args.Hostname, args.Repo+".git")
	content, err := readFileFromRepo(barePath, args.Path)
	if err != nil {
		slog.Error("get_file failed", "hostname", args.Hostname, "repo", args.Repo, "path", args.Path, "error", err)
		return nil, nil, fmt.Errorf("get_file failed: %v", err)
	}

	if args.Offset > 0 || args.Limit > 0 {
		content = sliceLines(content, args.Offset, args.Limit)
	}

	slog.Info("get_file complete", "hostname", args.Hostname, "repo", args.Repo, "path", args.Path, "bytes", len(content))

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: content}},
	}, nil, nil
}

// readFileFromRepo reads a file from a bare git repository at HEAD.
func readFileFromRepo(barePath, filePath string) (string, error) {
	repo, err := git.PlainOpen(barePath)
	if err != nil {
		return "", fmt.Errorf("open repo %s: %w", barePath, err)
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("get HEAD: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", fmt.Errorf("get commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("get tree: %w", err)
	}
	file, err := tree.File(filePath)
	if err != nil {
		return "", fmt.Errorf("file not found: %s: %w", filePath, err)
	}
	return file.Contents()
}

// sliceLines extracts a window of lines from content.
// offset skips the first N lines; limit caps how many lines to return.
func sliceLines(content string, offset, limit int) string {
	lines := strings.SplitAfter(content, "\n")
	if offset > 0 {
		if offset >= len(lines) {
			return ""
		}
		lines = lines[offset:]
	}
	if limit > 0 && limit < len(lines) {
		lines = lines[:limit]
	}
	return strings.Join(lines, "")
}

// --- JSON formatters ---

// repoBranch is one indexed branch and the commit it was indexed at.
type repoBranch struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// repoDetail is a repository plus the metadata needed to tell whether
// anything indexed from it could have changed. Version is the commit the
// branch was indexed at, so a caller holding results derived from this
// repo can compare one SHA instead of re-reading files.
type repoDetail struct {
	Name             string       `json:"name"`
	Branches         []repoBranch `json:"branches"`
	LatestCommitDate time.Time    `json:"latest_commit_date"`
	IndexTime        time.Time    `json:"index_time"`
}

// formatRepoList renders a List response. With detail, each repo carries
// its indexed branches and timestamps; without, it stays the sorted list
// of names that callers of output_mode=repos already parse.
func formatRepoList(repoList *zoekt.RepoList, detail bool) string {
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

	details := make([]repoDetail, 0, len(repoList.Repos))
	for _, r := range repoList.Repos {
		branches := make([]repoBranch, 0, len(r.Repository.Branches))
		for _, b := range r.Repository.Branches {
			branches = append(branches, repoBranch{Name: b.Name, Version: b.Version})
		}
		details = append(details, repoDetail{
			Name:             r.Repository.Name,
			Branches:         branches,
			LatestCommitDate: r.Repository.LatestCommitDate,
			IndexTime:        r.IndexMetadata.IndexTime,
		})
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Name < details[j].Name })
	meta["results"] = details
	b, _ := json.Marshal(meta)
	return string(b)
}

// --- JSON formatter ---

func formatSearchResult(result *zoekt.SearchResult, outputMode string) string {
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

	// lines mode
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
