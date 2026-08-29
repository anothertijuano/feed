package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"
)

/* ---------- text helpers ---------- */

var (
	tagRe      = regexp.MustCompile(`(?is)<[^>]+>`)
	blockTagRe = regexp.MustCompile(`(?is)</?(?:p|div|br|li|ul|ol|h[1-6]|blockquote|section|article|tr|table|pre)\b[^>]*>`)
)

// stripHTML removes tags and unescapes entities, producing plain text.
// Block-level tags become paragraph breaks; inline tags are removed.
func stripHTML(s string) string {
	s = stripBlocks(s)
	s = blockTagRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

// stripBlocks removes <script>…</script> and <style>…</style> blocks.
func stripBlocks(s string) string {
	lower := strings.ToLower(s)
	for {
		si := strings.Index(lower, "<script")
		sty := strings.Index(lower, "<style")
		start, endTag := -1, ""
		switch {
		case si >= 0 && (sty < 0 || si < sty):
			start, endTag = si, "</script>"
		case sty >= 0:
			start, endTag = sty, "</style>"
		default:
			return s
		}
		end := strings.Index(lower[start+1:], endTag)
		if end < 0 {
			return s[:start]
		}
		end += start + 1 + len(endTag)
		s = s[:start] + " " + s[end:]
		lower = strings.ToLower(s)
	}
}

var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true,
	"to": true, "in": true, "for": true, "on": true, "with": true, "is": true,
	"are": true, "was": true, "were": true, "be": true, "been": true, "at": true,
	"by": true, "it": true, "its": true, "as": true, "this": true, "that": true,
	"from": true, "but": true, "not": true, "you": true, "your": true, "we": true,
	"our": true, "they": true, "their": true, "he": true, "she": true, "his": true,
	"her": true, "have": true, "has": true, "had": true, "will": true, "would": true,
	"can": true, "could": true, "should": true, "about": true,
	"than": true, "then": true, "them": true, "these": true, "those": true,
	"what": true, "when": true, "where": true, "which": true, "who": true,
	"how": true, "all": true, "any": true, "both": true, "each": true,
	"more": true, "most": true, "some": true, "such": true, "only": true,
	"own": true, "same": true, "so": true, "too": true, "very": true,
	"just": true, "now": true, "get": true, "out": true, "over": true,
	"under": true, "again": true, "further": true, "once": true, "here": true,
	"there": true, "why": true, "while": true, "via": true, "after": true,
	"before": true, "between": true, "during": true, "without": true,
	"also": true, "itself": true, "other": true,
}

// tokenize splits text into lowercased keywords for the ranking model.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '\''
	})
	seen := make(map[string]bool)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, "'")
		if len(f) < 3 || stopwords[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

func shortHash(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])[:12]
}

func domainOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

func hasSVGExtension(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(u.Path), ".svg")
}

// chunkByWords splits text into chunks of at most size bytes at word
// boundaries.
func chunkByWords(text string, size int) []string {
	words := strings.Fields(text)
	var out []string
	var cur []string
	n := 0
	for _, w := range words {
		if n > 0 && n+1+len(w) > size {
			out = append(out, strings.Join(cur, " "))
			cur, n = nil, 0
		}
		cur = append(cur, w)
		n += len(w) + 1
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, " "))
	}
	return out
}

// paragraphsFrom turns raw feed text into at most three short paragraphs.
func paragraphsFrom(text string) []string {
	text = stripHTML(text)
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil
	}
	chunks := chunkByWords(text, 220)
	if len(chunks) > 3 {
		chunks = chunks[:3]
	}
	return chunks
}

var timeLayouts = []string{
	time.RFC3339,
	time.RFC1123Z,
	time.RFC1123,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"2006-01-02",
}

func parseAnyTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

/* ---------- feed parsing (RSS / Atom / JSON Feed) ---------- */

type feedInfo struct {
	Title   string
	Entries []feedEntry
}

type feedEntry struct {
	ID          string
	Title       string
	Link        string
	Published   string
	ContentHTML string
	ContentText string
	Media       []Media
}

// parseFeed detects the feed format and parses it.
func parseFeed(body []byte) (feedInfo, error) {
	trimmed := strings.TrimSpace(string(body))
	switch {
	case strings.HasPrefix(trimmed, "{"):
		return parseJSONFeed(body)
	case strings.Contains(trimmed[:min(len(trimmed), 300)], "<feed"):
		return parseAtom(body)
	default:
		return parseRSS(body)
	}
}

/* RSS 2.0 / 1.0 */

type rssDoc struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Encoded     string `xml:"encoded"`
	Enclosure   struct {
		URL  string `xml:"url,attr"`
		Type string `xml:"type,attr"`
	} `xml:"enclosure"`
	Thumbnails []struct {
		URL string `xml:"url,attr"`
	} `xml:"thumbnail"`
}

