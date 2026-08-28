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
		make(chan struct{}, 1), slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	reloaded, err := newRanker(items, r.blocked, r.seen, r.votes, filepath.Join(dir, "model.json"), filepath.Join(dir, "rank.json"), make(chan struct{}, 1), r.log)
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
