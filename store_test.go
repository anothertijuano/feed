package main

import (
	"path/filepath"
	"testing"
)

func TestStoreVoteSaveRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feed.json")

	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Vote("content/01.txt"); got != 0 {
		t.Fatalf("vote = %d, want 0", got)
	}
	if s.IsSaved("content/01.txt") {
		t.Fatal("expected not saved")
	}

	if err := s.SetVote("content/01.txt", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSaved("content/01.txt", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetVote("content/02.txt", -1); err != nil {
		t.Fatal(err)
	}

	// Reopen: state must survive a restart.
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Vote("content/01.txt"); got != 1 {
		t.Fatalf("vote = %d, want 1", got)
	}
	if !s2.IsSaved("content/01.txt") {
		t.Fatal("expected saved")
	}
	if got := s2.Vote("content/02.txt"); got != -1 {
		t.Fatalf("vote = %d, want -1", got)
	}

	// Clearing a vote removes it from the snapshot.
	if err := s2.SetVote("content/01.txt", 0); err != nil {
		t.Fatal(err)
	}
	if err := s2.SetSaved("content/01.txt", false); err != nil {
		t.Fatal(err)
	}
	s3, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	snap := s3.Snapshot()
	if _, ok := snap.Votes["content/01.txt"]; ok {
		t.Fatal("expected cleared vote to be absent")
	}
	if snap.Saved["content/01.txt"] {
		t.Fatal("expected cleared save to be absent")
	}
	if snap.Votes["content/02.txt"] != -1 {
		t.Fatalf("votes = %+v, want content/02.txt preserved", snap.Votes)
	}
}

func TestOpenStoreMissingFile(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "nested", "feed.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Writing creates the parent directory.
	if err := s.SetVote("content/01.txt", 1); err != nil {
		t.Fatal(err)
	}
}
