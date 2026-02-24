package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	gh "github.com/swibrow/github-actions-tui/internal/github"
	"github.com/swibrow/github-actions-tui/internal/ui"
)

func main() {
	owner, repo, err := gh.DetectRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client, err := gh.NewClient(owner, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	m := ui.NewModel(client)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
