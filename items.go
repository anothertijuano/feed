package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ItemStore keeps content items in memory and mirrors each one to a
// plain JSON file (data/items/<id>.json), consumable by other utilities.
type ItemStore struct {
	mu    sync.RWMutex
	dir   string
	items map[string]Item
}

func OpenItemStore(dir string) (*ItemStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &ItemStore{dir: dir, items: make(map[string]Item)}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var it Item
		if err := json.Unmarshal(b, &it); err != nil {
			continue
		}
		if it.ID == "" {
			it.ID = strings.TrimSuffix(name, ".json")
		}
		s.items[it.ID] = it
	}
	return s, nil
}

// Put stores an item and writes its JSON file.
func (s *ItemStore) Put(it Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.putLocked(it)
}

func (s *ItemStore) putLocked(it Item) error {
	s.items[it.ID] = it
	b, err := json.MarshalIndent(it, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, it.ID+".json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, it.ID+".json"))
}

// Get returns the item with the given ID.
func (s *ItemStore) Get(id string) (Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.items[id]
	return it, ok
}

// Delete removes an item and its file.
func (s *ItemStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return nil
	}
	delete(s.items, id)
	return os.Remove(filepath.Join(s.dir, id+".json"))
}

// DeleteBySubscription removes all items belonging to a subscription and
// returns how many were removed.
func (s *ItemStore) DeleteBySubscription(sub string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id, it := range s.items {
		if it.Subscription == sub {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		delete(s.items, id)
		if err := os.Remove(filepath.Join(s.dir, id+".json")); err != nil && !os.IsNotExist(err) {
			return len(ids), err
		}
	}
	return len(ids), nil
}

// All returns a copy of all items, newest first.
func (s *ItemStore) All() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Item, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FetchedAt > out[j].FetchedAt })
	return out
}

// Len returns the number of stored items.
func (s *ItemStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// String implements fmt.Stringer for debugging.
func (s *ItemStore) String() string {
	return fmt.Sprintf("ItemStore(%d items)", s.Len())
}
