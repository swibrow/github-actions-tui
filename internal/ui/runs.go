package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		{Title: "#", Width: 4},
		{Title: "Action", Width: 16},
		{Title: "Branch", Width: 16},
		{Title: "SHA", Width: 7},
		{Title: "Event", Width: 10},
		{Title: "Actor", Width: 14},
		{Title: "Age", Width: 6},
		{Title: "Dur", Width: 6},
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
	m.setRuns(runs, false)
}

func (m *RunsModel) SetRunsAndReset(runs []gh.WorkflowRun) {
	m.setRuns(runs, true)
}

func (m *RunsModel) setRuns(runs []gh.WorkflowRun, resetCursor bool) {
	m.runs = runs
	m.loading = false
	rows := make([]table.Row, 0, len(runs))
	for _, r := range runs {
		num := fmt.Sprintf("%d", r.Number)
		if r.RunAttempt > 1 {
			num = fmt.Sprintf("%d·%d", r.Number, r.RunAttempt)
		}
		sha := ""
		if len(r.HeadSHA) >= 7 {
			sha = r.HeadSHA[:7]
		}
		rows = append(rows, table.Row{
			StatusIconPlain(r.Status, r.Conclusion),
			num,
			r.Name,
			r.Branch,
			sha,
			r.Event,
			r.Actor,
			relativeTime(r.CreatedAt),
			formatDuration(r.Duration),
		})
	}
	m.table.SetRows(rows)
	if resetCursor && len(rows) > 0 {
		m.table.GotoTop()
	}
	m.resizeColumns()
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

// colSpec defines a column with its title, max width, and optional min width.
// Columns with min > 0 are flexible and will shrink when the table is too narrow.
type colSpec struct {
	title string
	max   int
	min   int // 0 = fixed (won't shrink)
}

var runsColumns = []colSpec{
	{title: " ", max: 2},
	{title: "#", max: 4},
	{title: "Action", max: 20, min: 8},
	{title: "Branch", max: 30, min: 16},
	{title: "SHA", max: 7},
	{title: "Event", max: 10},
	{title: "Actor", max: 20, min: 8},
	{title: "Age", max: 6},
	{title: "Duration", max: 8, min: 3},
}

func (m *RunsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	tableW := width - 4
	if tableW < 10 {
		tableW = 10
	}
	tableH := height - 5 // border + title + header
	if tableH < 1 {
		tableH = 1
	}
	m.table.SetWidth(tableW)
	m.table.SetHeight(tableH)
	m.resizeColumns()
}

func (m *RunsModel) resizeColumns() {
	if m.width == 0 {
		return
	}

	tableW := m.width - 4 // border + padding
	if tableW < 10 {
		tableW = 10
	}

	// Measure max content width per column from actual data
	rows := m.table.Rows()
	colWidths := make([]int, len(runsColumns))
	for i, col := range runsColumns {
		colWidths[i] = len(col.title)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}
	// Cap each column at its max
	for i, col := range runsColumns {
		if colWidths[i] > col.max {
			colWidths[i] = col.max
		}
	}

	// If total exceeds available width, shrink flexible columns
	total := 0
	for _, w := range colWidths {
		total += w
	}
	// Each cell has Padding(0,1) = 2 chars overhead per column
	cellPadding := len(colWidths) * 2
	totalRendered := total + cellPadding

	if totalRendered > tableW {
		excess := totalRendered - tableW
		// Shrink flexible columns (those with min > 0), largest first
		for excess > 0 {
			shrunk := false
			for i, col := range runsColumns {
				if col.min > 0 && colWidths[i] > col.min && excess > 0 {
					colWidths[i]--
					excess--
					shrunk = true
				}
			}
			if !shrunk {
				break // all flexible columns at their minimum
			}
		}
	}

	cols := make([]table.Column, len(runsColumns))
	for i, col := range runsColumns {
		title := col.title
		if len(title) > colWidths[i] {
			title = title[:colWidths[i]]
		}
		cols[i] = table.Column{Title: title, Width: colWidths[i]}
	}
	m.table.SetColumns(cols)
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
	if innerH < 1 {
		innerH = 1
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	content = strings.Join(lines[:innerH], "\n")

	return style.Width(m.width - 2).Height(m.height - 2).Render(content)
}
