package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swibrow/github-actions-tui/internal/cache"
	gh "github.com/swibrow/github-actions-tui/internal/github"
	"github.com/swibrow/github-actions-tui/internal/ui"
)

func main() {
	// Enable debug logging only when GHA_DEBUG=1
	if os.Getenv("GHA_DEBUG") == "1" {
		log.SetFlags(log.Ltime | log.Lshortfile)
	} else {
		log.SetOutput(io.Discard)
	}

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

	// Cache init is non-fatal
	var ghClient gh.GitHubClient = client
	done := make(chan struct{})
	if path, err := cache.DefaultPath(); err == nil {
		if store, err := cache.Open(path); err == nil {
			defer store.Close()
			cache.StartCleanup(store, 1*time.Hour, done)
			ghClient = cache.NewCachedClient(client, store, owner, repo)
			log.Printf("cache: using %s", path)
		} else {
			log.Printf("cache: disabled: %v", err)
		}
	}

	m := ui.NewModel(ghClient)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	close(done)
}
