package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Memos mirrors saved content to a Memos (usememos.com) instance.
type Memos struct {
	client   *http.Client
	settings *SettingsStore
	store    *Store
	log      *slog.Logger
	wg       sync.WaitGroup
}

func newMemos(client *http.Client, settings *SettingsStore, store *Store, log *slog.Logger) *Memos {
	return &Memos{client: client, settings: settings, store: store, log: log}
}

// Wait blocks until all in-flight memo syncs have finished.
func (m *Memos) Wait() { m.wg.Wait() }

func (m *Memos) configured() bool {
	s := m.settings.Get()
	return s.MemosURL != "" && s.MemosToken != ""
}

// SyncSaved pushes every saved item that has no memo yet. Called when the
// user (re)configures Memos so existing saves are backfilled.
func (m *Memos) SyncSaved(items *ItemStore) {
	if !m.configured() {
		return
	}
	for _, key := range m.store.SavedKeys() {
		if m.store.MemoName(key) != "" {
			continue
		}
		if it, ok := items.Get(key); ok {
			m.SendOnSave(it)
		}
	}
}

// memoError formats an API error response for the settings status line,
// extracting the message from Memos' gRPC-gateway error JSON when present.
func memoError(resp *http.Response, body []byte) string {
	msg := strings.TrimSpace(string(body))
	var gerr struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &gerr) == nil && gerr.Message != "" {
		msg = fmt.Sprintf("code %d: %s", gerr.Code, gerr.Message)
	}
	if len(msg) > 240 {
		msg = msg[:240] + "…"
	}
	return "memos: " + resp.Status + ": " + msg
}

// memoContent formats a saved item using the same plain-text layout as the
// content files: title, link, then the body.
func memoContent(it Item) string {
	parts := []string{it.Title, it.Link}
	if it.Link == "" {
		parts = []string{it.Title}
	}
	if len(it.Paragraphs) > 0 {
		parts = append(parts, "", strings.Join(it.Paragraphs, "\n"))
	}
	return strings.Join(parts, "\n")
}

// SendOnSave posts a memo for a saved item. Best-effort and asynchronous;
// the outcome is recorded in the settings (last sync / last error).
func (m *Memos) SendOnSave(it Item) {
	if !m.configured() {
		return
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		s := m.settings.Get()
		payload, _ := json.Marshal(map[string]any{
			"content":    memoContent(it),
			"visibility": "PRIVATE",
		})
		req, err := http.NewRequest(http.MethodPost,
			strings.TrimRight(s.MemosURL, "/")+"/api/v1/memos", bytes.NewReader(payload))
		if err != nil {
			_ = m.settings.SetMemoStatus(time.Time{}, err.Error())
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.MemosToken)

		resp, err := m.client.Do(req)
		if err != nil {
			_ = m.settings.SetMemoStatus(time.Time{}, err.Error())
			m.log.Warn("memos save failed", "err", err)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = m.settings.SetMemoStatus(time.Time{}, memoError(resp, body))
			m.log.Warn("memos save failed", "status", resp.Status)
			return
		}
		var out struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(body, &out)
		if out.Name != "" {
			_ = m.store.SetMemoName(it.ID, out.Name)
		}
		_ = m.settings.SetMemoStatus(time.Now(), "")
		m.log.Info("memo saved", "item", it.ID, "name", out.Name)
	}()
}

// DeleteOnUnsave removes the memo previously created for an item.
// Best-effort and asynchronous.
func (m *Memos) DeleteOnUnsave(key string) {
	name := m.store.MemoName(key)
	if name == "" || !m.configured() {
		return
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		s := m.settings.Get()
		req, err := http.NewRequest(http.MethodDelete,
			strings.TrimRight(s.MemosURL, "/")+"/api/v1/"+name, nil)
		if err != nil {
			_ = m.settings.SetMemoStatus(time.Time{}, err.Error())
			return
		}
		req.Header.Set("Authorization", "Bearer "+s.MemosToken)

		resp, err := m.client.Do(req)
		if err != nil {
			_ = m.settings.SetMemoStatus(time.Time{}, err.Error())
			return
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = m.settings.SetMemoStatus(time.Time{}, fmt.Sprintf("memos delete: %s", resp.Status))
			return
		}
		_ = m.store.DeleteMemoName(key)
		_ = m.settings.SetMemoStatus(time.Now(), "")
		m.log.Info("memo deleted", "item", key)
	}()
}
