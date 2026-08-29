package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

// subsView lists subscriptions and manages them (add, delete, notify policy).
type subsView struct {
	client *client
	report reporter

	subs    []Subscription
	cursor  int
	loading bool
	loaded  bool
	err     error

	adding  bool
	confirm bool
	input   textinput.Model
}

func newSubsView(c *client, report reporter) *subsView {
	input := textinput.New()
	input.Placeholder = "https://example.com/feed.xml"
	input.Prompt = "> "
	input.Width = 60
	input.Focus()
	return &subsView{client: c, report: report, input: input}
}

func (s *subsView) activate() []tea.Cmd {
	if s.loading || s.loaded {
		return nil
	}
	return s.startLoad()
}

func (s *subsView) startLoad() []tea.Cmd {
	s.loading = true
	s.err = nil
	return []tea.Cmd{s.client.loadSubs()}
}

func (s *subsView) reload() []tea.Cmd {
	s.loaded = false
	s.err = nil
	return s.startLoad()
}

func (s *subsView) applyList(msg subsMsg) {
	s.loading = false
	if msg.err != nil {
		s.err = msg.err
		return
	}
	s.err = nil
	s.subs = msg.subs
	s.loaded = true
	s.cursor = clamp(s.cursor, 0, len(s.subs)-1)
}

func (s *subsView) applyNotify(msg notifyMsg) {
	for i := range s.subs {
		if s.subs[i].ID == msg.id {
			s.subs[i].Notify = msg.notify
			return
		}
	}
}

// Update handles key presses for the subscriptions view.
func (s *subsView) Update(msg tea.Msg) []tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	if s.adding {
		switch key.String() {
		case "esc":
			s.adding = false
			s.input.SetValue("")
			return nil
		case "enter":
			url := strings.TrimSpace(s.input.Value())
			s.adding = false
			s.input.SetValue("")
			if url == "" {
				return nil
			}
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				s.report("subscription URL must start with http:// or https://", true)
				return nil
			}
			return []tea.Cmd{s.client.addSub(url)}
		default:
			s.input, _ = s.input.Update(msg)
			return nil
		}
	}
	if s.confirm {
		if key.String() == "y" || key.String() == "enter" {
			s.confirm = false
			if s.cursor < len(s.subs) {
				return []tea.Cmd{s.client.deleteSub(s.subs[s.cursor].ID)}
			}
			return nil
		}
		s.confirm = false
		return nil
	}
	switch key.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < len(s.subs)-1 {
			s.cursor++
		}
	case "a":
		s.adding = true
		s.input.SetValue("")
		s.input.Focus()
	case "d":
		if len(s.subs) > 0 {
			s.confirm = true
		}
	case "n":
		if s.cursor < len(s.subs) {
			next := cycleNotify(s.subs[s.cursor].Notify)
			return []tea.Cmd{s.client.setNotify(s.subs[s.cursor].ID, next)}
		}
	case "r":
		return s.reload()
	}
	return nil
}

// cycleNotify rotates default → always → never → default.
func cycleNotify(current string) string {
	switch current {
	case "always":
		return "never"
	case "never":
		return "default"
	default:
		return "always"
	}
}

// View renders the subscription list within w columns and h rows.
func (s *subsView) View(w, h int) string {
	var lines []string
	extra := 0
	if s.adding {
		extra++
	}
	if s.confirm {
		extra++
	}
	rowsH := h - extra
	if rowsH < 0 {
		rowsH = 0
	}

	switch {
	case !s.loaded && s.loading:
		lines = append(lines, styleDim.Render("loading…"))
	case !s.loaded && s.err != nil:
		lines = append(lines, styleErr.Render("error: "+s.err.Error()))
	case s.loaded && len(s.subs) == 0:
		lines = append(lines, styleDim.Render("no subscriptions — press a to add one"))
	default:
		start := 0
		if s.cursor >= rowsH {
			start = s.cursor - rowsH + 1
		}
		if start+rowsH > len(s.subs) {
			start = len(s.subs) - rowsH
		}
		if start < 0 {
			start = 0
		}
		end := start + rowsH
		if end > len(s.subs) {
			end = len(s.subs)
		}
		for i := start; i < end; i++ {
			lines = append(lines, s.renderRow(s.subs[i], i == s.cursor, w))
		}
	}
	lines = padLines(lines, rowsH)
	if s.adding {
		lines = append(lines, "add URL: "+s.input.View())
	}
	if s.confirm && s.cursor < len(s.subs) {
		name := s.subs[s.cursor].Title
		if name == "" {
			name = s.subs[s.cursor].URL
		}
		lines = append(lines, styleWarn.Render("delete "+truncate(name, w-12)+"? (y/n)"))
	}
	return strings.Join(padLines(lines, h), "\n")
}

func (s *subsView) renderRow(sub Subscription, sel bool, w int) string {
	prefix := "  "
	if sel {
		prefix = "▸ "
	}
	title := sub.Title
	if title == "" {
		title = sub.URL
	}
	right := fmt.Sprintf("%d items", sub.ItemCount) + "  " + notifyBadge(sub.Notify)
	rightW := runewidth.StringWidth(right)
	if w < 40 {
		line := runewidth.Truncate(prefix+title, w, "…")
		if sel {
			line = styleSel.Width(w).Render(line)
		}
		return line
	}
	urlW := w / 3
	if urlW > 48 {
		urlW = 48
	}
	titleW := w - runewidth.StringWidth(prefix) - urlW - rightW - 3
	if titleW < 6 {
		titleW = 6
	}
	titleT := truncate(title, titleW)
	urlT := truncate(sub.URL, urlW)
	line := prefix + titleT + "  " + styleMeta.Render(urlT)
	pad := w - runewidth.StringWidth(prefix) - runewidth.StringWidth(titleT) - 2 - runewidth.StringWidth(urlT) - 1 - rightW
	if pad < 1 {
		pad = 1
	}
	line += strings.Repeat(" ", pad) + right
	if sel {
		line = styleSel.Width(w).Render(line)
	}
	return line
}

// notifyBadge renders the notify policy with a colored dot.
func notifyBadge(n string) string {
	switch n {
	case "always":
		return styleOK.Render("● always")
	case "never":
		return styleErr.Render("● never")
	default:
		return styleDim.Render("○ default")
	}
}
