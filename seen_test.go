package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSeenStoreDedupeWindow(t *testing.T) {
	s, err := OpenSeenStore(filepath.Join(t.TempDir(), "seen.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Two reports inside the window count once.
	if err := s.Add("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("a"); err != nil {
		t.Fatal(err)
	}
	if got := s.Count("a"); got != 1 {
		t.Fatalf("count = %d, want 1 (deduped)", got)
	}

	// A report after the window counts again.
	s.mu.Lock()
	s.counts["a"] = seenEntry{Count: s.counts["a"].Count, LastAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)}
	s.mu.Unlock()
	if err := s.Add("a"); err != nil {
		t.Fatal(err)
	}
	if got := s.Count("a"); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}

	// Remove resets.
	if err := s.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if got := s.Count("a"); got != 0 {
		t.Fatalf("count = %d, want 0 after remove", got)
	}
}

func TestSeenStoreLegacyFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	if err := os.WriteFile(path, []byte(`["legacy-id-1", "legacy-id-2"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSeenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Count("legacy-id-1") != 1 || s.Count("legacy-id-2") != 1 {
		t.Fatalf("legacy counts wrong: %d %d", s.Count("legacy-id-1"), s.Count("legacy-id-2"))
	}
}
