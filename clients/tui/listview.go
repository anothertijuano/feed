package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

// listKind selects the data source of a listView.
type listKind int

const (
	listFeed listKind = iota
	listSaved
)

// listView renders a scrollable list of feed items (feed or saved).
type listView struct {
	kind   listKind
	client *client
	report reporter

	items      []Item
	total      int
	cursor     int
	nextOffset int
	loaded     bool
	loading    bool
	err        error
}

func newListView(kind listKind, c *client, report reporter) *listView {
	return &listView{kind: kind, client: c, report: report}
}

// activate starts the initial load if it hasn't happened yet.
func (v *listView) activate() []tea.Cmd {
	if v.loading || v.loaded {
		return nil
	}
	return v.startLoad()
}

func (v *listView) startLoad() []tea.Cmd {
	v.loading = true
	v.err = nil
	if v.kind == listSaved {
		return []tea.Cmd{v.client.loadSaved()}
	}
	return []tea.Cmd{v.client.loadFeed(v.nextOffset, pageSize)}
}

// reload clears the list and fetches the first page again.
func (v *listView) reload() []tea.Cmd {
	v.items = nil
	v.total = 0
	v.cursor = 0
	v.nextOffset = 0
	v.loaded = false
	v.err = nil
	return v.startLoad()
}

// maybeLoadMore fetches the next page when the cursor nears the bottom of the
// loaded items. Saved lists load in a single request, so it is a no-op there.
func (v *listView) maybeLoadMore() []tea.Cmd {
	if v.kind == listSaved || v.loading || !v.loaded {
		return nil
	}
	if v.total <= len(v.items) {
		return nil
	}
	if v.cursor >= len(v.items)-5 {
		v.loading = true
		return []tea.Cmd{v.client.loadFeed(v.nextOffset, pageSize)}
	}
	return nil
}

func (v *listView) applyPage(msg feedPageMsg) {
	// Ignore stale responses from a superseded load (e.g. after a reload).
	if msg.offset != v.nextOffset {
		return
	}
	v.loading = false
	if msg.err != nil {
		v.err = msg.err
		return
	}
	v.err = nil
	v.items = append(v.items, msg.items...)
	v.nextOffset += len(msg.items)
	v.total = msg.total
	v.loaded = true
}

func (v *listView) applySavedList(msg savedListMsg) {
	v.loading = false
	if msg.err != nil {
		v.err = msg.err
		return
	}
	v.err = nil
	v.items = msg.items
	v.total = len(msg.items)
	v.loaded = true
	v.cursor = clamp(v.cursor, 0, len(v.items)-1)
}

func (v *listView) indexOf(key string) int {
	for i := range v.items {
		if v.items[i].ID == key {
			return i
		}
	}
	return -1
}

// applyInteraction updates the local list to match a confirmed interaction.
func (v *listView) applyInteraction(msg interactionMsg) {
	idx := v.indexOf(msg.key)
	switch msg.kind {
	case "vote":
		if idx < 0 {
			if msg.vote == -1 && v.total > 0 {
				v.total--
			}
			return
		}
		if msg.vote == -1 {
			// A downvote removes the item from the system permanently.
			v.items = append(v.items[:idx], v.items[idx+1:]...)
			if v.nextOffset > 0 {
				v.nextOffset--
			}
			if v.total > 0 {
				v.total--
			}
			v.cursor = clamp(v.cursor, 0, len(v.items)-1)
			return
		}
		v.items[idx].Vote = msg.vote
	case "save":
		if idx < 0 {
			return
		}
		v.items[idx].Saved = msg.saved
		if !msg.saved && v.kind == listSaved {
			v.items = append(v.items[:idx], v.items[idx+1:]...)
			if v.total > 0 {
				v.total--
			}
			v.cursor = clamp(v.cursor, 0, len(v.items)-1)
		}
	}
}

