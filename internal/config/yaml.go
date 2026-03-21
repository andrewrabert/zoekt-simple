package config

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type YAMLConfig struct {
	Listen         string        `yaml:"listen"`
	DataDir        string        `yaml:"data_dir"`
	IndexDir       string        `yaml:"index_dir"`
	FetchInterval  time.Duration `yaml:"fetch_interval"`
	MirrorInterval time.Duration `yaml:"mirror_interval"`
	IndexTimeout   time.Duration `yaml:"index_timeout"`
	CPUFraction    float64       `yaml:"cpu_fraction"`
	MaxLogAge      time.Duration `yaml:"max_log_age"`
	Instructions   string        `yaml:"instructions"`
	InstrFile      string        `yaml:"instructions_file"`
	Mirrors        []MirrorEntry `yaml:"mirrors"`
}

type MirrorEntry struct {
	GitHub          *GitHubMirror          `yaml:"github"`
	GitLab          *GitLabMirror          `yaml:"gitlab"`
	Gitea           *GiteaMirror           `yaml:"gitea"`
	Gerrit          *GerritMirror          `yaml:"gerrit"`
	BitbucketServer *BitbucketServerMirror `yaml:"bitbucket_server"`
	Gitiles         *GitilesMirror         `yaml:"gitiles"`
	CGit            *CGitMirror            `yaml:"cgit"`
}

type GitHubMirror struct {
	Org          string   `yaml:"org"`
	User         string   `yaml:"user"`
	Orgs         []string `yaml:"orgs"`
	Users        []string `yaml:"users"`
	URL          string   `yaml:"url"`
	Token        string   `yaml:"token"`
	Name         string   `yaml:"name"`
	ExcludeRepos string   `yaml:"exclude_repos"`
	Topics       []string `yaml:"topics"`
	ExcludeTopics []string `yaml:"exclude_topics"`
	Visibility   []string `yaml:"visibility"`
	Archived     bool     `yaml:"archived"`
	Forks        bool     `yaml:"forks"`
	KeepDeleted  bool     `yaml:"keep_deleted"`
	DiscoverOrgs bool     `yaml:"discover_orgs"`
	Include      []string `yaml:"include"`
	Exclude      []string `yaml:"exclude"`
}

type GitLabMirror struct {
	URL              string `yaml:"url"`
	Token            string `yaml:"token"`
	Name             string `yaml:"name"`
	ExcludeRepos     string `yaml:"exclude_repos"`
	OnlyPublic       bool   `yaml:"only_public"`
	ExcludeUserRepos bool   `yaml:"exclude_user_repos"`
	Archived         bool   `yaml:"archived"`
	KeepDeleted      bool   `yaml:"keep_deleted"`
}

type GiteaMirror struct {
	URL           string   `yaml:"url"`
	Org           string   `yaml:"org"`
	User          string   `yaml:"user"`
	Token         string   `yaml:"token"`
	Name          string   `yaml:"name"`
	ExcludeRepos  string   `yaml:"exclude_repos"`
	Topics        []string `yaml:"topics"`
	ExcludeTopics []string `yaml:"exclude_topics"`
	Archived      bool     `yaml:"archived"`
	Forks         bool     `yaml:"forks"`
	KeepDeleted   bool     `yaml:"keep_deleted"`
}

type GerritMirror struct {
	URL             string `yaml:"url"`
	Credentials     string `yaml:"credentials"`
	Name            string `yaml:"name"`
	Exclude         string `yaml:"exclude"`
	Active          bool   `yaml:"active"`
	FetchMetaConfig bool   `yaml:"fetch_meta_config"`
	RepoNameFormat  string `yaml:"repo_name_format"`
	KeepDeleted     bool   `yaml:"keep_deleted"`
}

