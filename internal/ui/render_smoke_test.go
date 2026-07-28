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
		"runs-list": func(m Model) Model {
			m.runs.SetLayout(LayoutList)
			return m
		},
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

// TestToggleLayoutSwitchesRendering asserts v flips the runs pane between the
// card grid and the row list, and that the selection survives the switch.
func TestToggleLayoutSwitchesRendering(t *testing.T) {
	m := seedModel(t)
	grid := m.View().Content

	tm, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = tm.(Model)
	if m.runs.Layout() != LayoutList {
		t.Fatalf("expected list layout, got %v", m.runs.Layout())
	}
	list := m.View().Content
	if list == grid {
		t.Fatal("list frame identical to grid frame")
	}
	if !strings.Contains(list, "Branch") {
		t.Fatal("list frame missing column header")
	}
	if got := m.runs.SelectedRun(); got == nil || got.Number != 412 {
		t.Fatalf("selection lost across toggle: %+v", got)
	}

	tm, _ = m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if tm.(Model).runs.Layout() != LayoutGrid {
		t.Fatal("expected toggle back to grid")
	}
}

// TestListLayoutFitsWidth asserts the columns never overflow the pane and fill
// it exactly once there is room for the full set.
func TestListLayoutFitsWidth(t *testing.T) {
	for w := 1; w <= 200; w++ {
		idx, widths := listLayout(w)
		if len(idx) == 0 {
			t.Fatalf("width %d: no columns selected", w)
		}
		total := listColGap * (len(idx) - 1)
		for _, cw := range widths {
			total += cw
		}
		if total > w {
			t.Fatalf("width %d: columns total %d, overflows by %d", w, total, total-w)
		}
		if w >= 90 && total != w {
			t.Fatalf("width %d: columns total %d, does not fill the pane", w, total)
		}
	}
}
