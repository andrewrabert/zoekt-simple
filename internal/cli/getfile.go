package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newGetFileCmd() *cobra.Command {
	var (
		offset   int
		limit    int
		zoektURL string
	)

	cmd := &cobra.Command{
		Use:   "get-file [flags] <repo> <path>",
		Short: "Retrieve a file from a zoekt server",
		Long: `Retrieve a file from a zoekt server by repository name and path.

  repo: full repository name (e.g. github.com/org/repo)
  path: file path within the repository`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetFile(zoektURL, args[0], args[1], offset, limit)
		},
	}

	cmd.Flags().IntVar(&offset, "offset", 0, "Skip first N lines")
	cmd.Flags().IntVar(&limit, "limit", 0, "Return only N lines (0 = all)")
	cmd.Flags().StringVar(&zoektURL, "zoekt-url", os.Getenv("ZOEKT_URL"), "Zoekt server URL (required, or set ZOEKT_URL)")

	return cmd
}

func runGetFile(baseURL, repo, path string, offset, limit int) error {
	if baseURL == "" {
		return fmt.Errorf("zoekt-url is required (set via --zoekt-url flag or ZOEKT_URL env var)")
	}

	content, err := getFile(baseURL, repo, path)
	if err != nil {
		return err
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
	return nil
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

// RunGetFile is exported so the legacy zoekt-get-file binary can delegate to it.
func RunGetFile(args []string) {
	cmd := newGetFileCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
