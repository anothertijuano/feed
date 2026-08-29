package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Shared styles.
var (
	styleSel   = lipgloss.NewStyle().Bold(true).Reverse(true)
	styleMeta  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleTab   = lipgloss.NewStyle().Bold(true).Underline(true)
	styleTitle = lipgloss.NewStyle().Bold(true)
)

// truncate cuts s to at most w display columns, appending "…" when needed.
func truncate(s string, w int) string {
	return runewidth.Truncate(s, w, "…")
}

// clamp bounds n to [lo, hi].
func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// padLines appends blank lines until lines has at least h entries.
func padLines(lines []string, h int) []string {
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}

// relAge renders a compact relative age ("3h") for an RFC3339 timestamp.
func relAge(now time.Time, ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dw", int(d/(7*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy", int(d/(365*24*time.Hour)))
	}
}

// itemAge returns the relative age of an item, preferring PublishedAt.
func itemAge(it Item) string {
	ts := it.PublishedAt
	if ts == "" {
		ts = it.FetchedAt
	}
	return relAge(time.Now(), ts)
}
