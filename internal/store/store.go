// Package store provides JSON-file persistence for devices, plugin data and app state.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store persists named collections as JSON files under a data directory.
type Store struct {
	mu  sync.Mutex
	dir string
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string { return s.dir }

// Save writes v as <name>.json atomically (tmp + rename).
func (s *Store) Save(name string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, name+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads <name>.json into v; missing file is not an error (v stays zero).
func (s *Store) Load(name string, v any) error {
	data, err := os.ReadFile(filepath.Join(s.dir, name+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// LoadRaw returns raw bytes of <name>.json (nil when missing).
func (s *Store) LoadRaw(name string) []byte {
	data, err := os.ReadFile(filepath.Join(s.dir, name+".json"))
	if err != nil {
		return nil
	}
	return data
}

func (s *Store) Path(name string) string { return filepath.Join(s.dir, name+".json") }
