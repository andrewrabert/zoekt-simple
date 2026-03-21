package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/google/uuid"

	"github.com/sourcegraph/zoekt-simple/internal/indexer"
)

// Task represents a reindex task.
type Task struct {
	ID     string  `json:"id"`
	Repo   string  `json:"repo"`
	Status string  `json:"status"`
	Error  *string `json:"error"`
}

// TaskTracker tracks reindex tasks. It implements indexer.TaskUpdater.
type TaskTracker struct {
	mu    sync.Mutex
	tasks map[string]*Task
}

func (t *TaskTracker) create(repo string) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Evict completed/failed tasks when map grows too large.
	if len(t.tasks) > 1000 {
		for id, tk := range t.tasks {
			if tk.Status == "completed" || tk.Status == "failed" {
				delete(t.tasks, id)
			}
		}
	}

	id := uuid.New().String()
	t.tasks[id] = &Task{ID: id, Repo: repo, Status: "pending", Error: nil}
	return id
}

// Update updates the status of a task. It implements indexer.TaskUpdater.
func (t *TaskTracker) Update(id, status string, errMsg *string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tk, ok := t.tasks[id]; ok {
		tk.Status = status
		tk.Error = errMsg
	}
}

func (t *TaskTracker) get(id string) *Task {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tasks[id]
}

func (a *app) postReindex(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Repo string `json:"repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.Repo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'repo' field"})
		return
	}

	barePath := filepath.Join(a.reposDir, body.Repo+".git")
	taskID := a.tracker.create(body.Repo)
	req := indexer.Request{RepoDir: barePath, TaskID: taskID, Repo: body.Repo}
	if !a.queue.PushHigh(req) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "index queue full"})
		return
	}
	slog.Info("POST /api/reindex: accepted", "repo", body.Repo, "task_id", taskID)
	http.Redirect(w, r, "/api/reindex/"+taskID, http.StatusSeeOther)
}

func (a *app) getReindex(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")
	t := a.tracker.get(taskID)
	if t == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (a *app) getFile(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repo")
	path := r.URL.Query().Get("path")
	if repo == "" || path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'repo' and/or 'path' query params"})
		return
	}

	barePath := filepath.Join(a.reposDir, repo+".git")
	content, err := readFileFromRepo(barePath, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(content))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
