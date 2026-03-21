package config

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_TOKEN", "secret123")
	got := expandEnv("${TEST_TOKEN}")
	if got != "secret123" {
		t.Fatalf("expected secret123, got %q", got)
	}
}

func TestExpandEnvDefault(t *testing.T) {
	os.Unsetenv("MISSING_VAR")
	got := expandEnv("${MISSING_VAR:-fallback}")
	if got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestExpandEnvNoMatch(t *testing.T) {
	got := expandEnv("literal-value")
	if got != "literal-value" {
		t.Fatalf("expected literal-value, got %q", got)
	}
}

func TestResolveCredentialEnvVar(t *testing.T) {
	t.Setenv("MY_TOKEN", "tok123")
	got, err := resolveCredential("${MY_TOKEN}")
	if err != nil {
		t.Fatal(err)
	}
	if got != "tok123" {
		t.Fatalf("expected tok123, got %q", got)
	}
}

func TestResolveCredentialFilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	os.WriteFile(path, []byte("file-token\n"), 0o644)

	got, err := resolveCredential("file://" + path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-token" {
		t.Fatalf("expected file-token, got %q", got)
	}
}

func TestResolveCredentialLiteral(t *testing.T) {
	got, err := resolveCredential("ghp_literal")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ghp_literal" {
		t.Fatalf("expected ghp_literal, got %q", got)
	}
}

