package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sourcegraph/zoekt/gitindex"
	"github.com/sourcegraph/zoekt/index"

	"github.com/sourcegraph/zoekt-simple/internal/config"
)

const day = time.Hour * 24

// TaskUpdater is an interface for updating task status. This avoids a
// circular dependency between the indexer and server packages.
type TaskUpdater interface {
	Update(id, status string, errMsg *string)
}

// Options configures the background indexing loop.
type Options struct {
	DataDir        string
	IndexDir       string
	MirrorConfig   string
	FetchInterval  time.Duration
	MirrorInterval time.Duration
	IndexTimeout   time.Duration
	CPUFraction    float64
	MaxLogAge      time.Duration
	MirrorEntries  []config.ConfigEntry
	// computed
	cpuCount int
	repoDir  string
}

// validate fills in defaults and panics on invalid values.
func (o *Options) validate() {
	if o.CPUFraction == 0 {
		o.CPUFraction = 0.25
	}
	if o.CPUFraction < 0 || o.CPUFraction > 1.0 {
		panic(fmt.Sprintf("invalid cpu_fraction: %f", o.CPUFraction))
	}
	o.cpuCount = int(math.Max(1, math.Round(float64(runtime.NumCPU())*o.CPUFraction)))

	if o.FetchInterval == 0 {
		o.FetchInterval = 5 * time.Minute
	}
	if o.MirrorInterval == 0 {
		o.MirrorInterval = 24 * time.Hour
	}
	if o.IndexTimeout == 0 {
		o.IndexTimeout = 1 * time.Hour
	}
	if o.MaxLogAge == 0 {
		o.MaxLogAge = 3 * day
	}
	o.repoDir = filepath.Join(o.DataDir, "repos")
}

// Indexer manages periodic mirroring, fetching, and indexing of git repos.
type Indexer struct {
	opts    Options
	queue   *Queue
	tracker TaskUpdater
}

// New creates an Indexer with the given options and task updater.
func New(opts Options, tracker TaskUpdater) *Indexer {
	opts.validate()
	return &Indexer{
		opts:    opts,
		queue:   NewQueue(),
		tracker: tracker,
	}
}

// Queue returns the index queue used by this indexer.
func (idx *Indexer) Queue() *Queue {
	return idx.queue
}

// Run starts all background loops. It blocks until ctx is cancelled.
// It launches goroutines for periodicMirror, deleteOrphanIndexes,
// deleteLogsLoop, and indexPending. The calling goroutine runs periodicFetch.
func (idx *Indexer) Run(ctx context.Context) {
	go idx.periodicMirror(ctx)
	go idx.deleteOrphanIndexes(ctx)
	go idx.deleteLogsLoop(ctx)
	go idx.indexPending(ctx)

	idx.periodicFetch(ctx)
}

