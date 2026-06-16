package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

// seedModel builds a Model populated with fake workflows + runs at a fixed size.
func seedModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil, "swibrow", "github-actions-tui")
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = tm.(Model)

	tm, _ = m.Update(WorkflowsMsg{Workflows: []gh.Workflow{
		{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml"},
		{ID: 2, Name: "Release", Path: ".github/workflows/release.yml"},
		{ID: 3, Name: "CodeQL", Path: ".github/workflows/codeql.yml"},
	}})
	m = tm.(Model)

	now := time.Now()
	tm, _ = m.Update(RunsMsg{ResetCursor: true, Runs: []gh.WorkflowRun{
		{ID: 101, WorkflowID: 1, Number: 412, Name: "CI", Status: "completed", Conclusion: "success", Branch: "main", HeadSHA: "a1b2c3d4e5", Event: "push", Actor: "swibrow", CreatedAt: now.Add(-5 * time.Minute), Duration: 92 * time.Second},
		{ID: 102, WorkflowID: 1, Number: 411, Name: "CI", Status: "in_progress", Branch: "feat/pretty", HeadSHA: "f6g7h8i9j0", Event: "pull_request", Actor: "octocat", CreatedAt: now.Add(-12 * time.Minute)},
		{ID: 103, WorkflowID: 1, Number: 410, Name: "CI", Status: "completed", Conclusion: "failure", Branch: "fix/bug", HeadSHA: "k1l2m3n4o5", Event: "push", Actor: "swibrow", CreatedAt: now.Add(-2 * time.Hour), Duration: 45 * time.Second},
	}})
	return tm.(Model)
}

// TestRenderViewsNoPanic exercises every top-level view + overlay and asserts
// each produces a non-empty frame without panicking.
func TestRenderViewsNoPanic(t *testing.T) {
	base := seedModel(t)

	cases := map[string]func(Model) Model{
		"runs": func(m Model) Model { return m },
		"jobs": func(m Model) Model {
			m.view = ViewJobs
			m.graph.SetJobs([]gh.WorkflowJob{
				{Name: "build", Status: "completed", Conclusion: "success", Duration: 30 * time.Second},
				{Name: "test", Status: "in_progress"},
			}, nil, "#412 main")
			return m
		},
		"help":    func(m Model) Model { m.showHelp = true; return m },
		"quit":    func(m Model) Model { m.confirmQuit = true; return m },
		"rerun":   func(m Model) Model { m.rerunChoice = true; return m },
		"sidebar": func(m Model) Model { tm, _ := m.toggleFocus(); return tm.(Model) },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			frame := mutate(base).View().Content
			if strings.TrimSpace(frame) == "" {
				t.Fatalf("%s: empty frame", name)
			}
		})
	}
}
