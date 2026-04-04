package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sourcegraph/zoekt-simple/internal/config"
	"github.com/sourcegraph/zoekt-simple/internal/indexer"
	"github.com/sourcegraph/zoekt/index"
)

// IndexResponse is the JSON representation of an index returned by the API.
type IndexResponse struct {
	Name    string   `json:"name"`
	Source  string   `json:"source"` // "static" or "dynamic"
	RepoURL string   `json:"repo_url,omitempty"`
	Refs    []string `json:"refs,omitempty"`
}

// IndexesAPI handles CRUD operations for dynamic indexes. Static indexes from
// config.yaml are visible via GET but cannot be modified.
type IndexesAPI struct {
	store   *config.DynamicStore
	static  map[string]config.IndexConfig
	queue   *indexer.Queue
	dataDir string
	repoDir string
}

// NewIndexesAPI creates a new IndexesAPI.
func NewIndexesAPI(store *config.DynamicStore, static map[string]config.IndexConfig, queue *indexer.Queue, dataDir string) *IndexesAPI {
	return &IndexesAPI{
		store:   store,
		static:  static,
		queue:   queue,
		dataDir: dataDir,
		repoDir: filepath.Join(dataDir, "repos"),
	}
}

// RegisterHandlers registers the /api/indexes routes on the given mux.
func (a *IndexesAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/indexes", a.listIndexes)
	mux.HandleFunc("POST /api/indexes", a.createIndex)
	mux.HandleFunc("GET /api/indexes/{name}", a.getIndex)
	mux.HandleFunc("PUT /api/indexes/{name}", a.updateIndex)
	mux.HandleFunc("DELETE /api/indexes/{name}", a.deleteIndex)
}

func (a *IndexesAPI) listIndexes(w http.ResponseWriter, r *http.Request) {
	var results []IndexResponse

	// Static indexes.
	for name := range a.static {
		results = append(results, IndexResponse{
			Name:   name,
			Source: "static",
		})
	}

	// Dynamic indexes.
	for _, idx := range a.store.List() {
		results = append(results, IndexResponse{
			Name:    idx.Name,
			Source:  "dynamic",
			RepoURL: idx.RepoURL,
			Refs:    idx.Refs,
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	writeJSON(w, http.StatusOK, results)
}

func (a *IndexesAPI) getIndex(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Check static first.
	if _, ok := a.static[name]; ok {
		writeJSON(w, http.StatusOK, IndexResponse{Name: name, Source: "static"})
		return
	}

	// Check dynamic.
	idx, ok := a.store.Get(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "index not found"})
		return
	}
	writeJSON(w, http.StatusOK, IndexResponse{
		Name:    idx.Name,
		Source:  "dynamic",
		RepoURL: idx.RepoURL,
		Refs:    idx.Refs,
	})
}

func (a *IndexesAPI) createIndex(w http.ResponseWriter, r *http.Request) {
	var body config.DynamicIndex
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'name' field"})
		return
	}
	if body.RepoURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'repo_url' field"})
		return
	}

	// Reject if name collides with a static index.
	if _, ok := a.static[body.Name]; ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot create: name conflicts with a static index"})
		return
	}

	// Reject if already exists as dynamic.
	if _, ok := a.store.Get(body.Name); ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "index already exists"})
		return
	}

	if err := a.store.Put(body); err != nil {
		slog.Error("create dynamic index", "name", body.Name, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist index"})
		return
	}

	// Trigger mirroring + indexing via the queue.
	a.enqueueDynamic(body)

	slog.Info("POST /api/indexes: created", "name", body.Name)
	writeJSON(w, http.StatusCreated, IndexResponse{
		Name:    body.Name,
		Source:  "dynamic",
		RepoURL: body.RepoURL,
		Refs:    body.Refs,
	})
}

func (a *IndexesAPI) updateIndex(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Static indexes are immutable via the API.
	if _, ok := a.static[name]; ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cannot modify a static index from config.yaml"})
		return
	}

	if _, ok := a.store.Get(name); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "index not found"})
		return
	}

	var body config.DynamicIndex
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	// Ensure the name in the URL wins.
	body.Name = name

	if body.RepoURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'repo_url' field"})
		return
	}

	if err := a.store.Put(body); err != nil {
		slog.Error("update dynamic index", "name", name, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist index"})
		return
	}

	// Re-trigger indexing.
	a.enqueueDynamic(body)

	slog.Info("PUT /api/indexes: updated", "name", name)
	writeJSON(w, http.StatusOK, IndexResponse{
		Name:    body.Name,
		Source:  "dynamic",
		RepoURL: body.RepoURL,
		Refs:    body.Refs,
	})
}

func (a *IndexesAPI) deleteIndex(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Static indexes are immutable via the API.
	if _, ok := a.static[name]; ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cannot delete a static index from config.yaml"})
		return
	}

	deleted, err := a.store.Delete(name)
	if err != nil {
		slog.Error("delete dynamic index", "name", name, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete index"})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "index not found"})
		return
	}

	// Clean up shards for this dynamic index.
	a.cleanupShards(name)

	slog.Info("DELETE /api/indexes: deleted", "name", name)
	w.WriteHeader(http.StatusNoContent)
}

// enqueueDynamic pushes a mirror+index request for a dynamic index entry.
func (a *IndexesAPI) enqueueDynamic(idx config.DynamicIndex) {
	// Build a ConfigEntry so the existing mirror pipeline can clone the repo.
	entry := config.ConfigEntry{
		GitURL: idx.RepoURL,
		Name:   idx.Name,
	}
	notify := func(dir string) {
		a.queue.PushLow(indexer.Request{RepoDir: dir})
	}
	config.ExecuteMirror([]config.ConfigEntry{entry}, a.repoDir, notify)
}

// cleanupShards removes index shards whose repo name matches the deleted
// dynamic index name. It walks all target index directories under dataDir/index/.
func (a *IndexesAPI) cleanupShards(name string) {
	indexBaseDir := filepath.Join(a.dataDir, "index")
	entries, err := os.ReadDir(indexBaseDir)
	if err != nil {
		slog.Warn("cleanupShards: read index base dir", "error", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		targetDir := filepath.Join(indexBaseDir, entry.Name())
		shards, err := filepath.Glob(filepath.Join(targetDir, "*.zoekt"))
		if err != nil {
			continue
		}
		for _, shard := range shards {
			repos, _, err := index.ReadMetadataPath(shard)
			if err != nil {
				continue
			}
			for _, repo := range repos {
				repoName := repo.Name
				// Dynamic git repos are named with the display name from the
				// DynamicIndex (e.g. "my-project"). The bare repo is cloned
				// under repos/git/<base64>.git so the shard's repo name is
				// "git/<name>". Match either the full name or the name part
				// after "git/".
				if repoName == name || strings.TrimPrefix(repoName, "git/") == name {
					paths, pathErr := index.IndexFilePaths(shard)
					if pathErr != nil {
						continue
					}
					for _, p := range paths {
						os.Remove(p)
					}
					slog.Info("cleanupShards: removed shard", "shard", shard, "name", name)
				}
			}
		}
	}

	// Also remove the bare repo clone under repos/git/.
	config.CleanupDynamicRepo(a.repoDir, name)
}
