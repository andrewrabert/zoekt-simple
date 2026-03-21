// Copyright 2016 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type ConfigEntry struct {
	GithubUser             string
	GithubOrg              string
	BitBucketServerProject string
	GitHubURL              string
	GitilesURL             string
	CGitURL                string
	BitBucketServerURL     string
	GiteaURL               string
	GiteaUser              string
	GiteaOrg               string
	DisableTLS             bool
	CredentialPath         string
	ProjectType            string
	Name                   string
	Exclude                string
	GitLabURL              string
	OnlyPublic             bool
	GerritApiURL           string
	Topics                 []string
	ExcludeTopics          []string
	Active                 bool
	NoArchived             bool
	KeepDeleted            bool
	GerritFetchMetaConfig  bool
	GerritRepoNameFormat   string
	ExcludeUserRepos       bool
	Forks                  bool
	Visibility             []string
}

func Randomize(entries []ConfigEntry) []ConfigEntry {
	perm := rand.Perm(len(entries))

	var shuffled []ConfigEntry
	for _, i := range perm {
		shuffled = append(shuffled, entries[i])
	}

	return shuffled
}

func isHTTP(u string) bool {
	asURL, err := url.Parse(u)
	return err == nil && (asURL.Scheme == "http" || asURL.Scheme == "https")
}

