package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/sourcegraph/zoekt-simple/internal/config"
	"github.com/sourcegraph/zoekt-simple/internal/indexer"
)

func setupIndexesAPI(t *testing.T) (*IndexesAPI, *http.ServeMux) {
	t.Helper()
	dir := t.TempDir()
	storePath := filepath.Join(dir, "dynamic.json")
	store, err := config.NewDynamicStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	static := map[string]config.IndexConfig{
		"default": {},
	}
	queue := indexer.NewQueue()
	api := NewIndexesAPI(store, static, queue, dir)
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)
	return api, mux
}

func TestListIndexesEmpty(t *testing.T) {
	_, mux := setupIndexesAPI(t)
	req := httptest.NewRequest("GET", "/api/indexes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var results []IndexResponse
	json.Unmarshal(w.Body.Bytes(), &results)
	// Should have the static "default" index.
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Source != "static" {
		t.Fatalf("expected static, got %s", results[0].Source)
	}
}

func TestCreateAndGetDynamic(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	body := `{"name":"my-repo","repo_url":"https://github.com/org/repo"}`
	req := httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// GET the created index.
	req = httptest.NewRequest("GET", "/api/indexes/my-repo", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp IndexResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Source != "dynamic" {
		t.Fatalf("expected dynamic, got %s", resp.Source)
	}
	if resp.RepoURL != "https://github.com/org/repo" {
		t.Fatalf("expected repo URL, got %s", resp.RepoURL)
	}
}

func TestCreateDuplicateName(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	body := `{"name":"dup","repo_url":"https://example.com/a"}`
	req := httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// Try to create again.
	req = httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestCreateConflictWithStatic(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	body := `{"name":"default","repo_url":"https://example.com/a"}`
	req := httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestUpdateDynamic(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	// Create.
	body := `{"name":"upd","repo_url":"https://example.com/old"}`
	req := httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// Update.
	body = `{"repo_url":"https://example.com/new","refs":["main"]}`
	req = httptest.NewRequest("PUT", "/api/indexes/upd", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.RepoURL != "https://example.com/new" {
		t.Fatalf("expected new URL, got %s", resp.RepoURL)
	}
}

func TestUpdateStaticForbidden(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	body := `{"repo_url":"https://example.com/a"}`
	req := httptest.NewRequest("PUT", "/api/indexes/default", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDeleteDynamic(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	// Create.
	body := `{"name":"del","repo_url":"https://example.com/del"}`
	req := httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// Delete.
	req = httptest.NewRequest("DELETE", "/api/indexes/del", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify gone.
	req = httptest.NewRequest("GET", "/api/indexes/del", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteStaticForbidden(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	req := httptest.NewRequest("DELETE", "/api/indexes/default", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDeleteNotFound(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	req := httptest.NewRequest("DELETE", "/api/indexes/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetStaticIndex(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	req := httptest.NewRequest("GET", "/api/indexes/default", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp IndexResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Source != "static" {
		t.Fatalf("expected static, got %s", resp.Source)
	}
}

func TestCreateMissingFields(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	// Missing name.
	body := `{"repo_url":"https://example.com/a"}`
	req := httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing name, got %d", w.Code)
	}

	// Missing repo_url.
	body = `{"name":"test"}`
	req = httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing repo_url, got %d", w.Code)
	}
}

func TestListMixed(t *testing.T) {
	_, mux := setupIndexesAPI(t)

	// Create a dynamic index.
	body := `{"name":"dyn","repo_url":"https://example.com/dyn"}`
	req := httptest.NewRequest("POST", "/api/indexes", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// List all.
	req = httptest.NewRequest("GET", "/api/indexes", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var results []IndexResponse
	json.Unmarshal(w.Body.Bytes(), &results)
	if len(results) != 2 {
		t.Fatalf("expected 2 indexes, got %d", len(results))
	}
	// Sorted alphabetically: "default" (static), "dyn" (dynamic).
	if results[0].Name != "default" || results[0].Source != "static" {
		t.Fatalf("unexpected first entry: %+v", results[0])
	}
	if results[1].Name != "dyn" || results[1].Source != "dynamic" {
		t.Fatalf("unexpected second entry: %+v", results[1])
	}
}
