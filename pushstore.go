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

// PushStore persists the registered Web Push subscriptions
// (data/push.json).
type PushStore struct {
	mu   sync.RWMutex
	path string
	subs []PushSub
}

func OpenPushStore(path string) (*PushStore, error) {
	s := &PushStore{path: path, subs: []PushSub{}}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &s.subs); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// Start empty.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

// All returns a copy of the registered subscriptions.
func (s *PushStore) All() []PushSub {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PushSub, len(s.subs))
	copy(out, s.subs)
	return out
}

// Add registers (or refreshes) a subscription, deduplicated by endpoint.
func (s *PushStore) Add(sub PushSub) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	for i, cur := range s.subs {
		if cur.Endpoint == sub.Endpoint {
			s.subs[i] = sub
			return s.saveLocked()
		}
	}
	s.subs = append(s.subs, sub)
	return s.saveLocked()
}

// Remove deletes a subscription by endpoint.
func (s *PushStore) Remove(endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, cur := range s.subs {
		if cur.Endpoint == endpoint {
			s.subs = append(s.subs[:i], s.subs[i+1:]...)
			return s.saveLocked()
		}
	}
	return nil
}

func (s *PushStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subs)
}

func (s *PushStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.subs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
