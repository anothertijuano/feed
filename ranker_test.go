package main

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func newTestRanker(t *testing.T) (*Ranker, *ItemStore, *BlockStore, string) {
	t.Helper()
	dir := t.TempDir()
	items, err := OpenItemStore(filepath.Join(dir, "items"))
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := OpenBlockStore(filepath.Join(dir, "blocked.json"))
	if err != nil {
		t.Fatal(err)
	}
	seen, err := OpenSeenStore(filepath.Join(dir, "seen.json"))
	if err != nil {
		t.Fatal(err)
	}
	votes, err := OpenStore(filepath.Join(dir, "feed.json"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := newRanker(items, blocked, seen, votes,
		filepath.Join(dir, "model.json"), filepath.Join(dir, "rank.json"),
		make(chan struct{}, 1), 4, 3, 120*time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return r, items, blocked, dir
}

func putTestItem(t *testing.T, items *ItemStore, id, title, link string) {
	t.Helper()
	err := items.Put(Item{
		ID:         id,
		Title:      title,
		Link:       link,
		SourceName: domainOf(link),
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRankerLikeTrainsAndPersists(t *testing.T) {
	r, items, _, dir := newTestRanker(t)
	putTestItem(t, items, "r1", "Rust 1.87 released with async improvements", "https://blog.rust-lang.org/x")

	r.Like("r1")

	if got := r.model.Sources["blog.rust-lang.org"]; got <= 0 {
		t.Fatalf("source weight = %v, want > 0", got)
	}
	if got := r.model.Tokens["rust"]; got <= 0 {
		t.Fatalf("token weight for rust = %v, want > 0", got)
	}

	// The model must survive a reload.
	reloaded, err := newRanker(items, r.blocked, r.seen, r.votes, filepath.Join(dir, "model.json"), filepath.Join(dir, "rank.json"), make(chan struct{}, 1), 4, 3, 120*time.Hour, r.log)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.model.Tokens["rust"]; got <= 0 {
		t.Fatalf("reloaded token weight = %v, want > 0", got)
	}
}

func TestRankerDislikeRemovesBlocksTrains(t *testing.T) {
	r, items, blocked, _ := newTestRanker(t)
	link := "https://spam.example.com/post"
	putTestItem(t, items, "r1", "Buy cheap crypto now", link)

	r.Dislike("r1")

	if _, ok := items.Get("r1"); ok {
		t.Fatal("item still present after dislike")
	}
	if !blocked.Has(link) {
		t.Fatal("link not blocked after dislike")
	}
	if got := r.model.Sources["spam.example.com"]; got >= 0 {
		t.Fatalf("source weight = %v, want < 0", got)
	}
	if got := r.model.Tokens["crypto"]; got >= 0 {
		t.Fatalf("crypto weight = %v, want < 0", got)
	}
}

func TestRankerSourceDiversity(t *testing.T) {
	r, items, _, _ := newTestRanker(t)

	// Six articles from one source, two from another.
	for i := 1; i <= 6; i++ {
		putTestItem(t, items, "a"+itoa(i), "A article "+itoa(i), "https://source-a.example.com/"+itoa(i))
	}
	putTestItem(t, items, "b1", "B article 1", "https://source-b.example.com/1")
	putTestItem(t, items, "b2", "B article 2", "https://source-b.example.com/2")

	r.rank()
	ranked := r.Ranked(0, 100)
	if len(ranked) != 8 {
		t.Fatalf("ranked len = %d, want 8", len(ranked))
	}

	// In the first 6 positions (two sources × cap 4 = 8 diversified slots,
	// but only 6 A-items + 2 B-items), source A must never appear more
	// than 4 times before the overflow begins.
	countA := 0
	for _, it := range ranked[:6] {
		if it.SourceName == "source-a.example.com" {
			countA++
		}
	}
	if countA > 4 {
		t.Fatalf("source A dominates the top: %+v", ranked)
	}

	// B items must both appear in the top 4 slots.
	foundB := 0
	for _, it := range ranked[:4] {
		if it.SourceName == "source-b.example.com" {
			foundB++
		}
	}
	if foundB < 1 {
		t.Fatalf("source B missing from the top: %+v", ranked)
	}
}

func TestRankerExpiry(t *testing.T) {
	r, items, _, _ := newTestRanker(t)

	// Old item: in the feed for longer than maxAge.
	old := notifItemFor(t, "old-item", "feed.example.com", 6*24*time.Hour)
	old.Subscription = ""
	if err := items.Put(old); err != nil {
		t.Fatal(err)
	}

	// Presented item: seen 3 times without reaction.
	putTestItem(t, items, "tired-item", "Tired of this", "https://feed.example.com/tired")
	for i := 0; i < 3; i++ {
		if err := r.seen.Add("tired-item"); err != nil {
			t.Fatal(err)
		}
		// Age each presentation past the dedupe window.
		r.seen.mu.Lock()
		e := r.seen.counts["tired-item"]
		e.LastAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
		r.seen.counts["tired-item"] = e
		r.seen.mu.Unlock()
	}

	// Reacted item: seen twice, then liked — must survive.
	putTestItem(t, items, "reacted-item", "Reacted to", "https://feed.example.com/reacted")
	_ = r.seen.Add("reacted-item")
	r.seen.mu.Lock()
	e := r.seen.counts["reacted-item"]
	e.LastAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	r.seen.counts["reacted-item"] = e
	r.seen.mu.Unlock()
	_ = r.seen.Add("reacted-item")
	_ = r.votes.SetVote("reacted-item", 1)
	_ = r.seen.Remove("reacted-item")

	// A fresh untouched item.
	putTestItem(t, items, "fresh-item", "Fresh", "https://feed.example.com/fresh")

	r.rank()
	ranked := r.Ranked(0, 100)
	ids := make([]string, 0, len(ranked))
	for _, it := range ranked {
		ids = append(ids, it.ID)
	}
	if contains(ids, "old-item") {
		t.Fatal("aged item was not expired")
	}
	if contains(ids, "tired-item") {
		t.Fatal("over-presented item was not expired")
	}
	if !contains(ids, "reacted-item") {
		t.Fatal("reacted item was expired")
	}
	if ids[0] != "fresh-item" {
		t.Fatalf("top item = %s, want fresh-item", ids[0])
	}
}

func itoa(n int) string {
	return string(rune('0' + n))
}

func notifItemFor(t *testing.T, id, source string, age time.Duration) Item {
	t.Helper()
	return Item{
		ID:         id,
		Title:      "Item " + id,
		Link:       "https://feed.example.com/" + id,
		SourceName: source,
		FetchedAt:  time.Now().Add(-age).UTC().Format(time.RFC3339),
	}
}

func TestRankerSinksSeenAndLiked(t *testing.T) {
	r, items, _, _ := newTestRanker(t)

	putTestItem(t, items, "seen-item", "Seen article", "https://rust.example.com/seen")
	putTestItem(t, items, "liked-item", "Liked article", "https://rust.example.com/liked")
	putTestItem(t, items, "fresh-item", "Fresh article", "https://rust.example.com/fresh")

	// Boost the source so score would normally put them on top.
	r.mu.Lock()
	r.model.Sources["rust.example.com"] = 0.9
	r.mu.Unlock()

	if err := r.seen.Add("seen-item"); err != nil {
		t.Fatal(err)
	}
	if err := r.votes.SetVote("liked-item", 1); err != nil {
		t.Fatal(err)
	}

	r.rank()
	ranked := r.Ranked(0, 10)
	if len(ranked) != 3 {
		t.Fatalf("ranked = %+v", ranked)
	}
	if ranked[0].ID != "fresh-item" {
		t.Fatalf("top item = %s, want fresh-item", ranked[0].ID)
	}
	for _, it := range ranked[1:] {
		if it.ID == "fresh-item" {
			t.Fatalf("fresh item duplicated: %+v", ranked)
		}
	}
}

func TestRankerOrdersByRelevance(t *testing.T) {
	r, items, _, _ := newTestRanker(t)

	// Train the model: like rust content, dislike AI content.
	putTestItem(t, items, "liked", "Rust patterns", "https://rust.example.com/a")
	putTestItem(t, items, "disliked", "AI hype", "https://ai.example.com/b")
	r.Like("liked")
	r.Dislike("disliked")

	// Fresh items from both sources.
	putTestItem(t, items, "fresh-rust", "Rust for beginners", "https://rust.example.com/c")
	putTestItem(t, items, "fresh-ai", "More AI hype", "https://ai.example.com/d")

	r.rank()
	ranked := r.Ranked(0, 10)
	iRust, iAI := -1, -1
	for i, it := range ranked {
		switch it.ID {
		case "fresh-rust":
			iRust = i
		case "fresh-ai":
			iAI = i
		}
	}
	if iRust < 0 || iAI < 0 {
		t.Fatalf("fresh items missing from ranked = %+v", ranked)
	}
	if iRust > iAI {
		t.Fatalf("fresh-rust ranked %d after fresh-ai at %d: %+v", iRust, iAI, ranked)
	}
}
