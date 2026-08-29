// Package main implements feedtui, a terminal client for the feed.
// self-hosted feed reader.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	server := flag.String("server", "", "feed server URL (default "+defaultServer+")")
	token := flag.String("token", "", "API token for the feed server")
	flag.Parse()

	cfgPath, err := configPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "feedtui: cannot determine config path: %v\n", err)
		os.Exit(1)
	}
	cfg := loadConfig(cfgPath)
	// Precedence: flag > environment > config file > built-in default.
	if *server != "" {
		cfg.Server = *server
	} else if env := os.Getenv("FEED_SERVER"); env != "" {
		cfg.Server = env
	}
	if *token != "" {
		cfg.Token = *token
	} else if env := os.Getenv("FEED_TOKEN"); env != "" {
		cfg.Token = env
	}
	if cfg.Server == "" {
		cfg.Server = defaultServer
	}

	c := newClient(cfg.Server, cfg.Token)
	p := tea.NewProgram(newApp(c, cfgPath, cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "feedtui: %v\n", err)
		os.Exit(1)
	}
}
