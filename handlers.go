package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// keyPattern keeps interaction keys to plain identifiers (e.g. "seed-01").
var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/._-]{0,255}$`)

type interactionRequest struct {
	Key   string          `json:"key"`
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

type interactionResponse struct {
	Key   string `json:"key"`
	Vote  int    `json:"vote"`
	Saved bool   `json:"saved"`
}

// api bundles everything the HTTP handlers need.
type api struct {
	store     *Store
	items     *ItemStore
	subs      *SubscriptionStore
	settings  *SettingsStore
	blocked   *BlockStore
	ranker    *Ranker
	fetcher   *Fetcher
	extractor *Extractor
	memos     *Memos
	push      *PushStore
	vapid     *VAPID
	notifier  *Notifier
	ht        *Htpasswd

	log    *slog.Logger
	client *http.Client
	addr   string
}

func (a *api) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("GET /api/feed", a.feed)
	mux.HandleFunc("GET /api/saved", a.saved)
	mux.HandleFunc("GET /api/interactions", a.getInteractions)
	mux.HandleFunc("POST /api/interactions", a.postInteraction)
	mux.HandleFunc("GET /api/settings", a.getSettings)
	mux.HandleFunc("POST /api/settings", a.postSettings)
	mux.HandleFunc("GET /api/subscriptions", a.getSubscriptions)
	mux.HandleFunc("POST /api/subscriptions", a.postSubscription)
	mux.HandleFunc("DELETE /api/subscriptions/{id}", a.deleteSubscription)
	mux.HandleFunc("POST /api/subscriptions/{id}", a.postSubscriptionNotify)
	mux.HandleFunc("POST /api/refresh", a.refresh)
	mux.HandleFunc("GET /api/push/key", a.pushKey)
	mux.HandleFunc("POST /api/push/subscribe", a.pushSubscribe)
	mux.HandleFunc("DELETE /api/push/unsubscribe", a.pushUnsubscribe)

	mux.Handle("/", http.FileServerFS(frontendFS))
	return mux
}

/* ---------- feed / saved ---------- */

// feed returns a page of items sorted by relevance.
func (a *api) feed(w http.ResponseWriter, r *http.Request) {
	limit := intParam(r, "limit", 20)
	if limit < 1 {
		limit = 1
	} else if limit > 100 {
		limit = 100
	}
	offset := intParam(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	items := a.ranker.Ranked(offset, limit)
	out := make([]viewItem, 0, len(items))
	for _, it := range items {
		out = append(out, a.view(it))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": a.ranker.Len()})
}

// saved returns all saved items, newest save first.
func (a *api) saved(w http.ResponseWriter, r *http.Request) {
	keys := a.store.SavedKeys()
	out := make([]viewItem, 0, len(keys))
	for _, k := range keys {
		if it, ok := a.items.Get(k); ok {
			out = append(out, a.view(it))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// view annotates an item with the user's current interaction state.
func (a *api) view(it Item) viewItem {
	return viewItem{
		Item:  it,
		Vote:  a.store.Vote(it.ID),
		Saved: a.store.IsSaved(it.ID),
	}
}

/* ---------- interactions ---------- */

// postInteraction handles POST /api/interactions.
//
//	{"key":"…","kind":"vote","value":-1|0|1}
//	{"key":"…","kind":"save","value":true|false}
//
// Votes train the recommendation model; a downvote also removes the item
// from the system so it is never fetched again.
func (a *api) postInteraction(w http.ResponseWriter, r *http.Request) {
	var req interactionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if !keyPattern.MatchString(req.Key) {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}

	switch req.Kind {
	case "vote":
		var v int
		if err := json.Unmarshal(req.Value, &v); err != nil || v < -1 || v > 1 {
			writeError(w, http.StatusBadRequest, "vote value must be -1, 0 or 1")
			return
		}
		switch v {
		case 1:
			if err := a.store.SetVote(req.Key, 1); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			a.ranker.Like(req.Key)
		case -1:
			if err := a.store.SetVote(req.Key, 0); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			// Remove any memo mirrored to Memos before clearing the save
			// state (which forgets the memo resource name).
			a.memos.DeleteOnUnsave(req.Key)
			if err := a.store.ClearSaved(req.Key); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			a.ranker.Dislike(req.Key)
		case 0:
			if err := a.store.SetVote(req.Key, 0); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	case "save":
		var b bool
		if err := json.Unmarshal(req.Value, &b); err != nil {
			writeError(w, http.StatusBadRequest, "save value must be true or false")
			return
		}
		if b {
			if err := a.store.SetSaved(req.Key, true); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if it, ok := a.items.Get(req.Key); ok {
				a.memos.SendOnSave(it)
			}
		} else {
			// Fire the memo delete first: it needs the memo resource name,
			// which SetSaved(false) removes from the store.
			a.memos.DeleteOnUnsave(req.Key)
			if err := a.store.SetSaved(req.Key, false); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	default:
		writeError(w, http.StatusBadRequest, `kind must be "vote" or "save"`)
		return
	}

	a.log.Info("interaction", "key", req.Key, "kind", req.Kind, "value", string(req.Value))
	writeJSON(w, http.StatusOK, interactionResponse{
		Key:   req.Key,
		Vote:  a.store.Vote(req.Key),
		Saved: a.store.IsSaved(req.Key),
	})
}

// getInteractions returns the full interaction state.
func (a *api) getInteractions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.store.Snapshot())
}

/* ---------- settings ---------- */

func (a *api) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.settings.Get())
}

func (a *api) postSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MemosURL   string `json:"memosUrl"`
		MemosToken string `json:"memosToken"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	req.MemosURL = strings.TrimSpace(req.MemosURL)
	req.MemosToken = strings.TrimSpace(req.MemosToken)
	if req.MemosURL != "" {
		u, err := url.Parse(req.MemosURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeError(w, http.StatusBadRequest, "memosUrl must be a valid http(s) URL")
			return
		}
		req.MemosURL = strings.TrimRight(req.MemosURL, "/")
	}

	cur := a.settings.Get()
	cur.MemosURL = req.MemosURL
	cur.MemosToken = req.MemosToken
	if err := a.settings.Update(cur); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Backfill: push previously saved items to the newly configured instance.
	if cur.MemosURL != "" && cur.MemosToken != "" {
		a.memos.SyncSaved(a.items)
	}
	writeJSON(w, http.StatusOK, a.settings.Get())
}