type BitbucketServerMirror struct {
	URL         string `yaml:"url"`
	Project     string `yaml:"project"`
	Credentials string `yaml:"credentials"`
	ProjectType string `yaml:"project_type"`
	DisableTLS  bool   `yaml:"disable_tls"`
	Name        string `yaml:"name"`
	Exclude     string `yaml:"exclude"`
	KeepDeleted bool   `yaml:"keep_deleted"`
}

type GitilesMirror struct {
	URL     string `yaml:"url"`
	Name    string `yaml:"name"`
	Exclude string `yaml:"exclude"`
}

type CGitMirror struct {
	URL     string `yaml:"url"`
	Name    string `yaml:"name"`
	Exclude string `yaml:"exclude"`
}

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

func expandEnv(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-1]
		if idx := strings.Index(inner, ":-"); idx >= 0 {
			name := inner[:idx]
			fallback := inner[idx+2:]
			if v, ok := os.LookupEnv(name); ok {
				return v
			}
			return fallback
		}
		return os.Getenv(inner)
	})
}

func resolveCredential(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	s = expandEnv(s)
	if strings.HasPrefix(s, "file://") {
		path := strings.TrimPrefix(s, "file://")
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read credential file %s: %w", path, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return s, nil
}

// LoadYAMLConfig reads a YAML config file, expands environment variables
// in the raw YAML, and unmarshals into a YAMLConfig.
func LoadYAMLConfig(path string) (*YAMLConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	expanded := expandEnv(string(data))
	var cfg YAMLConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

// writeCredentialFile resolves a credential string and writes it to a temp file.
// Returns the path to the temp file, or empty string if the credential is empty.
func writeCredentialFile(cred string, tmpFiles *[]string) (string, error) {
	resolved, err := resolveCredential(cred)
	if err != nil {
		return "", err
	}
	if resolved == "" {
		return "", nil
	}
	f, err := os.CreateTemp("", "zoekt-cred-*")
	if err != nil {
		return "", fmt.Errorf("create temp credential file: %w", err)
	}
	if _, err := f.WriteString(resolved); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("write temp credential file: %w", err)
	}
	f.Close()
	*tmpFiles = append(*tmpFiles, f.Name())
	return f.Name(), nil
}

// discoverGitHubOrgs fetches the list of orgs the authenticated user belongs to
// from the GitHub API. It handles pagination and supports both github.com and
// GitHub Enterprise instances.
func discoverGitHubOrgs(token, ghURL string) ([]string, error) {
	apiBase := "https://api.github.com"
	if ghURL != "" {
		apiBase = strings.TrimRight(ghURL, "/") + "/api/v3"
	}

	resolved, err := resolveCredential(token)
	if err != nil {
		return nil, fmt.Errorf("resolve token: %w", err)
	}
	if resolved == "" {
		return nil, fmt.Errorf("token is required for discover_orgs")
	}

	var orgs []string
	page := 1
	for {
		url := fmt.Sprintf("%s/user/orgs?per_page=100&page=%d", apiBase, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "token "+resolved)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("GET %s: %w", url, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
		}

		var pageOrgs []struct {
			Login string `json:"login"`
		}
		if err := json.Unmarshal(body, &pageOrgs); err != nil {
			return nil, fmt.Errorf("parse orgs response: %w", err)
		}
		if len(pageOrgs) == 0 {
			break
		}
		for _, o := range pageOrgs {
			orgs = append(orgs, o.Login)
		}
		page++
	}
	return orgs, nil
}

// NetrcEntry represents a single machine entry for a .netrc file.
type NetrcEntry struct {
	Machine  string
	Login    string
	Password string
}

// NetrcEntries extracts host/token pairs from mirror entries for writing a
// .netrc file. This is needed because upstream zoekt mirror commands use the
// token for API calls, but git clone/fetch needs .netrc for HTTPS auth.
func NetrcEntries(mirrors []MirrorEntry) ([]NetrcEntry, error) {
	seen := make(map[string]bool)
	var entries []NetrcEntry

	add := func(host, token string) error {
		if token == "" || seen[host] {
			return nil
		}
		resolved, err := resolveCredential(token)
		if err != nil {
			return err
		}
		if resolved == "" {
			return nil
		}
		seen[host] = true
		entries = append(entries, NetrcEntry{Machine: host, Login: "x-token-auth", Password: resolved})
		return nil
	}

	for _, m := range mirrors {
		switch {
		case m.GitHub != nil:
			host := "github.com"
			if m.GitHub.URL != "" {
				h, err := hostFromURL(m.GitHub.URL)
				if err != nil {
					return nil, err
				}
				host = h
			}
			if err := add(host, m.GitHub.Token); err != nil {
				return nil, err
			}
		case m.GitLab != nil:
			if m.GitLab.URL != "" && m.GitLab.Token != "" {
				h, err := hostFromURL(m.GitLab.URL)
				if err != nil {
					return nil, err
				}
				if err := add(h, m.GitLab.Token); err != nil {
					return nil, err
				}
			}
		case m.Gitea != nil:
			if m.Gitea.URL != "" && m.Gitea.Token != "" {
				h, err := hostFromURL(m.Gitea.URL)
				if err != nil {
					return nil, err
				}
				if err := add(h, m.Gitea.Token); err != nil {
					return nil, err
				}
			}
		}
	}
	return entries, nil
}

func hostFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url %q: %w", rawURL, err)
	}
	return u.Hostname(), nil
}

