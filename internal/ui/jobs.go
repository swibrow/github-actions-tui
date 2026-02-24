package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

type JobsModel struct {
	table   table.Model
	jobs    []gh.WorkflowJob
	focused bool
	loading bool
	width   int
	height  int
	runName string
}

func NewJobsModel() JobsModel {
	columns := []table.Column{
		{Title: " ", Width: 2},
		{Title: "Job", Width: 40},
		{Title: "Duration", Width: 10},
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

	return JobsModel{table: t}
}

func (m *JobsModel) SetJobs(jobs []gh.WorkflowJob, runName string) {
	m.jobs = jobs
	m.runName = runName
	m.loading = false
	rows := make([]table.Row, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, table.Row{
			StatusIcon(j.Status, j.Conclusion),
			truncate(j.Name, 40),
			formatDuration(j.Duration),
		})
	}
	m.table.SetRows(rows)
	if len(rows) > 0 {
		m.table.GotoTop()
	}
}

func (m JobsModel) SelectedJob() *gh.WorkflowJob {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.jobs) {
		return nil
	}
	j := m.jobs[idx]
	return &j
}

func (m *JobsModel) SetFocused(focused bool) {
	m.focused = focused
	if focused {
		m.table.Focus()
	} else {
		m.table.Blur()
	}
}

func (m *JobsModel) SetLoading(loading bool) {
	m.loading = loading
}

func (m *JobsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.table.SetWidth(width - 4)
	m.table.SetHeight(height - 5)

	jobW := width - 20
	if jobW < 20 {
		jobW = 20
	}
	m.table.SetColumns([]table.Column{
		{Title: " ", Width: 2},
		{Title: "Job", Width: jobW},
		{Title: "Duration", Width: 10},
	})
}

func (m JobsModel) Update(msg tea.Msg) (JobsModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m JobsModel) View() string {
	style := styleMainFocused

	title := styleTitle.Render(fmt.Sprintf("Jobs: %s", m.runName)) + "\n"
	var content string
	if m.loading {
		content = title + styleLoading.Render("  Loading jobs...")
	} else if len(m.jobs) == 0 {
		content = title + styleLoading.Render("  No jobs found")
	} else {
		content = title + m.table.View()
	}

	lines := strings.Split(content, "\n")
	innerH := m.height - 2
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	content = strings.Join(lines[:innerH], "\n")

	return style.Width(m.width - 2).Height(m.height - 2).Render(content)
}
