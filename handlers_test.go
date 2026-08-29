package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestApp builds the full application against a temp data dir and starts
// the ranker + extractor goroutines.
func newTestApp(t *testing.T) (*api, context.CancelFunc) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := newAPI(appOptions{
		addr:    ":0",
		dataDir: t.TempDir(),
		refresh: time.Hour,
		log:     logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 2)
	for _, run := range []func(context.Context){a.ranker.Run, a.extractor.Run} {
		go func(run func(context.Context)) {
			defer func() { done <- struct{}{} }()
			run(ctx)
		}(run)
	}
	return a, func() {
		cancel()
		<-done
		<-done
	}
}

func doJSON(t *testing.T, mux http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeResponse[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return out
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestHealth(t *testing.T) {
	a, cancel := newTestApp(t)
	defer cancel()
	rec := doJSON(t, a.routes(), http.MethodGet, "/api/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// addFeedItem stores an item directly (as the extractor would) and waits
// for the ranker to pick it up.
func addFeedItem(t *testing.T, a *api, id, title, link string) Item {
	t.Helper()
	it := Item{
		ID:         id,
		Title:      title,
		Link:       link,
		SourceName: domainOf(link),
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if err := a.items.Put(it); err != nil {
		t.Fatal(err)
	}
	a.ranker.nudge()
	waitFor(t, 3*time.Second, func() bool {
		for _, ranked := range a.ranker.Ranked(0, 1000) {
			if ranked.ID == id {
				return true
			}
		}
		return false
	})
	return it
}

func TestFeedEmptyInitially(t *testing.T) {
	a, cancel := newTestApp(t)
	defer cancel()

	rec := doJSON(t, a.routes(), http.MethodGet, "/api/feed?limit=50", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Items []viewItem `json:"items"`
		Total int        `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 0 || got.Total != 0 {
		t.Fatal("expected an empty feed without subscriptions")
	}
}

func TestLikeUnvote(t *testing.T) {
	a, cancel := newTestApp(t)
	defer cancel()
	addFeedItem(t, a, "item-01", "Xiaomi test", "https://x.com/status/1")

	rec := doJSON(t, a.routes(), http.MethodPost, "/api/interactions",
		`{"key":"item-01","kind":"vote","value":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	if got := a.store.Vote("item-01"); got != 1 {
		t.Fatalf("vote = %d, want 1", got)
	}

	rec = doJSON(t, a.routes(), http.MethodPost, "/api/interactions",
		`{"key":"item-01","kind":"vote","value":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	if got := a.store.Vote("item-01"); got != 0 {
		t.Fatalf("vote = %d, want 0", got)
	}
}

func TestDownvoteRemovesAndBlocks(t *testing.T) {
	a, cancel := newTestApp(t)
	defer cancel()
	it := addFeedItem(t, a, "item-01", "Block me please", "https://spam.example.com/post")
	before := a.ranker.Len()

	rec := doJSON(t, a.routes(), http.MethodPost, "/api/interactions",
		`{"key":"item-01","kind":"vote","value":-1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	waitFor(t, 2*time.Second, func() bool { return a.ranker.Len() == before-1 })
	if _, ok := a.items.Get(it.ID); ok {
		t.Fatal("item still in store after downvote")
	}
	if !a.blocked.Has(it.Link) {
		t.Fatalf("link %q not blocked", it.Link)
	}
}

func TestSavedFlow(t *testing.T) {
	a, cancel := newTestApp(t)
	defer cancel()
	addFeedItem(t, a, "item-02", "Saved test", "https://example.com/saved")

	rec := doJSON(t, a.routes(), http.MethodPost, "/api/interactions",
		`{"key":"item-02","kind":"save","value":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	rec = doJSON(t, a.routes(), http.MethodGet, "/api/saved", "")
	var got struct {
		Items []viewItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "item-02" || !got.Items[0].Saved {
		t.Fatalf("saved items = %+v", got.Items)
	}

	rec = doJSON(t, a.routes(), http.MethodPost, "/api/interactions",
		`{"key":"item-02","kind":"save","value":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	rec = doJSON(t, a.routes(), http.MethodGet, "/api/saved", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Items) != 0 {
		t.Fatalf("expected empty saved list, got %+v", got.Items)
	}
}

func TestSettingsRoundtrip(t *testing.T) {
	a, cancel := newTestApp(t)
	defer cancel()

	rec := doJSON(t, a.routes(), http.MethodPost, "/api/settings",
		`{"memosUrl":"https://memo.example.com/","memosToken":"tok123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	rec = doJSON(t, a.routes(), http.MethodGet, "/api/settings", "")
	s := decodeResponse[Settings](t, rec)
	if s.MemosURL != "https://memo.example.com" || s.MemosToken != "tok123" {
		t.Fatalf("settings = %+v", s)
	}

	rec = doJSON(t, a.routes(), http.MethodPost, "/api/settings",
		`{"memosUrl":"not a url","memosToken":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

const testRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>First post</title>
      <link>https://blog.example.com/first-post</link>
      <guid>https://blog.example.com/first-post</guid>
      <pubDate>Mon, 01 Jun 2026 12:00:00 GMT</pubDate>
      <description><![CDATA[<p>Hello <b>world</b>!</p>]]></description>
      <enclosure url="https://blog.example.com/img.jpg" type="image/jpeg"/>
    </item>
    <item>
      <title>Second post</title>
      <link>https://blog.example.com/second-post</link>
      <guid>guid-two</guid>
      <description>Just text.</description>
    </item>
  </channel>
</rss>`

func TestSubscriptionLifecycle(t *testing.T) {
	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, testRSS)
	}))
	defer feedServer.Close()

	a, cancel := newTestApp(t)
	defer cancel()

	rec := doJSON(t, a.routes(), http.MethodPost, "/api/subscriptions",
		`{"url":"`+feedServer.URL+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	sub := decodeResponse[Subscription](t, rec)
	if sub.Title != "Test Feed" || sub.ID == "" {
		t.Fatalf("sub = %+v", sub)
	}

	waitFor(t, 3*time.Second, func() bool {
		for _, it := range a.items.All() {
			if it.Subscription == sub.ID {
				return true
			}
		}
		return false
	})

	// The new items must be ranked and served.
	waitFor(t, 3*time.Second, func() bool {
		rec := doJSON(t, a.routes(), http.MethodGet, "/api/feed?limit=100", "")
		var feed struct {
			Items []viewItem `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
			return false
		}
		for _, it := range feed.Items {
			if it.ID == "r"+shortHash("https://blog.example.com/first-post") {
				return true
			}
		}
		return false
	})

	// Removing the subscription removes its content.
	rec = doJSON(t, a.routes(), http.MethodDelete, "/api/subscriptions/"+sub.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	waitFor(t, 3*time.Second, func() bool {
		for _, it := range a.items.All() {
			if it.Subscription == sub.ID {
				return false
			}
		}
		return true
	})
}

func TestRefreshAddsNewItems(t *testing.T) {
	var mu sync.Mutex
	posts := 1
	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := posts
		mu.Unlock()
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>Refresh Feed</title>`)
		for i := 1; i <= n; i++ {
			fmt.Fprintf(&b, `<item><title>Post %d</title><link>https://refresh.example.com/post-%d</link><guid>guid-%d</guid></item>`, i, i, i)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = io.WriteString(w, b.String())
	}))
	defer feedServer.Close()

	a, cancel := newTestApp(t)
	defer cancel()

	rec := doJSON(t, a.routes(), http.MethodPost, "/api/subscriptions", `{"url":"`+feedServer.URL+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	// Wait for the initial ingest (post 1) to land.
	id1 := "r" + shortHash("guid-1")
	waitFor(t, 3*time.Second, func() bool {
		_, ok := a.items.Get(id1)
		return ok
	})

	// The source publishes a second post.
	mu.Lock()
	posts = 2
	mu.Unlock()

	rec = doJSON(t, a.routes(), http.MethodPost, "/api/refresh", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		New int `json:"new"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.New != 1 {
		t.Fatalf("new = %d, want 1", resp.New)
	}
	if _, ok := a.items.Get("r" + shortHash("guid-2")); !ok {
		t.Fatal("post 2 was not ingested")
	}
}

func TestMemosSendOnSaveAndDelete(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath, gotAuth, gotBody string
	memosSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"name":"memos/42","content":"x","visibility":"PRIVATE"}`)
		} else {
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer memosSrv.Close()

	a, cancel := newTestApp(t)
	defer cancel()
	addFeedItem(t, a, "item-01", "Xiaomi sync test", "https://x.com/status/1")

	rec := doJSON(t, a.routes(), http.MethodPost, "/api/settings",
		fmt.Sprintf(`{"memosUrl":%q,"memosToken":"tok123"}`, memosSrv.URL))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	// Saving an item must POST a memo to /api/v1/memos.
	rec = doJSON(t, a.routes(), http.MethodPost, "/api/interactions",
		`{"key":"item-01","kind":"save","value":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	waitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return gotMethod != ""
	})

	mu.Lock()
	method, path, auth, body := gotMethod, gotPath, gotAuth, gotBody
	mu.Unlock()
	if method != http.MethodPost || path != "/api/v1/memos" {
		t.Fatalf("request = %s %s, want POST /api/v1/memos", method, path)
	}
	if auth != "Bearer tok123" {
		t.Fatalf("auth = %q, want Bearer tok123", auth)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["visibility"] != "PRIVATE" {
		t.Fatalf("visibility = %v", payload["visibility"])
	}
	if !strings.Contains(payload["content"].(string), "Xiaomi") {
		t.Fatalf("content = %q", payload["content"])
	}

	// The memo resource name must be remembered…
	waitFor(t, 3*time.Second, func() bool { return a.store.MemoName("item-01") == "memos/42" })

	// …and unsaving must DELETE /api/v1/memos/42.
	rec = doJSON(t, a.routes(), http.MethodPost, "/api/interactions",
		`{"key":"item-01","kind":"save","value":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	waitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return gotMethod == http.MethodDelete
	})
	mu.Lock()
	method, path = gotMethod, gotPath
	mu.Unlock()
	if method != http.MethodDelete || path != "/api/v1/memos/42" {
		t.Fatalf("request = %s %s, want DELETE /api/v1/memos/42", method, path)
	}

	// Join the async memo goroutines so they can't write into the temp
	// data dir while the test is cleaning up.
	a.memos.Wait()
}

func TestSubscriptionNotifyPolicy(t *testing.T) {
	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, testRSS)
	}))
	defer feedServer.Close()

	a, cancel := newTestApp(t)
	defer cancel()

	rec := doJSON(t, a.routes(), http.MethodPost, "/api/subscriptions", `{"url":"`+feedServer.URL+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	sub := decodeResponse[Subscription](t, rec)

	rec = doJSON(t, a.routes(), http.MethodPost, "/api/subscriptions/"+sub.ID, `{"notify":"always"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	if got := decodeResponse[Subscription](t, rec); got.Notify != "always" {
		t.Fatalf("notify = %q, want always", got.Notify)
	}

	rec = doJSON(t, a.routes(), http.MethodPost, "/api/subscriptions/"+sub.ID, `{"notify":"bogus"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy: status = %d, want 400", rec.Code)
	}

	rec = doJSON(t, a.routes(), http.MethodPost, "/api/subscriptions/nope", `{"notify":"always"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: status = %d, want 404", rec.Code)
	}
}

func TestTokenAPI(t *testing.T) {
	a, cancel := newTestApp(t)
	defer cancel()

	// With no auth configured the API is open.
	rec := doJSON(t, a.routes(), http.MethodGet, "/api/feed", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (open)", rec.Code)
	}

	// Create a token through the API (still open until tokens exist).
	rec = doJSON(t, a.routes(), http.MethodPost, "/api/tokens", `{"name":"iphone"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	var created struct {
		Token string `json:"token"`
		ID    string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}

	// Now that a token exists, auth is enforced.
	wrapped := a.wrappedRoutes()
	rec = doJSON(t, wrapped, http.MethodGet, "/api/feed", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 after token created", rec.Code)
	}

	authed := func(path string, token string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		out := httptest.NewRecorder()
		wrapped.ServeHTTP(out, req)
		return out.Code
	}
	if got := authed("/api/feed", created.Token); got != http.StatusOK {
		t.Fatalf("authed status = %d, want 200", got)
	}

	// Revoke it → 401 again.
	revoke := httptest.NewRequest(http.MethodDelete, "/api/tokens/"+created.ID, nil)
	revoke.Header.Set("Authorization", "Bearer "+created.Token)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, revoke)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d; body: %s", rec.Code, rec.Body)
	}
	if got := authed("/api/feed", created.Token); got != http.StatusUnauthorized {
		t.Fatalf("revoked status = %d, want 401", got)
	}
}

func TestSeenSinksItem(t *testing.T) {
	a, cancel := newTestApp(t)
	defer cancel()

	addFeedItem(t, a, "old-item", "Old article", "https://example.com/old")
	addFeedItem(t, a, "new-item", "New article", "https://example.com/new")

	// Mark the old one as seen.
	rec := doJSON(t, a.routes(), http.MethodPost, "/api/interactions",
		`{"key":"old-item","kind":"seen","value":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	waitFor(t, 3*time.Second, func() bool {
		ranked := a.ranker.Ranked(0, 10)
		return len(ranked) > 0 && ranked[0].ID == "new-item"
	})
}

func TestMigrateFeedEntryTimes(t *testing.T) {
	dir := t.TempDir()
	items, err := OpenItemStore(filepath.Join(dir, "items"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := Item{
		ID:        "r-legacy",
		Title:     "Legacy",
		Link:      "https://example.com/legacy",
		FetchedAt: "2026-01-01T00:00:00Z", // old pubDate, no PublishedAt
	}
	if err := items.Put(legacy); err != nil {
		t.Fatal(err)
	}

	if err := migrateFeedEntryTimes(items, dir, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}

	got, ok := items.Get("r-legacy")
	if !ok {
		t.Fatal("item missing")
	}
	if got.PublishedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("PublishedAt = %q, want old FetchedAt", got.PublishedAt)
	}
	if got.FetchedAt == "2026-01-01T00:00:00Z" {
		t.Fatal("FetchedAt was not restarted")
	}

	// A second run must be a no-op.
	again, _ := items.Get("r-legacy")
	if err := migrateFeedEntryTimes(items, dir, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	after, _ := items.Get("r-legacy")
	if again.FetchedAt != after.FetchedAt {
		t.Fatal("migration ran twice")
	}
}

func TestPostInvalid(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"bad json", `{`},
		{"bad kind", `{"key":"seed-01","kind":"meh","value":1}`},
		{"bad vote value", `{"key":"seed-01","kind":"vote","value":5}`},
		{"bad save value", `{"key":"seed-01","kind":"save","value":"yes"}`},
		{"bad key", `{"key":"../etc/passwd","kind":"vote","value":1}`},
		{"unknown field", `{"key":"seed-01","kind":"vote","value":1,"extra":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, cancel := newTestApp(t)
			defer cancel()
			rec := doJSON(t, a.routes(), http.MethodPost, "/api/interactions", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
			}
		})
	}
}
