package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDynamicStoreCreateAndList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dynamic.json")
	s, err := NewDynamicStore(path)
	if err != nil {
		t.Fatal(err)
	}

	idx := DynamicIndex{Name: "my-repo", RepoURL: "https://github.com/org/repo"}
	if err := s.Put(idx); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].Name != "my-repo" {
		t.Fatalf("expected my-repo, got %s", list[0].Name)
	}
}

func TestDynamicStoreGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dynamic.json")
	s, err := NewDynamicStore(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Put(DynamicIndex{Name: "a", RepoURL: "https://example.com/a"}); err != nil {
		t.Fatal(err)
	}

	got, ok := s.Get("a")
	if !ok {
		t.Fatal("expected to find 'a'")
	}
	if got.RepoURL != "https://example.com/a" {
		t.Fatalf("expected https://example.com/a, got %s", got.RepoURL)
	}

	_, ok = s.Get("missing")
	if ok {
		t.Fatal("expected not found for 'missing'")
	}
}

func TestDynamicStoreDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dynamic.json")
	s, err := NewDynamicStore(path)
	if err != nil {
		t.Fatal(err)
	}

	s.Put(DynamicIndex{Name: "del-me", RepoURL: "https://example.com/del"})

	ok, err := s.Delete("del-me")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected delete to return true")
	}

	ok, err = s.Delete("del-me")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected delete of missing entry to return false")
	}

	if len(s.List()) != 0 {
		t.Fatal("expected empty list after delete")
	}
}

func TestDynamicStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dynamic.json")
	s, err := NewDynamicStore(path)
	if err != nil {
		t.Fatal(err)
	}

	s.Put(DynamicIndex{Name: "persist", RepoURL: "https://example.com/p", Refs: []string{"main", "dev"}})

	// Re-open from disk.
	s2, err := NewDynamicStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("persist")
	if !ok {
		t.Fatal("expected to find 'persist' after re-open")
	}
	if got.RepoURL != "https://example.com/p" {
		t.Fatalf("unexpected repo URL: %s", got.RepoURL)
	}
	if len(got.Refs) != 2 || got.Refs[0] != "main" || got.Refs[1] != "dev" {
		t.Fatalf("unexpected refs: %v", got.Refs)
	}
}

func TestDynamicStoreEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dynamic.json")
	os.WriteFile(path, []byte(""), 0o644)

	s, err := NewDynamicStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 0 {
		t.Fatal("expected empty list for empty file")
	}
}

func TestDynamicStoreUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dynamic.json")
	s, err := NewDynamicStore(path)
	if err != nil {
		t.Fatal(err)
	}

	s.Put(DynamicIndex{Name: "upd", RepoURL: "https://example.com/old"})
	s.Put(DynamicIndex{Name: "upd", RepoURL: "https://example.com/new"})

	got, ok := s.Get("upd")
	if !ok {
		t.Fatal("expected to find 'upd'")
	}
	if got.RepoURL != "https://example.com/new" {
		t.Fatalf("expected updated URL, got %s", got.RepoURL)
	}
	if len(s.List()) != 1 {
		t.Fatal("expected 1 entry after update")
	}
}
