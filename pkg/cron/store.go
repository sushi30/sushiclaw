package cron

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store persists jobs to a JSON file.
type Store struct {
	path string
	mu   sync.RWMutex
}

// NewStore creates a Store for the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads jobs from disk.
func (s *Store) Load() ([]Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []Job{}, nil
	}
	if err != nil {
		return nil, err
	}
	var store StoreFile
	if err := json.Unmarshal(data, &store); err == nil && store.Version > 0 {
		return store.Jobs, nil
	}
	var jobs []Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

// Save writes jobs to disk atomically.
func (s *Store) Save(jobs []Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(StoreFile{Version: 1, Jobs: jobs}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
