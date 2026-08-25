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

// SubscriptionStore persists RSS/Atom/JSON feed subscriptions to a plain
// JSON file (a JSON array, one object per feed).
type SubscriptionStore struct {
	mu   sync.RWMutex
	path string
	subs map[string]Subscription
}

func OpenSubscriptionStore(path string) (*SubscriptionStore, error) {
	s := &SubscriptionStore{path: path, subs: make(map[string]Subscription)}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		var list []Subscription
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, sub := range list {
			if sub.ID != "" {
				s.subs[sub.ID] = sub
			}
		}
	case errors.Is(err, os.ErrNotExist):
		// Start empty.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

// All returns all subscriptions, oldest first.
func (s *SubscriptionStore) All() []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		out = append(out, sub)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AddedAt < out[j].AddedAt })
	return out
}

// Get returns the subscription with the given ID.
func (s *SubscriptionStore) Get(id string) (Subscription, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subs[id]
	return sub, ok
}

// GetByURL returns the subscription with the given URL, if any.
func (s *SubscriptionStore) GetByURL(rawURL string) (Subscription, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subs {
		if sub.URL == rawURL {
			return sub, true
		}
	}
	return Subscription{}, false
}

// Add inserts a new subscription.
func (s *SubscriptionStore) Add(sub Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.subs[sub.ID]; exists {
		return fmt.Errorf("subscription %s already exists", sub.ID)
	}
	s.subs[sub.ID] = sub
	return s.saveLocked()
}

// Remove deletes a subscription.
func (s *SubscriptionStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, id)
	return s.saveLocked()
}

// SetTitle updates the feed title detected from the feed itself.
func (s *SubscriptionStore) SetTitle(id, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return nil
	}
	sub.Title = title
	s.subs[id] = sub
	return s.saveLocked()
}

// SetNotify updates the push-notification policy of a subscription.
func (s *SubscriptionStore) SetNotify(id, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return fmt.Errorf("subscription %s not found", id)
	}
	sub.Notify = mode
	s.subs[id] = sub
	return s.saveLocked()
}

// SetETag remembers HTTP caching validators for conditional requests.
func (s *SubscriptionStore) SetETag(id, etag, lastModified string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return nil
	}
	if etag != "" {
		sub.ETag = etag
	}
	if lastModified != "" {
		sub.LastModified = lastModified
	}
	s.subs[id] = sub
	return s.saveLocked()
}

// SetFetchOK records a successful fetch (304 etc.) without new items.
func (s *SubscriptionStore) SetFetchOK(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return nil
	}
	sub.LastFetchedAt = time.Now().UTC().Format(time.RFC3339)
	sub.LastError = ""
	s.subs[id] = sub
	return s.saveLocked()
}

// SetFetchResult records a successful fetch with the number of new items.
func (s *SubscriptionStore) SetFetchResult(id string, count int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return nil
	}
	sub.LastFetchedAt = time.Now().UTC().Format(time.RFC3339)
	sub.ItemCount += count
	sub.LastError = ""
	s.subs[id] = sub
	return s.saveLocked()
}

// SetFetchError records a failed fetch.
func (s *SubscriptionStore) SetFetchError(id, msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return nil
	}
	sub.LastError = msg
	s.subs[id] = sub
	return s.saveLocked()
}

// saveLocked writes the subscription list as a JSON array. Caller must
// hold s.mu.
func (s *SubscriptionStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	list := make([]Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		list = append(list, sub)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].AddedAt < list[j].AddedAt })
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
