package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// settingsView edits the local client configuration (server, token) and the
// server-side memos settings. Saving persists the local config to disk,
// applies it to the running client, pushes the memos settings to the server
// and health-checks the new server.
type settingsView struct {
	client  *client
	cfgPath string
	report  reporter

	server     textinput.Model
	token      textinput.Model
	memosURL   textinput.Model
	memosToken textinput.Model

	fields []*textinput.Model
	focus  int // -1 = browsing, 0..len(fields)-1 = editing a field

	serverLoaded bool
	loading      bool
}

func newSettingsView(c *client, cfgPath string, cfg Config, report reporter) *settingsView {
	s := &settingsView{
		client:  c,
		cfgPath: cfgPath,
		report:  report,
		focus:   -1,
	}
	mk := func(placeholder string, secret bool) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.Prompt = ""
		t.Width = 50
		if secret {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		return t
	}
	s.server = mk(defaultServer, false)
	s.token = mk("feed token", true)
	s.memosURL = mk("https://memos.example.com", false)
	s.memosToken = mk("memos token", true)
	s.server.SetValue(cfg.Server)
	s.token.SetValue(cfg.Token)
	s.fields = []*textinput.Model{&s.server, &s.token, &s.memosURL, &s.memosToken}
	return s
}

// focused reports whether a text field is being edited (the root model routes
// tab/shift+tab to the fields while this is true).
func (s *settingsView) focused() bool { return s.focus >= 0 }

func (s *settingsView) activate() []tea.Cmd {
	if s.loading || s.serverLoaded {
		return nil
	}
	return s.startLoad()
}

func (s *settingsView) startLoad() []tea.Cmd {
	s.loading = true
	return []tea.Cmd{s.client.loadSettings()}
}

func (s *settingsView) applyServerSettings(ss serverSettings) {
	s.loading = false
	s.serverLoaded = true
	if s.memosURL.Value() == "" {
		s.memosURL.SetValue(ss.MemosURL)
	}
	if s.memosToken.Value() == "" {
		s.memosToken.SetValue(ss.MemosToken)
	}
}

func (s *settingsView) loadFailed() {
	s.loading = false
	s.serverLoaded = false
}

func (s *settingsView) setFocus(i int) {
	s.focus = clamp(i, -1, len(s.fields)-1)
	for _, f := range s.fields {
		f.Blur()
	}
	if s.focus >= 0 {
		s.fields[s.focus].Focus()
	}
}

// save persists the local config and pushes the memos settings to the server.
func (s *settingsView) save() []tea.Cmd {
	server := strings.TrimRight(strings.TrimSpace(s.server.Value()), "/")
	if server == "" {
		server = defaultServer
	}
	token := strings.TrimSpace(s.token.Value())
	changed := server != s.client.server || token != s.client.token
	s.client.setServer(server)
	s.client.setToken(token)

	cfg := Config{Server: server, Token: token}
	if err := saveConfig(s.cfgPath, cfg); err != nil {
		s.report("config save failed: "+err.Error(), true)
	} else {
		s.report("config saved to "+s.cfgPath, false)
	}

	cmds := []tea.Cmd{
		s.client.saveSettings(serverSettings{
			MemosURL:   strings.TrimSpace(s.memosURL.Value()),
			MemosToken: strings.TrimSpace(s.memosToken.Value()),
		}),
		s.client.checkHealth(),
	}
	if changed {
		// Refetch the memos settings from the new server.
		s.serverLoaded = false
		cmds = append(cmds, s.client.loadSettings())
	}
	return cmds
}

// Update handles key presses for the settings view.
func (s *settingsView) Update(msg tea.Msg) []tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	if s.focus >= 0 {
		switch key.String() {
		case "esc":
			s.setFocus(-1)
			return nil
		case "tab", "down":
			s.setFocus((s.focus + 1) % len(s.fields))
			return nil
		case "shift+tab", "up":
			s.setFocus((s.focus - 1 + len(s.fields)) % len(s.fields))
			return nil
		case "enter":
			if s.focus == len(s.fields)-1 {
				s.setFocus(-1)
				return s.save()
			}
			s.setFocus(s.focus + 1)
			return nil
		case "ctrl+s":
			s.setFocus(-1)
			return s.save()
		default:
			in := s.fields[s.focus]
			updated, _ := in.Update(msg)
			*s.fields[s.focus] = updated
			return nil
		}
	}
	switch key.String() {
	case "e", "enter":
		s.setFocus(0)
	case "r":
		s.serverLoaded = false
		s.loading = false
		return s.startLoad()
	}
	return nil
}

// View renders the settings form within w columns and h rows.
func (s *settingsView) View(w, h int) string {
	inputW := w - 22
	if inputW > 50 {
		inputW = 50
	}
	if inputW < 10 {
		inputW = 10
	}
	for _, f := range s.fields {
		f.Width = inputW
	}

	labels := []string{"Server", "Token", "Memos URL", "Memos token"}
	var lines []string
	lines = append(lines,
		styleTitle.Render("Settings")+"  "+
			styleDim.Render("e edit · tab next field · enter saves on the last field · ctrl+s save · esc cancel · r reload"))
	lines = append(lines, "")
	for i, f := range s.fields {
		marker := "  "
		if s.focus == i {
			marker = "▸ "
		}
		lines = append(lines, marker+styleDim.Render(fmt.Sprintf("%-12s", labels[i]))+" "+f.View())
	}
	lines = append(lines, "")
	lines = append(lines, styleDim.Render("config file: "+s.cfgPath))
	if !s.serverLoaded && !s.loading {
		lines = append(lines, styleDim.Render("server memos settings not loaded — press r"))
	}
	return strings.Join(padLines(lines, h), "\n")
}
