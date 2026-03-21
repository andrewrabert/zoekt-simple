package config

import (
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
