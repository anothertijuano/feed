package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// pageSize is the number of items fetched per /api/feed request.
const pageSize = 20

// Item is one entry of the feed as served by /api/feed and /api/saved.
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
	PublishedAt  string   `json:"publishedAt,omitempty"`
	Vote         int      `json:"vote"`
	Saved        bool     `json:"saved"`
}

// Media is an image or other media attached to an item.
type Media struct {
	Src     string `json:"src"`
	Contain bool   `json:"contain"`
}

// Subscription is one feed source.
type Subscription struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Title         string `json:"title,omitempty"`
	ETag          string `json:"etag,omitempty"`
	LastModified  string `json:"lastModified,omitempty"`
	AddedAt       string `json:"addedAt"`
	LastFetchedAt string `json:"lastFetchedAt,omitempty"`
	ItemCount     int    `json:"itemCount,omitempty"`
	Notify        string `json:"notify,omitempty"`
}

// serverSettings is the server-side settings document.
type serverSettings struct {
	MemosURL   string `json:"memosUrl"`
	MemosToken string `json:"memosToken"`
}

// client talks to a feed server. All methods return tea.Cmds so that network
// calls never block the UI.
type client struct {
	server string
	token  string
	http   *http.Client
}

func newClient(server, token string) *client {
	return &client{
		server: strings.TrimRight(server, "/"),
		token:  token,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *client) setServer(server string) { c.server = strings.TrimRight(server, "/") }
func (c *client) setToken(token string)   { c.token = token }

// do performs one JSON API call. body is marshalled to JSON when non-nil;
// out, when non-nil, receives the decoded response body. Errors carry the
// server's "error" message when one is present.
func (c *client) do(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.server+path, reader)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// --- messages returned by the commands below ---

type feedPageMsg struct {
	items  []Item
	total  int
	offset int
	err    error
}

type savedListMsg struct {
	items []Item
	err   error
}

type interactionMsg struct {
	key   string
	kind  string // "vote", "save" or "seen"
	vote  int
	saved bool
	err   error
}

type subsMsg struct {
	subs []Subscription
	err  error
}

type subAddedMsg struct {
	sub Subscription
	err error
}

type subDeletedMsg struct {
	id  string
	err error
}

type notifyMsg struct {
	id     string
	notify string
	err    error
}

type refreshMsg struct {
	new int
	err error
}

type settingsMsg struct {
	settings serverSettings
	err      error
}

type settingsSavedMsg struct {
	err error
}

type healthMsg struct {
	err error
}

type openMsg struct {
	err error
}

// --- commands ---

func (c *client) loadFeed(offset, limit int) tea.Cmd {
	return func() tea.Msg {
		var page struct {
			Total int    `json:"total"`
			Items []Item `json:"items"`
		}
		err := c.do("GET", fmt.Sprintf("/api/feed?limit=%d&offset=%d", limit, offset), nil, &page)
		return feedPageMsg{items: page.Items, total: page.Total, offset: offset, err: err}
	}
}

func (c *client) loadSaved() tea.Cmd {
	return func() tea.Msg {
		var list struct {
			Items []Item `json:"items"`
		}
		err := c.do("GET", "/api/saved", nil, &list)
		return savedListMsg{items: list.Items, err: err}
	}
}

// sendInteraction posts one interaction (vote, save or seen).
func (c *client) sendInteraction(key, kind string, value any) tea.Cmd {
	return func() tea.Msg {
		var out struct {
			Key   string `json:"key"`
			Vote  int    `json:"vote"`
			Saved bool   `json:"saved"`
		}
		err := c.do("POST", "/api/interactions", map[string]any{
			"key": key, "kind": kind, "value": value,
		}, &out)
		return interactionMsg{key: key, kind: kind, vote: out.Vote, saved: out.Saved, err: err}
	}
}

func (c *client) markSeen(key string) tea.Cmd {
	return c.sendInteraction(key, "seen", true)
}

func (c *client) loadSubs() tea.Cmd {
	return func() tea.Msg {
		var list struct {
			Items []Subscription `json:"items"`
		}
		err := c.do("GET", "/api/subscriptions", nil, &list)
		return subsMsg{subs: list.Items, err: err}
	}
}

func (c *client) addSub(url string) tea.Cmd {
	return func() tea.Msg {
		var sub Subscription
		err := c.do("POST", "/api/subscriptions", map[string]string{"url": url}, &sub)
		return subAddedMsg{sub: sub, err: err}
	}
}

func (c *client) deleteSub(id string) tea.Cmd {
	return func() tea.Msg {
		err := c.do("DELETE", "/api/subscriptions/"+id, nil, nil)
		return subDeletedMsg{id: id, err: err}
	}
}

func (c *client) setNotify(id, notify string) tea.Cmd {
	return func() tea.Msg {
		var sub Subscription
		err := c.do("POST", "/api/subscriptions/"+id, map[string]string{"notify": notify}, &sub)
		return notifyMsg{id: id, notify: notify, err: err}
	}
}

func (c *client) refreshFeed() tea.Cmd {
	return func() tea.Msg {
		var out struct {
			New int `json:"new"`
		}
		err := c.do("POST", "/api/refresh", nil, &out)
		return refreshMsg{new: out.New, err: err}
	}
}

func (c *client) loadSettings() tea.Cmd {
	return func() tea.Msg {
		var s serverSettings
		err := c.do("GET", "/api/settings", nil, &s)
		return settingsMsg{settings: s, err: err}
	}
}

func (c *client) saveSettings(s serverSettings) tea.Cmd {
	return func() tea.Msg {
		err := c.do("POST", "/api/settings", s, nil)
		return settingsSavedMsg{err: err}
	}
}

func (c *client) checkHealth() tea.Cmd {
	return func() tea.Msg {
		err := c.do("GET", "/api/health", nil, nil)
		return healthMsg{err: err}
	}
}

// openURL launches the system browser for url.
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "linux":
			cmd = exec.Command("xdg-open", url)
		default:
			return openMsg{err: fmt.Errorf("no browser opener for GOOS %s", runtime.GOOS)}
		}
		if err := cmd.Start(); err != nil {
			return openMsg{err: err}
		}
		return openMsg{}
	}
}
