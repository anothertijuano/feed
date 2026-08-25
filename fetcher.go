package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const maxFeedBytes = 8 << 20 // 8 MiB cap per feed

// FetchResult carries a raw downloaded feed to the extractor.
type FetchResult struct {
	SubID string
	URL   string
	Body  []byte
}

// Fetcher consumes new content from the RSS/JSON subscriptions. Run as its
// own goroutine: it polls all feeds on a ticker and can be nudged to fetch
// immediately.
type Fetcher struct {
	subs     *SubscriptionStore
	out      chan<- FetchResult
	client   *http.Client
	interval time.Duration
	log      *slog.Logger
}

func newFetcher(subs *SubscriptionStore, out chan<- FetchResult, interval time.Duration, client *http.Client, log *slog.Logger) *Fetcher {
	return &Fetcher{
		subs:     subs,
		out:      out,
		client:   client,
		interval: interval,
		log:      log,
	}
}

func (f *Fetcher) Run(ctx context.Context) {
	t := time.NewTicker(f.interval)
	defer t.Stop()
	f.fetchAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.fetchAll(ctx)
		}
	}
}

func (f *Fetcher) fetchAll(ctx context.Context) {
	for _, sub := range f.subs.All() {
		if ctx.Err() != nil {
			return
		}
		f.fetch(ctx, sub)
	}
}

// fetch downloads one feed with conditional HTTP (ETag / Last-Modified)
// and hands the body to the extractor when it changed.
func (f *Fetcher) fetch(ctx context.Context, sub Subscription) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sub.URL, nil)
	if err != nil {
		_ = f.subs.SetFetchError(sub.ID, err.Error())
		return
	}
	req.Header.Set("User-Agent", "feed2/0.2 (+personal feed reader)")
	if sub.ETag != "" {
		req.Header.Set("If-None-Match", sub.ETag)
	} else if sub.LastModified != "" {
		req.Header.Set("If-Modified-Since", sub.LastModified)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		_ = f.subs.SetFetchError(sub.ID, err.Error())
		return
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified:
		_ = f.subs.SetFetchOK(sub.ID)
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes))
		if err != nil {
			_ = f.subs.SetFetchError(sub.ID, err.Error())
			return
		}
		_ = f.subs.SetETag(sub.ID, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"))
		f.out <- FetchResult{SubID: sub.ID, URL: sub.URL, Body: body}
	default:
		_ = f.subs.SetFetchError(sub.ID, "HTTP "+resp.Status)
	}
}
