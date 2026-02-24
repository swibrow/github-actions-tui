package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

type ViewState int

const (
	ViewWorkflowRuns ViewState = iota
	ViewJobs
	ViewLogs
)

type FocusPane int

const (
	FocusSidebar FocusPane = iota
	FocusMain
)

// Messages
type WorkflowsMsg struct {
	Workflows []gh.Workflow
	Err       error
}

type RunsMsg struct {
	Runs []gh.WorkflowRun
	Err  error
}

type JobsMsg struct {
	Jobs []gh.WorkflowJob
	Err  error
}

type LogsMsg struct {
	Content string
	Err     error
}

type TickMsg time.Time

type GGTimeoutMsg struct{}

type Model struct {
	client *gh.Client
	ctx    context.Context

	sidebar SidebarModel
	runs    RunsModel
	jobs    JobsModel
	logs    LogsModel
	filter  FilterModel

	view       ViewState
	focus      FocusPane
	showHelp   bool
	pendingG   bool
	err        error
	width      int
	height     int
	hasActive  bool
	workflows  []gh.Workflow
	currentRun *gh.WorkflowRun
	currentJob *gh.WorkflowJob
}

func NewModel(client *gh.Client) Model {
	return Model{
		client:  client,
		ctx:     context.Background(),
		sidebar: NewSidebarModel(),
		runs:    NewRunsModel(),
		jobs:    NewJobsModel(),
		logs:    NewLogsModel(),
		filter:  NewFilterModel(),
		view:    ViewWorkflowRuns,
		focus:   FocusMain,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchWorkflows(),
		m.fetchRuns(gh.RunFilter{}),
	)
}

func (m Model) fetchWorkflows() tea.Cmd {
	return func() tea.Msg {
		workflows, err := m.client.FetchWorkflows(m.ctx)
		return WorkflowsMsg{Workflows: workflows, Err: err}
	}
}

func (m Model) fetchRuns(filter gh.RunFilter) tea.Cmd {
	return func() tea.Msg {
		runs, err := m.client.FetchRuns(m.ctx, filter)
		return RunsMsg{Runs: runs, Err: err}
	}
}

func (m Model) fetchJobs(runID int64) tea.Cmd {
	return func() tea.Msg {
		jobs, err := m.client.FetchJobs(m.ctx, runID)
		return JobsMsg{Jobs: jobs, Err: err}
	}
}

