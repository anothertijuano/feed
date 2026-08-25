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

// SettingsStore persists user settings to a plain JSON file.
type SettingsStore struct {
	mu   sync.RWMutex
	path string
	data Settings
}

func OpenSettingsStore(path string) (*SettingsStore, error) {
	s := &SettingsStore{path: path}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// Start with empty settings.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

// Get returns a copy of the current settings.
func (s *SettingsStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// Update replaces the settings.
func (s *SettingsStore) Update(next Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = next
	return s.saveLocked()
}

// SetMemoStatus records the outcome of a Memos sync attempt.
func (s *SettingsStore) SetMemoStatus(syncAt time.Time, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !syncAt.IsZero() {
		s.data.MemoLastSyncAt = syncAt.UTC().Format(time.RFC3339)
	}
	s.data.MemoLastError = errMsg
	return s.saveLocked()
}

func (s *SettingsStore) saveLocked() error {
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
