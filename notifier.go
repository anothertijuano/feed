package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

/* The notifier decides whether a new item deserves a push notification
   and delivers it to all registered Web Push subscriptions. Run as its
   own goroutine. */

// NotifiedStore remembers which item IDs were already pushed, so nothing
// is ever notified twice.
type NotifiedStore struct {
	mu    sync.RWMutex
	path  string
	order []string
	set   map[string]bool
}

func OpenNotifiedStore(path string) (*NotifiedStore, error) {
	s := &NotifiedStore{path: path, set: make(map[string]bool)}
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

func (s *NotifiedStore) Has(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.set[id]
}

func (s *NotifiedStore) Add(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.set[id] {
		return nil
	}
	s.set[id] = true
	s.order = append(s.order, id)
	// Keep the file bounded.
	if len(s.order) > 500 {
		drop := s.order[:len(s.order)-500]
		s.order = s.order[len(s.order)-500:]
		for _, d := range drop {
			delete(s.set, d)
		}
	}
	return s.saveLocked()
}

func (s *NotifiedStore) saveLocked() error {
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

// Notifier applies the notification policy and delivers push messages.
type Notifier struct {
	in        chan Item
	subs      *SubscriptionStore
	ranker    *Ranker
	push      *PushStore
	notified  *NotifiedStore
	pusher    *WebPusher
	threshold float64
	maxAge    time.Duration
	log       *slog.Logger
}

func newNotifier(subs *SubscriptionStore, ranker *Ranker, push *PushStore, notified *NotifiedStore, pusher *WebPusher, threshold float64, maxAge time.Duration, log *slog.Logger) *Notifier {
	return &Notifier{
		in:        make(chan Item, 64),
		subs:      subs,
		ranker:    ranker,
		push:      push,
		notified:  notified,
		pusher:    pusher,
		threshold: threshold,
		maxAge:    maxAge,
		log:       log,
	}
}

// Notify queues a freshly ingested item for consideration. Non-blocking.
func (n *Notifier) Notify(it Item) {
	select {
	case n.in <- it:
	default:
		n.log.Warn("notifier busy, dropping item", "id", it.ID)
	}
}

// channel returns the channel the extractor sends new items on.
func (n *Notifier) channel() chan<- Item { return n.in }

func (n *Notifier) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case it := <-n.in:
			n.handle(it)
		}
	}
}

func (n *Notifier) handle(it Item) {
	// Never notify about ancient content (e.g. the first fetch of a feed).
	if t, err := time.Parse(time.RFC3339, it.FetchedAt); err == nil {
		if time.Since(t) > n.maxAge {
			return
		}
	}
	if n.notified.Has(it.ID) {
		return
	}

	// Per-source policy: never / always / default (rank-based).
	policy := "default"
	if sub, ok := n.subs.Get(it.Subscription); ok && sub.Notify != "" {
		policy = sub.Notify
	}
	switch policy {
	case "never":
		return
	case "always":
		// Send regardless of rank.
	case "default":
		score, ok := n.ranker.ScoreOf(it.ID)
		if !ok || score < n.threshold {
			return
		}
	default:
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"title": it.Title,
		"body":  pushBody(it),
		"url":   it.Link,
	})

	sent := false
	for _, sub := range n.push.All() {
		if err := n.pusher.Send(sub, payload, 3600); err != nil {
			if gone, ok := err.(*pushDeliveryError); ok && gone.Gone() {
				_ = n.push.Remove(sub.Endpoint)
				n.log.Info("push subscription pruned", "endpoint", sub.Endpoint)
				continue
			}
			n.log.Warn("push send failed", "endpoint", sub.Endpoint, "err", err)
			continue
		}
		sent = true
	}

	if sent {
		_ = n.notified.Add(it.ID)
	}
	n.log.Info("notification", "id", it.ID, "policy", policy, "sent", sent)
}

func pushBody(it Item) string {
	if len(it.Paragraphs) > 0 {
		return it.Paragraphs[0]
	}
	if it.SourceName != "" {
		return it.SourceName
	}
	return ""
}