func TestResolveCredentialEmpty(t *testing.T) {
	got, err := resolveCredential("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestConvertGitHubMirror(t *testing.T) {
	t.Setenv("GH_TOKEN", "tok")
	entries, cleanup, err := ConvertMirrors([]MirrorEntry{
		{GitHub: &GitHubMirror{
			Org:        "myorg",
			Token:      "${GH_TOKEN}",
			Archived: false,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.GithubOrg != "myorg" {
		t.Fatalf("expected myorg, got %s", e.GithubOrg)
	}
	if !e.NoArchived {
		t.Fatal("expected NoArchived=true when archived=false")
	}
	if e.CredentialPath == "" {
		t.Fatal("expected CredentialPath to be set")
	}
	data, err := os.ReadFile(e.CredentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "tok" {
		t.Fatalf("expected tok, got %s", string(data))
	}
}

func TestConvertGitLabMirror(t *testing.T) {
	entries, cleanup, err := ConvertMirrors([]MirrorEntry{
		{GitLab: &GitLabMirror{
			URL:              "https://gitlab.example.com/api/v4/",
			Token:            "glpat-test",
			ExcludeUserRepos: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.GitLabURL != "https://gitlab.example.com/api/v4/" {
		t.Fatalf("unexpected GitLabURL: %s", e.GitLabURL)
	}
	if !e.ExcludeUserRepos {
		t.Fatal("expected ExcludeUserRepos=true")
	}
}

func TestConvertMultipleMirrors(t *testing.T) {
	entries, cleanup, err := ConvertMirrors([]MirrorEntry{
		{GitHub: &GitHubMirror{Org: "org1"}},
		{Gitiles: &GitilesMirror{URL: "https://example.com"}},
		{CGit: &CGitMirror{URL: "https://cgit.example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestConvertGitHubDiscoverOrgs(t *testing.T) {
	orgs := []struct {
		Login string `json:"login"`
	}{
		{Login: "org-a"},
		{Login: "org-b"},
		{Login: "org-c"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v3/user/orgs" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			w.Write([]byte("[]"))
			return
		}
		json.NewEncoder(w).Encode(orgs)
	}))
	defer srv.Close()

	entries, cleanup, err := ConvertMirrors([]MirrorEntry{
		{GitHub: &GitHubMirror{
			DiscoverOrgs: true,
			URL:          srv.URL,
			Token:        "test-token",
			Archived:     false,
			Exclude:      []string{"org-b"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].GithubOrg != "org-a" {
		t.Fatalf("expected org-a, got %s", entries[0].GithubOrg)
	}
	if entries[1].GithubOrg != "org-c" {
		t.Fatalf("expected org-c, got %s", entries[1].GithubOrg)
	}
	if !entries[0].NoArchived {
		t.Fatal("expected NoArchived=true when archived=false on discovered org")
	}
}

func TestConvertGitHubDiscoverOrgsInclude(t *testing.T) {
	orgs := []struct {
		Login string `json:"login"`
	}{
		{Login: "team-alpha"},
		{Login: "team-beta"},
		{Login: "other-org"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.Write([]byte("[]"))
			return
		}
		json.NewEncoder(w).Encode(orgs)
	}))
	defer srv.Close()

	entries, cleanup, err := ConvertMirrors([]MirrorEntry{
		{GitHub: &GitHubMirror{
			DiscoverOrgs: true,
			URL:          srv.URL,
			Token:        "test-token",
			Include:      []string{"team-alpha", "team-beta"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].GithubOrg != "team-alpha" {
		t.Fatalf("expected team-alpha, got %s", entries[0].GithubOrg)
	}
	if entries[1].GithubOrg != "team-beta" {
		t.Fatalf("expected team-beta, got %s", entries[1].GithubOrg)
	}
}

func TestConvertGitHubPluralOrgsAndUsers(t *testing.T) {
	entries, cleanup, err := ConvertMirrors([]MirrorEntry{
		{GitHub: &GitHubMirror{
			Orgs:  []string{"jellyfin", "jellyfin-labs"},
			Users: []string{"andrewrabert"},
			Token: "tok",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].GithubOrg != "jellyfin" {
		t.Fatalf("expected jellyfin, got %s", entries[0].GithubOrg)
	}
	if entries[1].GithubOrg != "jellyfin-labs" {
		t.Fatalf("expected jellyfin-labs, got %s", entries[1].GithubOrg)
	}
	if entries[2].GithubUser != "andrewrabert" {
		t.Fatalf("expected andrewrabert, got %s", entries[2].GithubUser)
	}
}

func TestConvertGitHubSingularAndPluralMerge(t *testing.T) {
	entries, cleanup, err := ConvertMirrors([]MirrorEntry{
		{GitHub: &GitHubMirror{
			Org:   "org1",
			Orgs:  []string{"org2"},
			User:  "user1",
			Token: "tok",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].GithubOrg != "org2" {
		t.Fatalf("expected org2, got %s", entries[0].GithubOrg)
	}
	if entries[1].GithubOrg != "org1" {
		t.Fatalf("expected org1, got %s", entries[1].GithubOrg)
	}
	if entries[2].GithubUser != "user1" {
		t.Fatalf("expected user1, got %s", entries[2].GithubUser)
	}
}

func TestLoadYAMLConfig(t *testing.T) {
	t.Setenv("TOK", "mytoken")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
listen: ":9000"
data_dir: /mydata
fetch_interval: 30m
mirrors:
  - github:
      org: testorg
      token: "${TOK}"
      archived: false
`), 0o644)

	cfg, err := LoadYAMLConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9000" {
		t.Fatalf("expected :9000, got %s", cfg.Listen)
	}
	if cfg.DataDir != "/mydata" {
		t.Fatalf("expected /mydata, got %s", cfg.DataDir)
	}
	if cfg.FetchInterval != 30*time.Minute {
		t.Fatalf("expected 30m, got %s", cfg.FetchInterval)
	}
	if len(cfg.Mirrors) != 1 {
		t.Fatalf("expected 1 mirror, got %d", len(cfg.Mirrors))
	}
	if cfg.Mirrors[0].GitHub.Org != "testorg" {
		t.Fatalf("expected testorg, got %s", cfg.Mirrors[0].GitHub.Org)
	}
}

func TestNetrcEntries(t *testing.T) {
	t.Setenv("GH_TOKEN", "ghp_abc")
	t.Setenv("GL_TOKEN", "glpat-xyz")

	entries, err := NetrcEntries([]MirrorEntry{
		{GitHub: &GitHubMirror{Org: "org1", Token: "${GH_TOKEN}"}},
		{GitHub: &GitHubMirror{Org: "org2", Token: "${GH_TOKEN}"}},
		{GitHub: &GitHubMirror{Org: "ghe-org", URL: "https://ghe.example.com", Token: "${GH_TOKEN}"}},
		{GitLab: &GitLabMirror{URL: "https://gitlab.example.com/api/v4/", Token: "${GL_TOKEN}"}},
		{Gitiles: &GitilesMirror{URL: "https://android.googlesource.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Machine != "github.com" || entries[0].Password != "ghp_abc" {
		t.Fatalf("unexpected entry[0]: %+v", entries[0])
	}
	if entries[1].Machine != "ghe.example.com" || entries[1].Password != "ghp_abc" {
		t.Fatalf("unexpected entry[1]: %+v", entries[1])
	}
	if entries[2].Machine != "gitlab.example.com" || entries[2].Password != "glpat-xyz" {
		t.Fatalf("unexpected entry[2]: %+v", entries[2])
	}
}

func TestWriteNetrc(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".netrc")
	err := WriteNetrc(path, []NetrcEntry{
		{Machine: "github.com", Login: "x-token-auth", Password: "tok123"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := "machine github.com login x-token-auth password tok123\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}
