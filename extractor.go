package main

import (
	"context"
	"log/slog"
	"time"
)

// Extractor extracts content items from raw feed bodies and stores them.
// Run as its own goroutine, consuming the fetcher's output.
type Extractor struct {
	in      chan FetchResult
	items   *ItemStore
	subs    *SubscriptionStore
	blocked *BlockStore
	notify  chan<- struct{}
	pushCh  chan<- Item
	log     *slog.Logger
}

func newExtractor(in chan FetchResult, items *ItemStore, subs *SubscriptionStore, blocked *BlockStore, notify chan<- struct{}, pushCh chan<- Item, log *slog.Logger) *Extractor {
	return &Extractor{in: in, items: items, subs: subs, blocked: blocked, notify: notify, pushCh: pushCh, log: log}
}

// Run consumes feed bodies until the context is done.
func (e *Extractor) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case res := <-e.in:
			e.ingest(res)
		}
	}
}

// Submit queues a raw feed body for extraction. Non-blocking.
func (e *Extractor) Submit(res FetchResult) {
	select {
	case e.in <- res:
	default:
		e.log.Warn("extractor busy, dropping feed", "url", res.URL)
	}
}

// IngestSync parses and stores one feed body in the caller's goroutine,
// returning the number of new items (used for on-demand refreshes).
func (e *Extractor) IngestSync(res FetchResult) int {
	return e.ingest(res)
}

// ingest parses one feed body into items, skipping blocked or
// already-known entries. Returns the number of new items stored.
func (e *Extractor) ingest(res FetchResult) int {
	feed, err := parseFeed(res.Body)
	if err != nil {
		_ = e.subs.SetFetchError(res.SubID, "parse: "+err.Error())
		e.log.Warn("feed parse failed", "url", res.URL, "err", err)
		return 0
	}
	if feed.Title != "" {
		_ = e.subs.SetTitle(res.SubID, feed.Title)
	}

	now := time.Now()
	count := 0
	var fresh []Item
	for _, it := range itemsFromEntries(res.SubID, feed.Entries, now) {
		if e.blocked.Has(it.Link) || (it.GUID != "" && e.blocked.Has(it.GUID)) {
			continue
		}
		if _, exists := e.items.Get(it.ID); exists {
			continue
		}
		if err := e.items.Put(it); err != nil {
			e.log.Error("save item", "id", it.ID, "err", err)
			continue
		}
		count++
		fresh = append(fresh, it)
	}
	_ = e.subs.SetFetchResult(res.SubID, count)
	e.log.Info("feed ingested", "url", res.URL, "new", count)
	if count > 0 {
		e.nudge()
		for _, it := range fresh {
			e.pushItem(it)
		}
	}
	return count
}

// pushItem hands a newly ingested item to the notifier.
func (e *Extractor) pushItem(it Item) {
	select {
	case e.pushCh <- it:
	default:
		e.log.Warn("notifier busy, dropping item", "id", it.ID)
	}
}

func (e *Extractor) nudge() {
	select {
	case e.notify <- struct{}{}:
	default:
	}
}
