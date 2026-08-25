package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// State is the persisted interaction state for the whole feed.
type State struct {
	Votes     map[string]int    `json:"votes"`
	Saved     map[string]bool   `json:"saved"`
	SavedAt   map[string]string `json:"savedAt,omitempty"`
	MemoNames map[string]string `json:"memoNames,omitempty"`
}

func newState() *State {
	return &State{
		Votes:     make(map[string]int),
		Saved:     make(map[string]bool),
		SavedAt:   make(map[string]string),
		MemoNames: make(map[string]string),
	}
}

// Store persists feed interactions (votes, saves) to a JSON file.
// It is safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	path string
	data *State
}

// OpenStore loads the store from path, starting empty if the file
// does not exist yet. The file is created on the first write.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, data: newState()}

	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, s.data); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		s.ensureMaps()
	case errors.Is(err, os.ErrNotExist):
		// Start empty; the file is created on the first write.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) ensureMaps() {
	if s.data.Votes == nil {
		s.data.Votes = make(map[string]int)
	}
	if s.data.Saved == nil {
		s.data.Saved = make(map[string]bool)
	}
	if s.data.SavedAt == nil {
		s.data.SavedAt = make(map[string]string)
	}
	if s.data.MemoNames == nil {
		s.data.MemoNames = make(map[string]string)
	}
}

// Snapshot returns a copy of the current state.
func (s *Store) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return State{
		Votes:     cloneIntMap(s.data.Votes),
		Saved:     cloneBoolMap(s.data.Saved),
		SavedAt:   cloneStrMap(s.data.SavedAt),
		MemoNames: cloneStrMap(s.data.MemoNames),
	}
}

// Vote returns the current vote (-1, 0, 1) for key.
func (s *Store) Vote(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Votes[key]
}

// SetVote records a vote; 0 removes it.
func (s *Store) SetVote(key string, v int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v == 0 {
		delete(s.data.Votes, key)
	} else {
		s.data.Votes[key] = v
	}
	return s.saveLocked()
}

// IsSaved reports whether key is saved.
func (s *Store) IsSaved(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Saved[key]
}

// SetSaved records or removes a save, keeping track of when it happened.
func (s *Store) SetSaved(key string, saved bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if saved {
		s.data.Saved[key] = true
		s.data.SavedAt[key] = time.Now().UTC().Format(time.RFC3339)
	} else {
		delete(s.data.Saved, key)
		delete(s.data.SavedAt, key)
		delete(s.data.MemoNames, key)
	}
	return s.saveLocked()
}

// ClearSaved removes any save state for a key (used when an item is removed).
func (s *Store) ClearSaved(key string) error {
	return s.SetSaved(key, false)
}

// SavedKeys returns the keys of saved items, newest first.
func (s *Store) SavedKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.data.Saved))
	for k := range s.data.Saved {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ti, tj := s.data.SavedAt[keys[i]], s.data.SavedAt[keys[j]]
		if ti != tj {
			return ti > tj
		}
		return keys[i] < keys[j]
	})
	return keys
}

// MemoName returns the Memos resource name created for key, if any.
func (s *Store) MemoName(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.MemoNames[key]
}

// SetMemoName remembers the Memos resource name created for key.
func (s *Store) SetMemoName(key, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.MemoNames[key] = name
	return s.saveLocked()
}

// DeleteMemoName forgets the Memos resource name for key.
func (s *Store) DeleteMemoName(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.MemoNames, key)
	return s.saveLocked()
}

// saveLocked writes the state to disk atomically. Caller must hold s.mu.
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func cloneIntMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneBoolMap(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneStrMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
