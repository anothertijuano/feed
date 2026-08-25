package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// BlockStore remembers links/guids of downvoted content so the fetcher
// never downloads them again.
type BlockStore struct {
	mu    sync.RWMutex
	path  string
	items map[string]bool
}

func OpenBlockStore(path string) (*BlockStore, error) {
	s := &BlockStore{path: path, items: make(map[string]bool)}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		var list []string
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, u := range list {
			s.items[u] = true
		}
	case errors.Is(err, os.ErrNotExist):
		// Start empty.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

// Has reports whether the URL/guid is blocked.
func (s *BlockStore) Has(u string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items[u]
}

// Add blocks a URL/guid. No-op if it is already blocked.
func (s *BlockStore) Add(u string) error {
	if u == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items[u] {
		return nil
	}
	s.items[u] = true
	return s.saveLocked()
}

// All returns all blocked entries, sorted.
func (s *BlockStore) All() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.items))
	for u := range s.items {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

func (s *BlockStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	list := make([]string, 0, len(s.items))
	for u := range s.items {
		list = append(list, u)
	}
	sort.Strings(list)
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