func (m Model) fetchJobLogs(jobID int64) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.FetchJobLogs(m.ctx, jobID)
		return LogsMsg{Content: content, Err: err}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m Model) ggTimeoutCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(_ time.Time) tea.Msg {
		return GGTimeoutMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case WorkflowsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.workflows = msg.Workflows
		m.sidebar.SetWorkflows(msg.Workflows)
		return m, nil

	case RunsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.runs.SetRuns(msg.Runs)
		m.hasActive = gh.HasActiveRuns(msg.Runs)
		var cmd tea.Cmd
		if m.hasActive {
			cmd = m.tickCmd()
		}
		return m, cmd

	case JobsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		runName := ""
		if m.currentRun != nil {
			runName = fmt.Sprintf("#%d %s", m.currentRun.Number, m.currentRun.Branch)
		}
		m.jobs.SetJobs(msg.Jobs, runName)
		return m, nil

	case LogsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		jobName := ""
		if m.currentJob != nil {
			jobName = m.currentJob.Name
		}
		m.logs.SetContent(msg.Content, jobName)
		return m, nil

	case TickMsg:
		if m.view == ViewWorkflowRuns && m.hasActive {
			wfID, _ := m.sidebar.SelectedWorkflow()
			filter := m.filter.CurrentFilter(wfID)
			return m, tea.Batch(m.fetchRuns(filter), m.tickCmd())
		}
		return m, nil

	case GGTimeoutMsg:
		m.pendingG = false
		return m, nil

	case FilterAppliedMsg:
		wfID, _ := m.sidebar.SelectedWorkflow()
		msg.Filter.WorkflowID = wfID
		m.runs.SetLoading(true)
		return m, m.fetchRuns(msg.Filter)

	case FilterCancelledMsg:
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		// Filter captures all keys when visible
		if m.filter.Visible() {
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			return m, cmd
		}

		return m.handleKey(msg)
	}

	// Pass through to focused component
	return m.updateFocused(msg)
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Pass mouse events to the focused component for scroll support
	switch m.view {
	case ViewLogs:
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		return m, cmd
	case ViewJobs:
		var cmd tea.Cmd
		m.jobs, cmd = m.jobs.Update(msg)
		return m, cmd
	case ViewWorkflowRuns:
		// Determine which pane the mouse is in based on x position
		sidebarW := clamp(m.width/4, 20, 35)
		if msg.X < sidebarW {
			if m.focus != FocusSidebar {
				m.focus = FocusSidebar
				m.sidebar.SetFocused(true)
				m.runs.SetFocused(false)
			}
			var cmd tea.Cmd
			m.sidebar, cmd = m.sidebar.Update(msg)
			return m, cmd
		}
		if m.focus != FocusMain {
			m.focus = FocusMain
			m.sidebar.SetFocused(false)
			m.runs.SetFocused(true)
		}
		var cmd tea.Cmd
		m.runs, cmd = m.runs.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch {
	case key.Matches(msg, Keys.Quit):
		if m.view == ViewWorkflowRuns {
			return m, tea.Quit
		}
		// In other views, treat q as back
		return m.goBack()

	case key.Matches(msg, Keys.Help):
		m.showHelp = !m.showHelp
		return m, nil

	case key.Matches(msg, Keys.Back):
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m.goBack()
	}

	// View-specific keys
	switch m.view {
	case ViewLogs:
		return m.updateFocused(msg)

	case ViewWorkflowRuns:
		switch {
		case key.Matches(msg, Keys.Filter):
			m.filter.Show()
			return m, nil

		case key.Matches(msg, Keys.Refresh):
			wfID, _ := m.sidebar.SelectedWorkflow()
			filter := m.filter.CurrentFilter(wfID)
			m.runs.SetLoading(true)
			return m, m.fetchRuns(filter)

		case key.Matches(msg, Keys.Left):
			m.focus = FocusSidebar
			m.sidebar.SetFocused(true)
			m.runs.SetFocused(false)
			return m, nil

		case key.Matches(msg, Keys.Right):
			m.focus = FocusMain
			m.sidebar.SetFocused(false)
			m.runs.SetFocused(true)
			return m, nil

		case key.Matches(msg, Keys.Up), key.Matches(msg, Keys.Down):
			// Route up/down (arrows + j/k) to the focused component
			return m.updateFocused(msg)

		case key.Matches(msg, Keys.Enter):
			return m.handleEnter()

		case key.Matches(msg, Keys.Bottom):
			return m.updateFocused(msg)

		case key.Matches(msg, Keys.Top):
			if m.pendingG {
				// gg: go to top
				m.pendingG = false
				topMsg := tea.KeyMsg{Type: tea.KeyHome}
				return m.updateFocused(topMsg)
			}
			m.pendingG = true
			return m, m.ggTimeoutCmd()
		}

		return m.updateFocused(msg)

	case ViewJobs:
		switch {
		case key.Matches(msg, Keys.Enter):
			return m.handleEnter()

		case key.Matches(msg, Keys.Refresh):
			if m.currentRun != nil {
				m.jobs.SetLoading(true)
				return m, m.fetchJobs(m.currentRun.ID)
			}
			return m, nil

		case key.Matches(msg, Keys.Up), key.Matches(msg, Keys.Down):
			return m.updateFocused(msg)

		case key.Matches(msg, Keys.Bottom):
			return m.updateFocused(msg)

		case key.Matches(msg, Keys.Top):
			if m.pendingG {
				m.pendingG = false
				topMsg := tea.KeyMsg{Type: tea.KeyHome}
				return m.updateFocused(topMsg)
			}
			m.pendingG = true
			return m, m.ggTimeoutCmd()
		}
		return m.updateFocused(msg)
	}

	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewWorkflowRuns:
		if m.focus == FocusSidebar {
			// Sidebar: select workflow and refresh runs
			wfID, wfName := m.sidebar.SelectedWorkflow()
			if wfName == "All Workflows" {
				m.runs.SetTitle("Workflow Runs")
			} else {
				m.runs.SetTitle(wfName)
			}
			filter := m.filter.CurrentFilter(wfID)
			m.runs.SetLoading(true)
			return m, m.fetchRuns(filter)
		}
		// Main: drill into jobs
		run := m.runs.SelectedRun()
		if run == nil {
			return m, nil
		}
		m.currentRun = run
		m.view = ViewJobs
		m.jobs.SetLoading(true)
		m.jobs.SetFocused(true)
		return m, m.fetchJobs(run.ID)

	case ViewJobs:
		job := m.jobs.SelectedJob()
		if job == nil {
			return m, nil
		}
		m.currentJob = job
		m.view = ViewLogs
		m.logs.SetLoading(true)
		m.updateLayout()
		return m, m.fetchJobLogs(job.ID)
	}
	return m, nil
}

