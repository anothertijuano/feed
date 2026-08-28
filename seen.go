package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SeenStore remembers which item IDs the user has already seen in the
// feed, so the ranker can sink consumed content below fresh content.
type SeenStore struct {
	mu    sync.RWMutex
	path  string
	order []string
	set   map[string]bool
}

func OpenSeenStore(path string) (*SeenStore, error) {
	s := &SeenStore{path: path, set: make(map[string]bool)}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		var list []string
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, id := range list {
			s.set[id] = true
		}
		s.order = list
	case errors.Is(err, os.ErrNotExist):
		// Start empty.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

// Has reports whether the item was seen.
func (s *SeenStore) Has(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.set[id]
}

// Add marks an item as seen.
func (s *SeenStore) Add(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.set[id] {
		return nil
	}
	s.set[id] = true
	s.order = append(s.order, id)
	// Keep the file bounded.
	if len(s.order) > 10000 {
		drop := s.order[:len(s.order)-10000]
		s.order = s.order[len(s.order)-10000:]
		for _, d := range drop {
			delete(s.set, d)
		}
	}
	return s.saveLocked()
}

// Remove un-marks an item (e.g. if an un-see is ever needed).
func (s *SeenStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.set[id] {
		return nil
	}
	delete(s.set, id)
	out := s.order[:0]
	for _, cur := range s.order {
		if cur != id {
			out = append(out, cur)
		}
	}
	s.order = out
	return s.saveLocked()
}

func (s *SeenStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.order, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
