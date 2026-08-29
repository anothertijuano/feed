package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpBox renders the help overlay shown while the ? key is held open.
func helpBox() string {
	lines := []string{
		" feedtui — keys",
		"",
		" tab / shift+tab   next / previous tab        1-4   jump to tab",
		" ↑ / ↓, j / k      move cursor",
		" u                 upvote        d   downvote (removes the item)",
		" 0                 clear vote    s   toggle save",
		" o / enter         open link in the system browser",
		" r                 refresh feed, then reload",
		" a                 add subscription (Subs tab)",
		" d                 delete subscription (Subs tab, confirm with y)",
		" n                 cycle notify policy (Subs tab)",
		" e / enter         edit a settings field",
		" enter / ctrl+s    save settings (enter saves on the last field)",
		" esc               cancel editing",
		" ?                 this help     q   quit",
	}
	width := 0
	for _, l := range lines {
		if n := len(l); n > width {
			width = n
		}
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(lines, "\n"))
}
