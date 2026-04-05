package cli

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/zoekt/gitindex"
	"github.com/spf13/cobra"
)

func newGitCloneCmd() *cobra.Command {
	var (
		dest     string
		name     string
		destName string
	)

	cmd := &cobra.Command{
		Use:   "git-clone [flags] <url>",
		Short: "Clone a git repository as a bare repo",
		Long:  "Clone a git repository as a bare repo, extending zoekt-git-clone with a --dest-name flag that controls the filesystem directory name independently from --name (which sets zoekt.name).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitClone(args[0], dest, name, destName)
		},
	}

	cmd.Flags().StringVar(&dest, "dest", "", "destination directory (required)")
	cmd.Flags().StringVar(&name, "name", "", "name of repository (sets zoekt.name)")
	cmd.Flags().StringVar(&destName, "dest-name", "", "directory name for the bare repo under --dest (overrides --name for filesystem path)")
	cmd.MarkFlagRequired("dest")

	return cmd
}

func runGitClone(repoURL, dest, name, destName string) error {
	u, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("url.Parse: %w", err)
	}

	if name == "" {
		name = filepath.Join(u.Host, u.Path)
		name = strings.TrimSuffix(name, ".git")
	}

	dirName := name
	if destName != "" {
		dirName = destName
	}

	destDir := filepath.Dir(filepath.Join(dest, dirName))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	config := map[string]string{
		"zoekt.name": name,
	}

	destRepo, err := gitindex.CloneRepo(destDir, filepath.Base(dirName), u.String(), config)
	if err != nil {
		return fmt.Errorf("CloneRepo: %w", err)
	}
	if destRepo != "" {
		fmt.Println(destRepo)
	}
	return nil
}

// RunGitClone is exported so the legacy zoekt-simple-git-clone binary can delegate to it.
func RunGitClone(args []string) {
	cmd := newGitCloneCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
