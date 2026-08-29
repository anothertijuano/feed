package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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
// the feed sorted by relevance. Content the user has seen or liked sinks
// below fresh content; content that has been presented too many times
// without a reaction, or has been in the feed too long, expires and is
// ignored. Run as its own goroutine.
type Ranker struct {
	mu           sync.RWMutex
	items        *ItemStore
	blocked      *BlockStore
	seen         *SeenStore
	votes        *Store
	modelPath    string
	rankPath     string
	model        Model
	ranked       []Item
	notify       chan struct{}
	maxPerSource int
	maxPresents  int
	maxAge       time.Duration
	log          *slog.Logger
}

func newRanker(items *ItemStore, blocked *BlockStore, seen *SeenStore, votes *Store, modelPath, rankPath string, notify chan struct{}, maxPerSource, maxPresents int, maxAge time.Duration, log *slog.Logger) (*Ranker, error) {
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
		notify:       notify,
		maxPerSource: maxPerSource,
		maxPresents:  maxPresents,
		maxAge:       maxAge,
		log:          log,
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
	if t, err := time.Parse(time.RFC3339, it.PublishedAt); err == nil {
		age := time.Since(t).Hours()
		if age < 0 {
			age = 0
		}
		s += 0.15 * math.Exp(-age/48) // ~2 day half-life
	} else if t, err := time.Parse(time.RFC3339, it.FetchedAt); err == nil {
		age := time.Since(t).Hours()
		if age < 0 {
			age = 0
		}
		s += 0.15 * math.Exp(-age/48)
	}
	return s
}

// dayJitter returns a deterministic pseudo-random value in [-0.1, 0.1]
// for an item, stable within a day and different across days. It keeps
// the feed from being a pure function of the model without reshuffling
// mid-session.
func dayJitter(id string) float64 {
	seed := sha256.Sum256([]byte(id + time.Now().UTC().Format("2006-01-02")))
	v := float64(binary.BigEndian.Uint64(seed[:8])) / float64(math.MaxUint64)
	return (v - 0.5) * 0.2
}

// expired reports whether an item should be ignored: presented too many
// times without a reaction, or in the feed for too long.
func (r *Ranker) expired(it Item) bool {
	if r.seen.Count(it.ID) >= r.maxPresents {
		return true
	}
	if t, err := time.Parse(time.RFC3339, it.FetchedAt); err == nil && time.Since(t) > r.maxAge {
		return true
	}
	return false
}

// rank recomputes the relevance ordering and persists it to rank.json.
// Layout: fresh (diversified by source) → revisits → liked content;
// expired items are excluded entirely.
func (r *Ranker) rank() {
	r.mu.Lock()
	defer r.mu.Unlock()

	type scored struct {
		it Item
		s  float64
	}
	fresh := make([]scored, 0)
	revisits := make([]scored, 0)
	consumed := make([]Item, 0)
	for _, it := range r.items.All() {
		if r.expired(it) {
			continue
		}
		if r.votes.Vote(it.ID) == 1 {
			consumed = append(consumed, it)
			continue
		}
		sc := scored{it, r.score(it) + dayJitter(it.ID)}
		if r.seen.Count(it.ID) > 0 {
			revisits = append(revisits, sc)
		} else {
			fresh = append(fresh, sc)
		}
	}

	byScore := func(a, b scored) bool {
		if a.s != b.s {
			return a.s > b.s
		}
		return a.it.FetchedAt > b.it.FetchedAt
	}
	sort.SliceStable(fresh, func(i, j int) bool { return byScore(fresh[i], fresh[j]) })
	sort.SliceStable(revisits, func(i, j int) bool { return byScore(revisits[i], revisits[j]) })
	sort.SliceStable(consumed, func(i, j int) bool {
		return consumed[i].FetchedAt > consumed[j].FetchedAt
	})

	diversify := func(items []scored, maxPerSource int) (diversified, overflow []scored) {
		grouped := make(map[string][]scored)
		var srcOrder []string
		for _, sc := range items {
			g, ok := grouped[sc.it.SourceName]
			if !ok {
				srcOrder = append(srcOrder, sc.it.SourceName)
			}
			grouped[sc.it.SourceName] = append(g, sc)
		}
		for i := 0; i < maxPerSource; i++ {
			for _, src := range srcOrder {
				g := grouped[src]
				if i < len(g) {
					diversified = append(diversified, g[i])
				}
			}
		}
		for _, src := range srcOrder {
			g := grouped[src]
			for i := maxPerSource; i < len(g); i++ {
				overflow = append(overflow, g[i])
			}
		}
		sort.SliceStable(overflow, func(i, j int) bool { return byScore(overflow[i], overflow[j]) })
		return diversified, overflow
	}

	r.ranked = make([]Item, 0, len(fresh)+len(revisits)+len(consumed))
	entries := make([]rankEntry, 0, len(r.ranked))
	appendRanked := func(it Item, s float64) {
		r.ranked = append(r.ranked, it)
		entries = append(entries, rankEntry{
			ID:    it.ID,
			Score: math.Round(s*1000) / 1000,
			Title: it.Title,
		})
	}

	// Diversify: interleave sources so no source leads with more than
	// maxPerSource articles. Overflow of a source lands after the
	// diversified block (still above revisits).
	diversified, overflow := diversify(fresh, r.maxPerSource)
	for _, sc := range diversified {
		appendRanked(sc.it, sc.s)
	}
	for _, sc := range overflow {
		appendRanked(sc.it, sc.s)
	}
	for _, sc := range revisits {
		appendRanked(sc.it, sc.s)
	}
	for _, it := range consumed {
		appendRanked(it, 0)
	}

	b, err := json.MarshalIndent(map[string]any{
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
		"fresh":     len(fresh),
		"revisits":  len(revisits),
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
