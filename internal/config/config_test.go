package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `[{"GithubOrg": "testorg", "GitHubURL": "https://github.com", "CredentialPath": "/tmp/token"}]`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadConfigURL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].GithubOrg != "testorg" {
		t.Fatalf("expected testorg, got %s", entries[0].GithubOrg)
	}
}

func TestReadConfigInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadConfigURL(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGitMirrorDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		entry    ConfigEntry
		expected string
	}{
		{
			name:     "custom name",
			entry:    ConfigEntry{GitURL: "https://example.com/repo.git", Name: "fartmonger"},
			expected: "fartmonger",
		},
		{
			name:     "from URL",
			entry:    ConfigEntry{GitURL: "https://gitlab.freedesktop.org/mesa/mesa"},
			expected: "gitlab.freedesktop.org/mesa/mesa",
		},
		{
			name:     "from URL with .git suffix",
			entry:    ConfigEntry{GitURL: "https://example.com/org/repo.git"},
			expected: "example.com/org/repo",
		},
		{
			name:     "utf8 name",
			entry:    ConfigEntry{GitURL: "https://example.com/repo.git", Name: "café/projet"},
			expected: "café/projet",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gitMirrorDisplayName(tt.entry)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestCleanupGitMirrorRepos(t *testing.T) {
	repoDir := t.TempDir()
	gitDir := filepath.Join(repoDir, "git")
	os.MkdirAll(gitDir, 0o755)

	// Create two bare repos
	keepName := base64.RawURLEncoding.EncodeToString([]byte("example.com/keep"))
	staleName := base64.RawURLEncoding.EncodeToString([]byte("example.com/stale"))
	os.MkdirAll(filepath.Join(gitDir, keepName+".git"), 0o755)
	os.MkdirAll(filepath.Join(gitDir, staleName+".git"), 0o755)

	// Config only references "keep"
	cfg := []ConfigEntry{
		{GitURL: "https://example.com/keep"},
	}

	CleanupGitMirrorRepos(repoDir, cfg)

	// "keep" should still exist
	if _, err := os.Stat(filepath.Join(gitDir, keepName+".git")); err != nil {
		t.Fatalf("expected keep repo to exist: %v", err)
	}
	// "stale" should be deleted
	if _, err := os.Stat(filepath.Join(gitDir, staleName+".git")); !os.IsNotExist(err) {
		t.Fatal("expected stale repo to be deleted")
	}
}
