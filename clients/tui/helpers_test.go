package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRelAge(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	ts := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{2 * 24 * time.Hour, "2d"},
		{3 * 7 * 24 * time.Hour, "3w"},
		{400 * 24 * time.Hour, "1y"},
	}
	for _, tc := range cases {
		if got := relAge(now, ts(tc.d)); got != tc.want {
			t.Errorf("relAge(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
	if got := relAge(now, "not-a-timestamp"); got != "" {
		t.Errorf("relAge(garbage) = %q, want empty", got)
	}
}

func TestItemAgeFallsBackToFetchedAt(t *testing.T) {
	it := Item{FetchedAt: time.Now().Add(-2 * time.Hour).Format(time.RFC3339)}
	if got := itemAge(it); got == "" {
		t.Error("itemAge with only fetchedAt returned empty")
	}
}

func TestCycleNotify(t *testing.T) {
	if got := cycleNotify(""); got != "always" {
		t.Errorf("cycleNotify(default) = %q, want always", got)
	}
	if got := cycleNotify("default"); got != "always" {
		t.Errorf("cycleNotify(default) = %q, want always", got)
	}
	if got := cycleNotify("always"); got != "never" {
		t.Errorf("cycleNotify(always) = %q, want never", got)
	}
	if got := cycleNotify("never"); got != "default" {
		t.Errorf("cycleNotify(never) = %q, want default", got)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feedtui", "config.json")
	want := Config{Server: "https://feed.example.com", Token: "tok"}
	if err := saveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	if got := loadConfig(path); got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	if got := loadConfig(filepath.Join(t.TempDir(), "missing.json")); got != (Config{}) {
		t.Fatalf("missing file = %+v, want zero config", got)
	}
}

func TestClampAndPadLines(t *testing.T) {
	if got := clamp(5, 0, 3); got != 3 {
		t.Errorf("clamp(5,0,3) = %d", got)
	}
	if got := clamp(-1, 0, 3); got != 0 {
		t.Errorf("clamp(-1,0,3) = %d", got)
	}
	lines := padLines([]string{"a"}, 4)
	if len(lines) != 4 || lines[0] != "a" || lines[3] != "" {
		t.Errorf("padLines = %#v", lines)
	}
}

func TestInteractionStatus(t *testing.T) {
	cases := []struct {
		msg  interactionMsg
		want string
	}{
		{interactionMsg{kind: "vote", vote: 1}, "upvoted"},
		{interactionMsg{kind: "vote", vote: -1}, "downvoted — item removed"},
		{interactionMsg{kind: "vote", vote: 0}, "vote cleared"},
		{interactionMsg{kind: "save", saved: true}, "saved"},
		{interactionMsg{kind: "save", saved: false}, "removed from saved"},
	}
	for _, tc := range cases {
		if got := interactionStatus(tc.msg); got != tc.want {
			t.Errorf("interactionStatus(%+v) = %q, want %q", tc.msg, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello world", 5); len(got) > 5+len("…") {
		t.Errorf("truncate too long: %q", got)
	}
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("truncate(hi,10) = %q, want hi", got)
	}
}