/* ---------- subscriptions ---------- */

func (a *api) getSubscriptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": a.subs.All()})
}

// postSubscription adds an RSS/Atom/JSON feed. The feed is fetched once
// synchronously (to validate it and read its title), then ingested in the
// background by the extractor.
func (a *api) postSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeError(w, http.StatusBadRequest, "url must be a valid http(s) feed URL")
		return
	}
	if sub, ok := a.subs.GetByURL(req.URL); ok {
		writeJSON(w, http.StatusOK, sub)
		return
	}

	body, err := a.fetchBody(r.Context(), req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not fetch feed: "+err.Error())
		return
	}
	title := ""
	if feed, err := parseFeed(body); err == nil {
		title = feed.Title
	}

	sub := Subscription{
		ID:      "s" + shortHash(req.URL),
		URL:     req.URL,
		Title:   title,
		AddedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := a.subs.Add(sub); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.extractor.Submit(FetchResult{SubID: sub.ID, URL: sub.URL, Body: body})
	a.log.Info("subscription added", "url", req.URL, "title", title)
	writeJSON(w, http.StatusOK, sub)
}

// deleteSubscription removes a feed and all content ingested from it.
func (a *api) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.subs.Get(id); !ok {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	if err := a.subs.Remove(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := a.items.DeleteBySubscription(id)
	a.ranker.nudge()
	a.log.Info("subscription removed", "id", id, "items", n)
	writeJSON(w, http.StatusOK, map[string]any{"removed": n})
}

// postSubscriptionNotify updates the push-notification policy of a feed.
// Body: {"notify": "default" | "always" | "never"}
func (a *api) postSubscriptionNotify(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.subs.Get(id); !ok {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	var req struct {
		Notify string `json:"notify"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	switch req.Notify {
	case "default", "always", "never":
	default:
		writeError(w, http.StatusBadRequest, `notify must be "default", "always" or "never"`)
		return
	}
	if err := a.subs.SetNotify(id, req.Notify); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sub, _ := a.subs.Get(id)
	writeJSON(w, http.StatusOK, sub)
}

/* ---------- push ---------- */

// pushKey returns the VAPID public key for pushManager.subscribe.
func (a *api) pushKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"publicKey": a.vapid.PublicKey()})
}

// pushSubscribe registers a Web Push subscription.
func (a *api) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	var sub PushSub
	if err := decodeJSON(r, &sub); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	sub.Endpoint = strings.TrimSpace(sub.Endpoint)
	if !strings.HasPrefix(sub.Endpoint, "https://") && !strings.HasPrefix(sub.Endpoint, "http://") {
		writeError(w, http.StatusBadRequest, "endpoint must be an http(s) URL")
		return
	}
	if sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "keys.p256dh and keys.auth are required")
		return
	}
	if err := a.push.Add(sub); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.log.Info("push subscription added", "endpoint", sub.Endpoint)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// pushUnsubscribe removes a Web Push subscription.
func (a *api) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if err := a.push.Remove(strings.TrimSpace(req.Endpoint)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

/* ---------- misc ---------- */

// refresh fetches all subscriptions now and ingests any new content
// synchronously, responding with how many new items arrived. (The periodic
// polling still happens in the fetcher goroutine; this is the on-demand
// path used by pull-to-refresh.)
func (a *api) refresh(w http.ResponseWriter, r *http.Request) {
	total := 0
	for _, sub := range a.subs.All() {
		if r.Context().Err() != nil {
			break
		}
		body, err := a.fetchBody(r.Context(), sub.URL)
		if err != nil {
			_ = a.subs.SetFetchError(sub.ID, err.Error())
			a.log.Warn("refresh fetch failed", "url", sub.URL, "err", err)
			continue
		}
		total += a.extractor.IngestSync(FetchResult{SubID: sub.ID, URL: sub.URL, Body: body})
	}
	a.log.Info("manual refresh", "new", total)
	writeJSON(w, http.StatusOK, map[string]any{"new": total, "status": "ok"})
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

/* ---------- helpers ---------- */

func (a *api) fetchBody(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "feed2/0.2 (+personal feed reader)")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes))
}

func intParam(r *http.Request, name string, def int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
