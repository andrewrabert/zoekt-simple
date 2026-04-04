package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// DynamicIndex represents an index configuration added via the API at runtime.
// It mirrors the structure of a YAML mirror + index target but is managed
// separately from the static config.yaml.
type DynamicIndex struct {
	Name    string   `json:"name"`
	RepoURL string   `json:"repo_url"`
	Refs    []string `json:"refs,omitempty"`
}

// DynamicStore persists dynamic index configurations to a JSON file.
// All operations are concurrency-safe.
type DynamicStore struct {
	mu   sync.RWMutex
	path string
	data map[string]DynamicIndex
}

// NewDynamicStore loads (or initializes) a dynamic index store backed by the
// given JSON file path. If the file does not exist, an empty store is created.
func NewDynamicStore(path string) (*DynamicStore, error) {
	s := &DynamicStore{
		path: path,
		data: make(map[string]DynamicIndex),
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read dynamic store %s: %w", path, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("parse dynamic store %s: %w", path, err)
	}
	return s, nil
}

// List returns all dynamic indexes.
func (s *DynamicStore) List() []DynamicIndex {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DynamicIndex, 0, len(s.data))
	for _, idx := range s.data {
		out = append(out, idx)
	}
	return out
}

// Get returns a single dynamic index by name, or false if not found.
func (s *DynamicStore) Get(name string) (DynamicIndex, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.data[name]
	return idx, ok
}

// Put creates or updates a dynamic index and persists to disk.
func (s *DynamicStore) Put(idx DynamicIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[idx.Name] = idx
	return s.flush()
}

// Delete removes a dynamic index by name. Returns false if the name was not found.
func (s *DynamicStore) Delete(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[name]; !ok {
		return false, nil
	}
	delete(s.data, name)
	return true, s.flush()
}

// flush writes the current state to disk. Must be called with mu held.
func (s *DynamicStore) flush() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dynamic store: %w", err)
	}
	if err := os.WriteFile(s.path, raw, 0o644); err != nil {
		return fmt.Errorf("write dynamic store %s: %w", s.path, err)
	}
	return nil
}
