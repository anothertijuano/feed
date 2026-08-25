package main

import (
	"testing"
	"time"
)

func TestParseRSS(t *testing.T) {
	feed, err := parseFeed([]byte(testRSS))
	if err != nil {
		t.Fatal(err)
	}
	if feed.Title != "Test Feed" {
		t.Fatalf("title = %q", feed.Title)
	}
	if len(feed.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(feed.Entries))
	}
	e := feed.Entries[0]
	if e.Title != "First post" || e.Link != "https://blog.example.com/first-post" {
		t.Fatalf("entry = %+v", e)
	}
	if len(e.Media) != 1 || e.Media[0].Src != "https://blog.example.com/img.jpg" || e.Media[0].Contain {
		t.Fatalf("media = %+v", e.Media)
	}

	items := itemsFromEntries("sub1", feed.Entries, time.Now())
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if got := items[0].Paragraphs; len(got) != 1 || got[0] != "Hello world!" {
		t.Fatalf("paragraphs = %q", got)
	}
	if items[0].SourceName != "blog.example.com" {
		t.Fatalf("sourceName = %q", items[0].SourceName)
	}
}

const testAtom = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <entry>
    <title>Atom entry</title>
    <id>tag:example.com,2026:1</id>
    <updated>2026-06-01T12:00:00Z</updated>
    <link rel="alternate" href="https://atom.example.com/entry-1"/>
    <summary type="html">&lt;p&gt;Summary here&lt;/p&gt;</summary>
  </entry>
</feed>`

func TestParseAtom(t *testing.T) {
	feed, err := parseFeed([]byte(testAtom))
	if err != nil {
		t.Fatal(err)
	}
	if feed.Title != "Atom Feed" || len(feed.Entries) != 1 {
		t.Fatalf("feed = %+v", feed)
	}
	items := itemsFromEntries("sub1", feed.Entries, time.Now())
	if len(items) != 1 || items[0].Link != "https://atom.example.com/entry-1" {
		t.Fatalf("items = %+v", items)
	}
	if len(items[0].Paragraphs) != 1 || items[0].Paragraphs[0] != "Summary here" {
		t.Fatalf("paragraphs = %q", items[0].Paragraphs)
	}
}

const testJSONFeed = `{
  "version": "https://jsonfeed.org/version/1.1",
  "title": "JSON Feed",
  "items": [
    {
      "id": "jf-1",
      "url": "https://json.example.com/post-1",
      "title": "JSON post",
      "content_text": "Some plain text content.",
      "date_published": "2026-06-01T12:00:00Z",
      "attachments": [{"url": "https://json.example.com/pic.png", "mime_type": "image/png"}]
    }
  ]
}`

func TestParseJSONFeed(t *testing.T) {
	feed, err := parseFeed([]byte(testJSONFeed))
	if err != nil {
		t.Fatal(err)
	}
	if feed.Title != "JSON Feed" || len(feed.Entries) != 1 {
		t.Fatalf("feed = %+v", feed)
	}
	items := itemsFromEntries("sub1", feed.Entries, time.Now())
	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	it := items[0]
	if it.Link != "https://json.example.com/post-1" || it.SourceName != "json.example.com" {
		t.Fatalf("item = %+v", it)
	}
	if len(it.Media) != 1 || it.Media[0].Src != "https://json.example.com/pic.png" {
		t.Fatalf("media = %+v", it.Media)
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("The Rust team announces Rust 1.87.0!")
	if contains(tokens, "the") {
		t.Fatalf("stopword 'the' should have been filtered: %v", tokens)
	}
	for _, want := range []string{"rust", "team", "announces"} {
		if !contains(tokens, want) {
			t.Fatalf("tokens = %v, want %q", tokens, want)
		}
	}
}

func TestStripHTML(t *testing.T) {
	in := `<p>Hello <b>world</b>!</p><script>alert('x')</script>&amp; goodbye`
	got := stripHTML(in)
	if got != "Hello world! & goodbye" {
		t.Fatalf("got %q", got)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