func (m Model) goBack() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewLogs:
		m.view = ViewJobs
		m.jobs.SetFocused(true)
		m.updateLayout()
		return m, nil
	case ViewJobs:
		m.view = ViewWorkflowRuns
		m.focus = FocusMain
		m.sidebar.SetFocused(false)
		m.runs.SetFocused(true)
		m.updateLayout()
		// Refresh runs when going back
		wfID, _ := m.sidebar.SelectedWorkflow()
		filter := m.filter.CurrentFilter(wfID)
		return m, m.fetchRuns(filter)
	case ViewWorkflowRuns:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) updateFocused(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.view {
	case ViewWorkflowRuns:
		if m.focus == FocusSidebar {
			m.sidebar, cmd = m.sidebar.Update(msg)
		} else {
			m.runs, cmd = m.runs.Update(msg)
		}
	case ViewJobs:
		m.jobs, cmd = m.jobs.Update(msg)
	case ViewLogs:
		m.logs, cmd = m.logs.Update(msg)
	}
	return m, cmd
}

func (m *Model) updateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	filterH := 0
	if m.filter.Visible() || m.filter.HasActiveFilter() {
		filterH = 3
	}
	helpH := 1

	switch m.view {
	case ViewWorkflowRuns:
		sidebarW := clamp(m.width/4, 20, 35)
		mainW := m.width - sidebarW
		contentH := m.height - filterH - helpH

		m.sidebar.SetSize(sidebarW, contentH)
		m.runs.SetSize(mainW, contentH)
		m.filter.SetSize(m.width)

	case ViewJobs:
		contentH := m.height - helpH
		m.jobs.SetSize(m.width, contentH)

	case ViewLogs:
		m.logs.SetSize(m.width, m.height)
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Error bar
	errBar := ""
	if m.err != nil {
		errBar = styleError.Width(m.width).Render(fmt.Sprintf("Error: %s", m.err))
		m.err = nil
	}

	// Help overlay
	if m.showHelp {
		return m.helpView()
	}

	switch m.view {
	case ViewLogs:
		return m.logs.View()
	case ViewJobs:
		content := m.jobs.View()
		help := m.helpBarView()
		if errBar != "" {
			return lipgloss.JoinVertical(lipgloss.Left, errBar, content, help)
		}
		return lipgloss.JoinVertical(lipgloss.Left, content, help)
	default:
		sidebar := m.sidebar.View()
		main := m.runs.View()
		content := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)

		filterView := m.filter.View()
		help := m.helpBarView()

		parts := []string{}
		if errBar != "" {
			parts = append(parts, errBar)
		}
		parts = append(parts, content)
		if filterView != "" {
			parts = append(parts, filterView)
		}
		parts = append(parts, help)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
}

func (m Model) helpBarView() string {
	return styleHelpBar.Render("↑↓/jk:move  ←→/hl:panes  enter:select  esc:back  /:filter  r:refresh  ?:help  q:quit")
}

func (m Model) helpView() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Width(50)

	help := `GitHub Actions TUI

Navigation:
  ↑/↓, j/k     Move cursor up/down
  ←/→, h/l     Switch panes
  enter         Select / drill in
  esc           Go back
  gg            Go to top
  G             Go to bottom

Mouse:
  click         Focus pane
  scroll        Scroll content

Actions:
  /             Open filter bar
  r             Refresh data
  ?             Toggle help
  q             Quit

Filter (when open):
  tab           Next field
  shift+tab     Previous field
  enter         Apply filter
  esc           Cancel

Press ? or esc to close`

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		style.Render(help))
}
