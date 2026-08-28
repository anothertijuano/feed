package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type appOptions struct {
	addr          string
	dataDir       string
	refresh       time.Duration
	htpasswd      string
	pushThreshold float64
	notifyAge     time.Duration
	vapidSubject  string
	log           *slog.Logger
}

// newAPI builds the whole application: stores, workers and handlers.
func newAPI(opts appOptions) (*api, error) {
	dir := opts.dataDir
	if err := os.MkdirAll(filepath.Join(dir, "items"), 0o755); err != nil {
		return nil, err
	}

	store, err := OpenStore(filepath.Join(dir, "feed.json"))
	if err != nil {
		return nil, err
	}
	items, err := OpenItemStore(filepath.Join(dir, "items"))
	if err != nil {
		return nil, err
	}
	subs, err := OpenSubscriptionStore(filepath.Join(dir, "subscriptions.json"))
	if err != nil {
		return nil, err
	}
	settings, err := OpenSettingsStore(filepath.Join(dir, "settings.json"))
	if err != nil {
		return nil, err
	}
	blocked, err := OpenBlockStore(filepath.Join(dir, "blocked.json"))
	if err != nil {
		return nil, err
	}
	push, err := OpenPushStore(filepath.Join(dir, "push.json"))
	if err != nil {
		return nil, err
	}
	tokens, err := OpenTokenStore(filepath.Join(dir, "tokens.json"))
	if err != nil {
		return nil, err
	}
	seen, err := OpenSeenStore(filepath.Join(dir, "seen.json"))
	if err != nil {
		return nil, err
	}
	notified, err := OpenNotifiedStore(filepath.Join(dir, "notified.json"))
	if err != nil {
		return nil, err
	}
	vapid, err := loadOrCreateVAPID(filepath.Join(dir, "vapid.json"), opts.vapidSubject)
	if err != nil {
		return nil, err
	}

	var ht *Htpasswd
	if opts.htpasswd != "" {
		ht, err = LoadHtpasswd(opts.htpasswd)
		if err != nil {
			return nil, err
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}

	extractCh := make(chan FetchResult, 32)
	notifyCh := make(chan struct{}, 1)

	ranker, err := newRanker(items, blocked, seen, store,
		filepath.Join(dir, "model.json"), filepath.Join(dir, "rank.json"),
		notifyCh, opts.log)
	if err != nil {
		return nil, err
	}
	extractor := newExtractor(extractCh, items, subs, blocked, notifyCh, nil, opts.log)
	fetcher := newFetcher(subs, extractCh, opts.refresh, client, opts.log)
	memos := newMemos(client, settings, store, opts.log)
	pusher := newWebPusher(vapid, client, opts.log)
	notifier := newNotifier(subs, ranker, push, notified, pusher, opts.pushThreshold, opts.notifyAge, opts.log)

	// The extractor feeds fresh items to the notifier.
	extractor.pushCh = notifier.channel()

	a := &api{
		store:     store,
		items:     items,
		subs:      subs,
		settings:  settings,
		blocked:   blocked,
		ranker:    ranker,
		fetcher:   fetcher,
		extractor: extractor,
		memos:     memos,
		push:      push,
		vapid:     vapid,
		notifier:  notifier,
		tokens:    tokens,
		seen:      seen,
		ht:        ht,
		log:       opts.log,
		client:    client,
		addr:      opts.addr,
	}
	return a, nil
}

// Run starts the background workers — fetcher, extractor, ranker, notifier
// — plus the HTTP server, and blocks until ctx is cancelled or the server
// fails.
//
// Goroutine layout:
//
//	main      — this caller (signal handling in main.go)
//	server    — serves content and listens to client messages
//	fetcher   — consumes new content from the RSS/JSON subscriptions
//	extractor — extracts items from raw feed bodies
//	ranker    — trains the recommendation model and sorts content
//	notifier  — sends push notifications for noteworthy new content
func (a *api) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              a.addr,
		Handler:           a.wrappedRoutes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workers := []func(context.Context){
		a.ranker.Run,
		a.extractor.Run,
		a.fetcher.Run,
		a.notifier.Run,
	}
	var wg sync.WaitGroup
	for _, run := range workers {
		wg.Add(1)
		go func(run func(context.Context)) {
			defer wg.Done()
			run(ctx)
		}(run)
	}

	errCh := make(chan error, 1)
	go func() {
		a.log.Info("listening", "addr", a.addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	var err error
	select {
	case <-ctx.Done():
		a.log.Info("shutting down")
	case err = <-errCh:
	}

	shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if serr := srv.Shutdown(shutdownCtx); serr != nil {
		a.log.Error("shutdown", "err", serr)
	}
	wg.Wait()
	return err
}

// wrappedRoutes applies middleware: logging ← CORS ← auth ← routes.
func (a *api) wrappedRoutes() http.Handler {
	var h http.Handler = a.routes()
	h = requireAuth(a.ht, a.tokens, "feed", h)
	h = corsMiddleware(h)
	return logRequests(a.log, h)
}