// ReadConfigURL reads a config file from a local path or HTTP URL and returns
// the parsed config entries.
func ReadConfigURL(u string) ([]ConfigEntry, error) {
	var body []byte
	var readErr error

	if isHTTP(u) {
		rep, err := http.Get(u)
		if err != nil {
			return nil, err
		}
		defer rep.Body.Close()

		body, readErr = io.ReadAll(rep.Body)
	} else {
		body, readErr = os.ReadFile(u)
	}

	if readErr != nil {
		return nil, readErr
	}

	var result []ConfigEntry
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func watchFile(path string) (<-chan struct{}, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := watcher.Add(filepath.Dir(path)); err != nil {
		return nil, err
	}

	out := make(chan struct{}, 1)
	go func() {
		var last time.Time
		for {
			select {
			case <-watcher.Events:
				fi, err := os.Stat(path)
				if err == nil && fi.ModTime() != last {
					out <- struct{}{}
					last = fi.ModTime()
				}
			case err := <-watcher.Errors:
				if err != nil {
					slog.Error("watcher error", "error", err)
				}
			}
		}
	}()
	return out, nil
}

// PeriodicMirrorFile reads a mirror config from configFile on a schedule,
// executing mirrors and calling notify for each repo that was mirrored.
func PeriodicMirrorFile(ctx context.Context, repoDir, configFile string, interval time.Duration, notify func(repoDir string)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var watcher <-chan struct{}
	if !isHTTP(configFile) {
		var err error
		watcher, err = watchFile(configFile)
		if err != nil {
			slog.Error("watchFile", "path", configFile, "error", err)
		}
	}

	var lastCfg []ConfigEntry
	for {
		cfg, err := ReadConfigURL(configFile)
		if err != nil {
			slog.Error("readConfig", "path", configFile, "error", err)
		} else {
			lastCfg = cfg
		}

		ExecuteMirror(lastCfg, repoDir, notify)

		select {
		case <-ctx.Done():
			return
		case <-watcher:
			slog.Info("mirror config changed", "path", configFile)
		case <-ticker.C:
		}
	}
}

// LoggedRun runs a command and captures stdout/stderr, logging any errors.
func LoggedRun(cmd *exec.Cmd) (out, errBytes []byte) {
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.Stdout = outBuf
	cmd.Stderr = io.MultiWriter(errBuf, os.Stderr)

	slog.Info("run", "args", cmd.Args)
	if err := cmd.Run(); err != nil {
		slog.Error("command failed", "args", cmd.Args, "error", err, "stdout", outBuf.String(), "stderr", errBuf.String())
	}

	return outBuf.Bytes(), errBuf.Bytes()
}

// ExecuteMirror runs mirror commands for each config entry and calls notify
// for each repo directory that was returned by the mirror command.
func ExecuteMirror(cfg []ConfigEntry, repoDir string, notify func(repoDir string)) {
	// Randomize the ordering in which we query
	// things. This is to ensure that quota limits don't
	// always hit the last one in the list.
	cfg = Randomize(cfg)
	for _, c := range cfg {
		var cmd *exec.Cmd
		if c.GitHubURL != "" || c.GithubUser != "" || c.GithubOrg != "" {
			cmd = exec.Command("zoekt-mirror-github",
				"-dest", repoDir)
			if c.GitHubURL != "" {
				cmd.Args = append(cmd.Args, "-url", c.GitHubURL)
			}
			if c.GithubUser != "" {
				cmd.Args = append(cmd.Args, "-user", c.GithubUser)
			} else if c.GithubOrg != "" {
				cmd.Args = append(cmd.Args, "-org", c.GithubOrg)
			}
			if c.Name != "" {
				cmd.Args = append(cmd.Args, "-name", c.Name)
			}
			if c.Exclude != "" {
				cmd.Args = append(cmd.Args, "-exclude", c.Exclude)
			}
			if c.CredentialPath != "" {
				cmd.Args = append(cmd.Args, "-token", c.CredentialPath)
			}
			for _, topic := range c.Topics {
				cmd.Args = append(cmd.Args, "-topic", topic)
			}
			for _, topic := range c.ExcludeTopics {
				cmd.Args = append(cmd.Args, "-exclude_topic", topic)
			}
			if c.NoArchived {
				cmd.Args = append(cmd.Args, "-no_archived")
			}
			if !c.KeepDeleted {
				cmd.Args = append(cmd.Args, "-delete")
			}
			if c.Forks {
				cmd.Args = append(cmd.Args, "-forks")
			}
			for _, v := range c.Visibility {
				cmd.Args = append(cmd.Args, "-visibility", v)
			}
		} else if c.GitilesURL != "" {
			cmd = exec.Command("zoekt-mirror-gitiles",
				"-dest", repoDir, "-name", c.Name)
			if c.Exclude != "" {
				cmd.Args = append(cmd.Args, "-exclude", c.Exclude)
			}
			cmd.Args = append(cmd.Args, c.GitilesURL)
		} else if c.CGitURL != "" {
			cmd = exec.Command("zoekt-mirror-gitiles",
				"-type", "cgit",
				"-dest", repoDir, "-name", c.Name)
			if c.Exclude != "" {
				cmd.Args = append(cmd.Args, "-exclude", c.Exclude)
			}
			cmd.Args = append(cmd.Args, c.CGitURL)
		} else if c.BitBucketServerURL != "" {
			cmd = exec.Command("zoekt-mirror-bitbucket-server",
				"-dest", repoDir, "-url", c.BitBucketServerURL)
			if c.BitBucketServerProject != "" {
				cmd.Args = append(cmd.Args, "-project", c.BitBucketServerProject)
			}
			if c.DisableTLS {
				cmd.Args = append(cmd.Args, "-disable-tls")
			}
			if c.ProjectType != "" {
				cmd.Args = append(cmd.Args, "-type", c.ProjectType)
			}
			if c.Name != "" {
				cmd.Args = append(cmd.Args, "-name", c.Name)
			}
			if c.Exclude != "" {
				cmd.Args = append(cmd.Args, "-exclude", c.Exclude)
			}
			if c.CredentialPath != "" {
				cmd.Args = append(cmd.Args, "-credentials", c.CredentialPath)
			}
			if !c.KeepDeleted {
				cmd.Args = append(cmd.Args, "-delete")
			}
		} else if c.GitLabURL != "" {
			cmd = exec.Command("zoekt-mirror-gitlab",
				"-dest", repoDir, "-url", c.GitLabURL)
			if c.Name != "" {
				cmd.Args = append(cmd.Args, "-name", c.Name)
			}
			if c.Exclude != "" {
				cmd.Args = append(cmd.Args, "-exclude", c.Exclude)
			}
			if c.OnlyPublic {
				cmd.Args = append(cmd.Args, "-public")
			}
			if c.ExcludeUserRepos {
				cmd.Args = append(cmd.Args, "-exclude_user")
			}
			if c.CredentialPath != "" {
				cmd.Args = append(cmd.Args, "-token", c.CredentialPath)
			}
			if c.NoArchived {
				cmd.Args = append(cmd.Args, "-no_archived")
			}
			if !c.KeepDeleted {
				cmd.Args = append(cmd.Args, "-delete")
			}
		} else if c.GerritApiURL != "" {
			cmd = exec.Command("zoekt-mirror-gerrit",
				"-dest", repoDir)
			if c.CredentialPath != "" {
				cmd.Args = append(cmd.Args, "-http-credentials", c.CredentialPath)
			}
			if c.Name != "" {
				cmd.Args = append(cmd.Args, "-name", c.Name)
			}
			if c.Exclude != "" {
				cmd.Args = append(cmd.Args, "-exclude", c.Exclude)
			}
			if c.Active {
				cmd.Args = append(cmd.Args, "-active")
			}
			if c.GerritFetchMetaConfig {
				cmd.Args = append(cmd.Args, "-fetch-meta-config")
			}
			if c.GerritRepoNameFormat != "" {
				cmd.Args = append(cmd.Args, "-repo-name-format", c.GerritRepoNameFormat)
			}
			if !c.KeepDeleted {
				cmd.Args = append(cmd.Args, "-delete")
			}
			cmd.Args = append(cmd.Args, c.GerritApiURL)
		} else if c.GiteaURL != "" {
			cmd = exec.Command("zoekt-mirror-gitea", "-dest", repoDir)
			cmd.Args = append(cmd.Args, "-url", c.GiteaURL)
			if c.GiteaUser != "" {
				cmd.Args = append(cmd.Args, "-user", c.GiteaUser)
			} else if c.GiteaOrg != "" {
				cmd.Args = append(cmd.Args, "-org", c.GiteaOrg)
			}
			if c.Name != "" {
				cmd.Args = append(cmd.Args, "-name", c.Name)
			}
			if c.Exclude != "" {
				cmd.Args = append(cmd.Args, "-exclude", c.Exclude)
			}
			if c.CredentialPath != "" {
				cmd.Args = append(cmd.Args, "-token", c.CredentialPath)
			}
			for _, topic := range c.Topics {
				cmd.Args = append(cmd.Args, "-topic", topic)
			}
			for _, topic := range c.ExcludeTopics {
				cmd.Args = append(cmd.Args, "-exclude_topic", topic)
			}
			if c.NoArchived {
				cmd.Args = append(cmd.Args, "-no_archived")
			}
			if !c.KeepDeleted {
				cmd.Args = append(cmd.Args, "-delete")
			}
			if c.Forks {
				cmd.Args = append(cmd.Args, "-forks")
			}
		} else {
			slog.Info("ExecuteMirror: ignoring config, no valid repository definition", "config", c)
			continue
		}

		stdout, _ := LoggedRun(cmd)

		for _, fn := range bytes.Split(stdout, []byte{'\n'}) {
			if len(fn) == 0 {
				continue
			}

			slog.Info("mirror discovered repo", "dir", string(fn))
			notify(string(fn))
		}
	}
}
