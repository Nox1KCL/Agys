// Command agys provides a Bubble Tea TUI for browsing, searching, and resuming Google Antigravity CLI sessions.
package main

import (
	"fmt"
	"os"

	"github.com/Nox1KCL/Agys/internal/config"
	"github.com/Nox1KCL/Agys/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agys: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Default()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	program := tea.NewProgram(tui.New(cfg), tea.WithAltScreen(), tea.WithReportFocus())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}

	return nil
}
