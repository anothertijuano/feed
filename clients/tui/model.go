package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// reporter displays a one-line status message.
type reporter func(msg string, isErr bool)

// tabID identifies one of the app's tabs.
type tabID int

const (
	tabFeed tabID = iota
	tabSaved
	tabSubs
	tabSettings
)

var tabNames = []string{"Feed", "Saved", "Subs", "Settings"}

// app is the root bubbletea model.
type app struct {
	client  *client
	cfgPath string

	active tabID
	width  int
	height int

	spinner   spinner.Model
	pending   int // network operations in flight
	status    string
	statusErr bool
	help      bool

	seen map[string]bool

	feed     *listView
	saved    *listView
	subs     *subsView
	settings *settingsView
}

func newApp(c *client, cfgPath string, cfg Config) *app {
	m := &app{
		client:  c,
		cfgPath: cfgPath,
		width:   80,
		height:  24,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		seen:    make(map[string]bool),
	}
	m.feed = newListView(listFeed, c, m.setStatus)
	m.saved = newListView(listSaved, c, m.setStatus)
	m.subs = newSubsView(c, m.setStatus)
	m.settings = newSettingsView(c, cfgPath, cfg, m.setStatus)
	return m
}

func (m *app) Init() tea.Cmd {
	return m.run(m.feed.activate())
}

// run batches view commands and counts them as in-flight network work so the
// spinner stays visible until their results arrive.
func (m *app) run(cmds []tea.Cmd) tea.Cmd {
	m.pending += len(cmds)
	return tea.Batch(cmds...)
}

func (m *app) setStatus(msg string, isErr bool) {
	m.status = msg
	m.statusErr = isErr
}

func (m *app) setTab(t tabID) tea.Cmd {
	m.active = t
	return m.run(m.activateTab())
}

func (m *app) activateTab() []tea.Cmd {
	switch m.active {
	case tabFeed:
		return m.feed.activate()
	case tabSaved:
		return m.saved.activate()
	case tabSubs:
		return m.subs.activate()
	case tabSettings:
		return m.settings.activate()
	}
	return nil
}

// markSeen fires a best-effort seen interaction for every item that has not
// been displayed before.
func (m *app) markSeen(items []Item) []tea.Cmd {
	var cmds []tea.Cmd
	for _, it := range items {
		if it.ID == "" || m.seen[it.ID] {
			continue
		}
		m.seen[it.ID] = true
		cmds = append(cmds, m.client.markSeen(it.ID))
	}
	return cmds
}

func (m *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m, m.handleKey(msg)

	case feedPageMsg:
		m.pending--
		m.feed.applyPage(msg)
		if msg.err != nil {
			m.setStatus("feed: "+msg.err.Error(), true)
		} else if msg.offset == 0 {
			m.setStatus(fmt.Sprintf("feed loaded — %d items", msg.total), false)
		}
		return m, m.run(append(m.markSeen(msg.items), m.feed.maybeLoadMore()...))

	case savedListMsg:
		m.pending--
		m.saved.applySavedList(msg)
		if msg.err != nil {
			m.setStatus("saved: "+msg.err.Error(), true)
		}
		return m, nil

	case interactionMsg:
		m.pending--
		if msg.kind == "seen" {
			return m, nil // best effort: errors are ignored
		}
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		m.feed.applyInteraction(msg)
		m.saved.applyInteraction(msg)
		m.setStatus(interactionStatus(msg), false)
		return m, m.run(m.feed.maybeLoadMore())

	case subsMsg:
		m.pending--
		m.subs.applyList(msg)
		if msg.err != nil {
			m.setStatus("subs: "+msg.err.Error(), true)
		}
		return m, nil

	case subAddedMsg:
		m.pending--
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		name := msg.sub.Title
		if name == "" {
			name = msg.sub.URL
		}
		m.setStatus("added "+name, false)
		return m, m.run(m.subs.reload())

	case subDeletedMsg:
		m.pending--
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		m.setStatus("subscription deleted", false)
		return m, m.run(m.subs.reload())

	case notifyMsg:
		m.pending--
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		m.subs.applyNotify(msg)
		m.setStatus("notify: "+msg.notify, false)
		return m, nil

	case refreshMsg:
		m.pending--
		if msg.err != nil {
			m.setStatus("refresh: "+msg.err.Error(), true)
			return m, nil
		}
		m.setStatus(fmt.Sprintf("refresh done — %d new items", msg.new), false)
		return m, m.run(m.feed.reload())

	case settingsMsg:
		m.pending--
		if msg.err != nil {
			m.settings.loadFailed()
			m.setStatus("settings: "+msg.err.Error(), true)
			return m, nil
		}
		m.settings.applyServerSettings(msg.settings)
		return m, nil

	case settingsSavedMsg:
		m.pending--
		if msg.err != nil {
			m.setStatus("memos settings: "+msg.err.Error(), true)
			return m, nil
		}
		m.setStatus("memos settings updated", false)
		return m, nil

	case healthMsg:
		m.pending--
		if msg.err != nil {
			m.setStatus("server: "+msg.err.Error(), true)
			return m, nil
		}
		m.setStatus("server ok", false)
		return m, nil

	case openMsg:
		m.pending--
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		}
	}
	return m, nil
}

