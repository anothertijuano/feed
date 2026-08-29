package main

import (
	"os"
	"testing"
)

// TestLiveServer performs read-only requests against a running feed server to
// sanity-check the API client. It is skipped unless FEED_SERVER and
// FEED_TOKEN are set; run it with:
//
//	FEED_SERVER=https://feed.microbito.net FEED_TOKEN=... go test -run TestLiveServer -v
func TestLiveServer(t *testing.T) {
	server := os.Getenv("FEED_SERVER")
	token := os.Getenv("FEED_TOKEN")
	if server == "" || token == "" {
		t.Skip("FEED_SERVER and FEED_TOKEN not set; skipping live server test")
	}
	c := newClient(server, token)

	if err := c.do("GET", "/api/health", nil, nil); err != nil {
		t.Fatalf("health: %v", err)
	}

	var page struct {
		Total int    `json:"total"`
		Items []Item `json:"items"`
	}
	if err := c.do("GET", "/api/feed?limit=20&offset=0", nil, &page); err != nil {
		t.Fatalf("feed: %v", err)
	}
	t.Logf("feed total=%d page items=%d", page.Total, len(page.Items))
	for _, it := range page.Items {
		if it.ID == "" || it.Title == "" || it.Link == "" {
			t.Errorf("item missing required fields: %+v", it)
		}
		if it.Vote < -1 || it.Vote > 1 {
			t.Errorf("item vote out of range: %+v", it)
		}
	}

	var saved struct {
		Items []Item `json:"items"`
	}
	if err := c.do("GET", "/api/saved", nil, &saved); err != nil {
		t.Fatalf("saved: %v", err)
	}

	var subs struct {
		Items []Subscription `json:"items"`
	}
	if err := c.do("GET", "/api/subscriptions", nil, &subs); err != nil {
		t.Fatalf("subs: %v", err)
	}
	for _, sub := range subs.Items {
		if sub.ID == "" || sub.URL == "" {
			t.Errorf("subscription missing required fields: %+v", sub)
		}
	}
	t.Logf("subscriptions=%d saved=%d", len(subs.Items), len(saved.Items))
}
