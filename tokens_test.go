package main

import (
	"path/filepath"
	"testing"
)

func TestTokenStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	s, err := OpenTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}

	raw, tok, err := s.Create("iphone")
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || tok.ID == "" || tok.Hash == "" {
		t.Fatalf("raw=%q tok=%+v", raw, tok)
	}
	if len(raw) < 32 {
		t.Fatalf("token too short: %q", raw)
	}

	if !s.Verify(raw) {
		t.Fatal("created token does not verify")
	}
	if s.Verify(raw+"x") || s.Verify("") {
		t.Fatal("wrong token verified")
	}

	// The raw token is never persisted — only its hash.
	reloaded, err := OpenTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Verify(raw) {
		t.Fatal("token does not survive reload")
	}
	list := reloaded.List()
	if len(list) != 1 || list[0].Name != "iphone" || list[0].Hash != "" {
		t.Fatalf("list = %+v", list)
	}

	if err := s.Remove(tok.ID); err != nil {
		t.Fatal(err)
	}
	if s.Verify(raw) {
		t.Fatal("revoked token still verifies")
	}
	if err := s.Remove(tok.ID); err == nil {
		t.Fatal("removing an unknown token should fail")
	}
}

func TestTokenStoreHotReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")
	s, err := OpenTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// Another process (e.g. feed -gen-token) writes a token directly.
	other, err := OpenTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := other.Create("cli")
	if err != nil {
		t.Fatal(err)
	}

	// The running store must pick it up without a restart.
	if !s.Verify(raw) {
		t.Fatal("hot reload did not pick up the new token")
	}
	if len(s.List()) != 1 {
		t.Fatalf("list len = %d, want 1", len(s.List()))
	}

	// Revocation via another process is picked up too.
	if err := other.Remove(tkID(t, s)); err != nil {
		t.Fatal(err)
	}
	if s.Verify(raw) {
		t.Fatal("hot reload did not pick up the revocation")
	}
}

func tkID(t *testing.T, s *TokenStore) string {
	t.Helper()
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 token, got %d", len(list))
	}
	return list[0].ID
}

func TestTokenStoreMultipleTokens(t *testing.T) {
	s, err := OpenTokenStore(filepath.Join(t.TempDir(), "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw1, _, _ := s.Create("one")
	raw2, _, _ := s.Create("two")

	if !s.Verify(raw1) || !s.Verify(raw2) {
		t.Fatal("both tokens must verify")
	}
	if len(s.List()) != 2 {
		t.Fatalf("len = %d, want 2", len(s.List()))
	}
}
