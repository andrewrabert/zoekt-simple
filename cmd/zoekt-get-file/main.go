package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	var (
		offset int
		limit  int
		baseURL string
	)
	flag.IntVar(&offset, "offset", 0, "Skip first N lines")
	flag.IntVar(&limit, "limit", 0, "Return only N lines (0 = all)")
	flag.StringVar(&baseURL, "zoekt-url", os.Getenv("ZOEKT_URL"), "Zoekt server URL (required, or set ZOEKT_URL)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <repo> <path>\n\n  repo: full repository name (e.g. github.com/org/repo)\n  path: file path within the repository\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "error: zoekt-url is required (set via -zoekt-url flag or ZOEKT_URL env var)")
		os.Exit(2)
	}
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}
	repo := flag.Arg(0)
	path := flag.Arg(1)

	content, err := getFile(baseURL, repo, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if offset > 0 || limit > 0 {
		lines := strings.SplitAfter(content, "\n")
		if offset > 0 {
			if offset >= len(lines) {
				lines = nil
			} else {
				lines = lines[offset:]
			}
		}
		if limit > 0 && limit < len(lines) {
			lines = lines[:limit]
		}
		content = strings.Join(lines, "")
	}

	fmt.Print(content)
}

func getFile(baseURL, repo, filePath string) (string, error) {
	u := strings.TrimRight(baseURL, "/") + "/api/file?repo=" + url.QueryEscape(repo) + "&path=" + url.QueryEscape(filePath)
	resp, err := http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return string(body), nil
}
