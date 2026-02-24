package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

type RunsModel struct {
	table   table.Model
	runs    []gh.WorkflowRun
	focused bool
	loading bool
	width   int
	height  int
	title   string
}

func NewRunsModel() RunsModel {
	columns := []table.Column{
		{Title: " ", Width: 2},
		{Title: "#", Width: 6},
		{Title: "Branch", Width: 16},
		{Title: "Event", Width: 12},
		{Title: "Actor", Width: 14},
		{Title: "Age", Width: 8},
		{Title: "Duration", Width: 8},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorBorder).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("15")).
		Background(colorPrimary).
		Bold(false)
	t.SetStyles(s)

	return RunsModel{table: t, title: "Workflow Runs"}
}

func (m *RunsModel) SetRuns(runs []gh.WorkflowRun) {
	m.runs = runs
	m.loading = false
	rows := make([]table.Row, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, table.Row{
			StatusIcon(r.Status, r.Conclusion),
			fmt.Sprintf("#%d", r.Number),
			truncate(r.Branch, 16),
			r.Event,
			truncate(r.Actor, 14),
			relativeTime(r.CreatedAt),
			formatDuration(r.Duration),
		})
	}
	m.table.SetRows(rows)
	if len(rows) > 0 {
		m.table.GotoTop()
	}
}

func (m RunsModel) SelectedRun() *gh.WorkflowRun {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.runs) {
		return nil
	}
	r := m.runs[idx]
	return &r
}

func (m *RunsModel) SetFocused(focused bool) {
	m.focused = focused
	if focused {
		m.table.Focus()
	} else {
		m.table.Blur()
	}
}

func (m *RunsModel) SetLoading(loading bool) {
	m.loading = loading
}

func (m *RunsModel) SetTitle(title string) {
	m.title = title
}

func (m *RunsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.table.SetWidth(width - 4)
	m.table.SetHeight(height - 5) // border + title + header

	// Auto-size columns based on available width
	available := width - 8 // borders + padding
	fixed := 2 + 6 + 12 + 8 + 8
	remaining := available - fixed
	branchW := clamp(remaining*40/100, 10, 30)
	actorW := remaining - branchW
	actorW = clamp(actorW, 8, 20)

	m.table.SetColumns([]table.Column{
		{Title: " ", Width: 2},
		{Title: "#", Width: 6},
		{Title: "Branch", Width: branchW},
		{Title: "Event", Width: 12},
		{Title: "Actor", Width: actorW},
		{Title: "Age", Width: 8},
		{Title: "Duration", Width: 8},
	})
}

func (m RunsModel) Update(msg tea.Msg) (RunsModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m RunsModel) View() string {
	style := styleMainBlurred
	if m.focused {
		style = styleMainFocused
	}

	var content string
	header := styleTitle.Render(m.title) + "\n"

	if m.loading {
		content = header + styleLoading.Render("  Loading runs...")
	} else if len(m.runs) == 0 {
		content = header + styleLoading.Render("  No runs found")
	} else {
		content = header + m.table.View()
	}

	lines := strings.Split(content, "\n")
	innerH := m.height - 2
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	content = strings.Join(lines[:innerH], "\n")

	return style.Width(m.width - 2).Height(m.height - 2).Render(content)
}
