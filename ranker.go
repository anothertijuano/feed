package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// trainRate is how strongly a single vote moves the model toward ±1.
const trainRate = 0.15

// Model is the learned relevance model: per-source affinity and per-token
// (title keyword) affinity, each in [-1, 1].
type Model struct {
	Sources   map[string]float64 `json:"sources"`
	Tokens    map[string]float64 `json:"tokens"`
	UpdatedAt string             `json:"updatedAt"`
}

type rankEntry struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
	Title string  `json:"title"`
}

// Ranker is the recommendation algorithm: it learns from votes and keeps
// the feed sorted by relevance. Content the user has already seen or liked
// sinks below fresh content. Run as its own goroutine.
type Ranker struct {
	mu        sync.RWMutex
	items     *ItemStore
	blocked   *BlockStore
	seen      *SeenStore
	votes     *Store
	modelPath string
	rankPath  string
	model     Model
	ranked    []Item
	notify    chan struct{}
	log       *slog.Logger
}

func newRanker(items *ItemStore, blocked *BlockStore, seen *SeenStore, votes *Store, modelPath, rankPath string, notify chan struct{}, log *slog.Logger) (*Ranker, error) {
	r := &Ranker{
		items:     items,
		blocked:   blocked,
		seen:      seen,
		votes:     votes,
		modelPath: modelPath,
		rankPath:  rankPath,
		model: Model{
			Sources: make(map[string]float64),
			Tokens:  make(map[string]float64),
		},
		notify: notify,
		log:    log,
	}
	b, err := os.ReadFile(modelPath)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &r.model); err != nil {
			return nil, fmt.Errorf("parse %s: %w", modelPath, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// Start with an empty model.
	default:
		return nil, fmt.Errorf("read %s: %w", modelPath, err)
	}
	if r.model.Sources == nil {
		r.model.Sources = make(map[string]float64)
	}
	if r.model.Tokens == nil {
		r.model.Tokens = make(map[string]float64)
	}
	return r, nil
}

// Run re-ranks whenever new content or feedback arrives.
func (r *Ranker) Run(ctx context.Context) {
	r.rank()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.notify:
			r.rank()
		}
	}
}

// nudge asks the ranker goroutine to recompute the ordering.
func (r *Ranker) nudge() {
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

// Like records positive feedback and retrains the model.
func (r *Ranker) Like(id string) {
	it, ok := r.items.Get(id)
	if !ok {
		return
	}
	r.train(it, +1)
	r.nudge()
}

// Dislike removes the item, blocks its link/guid so it is never downloaded
// again, and retrains the model with negative feedback.
func (r *Ranker) Dislike(id string) {
	it, ok := r.items.Get(id)
	if !ok {
		return
	}
	_ = r.items.Delete(id)
	if it.Link != "" {
		_ = r.blocked.Add(it.Link)
	}
	if it.GUID != "" {
		_ = r.blocked.Add(it.GUID)
	}
	r.train(it, -1)
	r.nudge()
	r.log.Info("item removed by downvote", "id", id, "title", it.Title)
}

// train moves the model weights toward sign (+1 like, -1 dislike).
func (r *Ranker) train(it Item, sign float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	step := func(m map[string]float64, k string) {
		if k == "" {
			return
		}
		cur := m[k]
		nv := cur + trainRate*(sign-cur)
		if math.Abs(nv) < 0.02 {
			delete(m, k)
		} else {
			m[k] = math.Max(-1, math.Min(1, nv))
		}
	}
	step(r.model.Sources, it.SourceName)
	for _, t := range tokenize(it.Title) {
		step(r.model.Tokens, t)
	}
	r.model.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if b, err := json.MarshalIndent(r.model, "", "  "); err == nil {
		_ = os.MkdirAll(filepath.Dir(r.modelPath), 0o755)
		tmp := r.modelPath + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			_ = os.Rename(tmp, r.modelPath)
		}
	}
}

// score computes the relevance of an item from the current model:
// source affinity + average title-token affinity + a recency boost.
func (r *Ranker) score(it Item) float64 {
	s := 0.0
	if w, ok := r.model.Sources[it.SourceName]; ok {
		s += w
	}
	tokens := tokenize(it.Title)
	if len(tokens) > 0 {
		sum := 0.0
		for _, t := range tokens {
			sum += r.model.Tokens[t]
		}
		s += 0.5 * sum / float64(len(tokens))
	}
	if t, err := time.Parse(time.RFC3339, it.FetchedAt); err == nil {
		age := time.Since(t).Hours()
		if age < 0 {
			age = 0
		}
		s += 0.15 * math.Exp(-age/48) // ~2 day half-life
	}
	return s
}

// rank recomputes the relevance ordering and persists it to rank.json.
// Seen/liked items sink below fresh content.
func (r *Ranker) rank() {
	r.mu.Lock()
	defer r.mu.Unlock()

	type scored struct {
		it Item
		s  float64
	}
	fresh := make([]scored, 0)
	consumed := make([]Item, 0)
	for _, it := range r.items.All() {
		if r.votes.Vote(it.ID) == 1 || r.seen.Has(it.ID) {
			consumed = append(consumed, it)
		} else {
			fresh = append(fresh, scored{it, r.score(it)})
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		if fresh[i].s != fresh[j].s {
			return fresh[i].s > fresh[j].s
		}
		return fresh[i].it.FetchedAt > fresh[j].it.FetchedAt
	})
	sort.SliceStable(consumed, func(i, j int) bool {
		return consumed[i].FetchedAt > consumed[j].FetchedAt
	})

	r.ranked = make([]Item, 0, len(fresh)+len(consumed))
	entries := make([]rankEntry, 0, len(fresh)+len(consumed))
	for _, sc := range fresh {
		r.ranked = append(r.ranked, sc.it)
		entries = append(entries, rankEntry{
			ID:    sc.it.ID,
			Score: math.Round(sc.s*1000) / 1000,
			Title: sc.it.Title,
		})
	}
	for _, it := range consumed {
		r.ranked = append(r.ranked, it)
		entries = append(entries, rankEntry{ID: it.ID, Score: 0, Title: it.Title})
	}

	b, err := json.MarshalIndent(map[string]any{
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
		"fresh":     len(fresh),
		"consumed":  len(consumed),
		"order":     entries,
	}, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(r.rankPath), 0o755)
	tmp := r.rankPath + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, r.rankPath)
	}
}

// Ranked returns a page of the current relevance ordering.
func (r *Ranker) Ranked(offset, limit int) []Item {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if offset < 0 || offset >= len(r.ranked) {
		return nil
	}
	end := offset + limit
	if end > len(r.ranked) {
		end = len(r.ranked)
	}
	out := make([]Item, end-offset)
	copy(out, r.ranked[offset:end])
	return out
}

// Len returns the number of ranked items.
func (r *Ranker) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.ranked)
}

// ScoreOf returns the current relevance score of an item.
func (r *Ranker) ScoreOf(id string) (float64, bool) {
	it, ok := r.items.Get(id)
	if !ok {
		return 0, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.score(it), true
}
