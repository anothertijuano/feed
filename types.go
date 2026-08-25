package main

// Media is one image inside a content item.
type Media struct {
	Src     string `json:"src"`
	Contain bool   `json:"contain,omitempty"`
}

// Item is a piece of content in the feed.
type Item struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Link         string   `json:"link"`
	SourceName   string   `json:"sourceName"`
	Media        []Media  `json:"media,omitempty"`
	Paragraphs   []string `json:"paragraphs,omitempty"`
	Subscription string   `json:"subscription"`
	GUID         string   `json:"guid,omitempty"`
	FetchedAt    string   `json:"fetchedAt"`
}

// viewItem is an Item annotated with the user's current interaction state.
type viewItem struct {
	Item
	Vote  int  `json:"vote"`
	Saved bool `json:"saved"`
}

// Subscription is an RSS/Atom/JSON feed the fetcher polls.
type Subscription struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Title         string `json:"title,omitempty"`
	ETag          string `json:"etag,omitempty"`
	LastModified  string `json:"lastModified,omitempty"`
	AddedAt       string `json:"addedAt"`
	LastFetchedAt string `json:"lastFetchedAt,omitempty"`
	ItemCount     int    `json:"itemCount,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	// Notify is the push-notification policy for this source:
	// "default" (rank-based), "always" or "never".
	Notify string `json:"notify,omitempty"`
}

// Settings holds user configuration such as the Memos connection.
type Settings struct {
	MemosURL       string `json:"memosUrl"`
	MemosToken     string `json:"memosToken"`
	MemoLastSyncAt string `json:"memoLastSyncAt,omitempty"`
	MemoLastError  string `json:"memoLastError,omitempty"`
}