func parseRSS(b []byte) (feedInfo, error) {
	var doc rssDoc
	if err := xml.Unmarshal(b, &doc); err != nil {
		return feedInfo{}, fmt.Errorf("rss: %w", err)
	}
	info := feedInfo{Title: strings.TrimSpace(doc.Channel.Title)}
	for _, it := range doc.Channel.Items {
		e := feedEntry{
			ID:        strings.TrimSpace(it.GUID),
			Title:     strings.TrimSpace(it.Title),
			Link:      strings.TrimSpace(it.Link),
			Published: it.PubDate,
		}
		desc := it.Description
		if strings.TrimSpace(it.Encoded) != "" {
			desc = it.Encoded
		}
		if desc != "" {
			e.ContentHTML = desc
		}
		if it.Enclosure.URL != "" && strings.HasPrefix(strings.ToLower(it.Enclosure.Type), "image/") {
			e.Media = append(e.Media, Media{Src: it.Enclosure.URL, Contain: hasSVGExtension(it.Enclosure.URL)})
		}
		for _, th := range it.Thumbnails {
			if th.URL != "" {
				e.Media = append(e.Media, Media{Src: th.URL, Contain: hasSVGExtension(th.URL)})
				break
			}
		}
		info.Entries = append(info.Entries, e)
	}
	return info, nil
}

/* Atom */

type atomDoc struct {
	Title   string     `xml:"title"`
	Entries []atomItem `xml:"entry"`
}

type atomItem struct {
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Summary string      `xml:"summary"`
	Content atomContent `xml:"content"`
	Links   []atomLink  `xml:"link"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func parseAtom(b []byte) (feedInfo, error) {
	var doc atomDoc
	if err := xml.Unmarshal(b, &doc); err != nil {
		return feedInfo{}, fmt.Errorf("atom: %w", err)
	}
	info := feedInfo{Title: strings.TrimSpace(doc.Title)}
	for _, it := range doc.Entries {
		link := ""
		for _, l := range it.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.Href
				break
			}
		}
		if link == "" && len(it.Links) > 0 {
			link = it.Links[0].Href
		}
		e := feedEntry{
			ID:        strings.TrimSpace(it.ID),
			Title:     strings.TrimSpace(it.Title),
			Link:      strings.TrimSpace(link),
			Published: it.Updated,
		}
		if it.Content.Body != "" {
			e.ContentHTML = it.Content.Body
		} else if it.Summary != "" {
			e.ContentHTML = it.Summary
		}
		info.Entries = append(info.Entries, e)
	}
	return info, nil
}

/* JSON Feed (https://www.jsonfeed.org/) */

type jsonFeedDoc struct {
	Version string         `json:"version"`
	Title   string         `json:"title"`
	Items   []jsonFeedItem `json:"items"`
}

type jsonFeedItem struct {
	ID            string               `json:"id"`
	URL           string               `json:"url"`
	ExternalURL   string               `json:"external_url"`
	Title         string               `json:"title"`
	ContentHTML   string               `json:"content_html"`
	ContentText   string               `json:"content_text"`
	Summary       string               `json:"summary"`
	DatePublished string               `json:"date_published"`
	Attachments   []jsonFeedAttachment `json:"attachments"`
}

type jsonFeedAttachment struct {
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
}

func parseJSONFeed(b []byte) (feedInfo, error) {
	var doc jsonFeedDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return feedInfo{}, fmt.Errorf("json feed: %w", err)
	}
	if !strings.HasPrefix(doc.Version, "https://jsonfeed.org/version/") {
		return feedInfo{}, fmt.Errorf("json feed: unsupported version %q", doc.Version)
	}
	info := feedInfo{Title: strings.TrimSpace(doc.Title)}
	for _, it := range doc.Items {
		link := it.URL
		if link == "" {
			link = it.ExternalURL
		}
		e := feedEntry{
			ID:        strings.TrimSpace(it.ID),
			Title:     strings.TrimSpace(it.Title),
			Link:      strings.TrimSpace(link),
			Published: it.DatePublished,
		}
		switch {
		case it.ContentHTML != "":
			e.ContentHTML = it.ContentHTML
		case it.ContentText != "":
			e.ContentText = it.ContentText
		case it.Summary != "":
			e.ContentText = it.Summary
		}
		for _, at := range it.Attachments {
			if strings.HasPrefix(strings.ToLower(at.MimeType), "image/") {
				e.Media = append(e.Media, Media{Src: at.URL, Contain: hasSVGExtension(at.URL)})
			}
		}
		info.Entries = append(info.Entries, e)
	}
	return info, nil
}

/* entry → item */

func itemsFromEntries(subID string, entries []feedEntry, fetchedAt time.Time) []Item {
	items := make([]Item, 0, len(entries))
	for _, e := range entries {
		if it, ok := itemFromEntry(subID, e, fetchedAt); ok {
			items = append(items, it)
		}
	}
	return items
}

func itemFromEntry(subID string, e feedEntry, fetchedAt time.Time) (Item, bool) {
	link := strings.TrimSpace(e.Link)
	key := strings.TrimSpace(e.ID)
	if key == "" {
		key = link
	}
	if link == "" || (!strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://")) {
		return Item{}, false
	}
	title := strings.TrimSpace(e.Title)
	if title == "" {
		title = "(untitled)"
	}
	publishedAt := ""
	if t, ok := parseAnyTime(e.Published); ok {
		publishedAt = t.UTC().Format(time.RFC3339)
	}
	content := e.ContentHTML
	if content == "" {
		content = e.ContentText
	}
	return Item{
		ID:           "r" + shortHash(key),
		Title:        title,
		Link:         link,
		SourceName:   domainOf(link),
		Media:        e.Media,
		Paragraphs:   paragraphsFrom(content),
		Subscription: subID,
		GUID:         e.ID,
		FetchedAt:    fetchedAt.UTC().Format(time.RFC3339),
		PublishedAt:  publishedAt,
	}, true
}
