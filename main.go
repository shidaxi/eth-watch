package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	inK8s := isInK8s()

	// In K8s mode, RPCs are optional — discovery will populate them.
	// Outside K8s, at least one RPC must be configured.
	if !inK8s && len(cfg.RPCs) == 0 {
		fmt.Fprintf(os.Stderr, "No RPCs configured. Set via config file (%s) or RPCS env var.\n", *configPath)
		os.Exit(1)
	}

	m := newModel(cfg.RPCs, cfg.IntervalDuration(), inK8s)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