// WriteNetrc writes a .netrc file from the given entries.
func WriteNetrc(path string, entries []NetrcEntry) error {
	var buf strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&buf, "machine %s login %s password %s\n", e.Machine, e.Login, e.Password)
	}
	return os.WriteFile(path, []byte(buf.String()), 0o600)
}

// ConvertMirrors converts YAML mirror entries into upstream ConfigEntry values.
// It returns the entries, a cleanup function that removes temporary credential
// files, and any error encountered during conversion.
func ConvertMirrors(mirrors []MirrorEntry) ([]ConfigEntry, func(), error) {
	var entries []ConfigEntry
	var tmpFiles []string

	cleanup := func() {
		for _, f := range tmpFiles {
			os.Remove(f)
		}
	}

	for _, m := range mirrors {
		var e ConfigEntry
		var err error

		switch {
		case m.GitHub != nil:
			g := m.GitHub

			credPath, err := writeCredentialFile(g.Token, &tmpFiles)
			if err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("github mirror credential: %w", err)
			}

			// Collect all orgs and users from both singular and plural fields.
			allOrgs := g.Orgs
			if g.Org != "" {
				allOrgs = append(allOrgs, g.Org)
			}
			allUsers := g.Users
			if g.User != "" {
				allUsers = append(allUsers, g.User)
			}

			// If discover_orgs, fetch from API and merge.
			if g.DiscoverOrgs {
				discovered, discoverErr := discoverGitHubOrgs(g.Token, g.URL)
				if discoverErr != nil {
					cleanup()
					return nil, nil, fmt.Errorf("github discover_orgs: %w", discoverErr)
				}
				allOrgs = append(allOrgs, discovered...)
			}

			// Apply include/exclude filtering to both orgs and users.
			includeSet := make(map[string]bool, len(g.Include))
			for _, inc := range g.Include {
				includeSet[inc] = true
			}
			excludeSet := make(map[string]bool, len(g.Exclude))
			for _, ex := range g.Exclude {
				excludeSet[ex] = true
			}

			for _, org := range allOrgs {
				if len(includeSet) > 0 && !includeSet[org] {
					slog.Info("github mirror: skipping org not in include list", "org", org)
					continue
				}
				if excludeSet[org] {
					slog.Info("github mirror: excluding org", "org", org)
					continue
				}
				entries = append(entries, ConfigEntry{
					GithubOrg:      org,
					GitHubURL:      g.URL,
					Name:           g.Name,
					Exclude:        g.ExcludeRepos,
					Topics:         g.Topics,
					ExcludeTopics:  g.ExcludeTopics,
					Visibility:     g.Visibility,
					NoArchived:     !g.Archived,
					Forks:          g.Forks,
					KeepDeleted:    g.KeepDeleted,
					CredentialPath: credPath,
				})
			}
			for _, user := range allUsers {
				if len(includeSet) > 0 && !includeSet[user] {
					slog.Info("github mirror: skipping user not in include list", "user", user)
					continue
				}
				if excludeSet[user] {
					slog.Info("github mirror: excluding user", "user", user)
					continue
				}
				entries = append(entries, ConfigEntry{
					GithubUser:     user,
					GitHubURL:      g.URL,
					Name:           g.Name,
					Exclude:        g.ExcludeRepos,
					Topics:         g.Topics,
					ExcludeTopics:  g.ExcludeTopics,
					Visibility:     g.Visibility,
					NoArchived:     !g.Archived,
					Forks:          g.Forks,
					KeepDeleted:    g.KeepDeleted,
					CredentialPath: credPath,
				})
			}
			continue

		case m.GitLab != nil:
			g := m.GitLab
			e.GitLabURL = g.URL
			e.Name = g.Name
			e.Exclude = g.ExcludeRepos
			e.OnlyPublic = g.OnlyPublic
			e.ExcludeUserRepos = g.ExcludeUserRepos
			e.NoArchived = !g.Archived
			e.KeepDeleted = g.KeepDeleted
			e.CredentialPath, err = writeCredentialFile(g.Token, &tmpFiles)
			if err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("gitlab mirror %q: %w", g.URL, err)
			}

		case m.Gitea != nil:
			g := m.Gitea
			e.GiteaURL = g.URL
			e.GiteaOrg = g.Org
			e.GiteaUser = g.User
			e.Name = g.Name
			e.Exclude = g.ExcludeRepos
			e.Topics = g.Topics
			e.ExcludeTopics = g.ExcludeTopics
			e.NoArchived = !g.Archived
			e.Forks = g.Forks
			e.KeepDeleted = g.KeepDeleted
			e.CredentialPath, err = writeCredentialFile(g.Token, &tmpFiles)
			if err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("gitea mirror %q: %w", g.URL, err)
			}

		case m.Gerrit != nil:
			g := m.Gerrit
			e.GerritApiURL = g.URL
			e.Name = g.Name
			e.Exclude = g.Exclude
			e.Active = g.Active
			e.GerritFetchMetaConfig = g.FetchMetaConfig
			e.GerritRepoNameFormat = g.RepoNameFormat
			e.KeepDeleted = g.KeepDeleted
			e.CredentialPath, err = writeCredentialFile(g.Credentials, &tmpFiles)
			if err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("gerrit mirror %q: %w", g.URL, err)
			}

		case m.BitbucketServer != nil:
			g := m.BitbucketServer
			e.BitBucketServerURL = g.URL
			e.BitBucketServerProject = g.Project
			e.ProjectType = g.ProjectType
			e.DisableTLS = g.DisableTLS
			e.Name = g.Name
			e.Exclude = g.Exclude
			e.KeepDeleted = g.KeepDeleted
			e.CredentialPath, err = writeCredentialFile(g.Credentials, &tmpFiles)
			if err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("bitbucket server mirror %q: %w", g.URL, err)
			}

		case m.Gitiles != nil:
			g := m.Gitiles
			e.GitilesURL = g.URL
			e.Name = g.Name
			e.Exclude = g.Exclude

		case m.CGit != nil:
			g := m.CGit
			e.CGitURL = g.URL
			e.Name = g.Name
			e.Exclude = g.Exclude

		default:
			continue
		}

		entries = append(entries, e)
	}

	return entries, cleanup, nil
}
