package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestNotifier(t *testing.T) (*Notifier, *SubscriptionStore, *Ranker, *ItemStore, func() int) {
	t.Helper()

	var mu sync.Mutex
	received := 0
	pushSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		mu.Lock()
		received++
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(pushSrv.Close)

	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	items, err := OpenItemStore(filepath.Join(dir, "items"))
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := OpenBlockStore(filepath.Join(dir, "blocked.json"))
	if err != nil {
		t.Fatal(err)
	}
	ranker, err := newRanker(items, blocked,
		filepath.Join(dir, "model.json"), filepath.Join(dir, "rank.json"),
		make(chan struct{}, 1), logger)
	if err != nil {
		t.Fatal(err)
	}
	subs, err := OpenSubscriptionStore(filepath.Join(dir, "subscriptions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := subs.Add(Subscription{
		ID: "s1", URL: "https://feed.example.com/rss",
		AddedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	push, err := OpenPushStore(filepath.Join(dir, "push.json"))
	if err != nil {
		t.Fatal(err)
	}
	subPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	sub := PushSub{Endpoint: pushSrv.URL}
	sub.Keys.P256dh = b64u(subPriv.PublicKey().Bytes())
	sub.Keys.Auth = b64u(auth)
	if err := push.Add(sub); err != nil {
		t.Fatal(err)
	}

	notified, err := OpenNotifiedStore(filepath.Join(dir, "notified.json"))
	if err != nil {
		t.Fatal(err)
	}
	vapid, err := loadOrCreateVAPID(filepath.Join(dir, "vapid.json"), "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	pusher := newWebPusher(vapid, &http.Client{Timeout: 5 * time.Second}, logger)
	n := newNotifier(subs, ranker, push, notified, pusher, 0.3, 48*time.Hour, logger)

	ctx, cancel := context.WithCancel(context.Background())
	go n.Run(ctx)
	t.Cleanup(cancel)

	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return received
	}
	return n, subs, ranker, items, count
}

func notifItem(t *testing.T, items *ItemStore, id, source string, age time.Duration) Item {
	t.Helper()
	it := Item{
		ID:           id,
		Title:        "Hello rust world",
		Link:         "https://feed.example.com/" + id,
		SourceName:   source,
		Subscription: "s1",
		FetchedAt:    time.Now().Add(-age).UTC().Format(time.RFC3339),
	}
	if err := items.Put(it); err != nil {
		t.Fatal(err)
	}
	return it
}

func TestNotifierPolicies(t *testing.T) {
	n, subs, ranker, items, count := newTestNotifier(t)

	// "always" sends even with a low rank.
	if err := subs.SetNotify("s1", "always"); err != nil {
		t.Fatal(err)
	}
	n.Notify(notifItem(t, items, "a1", "feed.example.com", 0))
	waitFor(t, 3*time.Second, func() bool { return count() == 1 })

	// "never" never sends.
	if err := subs.SetNotify("s1", "never"); err != nil {
		t.Fatal(err)
	}
	n.Notify(notifItem(t, items, "a2", "feed.example.com", 0))
	time.Sleep(150 * time.Millisecond)
	if count() != 1 {
		t.Fatalf("never-policy sent a notification (count=%d)", count())
	}

	// "default" with a low rank does not send.
	if err := subs.SetNotify("s1", "default"); err != nil {
		t.Fatal(err)
	}
	n.Notify(notifItem(t, items, "a3", "feed.example.com", 0))
	time.Sleep(150 * time.Millisecond)
	if count() != 1 {
		t.Fatalf("low-rank item sent a notification (count=%d)", count())
	}

	// "default" with a high rank sends.
	ranker.mu.Lock()
	ranker.model.Sources["feed.example.com"] = 0.9
	ranker.mu.Unlock()
	n.Notify(notifItem(t, items, "a4", "feed.example.com", 0))
	waitFor(t, 3*time.Second, func() bool { return count() == 2 })

	// Old items are never notified, even with "always".
	if err := subs.SetNotify("s1", "always"); err != nil {
		t.Fatal(err)
	}
	n.Notify(notifItem(t, items, "a5", "feed.example.com", 72*time.Hour))
	time.Sleep(150 * time.Millisecond)
	if count() != 2 {
		t.Fatalf("old item sent a notification (count=%d)", count())
	}

	// An item is never notified twice.
	n.Notify(notifItem(t, items, "a4", "feed.example.com", 0))
	time.Sleep(150 * time.Millisecond)
	if count() != 2 {
		t.Fatalf("duplicate notification (count=%d)", count())
	}
}
