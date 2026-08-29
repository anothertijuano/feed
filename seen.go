package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// seenDedupeWindow: repeated "seen" reports for the same item within this
// window count as a single presentation (refresh-safe, multi-device-safe).
const seenDedupeWindow = time.Hour

// SeenStore counts how often each item was presented to the user. Items
// presented often enough without any reaction (vote/save) expire and are
// ignored by the ranker. A reaction resets the counter.
type SeenStore struct {
	mu     sync.RWMutex
	path   string
	counts map[string]seenEntry
}

type seenEntry struct {
	Count  int    `json:"count"`
	LastAt string `json:"lastAt"`
}

func OpenSeenStore(path string) (*SeenStore, error) {
	s := &SeenStore{path: path, counts: make(map[string]seenEntry)}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		// Current format: {"<id>": {"count": n, "lastAt": "…"}}
		if err := json.Unmarshal(b, &s.counts); err == nil {
			break
		}
		// Legacy format: ["<id>", …] — treat as one presentation each.
		var legacy []string
		if err := json.Unmarshal(b, &legacy); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		s.counts = make(map[string]seenEntry, len(legacy))
		for _, id := range legacy {
			s.counts[id] = seenEntry{Count: 1, LastAt: time.Now().UTC().Format(time.RFC3339)}
		}
	case errors.Is(err, os.ErrNotExist):
		// Start empty.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

// Count returns how many times the item was presented.
func (s *SeenStore) Count(id string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.counts[id].Count
}

// Add records a presentation. Reports within the dedupe window of the
// previous one are ignored, so refreshes and multiple devices don't burn
// through the presentation budget.
func (s *SeenStore) Add(id string) error {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.counts[id]
	if ok {
		if t, err := time.Parse(time.RFC3339, cur.LastAt); err == nil && now.Sub(t) < seenDedupeWindow {
			return nil
		}
	}
	s.counts[id] = seenEntry{Count: cur.Count + 1, LastAt: now.Format(time.RFC3339)}
	return s.saveLocked()
}

// Remove resets the presentation counter (a reaction: vote or save).
func (s *SeenStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.counts[id]; !ok {
		return nil
	}
	delete(s.counts, id)
	return s.saveLocked()
}

func (s *SeenStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.counts, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