// periodicFetch discovers git repos under DataDir/repos, runs git fetch on
// each, and pushes them onto the low-priority queue.
func (idx *Indexer) periodicFetch(ctx context.Context) {
	ticker := time.NewTicker(idx.opts.FetchInterval)
	defer ticker.Stop()

	for {
		repos, err := gitindex.FindGitRepos(idx.opts.repoDir)
		if err != nil {
			slog.Error("FindGitRepos", "error", err)
		} else {
			for _, dir := range repos {
				if err := fetchGitRepo(dir); err != nil {
					slog.Warn("fetch failed", "dir", dir, "error", err)
				}
				idx.queue.PushLow(Request{RepoDir: dir})
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// fetchGitRepo runs "git fetch origin --prune" for the given bare repo dir.
func fetchGitRepo(dir string) error {
	cmd := exec.Command("git", "--git-dir", dir, "fetch", "origin", "--prune")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// periodicMirror runs mirror operations on a schedule.
// If MirrorEntries is set (from YAML config), it uses them directly.
// Otherwise, it falls back to PeriodicMirrorFile which reads a JSON config.
func (idx *Indexer) periodicMirror(ctx context.Context) {
	notify := func(dir string) {
		idx.queue.PushLow(Request{RepoDir: dir})
	}

	if len(idx.opts.MirrorEntries) > 0 {
		ticker := time.NewTicker(idx.opts.MirrorInterval)
		defer ticker.Stop()
		for {
			config.ExecuteMirror(idx.opts.MirrorEntries, idx.opts.repoDir, notify)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}
	if idx.opts.MirrorConfig == "" {
		return
	}
	config.PeriodicMirrorFile(ctx, idx.opts.repoDir, idx.opts.MirrorConfig, idx.opts.MirrorInterval, notify)
}

// indexPending consumes from the queue and indexes each repo.
func (idx *Indexer) indexPending(ctx context.Context) {
	for {
		req, ok := idx.queue.Next(ctx)
		if !ok {
			return
		}

		if req.TaskID != "" {
			idx.tracker.Update(req.TaskID, "running", nil)
		}

		// If the repo doesn't exist, try to mirror it first.
		if req.Repo != "" {
			if _, statErr := os.Stat(req.RepoDir); os.IsNotExist(statErr) {
				if mirrorErr := idx.mirrorSingleRepo(req.Repo); mirrorErr != nil {
					slog.Error("mirror failed", "repo", req.Repo, "error", mirrorErr)
					if req.TaskID != "" {
						errMsg := mirrorErr.Error()
						idx.tracker.Update(req.TaskID, "failed", &errMsg)
					}
					continue
				}
			}
		}

		err := idx.indexRepo(req.RepoDir)

		// Clean up any leftover .tmp files in the index dir.
		if tmps, globErr := filepath.Glob(filepath.Join(idx.opts.IndexDir, "*.tmp")); globErr == nil {
			for _, tmp := range tmps {
				os.Remove(tmp)
			}
		}

		if req.TaskID != "" {
			if err != nil {
				errMsg := err.Error()
				idx.tracker.Update(req.TaskID, "failed", &errMsg)
			} else {
				idx.tracker.Update(req.TaskID, "completed", nil)
			}
		}
	}
}

// mirrorSingleRepo finds a matching mirror config entry for the given repo
// path (e.g. "github.com/org/repo") and runs the mirror command scoped to
// just that repo name. This reuses the same mirroring logic as periodic mirrors.
func (idx *Indexer) mirrorSingleRepo(repo string) error {
	// Parse repo path: "github.com/org/repo" → host, owner, name
	parts := strings.SplitN(repo, "/", 3)
	if len(parts) != 3 {
		return fmt.Errorf("invalid repo path %q: expected host/owner/name", repo)
	}
	owner, repoName := parts[1], parts[2]

	// Find a matching mirror entry.
	for _, entry := range idx.opts.MirrorEntries {
		if entry.GithubOrg == owner || entry.GithubUser == owner {
			// Create a copy scoped to just this repo.
			scoped := entry
			scoped.Name = "^" + repoName + "$"
			slog.Info("mirroring single repo", "repo", repo, "owner", owner, "name", repoName)
			config.ExecuteMirror([]config.ConfigEntry{scoped}, idx.opts.repoDir, func(string) {})
			return nil
		}
	}

	return fmt.Errorf("no mirror config found for %q", repo)
}

// findCTags returns the path to a universal-ctags binary, checking
// CTAGS_COMMAND, then "universal-ctags", then "ctags" on PATH.
func findCTags() string {
	if cmd := os.Getenv("CTAGS_COMMAND"); cmd != "" {
		return cmd
	}
	if p, err := exec.LookPath("universal-ctags"); err == nil {
		return p
	}
	if p, err := exec.LookPath("ctags"); err == nil {
		return p
	}
	return ""
}

// indexRepo runs gitindex.IndexGitRepo in a goroutine with a timeout.
// It recovers from panics.
func (idx *Indexer) indexRepo(dir string) error {
	opts := gitindex.Options{
		RepoDir:      dir,
		Incremental:  true,
		RepoCacheDir: idx.opts.repoDir,
		BuildOptions: index.Options{
			IndexDir:         idx.opts.IndexDir,
			Parallelism:      idx.opts.cpuCount,
			CTagsMustSucceed: true,
			CTagsPath:        findCTags(),
		},
		Branches: []string{"HEAD"},
	}

	type result struct {
		err error
	}
	done := make(chan result, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("indexer panic: %v", r)}
			}
		}()
		_, err := gitindex.IndexGitRepo(opts)
		done <- result{err: err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			slog.Error("indexRepo failed", "dir", dir, "error", res.err)
		} else {
			slog.Info("indexRepo done", "dir", dir)
		}
		return res.err
	case <-time.After(idx.opts.IndexTimeout):
		slog.Error("indexRepo timeout", "dir", dir, "timeout", idx.opts.IndexTimeout)
		return fmt.Errorf("index timeout after %s for %s", idx.opts.IndexTimeout, dir)
	}
}

// deleteOrphanIndexes periodically scans the index directory and removes
// shards whose source repo no longer exists on disk.
func (idx *Indexer) deleteOrphanIndexes(ctx context.Context) {
	ticker := time.NewTicker(day)
	defer ticker.Stop()

	for {
		idx.deleteOrphans()

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (idx *Indexer) deleteOrphans() {
	shards, err := filepath.Glob(filepath.Join(idx.opts.IndexDir, "*.zoekt"))
	if err != nil {
		slog.Error("glob index shards", "error", err)
		return
	}

	for _, shard := range shards {
		repos, _, err := index.ReadMetadataPath(shard)
		if err != nil {
			slog.Warn("read shard metadata", "shard", shard, "error", err)
			continue
		}

		for _, repo := range repos {
			if repo.Source == "" {
				continue
			}
			if _, err := os.Stat(repo.Source); os.IsNotExist(err) {
				slog.Info("deleting orphan shard", "shard", shard, "source", repo.Source)
				paths, pathErr := index.IndexFilePaths(shard)
				if pathErr != nil {
					slog.Warn("IndexFilePaths", "shard", shard, "error", pathErr)
					continue
				}
				for _, p := range paths {
					os.Remove(p)
				}
			}
		}
	}
}

// deleteLogsLoop periodically removes old log files.
func (idx *Indexer) deleteLogsLoop(ctx context.Context) {
	ticker := time.NewTicker(day)
	defer ticker.Stop()

	for {
		idx.deleteLogs()

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (idx *Indexer) deleteLogs() {
	logDir := filepath.Join(idx.opts.DataDir, "log")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("read log dir", "error", err)
		}
		return
	}

	cutoff := time.Now().Add(-idx.opts.MaxLogAge)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(logDir, e.Name())
			slog.Info("deleting old log", "path", path)
			os.Remove(path)
		}
	}
}
