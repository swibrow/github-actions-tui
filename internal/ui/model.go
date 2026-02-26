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

type WorkflowYAMLMsg struct {
	Path string
	Deps map[string][]string
	Err  error
}

// JobStatusMsg carries updated job info when polling for an in-progress job in log view.
type JobStatusMsg struct {
	Job *gh.WorkflowJob
	Err error
}

type TickMsg time.Time

type GGTimeoutMsg struct{}

type Model struct {
	client gh.GitHubClient
	ctx    context.Context

	tree   TreeModel
	runs   RunsModel
	graph  GraphModel
	logs   LogsModel
	filter FilterModel

	view           ViewState
	focus          FocusPane
	showHelp       bool
	pendingG       bool
	err            error
	width          int
	height         int
	workflows      []gh.Workflow
	currentRun     *gh.WorkflowRun
	currentJob     *gh.WorkflowJob
	sidebarVisible bool
	confirmQuit    bool
	yamlCache      map[string]map[string][]string // path -> job deps
}

func NewModel(client gh.GitHubClient) Model {
	runs := NewRunsModel()
	runs.SetFocused(true)

	return Model{
		client:         client,
		ctx:            context.Background(),
		tree:           NewTreeModel(),
		runs:           runs,
		graph:          NewGraphModel(),
		logs:           NewLogsModel(),
		filter:         NewFilterModel(),
		view:           ViewWorkflowRuns,
		focus:          FocusMain,
		sidebarVisible: true,
		yamlCache:      make(map[string]map[string][]string),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchWorkflows(),
		m.fetchRuns(gh.RunFilter{}),
		m.tickCmd(),
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

func (m Model) fetchRunsForTree(workflowID int64) tea.Cmd {
	return func() tea.Msg {
		runs, err := m.client.FetchRunsForWorkflow(m.ctx, workflowID, 8)
		return RunsForTreeMsg{WorkflowID: workflowID, Runs: runs, Err: err}
	}
}

func (m Model) fetchWorkflowYAML(path string) tea.Cmd {
	return func() tea.Msg {
		deps, err := m.client.FetchWorkflowYAML(m.ctx, path)
		return WorkflowYAMLMsg{Path: path, Deps: deps, Err: err}
	}
}

func (m Model) fetchJobLogs(jobID int64) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.FetchJobLogs(m.ctx, jobID)
		return LogsMsg{Content: content, Err: err}
	}
}

// fetchJobStatus fetches the current job from the run's jobs list to get live step status.
func (m Model) fetchJobStatus(runID, jobID int64) tea.Cmd {
	return func() tea.Msg {
		jobs, err := m.client.FetchJobs(m.ctx, runID)
		if err != nil {
			return JobStatusMsg{Err: err}
		}
		for i := range jobs {
			if jobs[i].ID == jobID {
				return JobStatusMsg{Job: &jobs[i]}
			}
		}
		return JobStatusMsg{Err: fmt.Errorf("job not found")}
	}
}

