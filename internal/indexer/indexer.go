package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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

// IndexTarget describes a named index with include/exclude filtering.
type IndexTarget struct {
	Name     string
	IndexDir string
	Include  []CompiledInclude
	Exclude  []*regexp.Regexp
}

// CompiledInclude is a compiled repo+ref regex pair.
type CompiledInclude struct {
	Repo *regexp.Regexp
	Refs *regexp.Regexp // nil = HEAD only; matches branch and tag names
}

// Options configures the background indexing loop.
type Options struct {
	DataDir        string
	Targets        []IndexTarget
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
	for _, t := range o.Targets {
		if err := os.MkdirAll(t.IndexDir, 0o755); err != nil {
			panic(fmt.Sprintf("create index dir %s: %v", t.IndexDir, err))
		}
	}
}

// Indexer manages periodic mirroring, fetching, and indexing of git repos.
type Indexer struct {
	mu      sync.RWMutex
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

// Reconfigure atomically replaces the indexer's options.
func (idx *Indexer) Reconfigure(opts Options) {
	opts.validate()
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.opts = opts
}

// CurrentOptions returns a snapshot of the current options.
func (idx *Indexer) CurrentOptions() Options {
	return idx.lockedOpts()
}

// lockedOpts returns a snapshot of the current options under the read lock.
func (idx *Indexer) lockedOpts() Options {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.opts
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
	curInterval := idx.lockedOpts().FetchInterval
	ticker := time.NewTicker(curInterval)
	defer ticker.Stop()

	for {
		opts := idx.lockedOpts()
		if opts.FetchInterval != curInterval {
			ticker.Reset(opts.FetchInterval)
			curInterval = opts.FetchInterval
		}
		repos, err := gitindex.FindGitRepos(opts.repoDir)
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
// It re-reads options each iteration so that mirrors added via hot-reload
// are picked up even if the server started with none.
func (idx *Indexer) periodicMirror(ctx context.Context) {
	notify := func(dir string) {
		idx.queue.PushLow(Request{RepoDir: dir})
	}

	curInterval := idx.lockedOpts().MirrorInterval
	ticker := time.NewTicker(curInterval)
	defer ticker.Stop()

	for {
		opts := idx.lockedOpts()
		if opts.MirrorInterval != curInterval {
			ticker.Reset(opts.MirrorInterval)
			curInterval = opts.MirrorInterval
		}
		if len(opts.MirrorEntries) > 0 {
			config.ExecuteMirror(opts.MirrorEntries, opts.repoDir, notify)
			config.CleanupGitMirrorRepos(opts.repoDir, opts.MirrorEntries)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// indexPending consumes from the queue and indexes each repo into all matching targets.
func (idx *Indexer) indexPending(ctx context.Context) {
	for {
		req, ok := idx.queue.Next(ctx)
		if !ok {
			return
		}

		opts := idx.lockedOpts()

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

		// Derive repo name once for all targets.
		repoName := ""
		if rel, err := filepath.Rel(opts.repoDir, req.RepoDir); err == nil {
			repoName = strings.TrimSuffix(rel, ".git")
		}

		var firstErr error
		for i := range opts.Targets {
			if err := idx.indexRepoForTarget(req.RepoDir, repoName, &opts.Targets[i]); err != nil && firstErr == nil {
				firstErr = err
			}
		}

		// Clean up any leftover .tmp files across all target index dirs.
		for _, t := range opts.Targets {
			if tmps, globErr := filepath.Glob(filepath.Join(t.IndexDir, "*.tmp")); globErr == nil {
				for _, tmp := range tmps {
					os.Remove(tmp)
				}
			}
		}

		if req.TaskID != "" {
			if firstErr != nil {
				errMsg := firstErr.Error()
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

	opts := idx.lockedOpts()
	// Find a matching mirror entry.
	for _, entry := range opts.MirrorEntries {
		if entry.GithubOrg == owner || entry.GithubUser == owner {
			// Create a copy scoped to just this repo.
			scoped := entry
			scoped.Name = "^" + repoName + "$"
			slog.Info("mirroring single repo", "repo", repo, "owner", owner, "name", repoName)
			config.ExecuteMirror([]config.ConfigEntry{scoped}, opts.repoDir, func(string) {})
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

// indexRepoForTarget indexes a repo into a specific target. It checks
// include/exclude filters, resolves branches, and runs gitindex.IndexGitRepo
// in a goroutine with a timeout.
func (idx *Indexer) indexRepoForTarget(dir, repoName string, target *IndexTarget) error {
	branches := resolveBranchesForTarget(repoName, dir, target)
	if branches == nil {
		return nil
	}

	idxOpts := idx.lockedOpts()
	slog.Info("indexing", "repo", repoName, "target", target.Name, "branches", branches)

	opts := gitindex.Options{
		RepoDir:      dir,
		Incremental:  true,
		RepoCacheDir: idxOpts.repoDir,
		BuildOptions: index.Options{
			IndexDir:         target.IndexDir,
			Parallelism:      idxOpts.cpuCount,
			CTagsMustSucceed: true,
			CTagsPath:        findCTags(),
		},
		BranchPrefix:       "refs/heads/",
		Branches:           branches,
		AllowMissingBranch: true,
		Submodules:         true,
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
			slog.Error("indexRepo failed", "dir", dir, "target", target.Name, "error", res.err)
		} else {
			slog.Info("indexRepo done", "dir", dir, "target", target.Name)
		}
		return res.err
	case <-time.After(idxOpts.IndexTimeout):
		slog.Error("indexRepo timeout", "dir", dir, "target", target.Name, "timeout", idxOpts.IndexTimeout)
		return fmt.Errorf("index timeout after %s for %s", idxOpts.IndexTimeout, dir)
	}
}

// resolveBranchesForTarget checks include/exclude filters and resolves which
// branches to index in a single pass. Returns nil if the repo should be skipped.
func resolveBranchesForTarget(repoName, bareDir string, target *IndexTarget) []string {
	for _, ex := range target.Exclude {
		if ex.MatchString(repoName) {
			return nil
		}
	}
	if len(target.Include) == 0 {
		return []string{"HEAD"} // Catch-all target.
	}
	for _, inc := range target.Include {
		if !inc.Repo.MatchString(repoName) {
			continue
		}
		if inc.Refs == nil {
			return []string{"HEAD"}
		}
		refs := resolveRefs(bareDir, inc.Refs)
		if len(refs) == 0 {
			return nil
		}
		return refs
	}
	return nil // No include matched.
}

// targetIncludesRepo checks whether a repo passes the target's include/exclude
// filters (without resolving branches). Used by orphan cleanup.
func targetIncludesRepo(repoName string, target *IndexTarget) bool {
	for _, ex := range target.Exclude {
		if ex.MatchString(repoName) {
			return false
		}
	}
	if len(target.Include) == 0 {
		return true
	}
	for _, inc := range target.Include {
		if inc.Repo.MatchString(repoName) {
			return true
		}
	}
	return false
}

// resolveRefs lists refs in a bare git repo and returns branch and tag names
// matching the given pattern.
func resolveRefs(bareDir string, pattern *regexp.Regexp) []string {
	repo, err := git.PlainOpen(bareDir)
	if err != nil {
		slog.Warn("open repo for ref listing", "dir", bareDir, "error", err)
		return nil
	}

	refs, err := repo.References()
	if err != nil {
		slog.Warn("list refs", "dir", bareDir, "error", err)
		return nil
	}

	var matched []string
	refs.ForEach(func(ref *plumbing.Reference) error {
		if !ref.Name().IsBranch() && !ref.Name().IsTag() {
			return nil
		}
		name := ref.Name().Short()
		if pattern.MatchString(name) {
			matched = append(matched, name)
		}
		return nil
	})
	return matched
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
	opts := idx.lockedOpts()
	for i := range opts.Targets {
		idx.deleteOrphansForTarget(&opts.Targets[i], opts.repoDir)
	}
}

func (idx *Indexer) deleteOrphansForTarget(target *IndexTarget, repoDir string) {
	shards, err := filepath.Glob(filepath.Join(target.IndexDir, "*.zoekt"))
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

			shouldDelete := false

			if _, err := os.Stat(repo.Source); os.IsNotExist(err) {
				// Bare repo no longer exists on disk.
				shouldDelete = true
			} else if rel, err := filepath.Rel(repoDir, repo.Source); err == nil {
				// Bare repo exists but may no longer belong to this target.
				repoName := strings.TrimSuffix(rel, ".git")
				if !targetIncludesRepo(repoName, target) {
					shouldDelete = true
				}
			}

			if shouldDelete {
				slog.Info("deleting orphan shard", "shard", shard, "target", target.Name, "source", repo.Source)
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
	opts := idx.lockedOpts()
	logDir := filepath.Join(opts.DataDir, "log")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("read log dir", "error", err)
		}
		return
	}

	cutoff := time.Now().Add(-opts.MaxLogAge)
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