// Update handles key presses for the list.
func (v *listView) Update(msg tea.Msg) []tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "up", "k":
		if v.cursor > 0 {
			v.cursor--
		}
		return v.maybeLoadMore()
	case "down", "j":
		if v.cursor < len(v.items)-1 {
			v.cursor++
		}
		return v.maybeLoadMore()
	case "enter", "o":
		if v.cursor >= len(v.items) {
			return nil
		}
		if v.items[v.cursor].Link == "" {
			v.report("item has no link", true)
			return nil
		}
		return []tea.Cmd{openURL(v.items[v.cursor].Link)}
	case "u", "d", "0":
		if v.cursor >= len(v.items) {
			return nil
		}
		value := 1
		if key.String() == "d" {
			value = -1
		} else if key.String() == "0" {
			value = 0
		}
		return []tea.Cmd{v.client.sendInteraction(v.items[v.cursor].ID, "vote", value)}
	case "s":
		if v.cursor >= len(v.items) {
			return nil
		}
		it := v.items[v.cursor]
		return []tea.Cmd{v.client.sendInteraction(it.ID, "save", !it.Saved)}
	case "r":
		return v.reload()
	}
	return nil
}

// View renders the list within w columns and h rows.
func (v *listView) View(w, h int) string {
	if !v.loaded {
		switch {
		case v.loading:
			return strings.Join(padLines([]string{styleDim.Render("loading…")}, h), "\n")
		case v.err != nil:
			lines := []string{
				styleErr.Render("error: " + v.err.Error()),
				styleDim.Render("press r to retry"),
			}
			return strings.Join(padLines(lines, h), "\n")
		default:
			return strings.Join(padLines(nil, h), "\n")
		}
	}
	if len(v.items) == 0 {
		msg := "no items — press r to refresh"
		if v.kind == listSaved {
			msg = "nothing saved yet — press s on a feed item to save it"
		}
		return strings.Join(padLines([]string{styleDim.Render(msg)}, h), "\n")
	}

	maxRows := h
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if v.cursor >= maxRows {
		start = v.cursor - maxRows + 1
	}
	if start+maxRows > len(v.items) {
		start = len(v.items) - maxRows
	}
	if start < 0 {
		start = 0
	}
	end := start + maxRows
	if end > len(v.items) {
		end = len(v.items)
	}
	lines := make([]string, 0, maxRows)
	for i := start; i < end; i++ {
		lines = append(lines, v.renderRow(v.items[i], i == v.cursor, w))
	}
	return strings.Join(padLines(lines, h), "\n")
}

// renderRow draws one item: cursor marker, vote (▲/▼), save mark (★), title,
// source and a right-aligned relative age.
func (v *listView) renderRow(it Item, selected bool, w int) string {
	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	vote := " "
	switch it.Vote {
	case 1:
		vote = "▲"
	case -1:
		vote = "▼"
	}
	save := " "
	if it.Saved {
		save = "★"
	}
	age := itemAge(it)
	meta := it.SourceName
	if meta == "" {
		meta = "-"
	}

	left := prefix + vote + save + " "
	leftW := runewidth.StringWidth(left)
	ageW := runewidth.StringWidth(age)
	metaW := runewidth.StringWidth(meta)

	// On very narrow terminals fall back to title only.
	if w < leftW+metaW+ageW+12 {
		line := runewidth.Truncate(left+it.Title, w, "…")
		if selected {
			line = styleSel.Width(w).Render(line)
		}
		return line
	}

	titleW := w - leftW - metaW - ageW - 4
	if titleW < 6 {
		titleW = 6
	}
	title := truncate(it.Title, titleW)
	line := left + title + " " + styleMeta.Render(meta)
	pad := w - leftW - runewidth.StringWidth(title) - 1 - metaW - 1 - ageW
	if pad < 0 {
		pad = 0
	}
	line += strings.Repeat(" ", pad) + " " + age
	if selected {
		line = styleSel.Width(w).Render(line)
	}
	return line
}
