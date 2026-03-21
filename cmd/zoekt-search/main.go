package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/sourcegraph/zoekt/query"
)

// Response types

type searchResponse struct {
	Result searchResult `json:"Result"`
}

type searchResult struct {
	FileCount int         `json:"FileCount"`
	Files     []fileMatch `json:"Files"`
}

type fileMatch struct {
	Repository  string      `json:"Repository"`
	FileName    string      `json:"FileName"`
	LineMatches []lineMatch `json:"LineMatches"`
}

type lineMatch struct {
	LineNumber    int            `json:"LineNumber"`
	Line          string         `json:"Line"`
	Before        string         `json:"Before"`
	After         string         `json:"After"`
	LineFragments []lineFragment `json:"LineFragments"`
}

type lineFragment struct {
	LineOffset  int `json:"LineOffset"`
	MatchLength int `json:"MatchLength"`
}

// ANSI colors

const (
	ansiMagenta = "\033[35m"
	ansiGreen   = "\033[32m"
	ansiBoldRed = "\033[1;31m"
	ansiReset   = "\033[0m"
)

// Helpers

func decodeBase64(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return s
	}
	return strings.TrimRight(string(b), "\n")
}

func decodeContext(s string) []string {
	if s == "" {
		return nil
	}
	decoded := decodeBase64(s)
	if decoded == "" {
		return nil
	}
	return strings.Split(decoded, "\n")
}