func (m Model) tickCmd() tea.Cmd {
	interval := 10 * time.Second
	// Poll faster when viewing an active job's steps
	if m.view == ViewLogs && m.logs.IsJobInProgress() {
		interval = 3 * time.Second
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg {
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
		m.tree.SetWorkflows(msg.Workflows)
		return m, nil

	case RunsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.runs.SetRuns(msg.Runs)
		return m, nil

	case JobsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		runName := ""
		if m.currentRun != nil {
			runName = fmt.Sprintf("#%d %s", m.currentRun.Number, m.currentRun.Branch)
		}

		// Try to find workflow path for YAML-based deps
		var deps map[string][]string
		var yamlCmd tea.Cmd
		if m.currentRun != nil {
			wfPath := m.workflowPath(m.currentRun.WorkflowID)
			if wfPath != "" {
				if cached, ok := m.yamlCache[wfPath]; ok {
					deps = cached
				} else {
					yamlCmd = m.fetchWorkflowYAML(wfPath)
				}
			}
		}
		m.graph.SetJobs(msg.Jobs, deps, runName)
		return m, yamlCmd

	case WorkflowYAMLMsg:
		if msg.Err != nil {
			// Silently ignore YAML fetch errors; graph already has inferred tiers
			return m, nil
		}
		m.yamlCache[msg.Path] = msg.Deps
		// Re-render graph with proper deps
		if m.view == ViewJobs && len(m.graph.jobs) > 0 {
			runName := ""
			if m.currentRun != nil {
				runName = fmt.Sprintf("#%d %s", m.currentRun.Number, m.currentRun.Branch)
			}
			m.graph.SetJobs(m.graph.jobs, msg.Deps, runName)
		}
		return m, nil

	case LogsMsg:
		if msg.Err != nil {
			// If job is in-progress, log fetch 404 is expected — don't show error
			if m.logs.IsJobInProgress() {
				return m, nil
			}
			m.err = msg.Err
			return m, nil
		}
		jobName := ""
		if m.currentJob != nil {
			jobName = m.currentJob.Name
		}
		m.logs.SetContent(msg.Content, jobName)
		return m, nil

	case JobStatusMsg:
		if msg.Err != nil {
			return m, nil
		}
		if m.view != ViewLogs || m.currentJob == nil {
			return m, nil
		}
		// Update stored job
		m.currentJob = msg.Job
		jobName := msg.Job.Name

		if msg.Job.Status == "completed" {
			// Job just finished — fetch real logs
			m.logs.SetLoading(true)
			return m, m.fetchJobLogs(msg.Job.ID)
		}
		// Still in progress — update step display
		m.logs.SetSteps(msg.Job.Steps, jobName, msg.Job.Status)
		return m, nil

	case TickMsg:
		cmds := []tea.Cmd{m.tickCmd()}
		switch m.view {
		case ViewWorkflowRuns:
			wfID, _ := m.tree.SelectedWorkflow()
			filter := m.filter.CurrentFilter(wfID)
			cmds = append(cmds, m.fetchRuns(filter))
		case ViewJobs:
			if m.currentRun != nil {
				cmds = append(cmds, m.fetchJobs(m.currentRun.ID))
			}
		case ViewLogs:
			if m.currentJob != nil {
				if m.logs.IsJobInProgress() && m.currentRun != nil {
					// Job still running — poll for step status updates
					cmds = append(cmds, m.fetchJobStatus(m.currentRun.ID, m.currentJob.ID))
				} else {
					// Job completed — refresh logs
					cmds = append(cmds, m.fetchJobLogs(m.currentJob.ID))
				}
			}
		}
		return m, tea.Batch(cmds...)

	case RunsForTreeMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.tree.SetRunsForWorkflow(msg.WorkflowID, msg.Runs)
		return m, nil

	case GGTimeoutMsg:
		m.pendingG = false
		return m, nil

	case FilterAppliedMsg:
		wfID, _ := m.tree.SelectedWorkflow()
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

		// Log search captures all keys when active
		if m.logs.Searching() {
			var cmd tea.Cmd
			m.logs, cmd = m.logs.Update(msg)
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
		if m.sidebarVisible {
			sidebarW := clamp(m.width/4, 20, 35)
			if msg.X < sidebarW {
				if m.focus != FocusSidebar {
					m.focus = FocusSidebar
					m.tree.SetFocused(true)
					m.logs.SetFocused(false)
				}
				var cmd tea.Cmd
				m.tree, cmd = m.tree.Update(msg)
				return m, cmd
			}
		}
		if m.focus != FocusMain {
			m.focus = FocusMain
			m.tree.SetFocused(false)
			m.logs.SetFocused(true)
		}
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		return m, cmd
	case ViewJobs:
		if m.sidebarVisible {
			sidebarW := clamp(m.width/4, 20, 35)
			if msg.X < sidebarW {
				if m.focus != FocusSidebar {
					m.focus = FocusSidebar
					m.tree.SetFocused(true)
					m.graph.SetFocused(false)
				}
				var cmd tea.Cmd
				m.tree, cmd = m.tree.Update(msg)
				return m, cmd
			}
		}
		if m.focus != FocusMain {
			m.focus = FocusMain
			m.tree.SetFocused(false)
			m.graph.SetFocused(true)
		}
		var cmd tea.Cmd
		m.graph, cmd = m.graph.Update(msg)
		return m, cmd
	case ViewWorkflowRuns:
		if m.sidebarVisible {
			// Determine which pane the mouse is in based on x position
			sidebarW := clamp(m.width/4, 20, 35)
			if msg.X < sidebarW {
				if m.focus != FocusSidebar {
					m.focus = FocusSidebar
					m.tree.SetFocused(true)
					m.runs.SetFocused(false)
				}
				var cmd tea.Cmd
				m.tree, cmd = m.tree.Update(msg)
				return m, cmd
			}
		}
		if m.focus != FocusMain {
			m.focus = FocusMain
			m.tree.SetFocused(false)
			m.runs.SetFocused(true)
		}
		var cmd tea.Cmd
		m.runs, cmd = m.runs.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle confirm quit dialog
	if m.confirmQuit {
		switch msg.String() {
		case "y", "enter", "q":
			return m, tea.Quit
		case "n", "esc":
			m.confirmQuit = false
			return m, nil
		}
		return m, nil
	}

	// Global keys
	switch {
	case key.Matches(msg, Keys.Quit):
		if m.view == ViewWorkflowRuns {
			m.confirmQuit = true
			return m, nil
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

	case key.Matches(msg, Keys.SwitchPane):
		if m.sidebarVisible {
			return m.toggleFocus()
		}
		return m, nil
	}

	// Sidebar tree navigation: h/l expand/collapse/drill-in
	if m.focus == FocusSidebar {
		switch {
		case key.Matches(msg, Keys.Right):
			return m.treeExpand()
		case key.Matches(msg, Keys.Left):
			return m.treeCollapse()
		case key.Matches(msg, Keys.Enter):
			return m.handleEnter()
		}
	}

	// View-specific keys
	switch m.view {
	case ViewLogs:
		switch {
		case key.Matches(msg, Keys.ToggleSidebar):
			m.sidebarVisible = !m.sidebarVisible
			if !m.sidebarVisible && m.focus == FocusSidebar {
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.logs.SetFocused(true)
			}
			m.updateLayout()
			return m, nil

		case key.Matches(msg, Keys.Filter):
			// / starts log search when in logs view
			if m.focus == FocusMain {
				m.logs.StartSearch()
				return m, nil
			}

		case key.Matches(msg, Keys.Enter):
			return m, nil
		}

		// Log-specific keys when main pane is focused
		if m.focus == FocusMain {
			switch msg.String() {
			case "t":
				m.logs.ToggleTimestamps()
				return m, nil
			case "n":
				m.logs.NextMatch()
				return m, nil
			case "N":
				m.logs.PrevMatch()
				return m, nil
			}
		}

		return m.updateFocused(msg)

	case ViewWorkflowRuns:
		switch {
		case key.Matches(msg, Keys.ToggleSidebar):
			m.sidebarVisible = !m.sidebarVisible
			if !m.sidebarVisible && m.focus == FocusSidebar {
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.runs.SetFocused(true)
			}
			m.updateLayout()
			return m, nil

		case key.Matches(msg, Keys.Filter):
			m.filter.Show()
			return m, nil

		case key.Matches(msg, Keys.Refresh):
			wfID, _ := m.tree.SelectedWorkflow()
			filter := m.filter.CurrentFilter(wfID)
			m.runs.SetLoading(true)
			return m, m.fetchRuns(filter)

		case key.Matches(msg, Keys.Enter):
			return m.handleEnter()

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

	case ViewJobs:
		switch {
		case key.Matches(msg, Keys.ToggleSidebar):
			m.sidebarVisible = !m.sidebarVisible
			if !m.sidebarVisible && m.focus == FocusSidebar {
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.graph.SetFocused(true)
			}
			m.updateLayout()
			return m, nil

		case key.Matches(msg, Keys.Enter):
			return m.handleEnter()

		case key.Matches(msg, Keys.Refresh):
			if m.currentRun != nil {
				m.graph.SetLoading(true)
				return m, m.fetchJobs(m.currentRun.ID)
			}
			return m, nil

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
			node := m.tree.SelectedNode()
			if node == nil {
				return m, nil
			}
			switch node.Kind {
			case NodeWorkflow:
				// Toggle expand/collapse; fetch runs if expanding and no children
				expanded, n := m.tree.ToggleExpand()
				if expanded && n != nil && n.Workflow != nil && len(n.Children) == 0 {
					m.tree.SetLoading(n.Workflow.ID)
					return m, m.fetchRunsForTree(n.Workflow.ID)
				}
				return m, nil
			case NodeRun:
				// Select run → transition to ViewJobs
				if node.Run == nil {
					return m, nil
				}
				run := node.Run
				m.currentRun = run
				m.view = ViewJobs
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.graph.SetLoading(true)
				m.graph.SetFocused(true)
				m.updateLayout()
				return m, m.fetchJobs(run.ID)
			}
			return m, nil
		}
		// Main: drill into jobs
		run := m.runs.SelectedRun()
		if run == nil {
			return m, nil
		}
		m.currentRun = run
		m.view = ViewJobs
		m.focus = FocusMain
		m.runs.SetFocused(false)
		m.tree.SetFocused(false)
		m.graph.SetLoading(true)
		m.graph.SetFocused(true)
		m.updateLayout()
		return m, m.fetchJobs(run.ID)

	case ViewJobs:
		if m.focus == FocusSidebar {
			node := m.tree.SelectedNode()
			if node == nil {
				return m, nil
			}
			switch node.Kind {
			case NodeWorkflow:
				expanded, n := m.tree.ToggleExpand()
				if expanded && n != nil && n.Workflow != nil && len(n.Children) == 0 {
					m.tree.SetLoading(n.Workflow.ID)
					return m, m.fetchRunsForTree(n.Workflow.ID)
				}
				return m, nil
			case NodeRun:
				// Switch to a different run's jobs
				if node.Run == nil {
					return m, nil
				}
				m.currentRun = node.Run
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.graph.SetLoading(true)
				m.graph.SetFocused(true)
				m.updateLayout()
				return m, m.fetchJobs(node.Run.ID)
			}
			return m, nil
		}
		job := m.graph.SelectedJob()
		if job == nil {
			return m, nil
		}
		m.currentJob = job
		m.view = ViewLogs
		m.focus = FocusMain
		m.graph.SetFocused(false)
		m.tree.SetFocused(false)
		m.logs.SetFocused(true)
		m.updateLayout()

		if job.Status != "completed" {
			// Job is in-progress — show step status, poll for updates
			m.logs.SetSteps(job.Steps, job.Name, job.Status)
			return m, nil
		}
		m.logs.SetLoading(true)
		return m, m.fetchJobLogs(job.ID)

	case ViewLogs:
		if m.focus == FocusSidebar {
			node := m.tree.SelectedNode()
			if node == nil {
				return m, nil
			}
			switch node.Kind {
			case NodeWorkflow:
				expanded, n := m.tree.ToggleExpand()
				if expanded && n != nil && n.Workflow != nil && len(n.Children) == 0 {
					m.tree.SetLoading(n.Workflow.ID)
					return m, m.fetchRunsForTree(n.Workflow.ID)
				}
				return m, nil
			case NodeRun:
				if node.Run == nil {
					return m, nil
				}
				// Switch to that run's jobs view
				m.currentRun = node.Run
				m.view = ViewJobs
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.graph.SetLoading(true)
				m.graph.SetFocused(true)
				m.updateLayout()
				return m, m.fetchJobs(node.Run.ID)
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) toggleFocus() (tea.Model, tea.Cmd) {
	if m.focus == FocusSidebar {
		m.focus = FocusMain
		m.tree.SetFocused(false)
		switch m.view {
		case ViewWorkflowRuns:
			m.runs.SetFocused(true)
		case ViewJobs:
			m.graph.SetFocused(true)
		case ViewLogs:
			m.logs.SetFocused(true)
		}
	} else {
		m.focus = FocusSidebar
		m.tree.SetFocused(true)
		switch m.view {
		case ViewWorkflowRuns:
			m.runs.SetFocused(false)
		case ViewJobs:
			m.graph.SetFocused(false)
		case ViewLogs:
			m.logs.SetFocused(false)
		}
	}
	return m, nil
}

func (m Model) goBack() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewLogs:
		m.view = ViewJobs
		m.focus = FocusMain
		m.logs.SetFocused(false)
		m.tree.SetFocused(false)
		m.graph.SetFocused(true)
		m.updateLayout()
		return m, nil
	case ViewJobs:
		m.view = ViewWorkflowRuns
		m.focus = FocusMain
		m.graph.SetFocused(false)
		m.tree.SetFocused(false)
		m.runs.SetFocused(true)
		m.updateLayout()
		// Refresh runs when going back
		wfID, _ := m.tree.SelectedWorkflow()
		filter := m.filter.CurrentFilter(wfID)
		return m, m.fetchRuns(filter)
	case ViewWorkflowRuns:
		m.confirmQuit = true
		return m, nil
	}
	return m, nil
}

// treeExpand handles right/l in the sidebar: expand workflow, or drill into run's jobs.
func (m Model) treeExpand() (tea.Model, tea.Cmd) {
	node := m.tree.SelectedNode()
	if node == nil {
		return m, nil
	}
	switch node.Kind {
	case NodeWorkflow:
		if !node.Expanded {
			expanded, n := m.tree.ToggleExpand()
			if expanded && n != nil && n.Workflow != nil && len(n.Children) == 0 {
				m.tree.SetLoading(n.Workflow.ID)
				return m, m.fetchRunsForTree(n.Workflow.ID)
			}
		}
		return m, nil
	case NodeRun:
		if node.Run == nil {
			return m, nil
		}
		// Drill into jobs for this run
		m.currentRun = node.Run
		m.view = ViewJobs
		m.focus = FocusMain
		m.runs.SetFocused(false)
		m.tree.SetFocused(false)
		m.graph.SetLoading(true)
		m.graph.SetFocused(true)
		m.updateLayout()
		return m, m.fetchJobs(node.Run.ID)
	}
	return m, nil
}

// treeCollapse handles left/h in the sidebar: collapse node, or go back a view.
func (m Model) treeCollapse() (tea.Model, tea.Cmd) {
	node := m.tree.SelectedNode()
	if node == nil {
		return m, nil
	}
	switch node.Kind {
	case NodeWorkflow:
		if node.Expanded {
			m.tree.ToggleExpand()
		}
		return m, nil
	case NodeRun:
		// Collapse parent workflow
		m.tree.CollapseParent()
		return m, nil
	}
	return m, nil
}

func (m Model) updateFocused(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.view {
	case ViewWorkflowRuns:
		if m.focus == FocusSidebar {
			m.tree, cmd = m.tree.Update(msg)
		} else {
			m.runs, cmd = m.runs.Update(msg)
		}
	case ViewJobs:
		if m.focus == FocusSidebar {
			m.tree, cmd = m.tree.Update(msg)
		} else {
			m.graph, cmd = m.graph.Update(msg)
		}
	case ViewLogs:
		if m.focus == FocusSidebar {
			m.tree, cmd = m.tree.Update(msg)
		} else {
			m.logs, cmd = m.logs.Update(msg)
		}
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
		contentH := clamp(m.height-filterH-helpH, 4, m.height)
		if m.sidebarVisible {
			sidebarW := clamp(m.width/4, 20, 35)
			mainW := clamp(m.width-sidebarW, 10, m.width)
			m.tree.SetSize(sidebarW, contentH)
			m.runs.SetSize(mainW, contentH)
		} else {
			m.runs.SetSize(m.width, contentH)
		}
		m.filter.SetSize(m.width)

	case ViewJobs:
		contentH := clamp(m.height-helpH, 4, m.height)
		if m.sidebarVisible {
			sidebarW := clamp(m.width/4, 20, 35)
			mainW := clamp(m.width-sidebarW, 10, m.width)
			m.tree.SetSize(sidebarW, contentH)
			m.graph.SetSize(mainW, contentH)
		} else {
			m.graph.SetSize(m.width, contentH)
		}

	case ViewLogs:
		contentH := clamp(m.height-helpH, 4, m.height)
		if m.sidebarVisible {
			sidebarW := clamp(m.width/4, 20, 35)
			mainW := clamp(m.width-sidebarW, 10, m.width)
			m.tree.SetSize(sidebarW, contentH)
			m.logs.SetSize(mainW, contentH)
		} else {
			m.logs.SetSize(m.width, contentH)
		}
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
	}

	// Confirm quit dialog
	if m.confirmQuit {
		return m.confirmQuitView()
	}

	// Help overlay
	if m.showHelp {
		return m.helpView()
	}

	var output string

	switch m.view {
	case ViewLogs:
		var content string
		if m.sidebarVisible {
			sidebar := m.tree.View()
			main := m.logs.View()
			content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
		} else {
			content = m.logs.View()
		}
		help := m.helpBarView()
		if errBar != "" {
			output = lipgloss.JoinVertical(lipgloss.Left, errBar, content, help)
		} else {
			output = lipgloss.JoinVertical(lipgloss.Left, content, help)
		}
	case ViewJobs:
		var content string
		if m.sidebarVisible {
			sidebar := m.tree.View()
			main := m.graph.View()
			content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
		} else {
			content = m.graph.View()
		}
		help := m.helpBarView()
		if errBar != "" {
			output = lipgloss.JoinVertical(lipgloss.Left, errBar, content, help)
		} else {
			output = lipgloss.JoinVertical(lipgloss.Left, content, help)
		}
	default:
		var content string
		if m.sidebarVisible {
			sidebar := m.tree.View()
			main := m.runs.View()
			content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
		} else {
			content = m.runs.View()
		}

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
		output = lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	// Hard-truncate output to terminal dimensions to prevent scrolling
	return truncateToFit(output, m.width, m.height)
}

func (m Model) helpBarView() string {
	return styleHelpBar.Render("↑↓/jk:move  ←→/hl:expand  tab:pane  enter:select  esc:back  /:filter  r:refresh  b:sidebar  ?:help  q:quit")
}

func (m Model) workflowPath(workflowID int64) string {
	for _, w := range m.workflows {
		if w.ID == workflowID {
			return w.Path
		}
	}
	return ""
}

func (m Model) confirmQuitView() string {
	dialog := styleConfirmDialog.Render("Quit? (y/n)")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
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
  tab           Switch panes
  enter         Select / expand / drill in
  esc           Go back
  gg            Go to top
  G             Go to bottom

Tree Sidebar:
  →/l           Expand workflow / view jobs
  ←/h           Collapse / go to parent
  enter         Expand/collapse or drill in

Mouse:
  click         Focus pane
  scroll        Scroll content

Actions:
  /             Open filter bar / search logs
  r             Refresh data
  b             Toggle sidebar
  ?             Toggle help
  q             Quit

Logs:
  /             Search logs
  n / N         Next / previous match
  t             Toggle timestamps

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