func (m *app) handleKey(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}
	if m.help {
		switch msg.String() {
		case "?", "esc", "q", "enter":
			m.help = false
		}
		return nil
	}
	// While typing in an input field, pass every key through to the view.
	if m.active == tabSettings && m.settings.focused() {
		return m.run(m.settings.Update(msg))
	}
	if m.active == tabSubs && m.subs.adding {
		return m.run(m.subs.Update(msg))
	}
	switch msg.String() {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
		return nil
	case "tab", "shift+tab":
		delta := 1
		if msg.String() == "shift+tab" {
			delta = -1
		}
		return m.setTab(tabID((int(m.active) + delta + len(tabNames)) % len(tabNames)))
	case "1":
		return m.setTab(tabFeed)
	case "2":
		return m.setTab(tabSaved)
	case "3":
		return m.setTab(tabSubs)
	case "4":
		return m.setTab(tabSettings)
	}
	switch m.active {
	case tabFeed:
		return m.run(m.feed.Update(msg))
	case tabSaved:
		return m.run(m.saved.Update(msg))
	case tabSubs:
		return m.run(m.subs.Update(msg))
	case tabSettings:
		return m.run(m.settings.Update(msg))
	}
	return nil
}

// View renders the header (tabs + spinner), the active view and the status
// footer.
func (m *app) View() string {
	if m.help {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, helpBox())
	}
	contentH := m.height - 2
	if contentH < 1 {
		contentH = 1
	}
	var content string
	switch m.active {
	case tabFeed:
		content = m.feed.View(m.width, contentH)
	case tabSaved:
		content = m.saved.View(m.width, contentH)
	case tabSubs:
		content = m.subs.View(m.width, contentH)
	case tabSettings:
		content = m.settings.View(m.width, contentH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), content, m.renderFooter())
}

func (m *app) renderHeader() string {
	var parts []string
	var plain []string
	for i, name := range tabNames {
		label := name
		switch tabID(i) {
		case tabFeed:
			if m.feed.loaded {
				label = fmt.Sprintf("%s(%d)", name, m.feed.total)
			}
		case tabSaved:
			if m.saved.loaded {
				label = fmt.Sprintf("%s(%d)", name, m.saved.total)
			}
		}
		plain = append(plain, label)
		if tabID(i) == m.active {
			parts = append(parts, styleTab.Render(label))
		} else {
			parts = append(parts, styleDim.Render(label))
		}
	}
	left := "feedtui  " + strings.Join(parts, "  ")
	leftW := runewidth.StringWidth("feedtui  " + strings.Join(plain, "  "))
	right := ""
	if m.pending > 0 {
		right = styleDim.Render(m.spinner.View() + " loading")
	}
	rightW := runewidth.StringWidth(m.spinner.View() + " loading")
	pad := m.width - leftW - rightW - 1
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

func (m *app) renderFooter() string {
	switch {
	case m.statusErr:
		return styleErr.Render("✗ " + m.status)
	case m.status != "":
		return styleOK.Render("✓ " + m.status)
	case m.pending > 0:
		return styleDim.Render(m.spinner.View() + " loading…")
	default:
		return styleDim.Render("q quit · ? help · tab switch tab")
	}
}

// interactionStatus renders the success message for a confirmed interaction.
func interactionStatus(msg interactionMsg) string {
	switch msg.kind {
	case "vote":
		switch msg.vote {
		case 1:
			return "upvoted"
		case -1:
			return "downvoted — item removed"
		default:
			return "vote cleared"
		}
	case "save":
		if msg.saved {
			return "saved"
		}
		return "removed from saved"
	}
	return "sent"
}
