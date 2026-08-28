package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Token is an access-token record. The token itself is never stored — only
// its SHA-256 hash — and is shown to the user exactly once at creation.
// The hash is persisted (the file is mode 0600) but never exposed over the
// API.
type Token struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"createdAt"`
}

// TokenStore persists access tokens (data/tokens.json). The file is
// hot-reloaded when it changes on disk, so tokens created with -gen-token
// are usable immediately without restarting the service.
type TokenStore struct {
	mu      sync.RWMutex
	path    string
	tokens  map[string]Token
	exists  bool // file ever created → auth stays enforced even if all tokens are revoked
	modTime int64
	size    int64
}

func OpenTokenStore(path string) (*TokenStore, error) {
	s := &TokenStore{path: path, tokens: make(map[string]Token)}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := s.loadLocked(b); err != nil {
			return nil, err
		}
	case errors.Is(err, os.ErrNotExist):
		// Start empty; auth is open until the first token is created.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

// loadLocked replaces the in-memory map with the parsed file content.
// Caller must hold s.mu.
func (s *TokenStore) loadLocked(b []byte) error {
	var list []Token
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("parse %s: %w", s.path, err)
	}
	m := make(map[string]Token, len(list))
	for _, t := range list {
		if t.ID != "" && t.Hash != "" {
			m[t.ID] = t
		}
	}
	s.tokens = m
	s.exists = true
	return nil
}

// reloadIfChanged re-reads the file when its mtime/size changed. If the
// file was deleted or is unparseable, the current state is kept.
func (s *TokenStore) reloadIfChanged() {
	info, err := os.Stat(s.path)
	if err != nil {
		return
	}
	s.mu.RLock()
	same := s.modTime == info.ModTime().UnixNano() && s.size == info.Size()
	s.mu.RUnlock()
	if same {
		return
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(b); err != nil {
		return
	}
	s.modTime = info.ModTime().UnixNano()
	s.size = info.Size()
}

// newToken returns a fresh random token string ("ft_" + 32 hex chars).
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ft_" + hex.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Create stores the hash of a fresh token and returns the raw token —
// the only time it is ever visible.
func (s *TokenStore) Create(name string) (string, Token, error) {
	raw, err := newToken()
	if err != nil {
		return "", Token{}, err
	}
	t := Token{
		ID:        "t" + shortHash(raw),
		Name:      name,
		Hash:      hashToken(raw),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[t.ID] = t
	if err := s.saveLocked(); err != nil {
		delete(s.tokens, t.ID)
		return "", Token{}, err
	}
	return raw, t, nil
}

// Verify checks a presented token against the stored hashes.
func (s *TokenStore) Verify(raw string) bool {
	if raw == "" {
		return false
	}
	s.reloadIfChanged()
	h := hashToken(raw)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tokens {
		if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(h)) == 1 {
			return true
		}
	}
	return false
}

// List returns all tokens (hashes redacted), oldest first.
func (s *TokenStore) List() []Token {
	s.reloadIfChanged()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		t.Hash = "" // never expose the stored hash
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// Remove revokes a token by ID.
func (s *TokenStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tokens[id]; !ok {
		return fmt.Errorf("token not found")
	}
	delete(s.tokens, id)
	return s.saveLocked()
}

// Len returns the number of stored tokens.
func (s *TokenStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tokens)
}

// Enforced reports whether authentication is in effect: true once a token
// store has ever been written, even if every token was later revoked.
func (s *TokenStore) Enforced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.exists
}

func (s *TokenStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	list := make([]Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt < list[j].CreatedAt })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.exists = true
	if info, err := os.Stat(s.path); err == nil {
		s.modTime = info.ModTime().UnixNano()
		s.size = info.Size()
	}
	return nil
}