func isRepoOnlyQuery(queryStr string) bool {
	q, err := query.Parse(queryStr)
	if err != nil {
		return false
	}
	repoOnly := true
	query.VisitAtoms(q, func(q query.Q) {
		if _, ok := q.(*query.Repo); !ok {
			repoOnly = false
		}
	})
	return repoOnly
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

// List API

type listResponse struct {
	List struct {
		Repos []listRepoEntry `json:"Repos"`
	} `json:"List"`
}

type listRepoEntry struct {
	Repository struct {
		Name string `json:"Name"`
	} `json:"Repository"`
}

func listRepos(baseURL, query string) (*listResponse, error) {
	body := map[string]any{"Q": query}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	u := strings.TrimRight(baseURL, "/") + "/api/list"
	resp, err := httpClient.Post(u, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
	}

	var result listResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Search API

func search(baseURL, query string, maxResults, contextLines int) (*searchResponse, error) {
	body := map[string]any{
		"Q": query,
		"Opts": map[string]any{
			"MaxDocDisplayCount": maxResults,
			"NumContextLines":   contextLines,
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	u := strings.TrimRight(baseURL, "/") + "/api/search"
	resp, err := httpClient.Post(u, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Formatters

func uniqueRepos(files []fileMatch) []string {
	seen := make(map[string]bool)
	for _, f := range files {
		seen[f.Repository] = true
	}
	repos := make([]string, 0, len(seen))
	for r := range seen {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	return repos
}

func uniquePaths(files []fileMatch) []string {
	seen := make(map[string]bool)
	for _, f := range files {
		seen[f.Repository+"/"+f.FileName] = true
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func formatPlain(resp *searchResponse, outputMode string) string {
	if outputMode == "repos" {
		return strings.Join(uniqueRepos(resp.Result.Files), "\n")
	}
	return strings.Join(uniquePaths(resp.Result.Files), "\n")
}

func formatJSON(resp *searchResponse, outputMode string) string {
	files := resp.Result.Files
	total := resp.Result.FileCount
	if total == 0 {
		total = len(files)
	}

	meta := map[string]any{
		"total_matches": total,
		"returned":      len(files),
		"truncated":     total > len(files),
	}

	if outputMode == "repos" {
		meta["results"] = uniqueRepos(files)
		b, _ := json.Marshal(meta)
		return string(b)
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

	// lines mode: group consecutive lines into ranges
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
			pe.lines = append(pe.lines, ld{m.LineNumber, decodeBase64(m.Line)})
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

type ripgrepOpts struct {
	heading        bool
	contextLines   int
	showColumn     bool
	showLineNumber bool
	useColor       bool
	vimgrep        bool
	filesOnly      bool
}

func formatRipgrep(resp *searchResponse, opts ripgrepOpts) string {
	files := resp.Result.Files
	useHeading := opts.heading && !opts.vimgrep

	styled := func(content, style string) string {
		if !opts.useColor {
			return content
		}
		var code string
		switch style {
		case "magenta":
			code = ansiMagenta
		case "green":
			code = ansiGreen
		case "bold red":
			code = ansiBoldRed
		default:
			return content
		}
		return code + content + ansiReset
	}

	if opts.filesOnly {
		var paths []string
		for _, f := range files {
			paths = append(paths, styled(f.Repository+"/"+f.FileName, "magenta"))
		}
		return strings.Join(paths, "\n")
	}

	highlightLine := func(lineText string, m lineMatch) string {
		if !opts.useColor {
			return lineText
		}
		frags := make([]lineFragment, len(m.LineFragments))
		copy(frags, m.LineFragments)
		sort.Slice(frags, func(i, j int) bool {
			return frags[i].LineOffset < frags[j].LineOffset
		})

		var buf strings.Builder
		pos := 0
		for _, frag := range frags {
			if frag.LineOffset > pos {
				buf.WriteString(lineText[pos:frag.LineOffset])
			}
			buf.WriteString(ansiBoldRed)
			end := frag.LineOffset + frag.MatchLength
			if end > len(lineText) {
				end = len(lineText)
			}
			buf.WriteString(lineText[frag.LineOffset:end])
			buf.WriteString(ansiReset)
			pos = end
		}
		if pos < len(lineText) {
			buf.WriteString(lineText[pos:])
		}
		return buf.String()
	}

	getColumn := func(m lineMatch) int {
		if len(m.LineFragments) > 0 {
			return m.LineFragments[0].LineOffset + 1
		}
		return 1
	}

	fmtCtxLine := func(pathStyled string, lineNum int, text string) string {
		ln := styled(fmt.Sprintf("%d", lineNum), "green")
		if useHeading {
			if opts.showLineNumber {
				return ln + "-" + text
			}
			return text
		}
		if opts.showLineNumber {
			return pathStyled + "-" + ln + "-" + text
		}
		return pathStyled + "-" + text
	}

	fmtMatchLine := func(pathStyled string, lineNum, colNum int, highlighted string) string {
		var parts []string
		if !useHeading {
			parts = append(parts, pathStyled)
		}
		if opts.showLineNumber && opts.showColumn {
			parts = append(parts, styled(fmt.Sprintf("%d:%d", lineNum, colNum), "green"))
		} else if opts.showLineNumber {
			parts = append(parts, styled(fmt.Sprintf("%d", lineNum), "green"))
		} else if opts.showColumn {
			parts = append(parts, styled(fmt.Sprintf("%d", colNum), "green"))
		}
		if len(parts) > 0 {
			return strings.Join(parts, ":") + ":" + highlighted
		}
		return highlighted
	}

	var output []string
	for fileIdx, f := range files {
		if len(f.LineMatches) == 0 {
			continue
		}

		path := f.Repository + "/" + f.FileName
		pathStyled := styled(path, "magenta")

		if useHeading {
			if fileIdx > 0 {
				output = append(output, "")
			}
			output = append(output, pathStyled)
		}

		matches := make([]lineMatch, len(f.LineMatches))
		copy(matches, f.LineMatches)
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].LineNumber < matches[j].LineNumber
		})

		var lastEndLine *int
		for _, m := range matches {
			lineNum := m.LineNumber
			colNum := 0
			if opts.showColumn {
				colNum = getColumn(m)
			}

			beforeLines := decodeContext(m.Before)
			afterLines := decodeContext(m.After)

			// Gap separator
			matchStart := lineNum - len(beforeLines)
			if opts.contextLines > 0 && lastEndLine != nil && matchStart > *lastEndLine+1 {
				output = append(output, "--")
			}

			// Before context
			for i, ctxLine := range beforeLines {
				ctxNum := lineNum - len(beforeLines) + i
				output = append(output, fmtCtxLine(pathStyled, ctxNum, ctxLine))
			}

			// Matched line
			lineText := decodeBase64(m.Line)
			highlighted := highlightLine(lineText, m)
			output = append(output, fmtMatchLine(pathStyled, lineNum, colNum, highlighted))

			// After context
			for i, ctxLine := range afterLines {
				ctxNum := lineNum + i + 1
				output = append(output, fmtCtxLine(pathStyled, ctxNum, ctxLine))
			}

			endLine := lineNum + len(afterLines)
			lastEndLine = &endLine
		}
	}

	return strings.Join(output, "\n")
}

func main() {
	var (
		maxResults       int
		jsonOutput       bool
		outputMode       string
		filesWithMatches bool
		contextLines     int
		column           bool
		vimgrep          bool
		headingMode      string
		noLineNumber     bool
		color            string
		url              string
	)

	flag.IntVar(&maxResults, "n", 50, "Maximum number of file matches")
	flag.IntVar(&maxResults, "max-results", 50, "Maximum number of file matches")
	flag.BoolVar(&jsonOutput, "json", false, "Output results as JSON")
	flag.StringVar(&outputMode, "output-mode", "lines", "Output format: lines, files, repos")
	flag.BoolVar(&filesWithMatches, "l", false, "Only print file paths with matches")
	flag.BoolVar(&filesWithMatches, "files-with-matches", false, "Only print file paths with matches")
	flag.IntVar(&contextLines, "C", 0, "Context lines around matches")
	flag.IntVar(&contextLines, "context", 0, "Context lines around matches")
	flag.BoolVar(&column, "column", false, "Show column numbers")
	flag.BoolVar(&vimgrep, "vimgrep", false, "file:line:column:content format")
	flag.StringVar(&headingMode, "heading", "auto", "Heading mode: auto, always, never")
	flag.BoolVar(&noLineNumber, "N", false, "Suppress line numbers")
	flag.StringVar(&color, "color", "auto", "Color output: auto, always, never")
	flag.StringVar(&url, "zoekt-url", os.Getenv("ZOEKT_URL"), "Zoekt server URL (required, or set ZOEKT_URL)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <query>\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nQuery syntax:")
		fmt.Fprintln(os.Stderr, "  Multiple terms are AND'd: \"class needle\" finds files with both")
		fmt.Fprintln(os.Stderr, "  r: or repo: - filter by repo (e.g. r:myrepo)")
		fmt.Fprintln(os.Stderr, "  f: or file: - filter by path (e.g. f:README.md)")
		fmt.Fprintln(os.Stderr, "  lang: - filter by language (e.g. lang:python)")
		fmt.Fprintln(os.Stderr, "  sym: - search symbol definitions (e.g. sym:MyClass)")
		fmt.Fprintln(os.Stderr, "  case:yes - case-sensitive search")
	}
	flag.Parse()

	if url == "" {
		fmt.Fprintln(os.Stderr, "error: zoekt-url is required (set via -zoekt-url flag or ZOEKT_URL env var)")
		os.Exit(2)
	}

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	query := strings.Join(flag.Args(), " ")
	lineNumber := !noLineNumber

	// Auto-detect repo-only queries (like the web UI does) and use the list endpoint
	if outputMode == "lines" && isRepoOnlyQuery(query) {
		outputMode = "repos"
	}

	if outputMode == "repos" && isRepoOnlyQuery(query) {
		listResp, err := listRepos(url, query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(2)
		}
		repos := make([]string, len(listResp.List.Repos))
		for i, r := range listResp.List.Repos {
			repos[i] = r.Repository.Name
		}
		sort.Strings(repos)
		if len(repos) == 0 {
			os.Exit(1)
		}
		if jsonOutput {
			b, _ := json.Marshal(map[string]any{
				"total_matches": len(repos),
				"returned":      len(repos),
				"truncated":     false,
				"results":       repos,
			})
			fmt.Println(string(b))
		} else {
			fmt.Println(strings.Join(repos, "\n"))
		}
		return
	}

	resp, err := search(url, query, maxResults, contextLines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	files := resp.Result.Files
	if len(files) == 0 {
		os.Exit(1)
	}

	total := resp.Result.FileCount
	if total == 0 {
		total = len(files)
	}

	var result string
	if jsonOutput {
		result = formatJSON(resp, outputMode)
	} else if outputMode == "repos" || outputMode == "files" {
		result = formatPlain(resp, outputMode)
	} else {
		useHeading := headingMode == "always" || (headingMode == "auto" && isTerminal() && !vimgrep)
		useColor := color == "always" || (color == "auto" && isTerminal() && os.Getenv("NO_COLOR") == "")

		result = formatRipgrep(resp, ripgrepOpts{
			heading:        useHeading,
			contextLines:   contextLines,
			showColumn:     column || vimgrep,
			showLineNumber: lineNumber,
			useColor:       useColor,
			vimgrep:        vimgrep,
			filesOnly:      filesWithMatches,
		})
	}

	fmt.Println(result)

	if total > len(files) {
		fmt.Fprintf(os.Stderr, "warning: results truncated (%d/%d files)\n", len(files), total)
		os.Exit(1)
	}
}
