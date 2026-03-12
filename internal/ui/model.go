package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

type ViewState int

const (
	ViewWorkflowRuns ViewState = iota
	ViewJobs
	ViewLogs
	ViewWorkflowFile
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
	Runs       []gh.WorkflowRun
	Err        error
	ResetCursor bool
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

// RerunMsg carries the result of a rerun request.
type RerunMsg struct {
	RunID      int64
	FailedOnly bool
	Err        error
}

// TriggerResultMsg carries the result of a workflow trigger request.
type TriggerResultMsg struct {
	Err error
}

// FileContentMsg carries fetched workflow file content.
type FileContentMsg struct {
	Content string
	Name    string
	Err     error
}

// StatusClearMsg clears the temporary status message.
type StatusClearMsg struct{}

type GGTimeoutMsg struct{}
type BackLockMsg struct{}

type RepoListMsg struct {
	Repos []gh.Repository
	Err   error
}

type UserOrgsMsg struct {
	Orgs []string
	Err  error
}

type OrgReposMsg struct {
	Repos []gh.Repository
	Err   error
}

type RepoSearchMsg struct {
	Repos []gh.Repository
	Err   error
}

type Model struct {
	client gh.GitHubClient
	ctx    context.Context

	tree       TreeModel
	runs       RunsModel
	graph      GraphModel
	logs       LogsModel
	filter     FilterModel
	repoPicker RepoPickerModel
	trigger    TriggerModel

	view           ViewState
	prevView       ViewState
	focus          FocusPane
	showHelp       bool
	pendingG       bool
	err            error
	statusMsg      string
	width          int
	height         int
	workflows      []gh.Workflow
	currentRun     *gh.WorkflowRun
	currentJob     *gh.WorkflowJob
	currentAttempt int
	sidebarVisible bool
	confirmQuit    bool
	rerunChoice    bool  // true when showing rerun choice dialog
	rerunRunID     int64 // run ID for pending rerun choice
	backLocked     bool
	yamlCache      map[string]map[string][]string // path -> job deps
	repoOwner      string
	repoName       string
}

func NewModel(client gh.GitHubClient, owner, repo string) Model {
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
		repoPicker:     NewRepoPickerModel(),
		trigger:        NewTriggerModel(),
		view:           ViewWorkflowRuns,
		focus:          FocusMain,
		sidebarVisible: true,
		yamlCache:      make(map[string]map[string][]string),
		repoOwner:      owner,
		repoName:       repo,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchWorkflows(),
		m.fetchRuns(gh.RunFilter{}, true),
		m.tickCmd(),
	)
}

func (m Model) fetchWorkflows() tea.Cmd {
	return func() tea.Msg {
		workflows, err := m.client.FetchWorkflows(m.ctx)
		return WorkflowsMsg{Workflows: workflows, Err: err}
	}
}

func (m Model) fetchRuns(filter gh.RunFilter, resetCursor bool) tea.Cmd {
	return func() tea.Msg {
		runs, err := m.client.FetchRuns(m.ctx, filter)
		return RunsMsg{Runs: runs, Err: err, ResetCursor: resetCursor}
	}
}

func (m Model) fetchJobsForAttempt(runID int64, attempt int) tea.Cmd {
	return func() tea.Msg {
		jobs, err := m.client.FetchJobsForAttempt(m.ctx, runID, attempt)
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

func (m Model) fetchUserRepos() tea.Cmd {
	return func() tea.Msg {
		repos, err := m.client.ListUserRepos(m.ctx)
		return RepoListMsg{Repos: repos, Err: err}
	}
}

func (m Model) fetchUserOrgs() tea.Cmd {
	return func() tea.Msg {
		orgs, err := m.client.ListUserOrgs(m.ctx)
		return UserOrgsMsg{Orgs: orgs, Err: err}
	}
}

func (m Model) fetchOrgRepos(org string) tea.Cmd {
	return func() tea.Msg {
		repos, err := m.client.ListOrgRepos(m.ctx, org)
		return OrgReposMsg{Repos: repos, Err: err}
	}
}

func (m Model) searchRepos(query string) tea.Cmd {
	return func() tea.Msg {
		repos, err := m.client.SearchRepos(m.ctx, query)
		return RepoSearchMsg{Repos: repos, Err: err}
	}
}

func (m Model) rerunWorkflow(runID int64) tea.Cmd {
	return func() tea.Msg {
		err := m.client.RerunWorkflow(m.ctx, runID)
		return RerunMsg{RunID: runID, Err: err}
	}
}

func (m Model) rerunFailedJobs(runID int64) tea.Cmd {
	return func() tea.Msg {
		err := m.client.RerunFailedJobs(m.ctx, runID)
		return RerunMsg{RunID: runID, FailedOnly: true, Err: err}
	}
}

func (m Model) triggerWorkflow(workflowID int64, ref string, inputs map[string]interface{}) tea.Cmd {
	return func() tea.Msg {
		err := m.client.TriggerWorkflow(m.ctx, workflowID, ref, inputs)
		return TriggerResultMsg{Err: err}
	}
}

func (m Model) fetchWorkflowInputs(workflowID int64, path string) tea.Cmd {
	return func() tea.Msg {
		inputs, err := m.client.FetchWorkflowInputs(m.ctx, path)
		return WorkflowInputsMsg{WorkflowID: workflowID, Inputs: inputs, Err: err}
	}
}

func (m Model) fetchWorkflowFileContent(path, name string) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.FetchWorkflowFileContent(m.ctx, path)
		return FileContentMsg{Content: content, Name: name, Err: err}
	}
}

func (m Model) openActionsInBrowser() tea.Cmd {
	url := fmt.Sprintf("https://github.com/%s/%s/actions", m.repoOwner, m.repoName)
	return m.openURL(url)
}

// openSelectedInBrowser opens a context-sensitive URL based on the current view and selection.
func (m Model) openSelectedInBrowser() tea.Cmd {
	base := fmt.Sprintf("https://github.com/%s/%s", m.repoOwner, m.repoName)

	// If sidebar is focused, use the tree selection
	if m.focus == FocusSidebar {
		node := m.tree.SelectedNode()
		if node == nil {
			return m.openActionsInBrowser()
		}
		switch node.Kind {
		case NodeWorkflow:
			if node.Workflow != nil && node.Workflow.Path != "" {
				return m.openURL(base + "/actions/workflows/" + workflowFileName(node.Workflow.Path))
			}
			return m.openActionsInBrowser()
		case NodeRun:
			if node.Run != nil {
				return m.openURL(fmt.Sprintf("%s/actions/runs/%d", base, node.Run.ID))
			}
		}
		return m.openActionsInBrowser()
	}

	// Main pane — context depends on view
	switch m.view {
	case ViewWorkflowRuns:
		run := m.runs.SelectedRun()
		if run != nil {
			return m.openURL(fmt.Sprintf("%s/actions/runs/%d", base, run.ID))
		}
	case ViewJobs:
		job := m.graph.SelectedJob()
		if job != nil && m.currentRun != nil {
			return m.openURL(fmt.Sprintf("%s/actions/runs/%d/job/%d", base, m.currentRun.ID, job.ID))
		}
		if m.currentRun != nil {
			return m.openURL(fmt.Sprintf("%s/actions/runs/%d", base, m.currentRun.ID))
		}
	case ViewLogs:
		if m.currentJob != nil && m.currentRun != nil {
			return m.openURL(fmt.Sprintf("%s/actions/runs/%d/job/%d", base, m.currentRun.ID, m.currentJob.ID))
		}
	}

	return m.openActionsInBrowser()
}

func (m Model) openURL(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "linux":
			cmd = exec.Command("xdg-open", url)
		default:
			cmd = exec.Command("open", url)
		}
		_ = cmd.Start()
		return nil
	}
}

// workflowFileName extracts the filename from a workflow path like ".github/workflows/ci.yml".
func workflowFileName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// openPROrBranch opens the PR page if the run is from a pull_request event,
// otherwise opens the branch on the repo's code page.
func (m Model) openPROrBranch() tea.Cmd {
	base := fmt.Sprintf("https://github.com/%s/%s", m.repoOwner, m.repoName)

	// Find the relevant run based on current context
	var run *gh.WorkflowRun
	if m.focus == FocusSidebar {
		node := m.tree.SelectedNode()
		if node != nil && node.Run != nil {
			run = node.Run
		}
	}
	if run == nil {
		switch m.view {
		case ViewWorkflowRuns:
			run = m.runs.SelectedRun()
		case ViewJobs, ViewLogs:
			run = m.currentRun
		}
	}

	if run == nil {
		return nil
	}

	if run.PRNumber > 0 {
		return m.openURL(fmt.Sprintf("%s/pull/%d", base, run.PRNumber))
	}
	return m.openURL(fmt.Sprintf("%s/tree/%s", base, run.Branch))
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

func (m Model) statusClearCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
		return StatusClearMsg{}
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
		m.repoPicker.SetSize(msg.Width, msg.Height)
		m.updateLayout()
		return m, nil

	case WorkflowsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.updateLayout()
			return m, nil
		}
		m.workflows = msg.Workflows
		m.tree.SetWorkflows(msg.Workflows)
		return m, nil

	case RunsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.updateLayout()
			return m, nil
		}
		if msg.ResetCursor {
			m.runs.SetRunsAndReset(msg.Runs)
		} else {
			m.runs.SetRuns(msg.Runs)
		}
		return m, nil

	case JobsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.updateLayout()
			return m, nil
		}
		m.updateGraphRunInfo()

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
		m.graph.SetJobs(msg.Jobs, deps, m.graph.runName)
		return m, yamlCmd

	case WorkflowYAMLMsg:
		if msg.Err != nil {
			// Silently ignore YAML fetch errors; graph already has inferred tiers
			return m, nil
		}
		m.yamlCache[msg.Path] = msg.Deps
		// Re-render graph with proper deps
		if m.view == ViewJobs && len(m.graph.jobs) > 0 {
			m.graph.SetJobs(m.graph.jobs, msg.Deps, m.graph.runName)
		}
		return m, nil

	case LogsMsg:
		if msg.Err != nil {
			// If job is in-progress, log fetch 404 is expected — don't show error
			if m.logs.IsJobInProgress() {
				return m, nil
			}
			m.err = msg.Err
			m.updateLayout()
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
			// Job just finished — update steps with final status, user stays in step view
			m.logs.SetSteps(msg.Job.Steps, jobName, msg.Job.Status)
			return m, nil
		}
		// Still in progress — update step display
		m.logs.SetSteps(msg.Job.Steps, jobName, msg.Job.Status)
		return m, nil

	case RerunMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.updateLayout()
			return m, nil
		}
		if msg.FailedOnly {
			m.statusMsg = fmt.Sprintf("Rerun failed jobs triggered for run #%d", msg.RunID)
		} else {
			m.statusMsg = fmt.Sprintf("Rerun triggered for run #%d", msg.RunID)
		}
		// Refresh current view after rerun
		if m.view == ViewJobs && m.currentRun != nil {
			m.graph.SetLoading(true)
			return m, tea.Batch(m.fetchJobsForAttempt(m.currentRun.ID, m.currentAttempt), m.statusClearCmd())
		}
		wfID, _ := m.tree.SelectedWorkflow()
		filter := m.filter.CurrentFilter(wfID)
		return m, tea.Batch(m.fetchRuns(filter, false), m.statusClearCmd())

	case TriggerResultMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.updateLayout()
			return m, nil
		}
		m.statusMsg = "Workflow triggered successfully"
		// Refresh runs list after trigger
		wfID, _ := m.tree.SelectedWorkflow()
		filter := m.filter.CurrentFilter(wfID)
		m.runs.SetLoading(true)
		return m, tea.Batch(m.fetchRuns(filter, false), m.statusClearCmd())

	case WorkflowInputsMsg:
		if msg.Err != nil {
			// Still show the dialog, just without inputs
			m.trigger.SetInputs(nil)
			return m, nil
		}
		m.trigger.SetInputs(msg.Inputs)
		return m, nil

	case TriggerSubmitMsg:
		return m, m.triggerWorkflow(msg.WorkflowID, msg.Ref, msg.Inputs)

	case TriggerCancelledMsg:
		return m, nil

	case TickMsg:
		cmds := []tea.Cmd{m.tickCmd()}
		switch m.view {
		case ViewWorkflowRuns:
			wfID, _ := m.tree.SelectedWorkflow()
			filter := m.filter.CurrentFilter(wfID)
			cmds = append(cmds, m.fetchRuns(filter, false))
		case ViewJobs:
			if m.currentRun != nil {
				cmds = append(cmds, m.fetchJobsForAttempt(m.currentRun.ID, m.currentAttempt))
			}
		case ViewLogs:
			if m.currentJob != nil && m.logs.IsJobInProgress() && m.currentRun != nil {
				// Job still running — poll for step status updates
				cmds = append(cmds, m.fetchJobStatus(m.currentRun.ID, m.currentJob.ID))
			}
		}
		return m, tea.Batch(cmds...)

	case RunsForTreeMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.updateLayout()
			return m, nil
		}
		m.tree.SetRunsForWorkflow(msg.WorkflowID, msg.Runs)
		return m, nil

	case RepoListMsg:
		if msg.Err != nil {
			// Non-fatal: just stop loading indicator
			m.repoPicker.loading = false
			return m, nil
		}
		m.repoPicker.AddRepos(msg.Repos)
		return m, nil

	case UserOrgsMsg:
		if msg.Err != nil {
			return m, nil
		}
		cmds := make([]tea.Cmd, 0, len(msg.Orgs))
		for _, org := range msg.Orgs {
			cmds = append(cmds, m.fetchOrgRepos(org))
		}
		return m, tea.Batch(cmds...)

	case OrgReposMsg:
		if msg.Err != nil {
			return m, nil
		}
		m.repoPicker.AddRepos(msg.Repos)
		return m, nil

	case RepoSearchMsg:
		if msg.Err != nil {
			m.repoPicker.searching = false
			return m, nil
		}
		m.repoPicker.SetSearchResults(msg.Repos)
		return m, nil

	case RepoSearchTriggerMsg:
		// Only fire search if query still matches (debounce)
		if m.repoPicker.Visible() && m.repoPicker.InputValue() == msg.Query {
			m.repoPicker.searching = true
			return m, m.searchRepos(msg.Query)
		}
		return m, nil

	case RepoSelectedMsg:
		return m.switchRepo(msg.Owner, msg.Repo)

	case RepoCancelledMsg:
		return m, nil

	case FileContentMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.updateLayout()
			return m, nil
		}
		m.logs.SetFileContent(msg.Content, msg.Name)
		return m, nil

	case StatusClearMsg:
		m.statusMsg = ""
		return m, nil

	case GGTimeoutMsg:
		m.pendingG = false
		return m, nil

	case BackLockMsg:
		m.backLocked = false
		return m, nil

	case FilterAppliedMsg:
		wfID, _ := m.tree.SelectedWorkflow()
		msg.Filter.WorkflowID = wfID
		m.runs.SetLoading(true)
		m.updateLayout()
		return m, m.fetchRuns(msg.Filter, true)

	case FilterCancelledMsg:
		m.updateLayout()
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		// Repo picker captures all keys when visible
		if m.repoPicker.Visible() {
			var cmd tea.Cmd
			m.repoPicker, cmd = m.repoPicker.Update(msg)
			return m, cmd
		}

		// Trigger dialog captures all keys when visible
		if m.trigger.Visible() {
			var cmd tea.Cmd
			m.trigger, cmd = m.trigger.Update(msg)
			return m, cmd
		}

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
	// Only handle scroll wheel — click navigation is disabled
	if _, ok := msg.(tea.MouseClickMsg); ok {
		return m, nil
	}

	// Compute vertical offset of the content area (error/status bar pushes content down)
	contentTopY := 0
	if m.err != nil || m.statusMsg != "" {
		contentTopY = 1
	}

	adjusted := m.adjustMouseY(msg, contentTopY)

	switch m.view {
	case ViewWorkflowRuns:
		if m.focus == FocusSidebar {
			var cmd tea.Cmd
			m.tree, cmd = m.tree.Update(adjusted)
			return m, cmd
		}
		var cmd tea.Cmd
		m.runs, cmd = m.runs.Update(adjusted)
		return m, cmd

	case ViewJobs:
		if m.focus == FocusSidebar {
			var cmd tea.Cmd
			m.tree, cmd = m.tree.Update(adjusted)
			return m, cmd
		}
		var cmd tea.Cmd
		m.graph, cmd = m.graph.Update(adjusted)
		return m, cmd

	case ViewLogs, ViewWorkflowFile:
		if m.focus == FocusSidebar {
			var cmd tea.Cmd
			m.tree, cmd = m.tree.Update(adjusted)
			return m, cmd
		}
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(adjusted)
		return m, cmd
	}
	return m, nil
}

// adjustMouseY returns a new mouse message with Y adjusted by subtracting topOffset.
func (m Model) adjustMouseY(msg tea.MouseMsg, topOffset int) tea.Msg {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		msg.Y -= topOffset
		return msg
	case tea.MouseReleaseMsg:
		msg.Y -= topOffset
		return msg
	case tea.MouseWheelMsg:
		msg.Y -= topOffset
		return msg
	case tea.MouseMotionMsg:
		msg.Y -= topOffset
		return msg
	}
	return msg
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

	// Handle rerun choice dialog
	if m.rerunChoice {
		switch msg.String() {
		case "a":
			m.rerunChoice = false
			return m, m.rerunWorkflow(m.rerunRunID)
		case "f":
			m.rerunChoice = false
			return m, m.rerunFailedJobs(m.rerunRunID)
		case "esc", "q", "n":
			m.rerunChoice = false
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
		return m.goBack()

	case key.Matches(msg, Keys.Help):
		m.showHelp = !m.showHelp
		return m, nil

	case key.Matches(msg, Keys.Back):
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.view == ViewWorkflowRuns {
			return m, nil
		}
		// Lock-based guard: terminals can send duplicate esc events
		// from a single keypress. Ignore esc while locked.
		if m.backLocked {
			return m, nil
		}
		m.backLocked = true
		result, cmd := m.goBack()
		unlockCmd := tea.Tick(300*time.Millisecond, func(_ time.Time) tea.Msg {
			return BackLockMsg{}
		})
		if cmd != nil {
			return result, tea.Batch(cmd, unlockCmd)
		}
		return result, unlockCmd

	case key.Matches(msg, Keys.SwitchPane):
		if m.sidebarVisible {
			return m.toggleFocus()
		}
		return m, nil

	case key.Matches(msg, Keys.SwitchRepo):
		m.repoPicker.Show()
		m.repoPicker.SetSize(m.width, m.height)
		return m, tea.Batch(m.fetchUserRepos(), m.fetchUserOrgs())

	case key.Matches(msg, Keys.OpenBrowser):
		return m, m.openActionsInBrowser()

	case key.Matches(msg, Keys.OpenSelected):
		return m, m.openSelectedInBrowser()

	case key.Matches(msg, Keys.OpenPRBranch):
		return m, m.openPROrBranch()

	case key.Matches(msg, Keys.RerunWorkflow):
		return m.handleRerun()

	case key.Matches(msg, Keys.TriggerWorkflow):
		return m.handleTrigger()

	case key.Matches(msg, Keys.ViewWorkflowFile):
		return m.handleViewWorkflowFile()
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
	case ViewWorkflowFile:
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
			if m.focus == FocusMain {
				m.logs.StartSearch()
				return m, nil
			}
		}

		if m.focus == FocusMain {
			switch msg.String() {
			case "n":
				m.logs.NextMatch()
				return m, nil
			case "N":
				m.logs.PrevMatch()
				return m, nil
			}
		}

		return m.updateFocused(msg)

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
			if m.focus == FocusMain && m.logs.InStepView() && m.currentJob != nil {
				m.logs.SetScrollToStep(m.logs.StepCursor())
				m.logs.SetLoading(true)
				return m, m.fetchJobLogs(m.currentJob.ID)
			}
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
			m.updateLayout()
			return m, nil

		case key.Matches(msg, Keys.Refresh):
			wfID, _ := m.tree.SelectedWorkflow()
			filter := m.filter.CurrentFilter(wfID)
			m.runs.SetLoading(true)
			return m, m.fetchRuns(filter, false)

		case key.Matches(msg, Keys.Enter):
			return m.handleEnter()

		case key.Matches(msg, Keys.Bottom):
			return m.updateFocused(msg)

		case key.Matches(msg, Keys.Top):
			if m.pendingG {
				m.pendingG = false
				topMsg := tea.KeyPressMsg{Code: tea.KeyHome}
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

		case key.Matches(msg, Keys.PrevAttempt):
			if m.currentRun != nil && m.currentAttempt > 1 {
				m.currentAttempt--
				m.graph.SetLoading(true)
				m.updateGraphRunInfo()
				return m, m.fetchJobsForAttempt(m.currentRun.ID, m.currentAttempt)
			}
			return m, nil

		case key.Matches(msg, Keys.NextAttempt):
			if m.currentRun != nil && m.currentAttempt < m.currentRun.RunAttempt {
				m.currentAttempt++
				m.graph.SetLoading(true)
				m.updateGraphRunInfo()
				return m, m.fetchJobsForAttempt(m.currentRun.ID, m.currentAttempt)
			}
			return m, nil

		case key.Matches(msg, Keys.Refresh):
			if m.currentRun != nil {
				m.graph.SetLoading(true)
				m.updateGraphRunInfo()
				return m, m.fetchJobsForAttempt(m.currentRun.ID, m.currentAttempt)
			}
			return m, nil

		case key.Matches(msg, Keys.Bottom):
			return m.updateFocused(msg)

		case key.Matches(msg, Keys.Top):
			if m.pendingG {
				m.pendingG = false
				topMsg := tea.KeyPressMsg{Code: tea.KeyHome}
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
				if node.Workflow == nil {
					// "All Workflows" — show all runs
					filter := m.filter.CurrentFilter(0)
					return m, m.fetchRuns(filter, true)
				}
				// Toggle expand/collapse; fetch runs if expanding and no children
				expanded, n := m.tree.ToggleExpand()
				if expanded && n != nil && len(n.Children) == 0 {
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
				m.currentAttempt = run.RunAttempt
				m.currentRun = run
				m.view = ViewJobs
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.graph.SetLoading(true)
				m.updateGraphRunInfo()
				m.graph.SetFocused(true)
				m.updateLayout()
				return m, m.fetchJobsForAttempt(run.ID, m.currentAttempt)
			}
			return m, nil
		}
		// Main: drill into jobs
		run := m.runs.SelectedRun()
		if run == nil {
			return m, nil
		}
		m.currentAttempt = run.RunAttempt
		m.currentRun = run
		m.view = ViewJobs
		m.focus = FocusMain
		m.runs.SetFocused(false)
		m.tree.SetFocused(false)
		m.graph.SetLoading(true)
		m.updateGraphRunInfo()
		m.graph.SetFocused(true)
		m.updateLayout()
		return m, m.fetchJobsForAttempt(run.ID, m.currentAttempt)

	case ViewJobs:
		if m.focus == FocusSidebar {
			node := m.tree.SelectedNode()
			if node == nil {
				return m, nil
			}
			switch node.Kind {
			case NodeWorkflow:
				if node.Workflow == nil {
					// "All Workflows" — go back to runs view showing all
					m.view = ViewWorkflowRuns
					m.currentRun = nil
					m.focus = FocusMain
					m.tree.SetFocused(false)
					m.runs.SetFocused(true)
					m.updateLayout()
					filter := m.filter.CurrentFilter(0)
					return m, m.fetchRuns(filter, true)
				}
				expanded, n := m.tree.ToggleExpand()
				if expanded && n != nil && len(n.Children) == 0 {
					m.tree.SetLoading(n.Workflow.ID)
					return m, m.fetchRunsForTree(n.Workflow.ID)
				}
				return m, nil
			case NodeRun:
				// Switch to a different run's jobs
				if node.Run == nil {
					return m, nil
				}
				m.currentAttempt = node.Run.RunAttempt
				m.currentRun = node.Run
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.graph.SetLoading(true)
				m.updateGraphRunInfo()
				m.graph.SetFocused(true)
				m.updateLayout()
				return m, m.fetchJobsForAttempt(node.Run.ID, m.currentAttempt)
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

		// Always show step view first — user presses Enter to fetch full logs
		m.logs.SetSteps(job.Steps, job.Name, job.Status)
		return m, nil

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
				m.currentAttempt = node.Run.RunAttempt
				m.currentRun = node.Run
				m.view = ViewJobs
				m.focus = FocusMain
				m.tree.SetFocused(false)
				m.graph.SetLoading(true)
				m.updateGraphRunInfo()
				m.graph.SetFocused(true)
				m.updateLayout()
				return m, m.fetchJobsForAttempt(node.Run.ID, m.currentAttempt)
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleRerun() (tea.Model, tea.Cmd) {
	var run *gh.WorkflowRun

	// Find the relevant run based on context
	if m.focus == FocusSidebar {
		node := m.tree.SelectedNode()
		if node != nil && node.Run != nil {
			run = node.Run
		}
	}
	if run == nil {
		switch m.view {
		case ViewWorkflowRuns:
			run = m.runs.SelectedRun()
		case ViewJobs, ViewLogs:
			run = m.currentRun
		}
	}

	if run == nil {
		return m, nil
	}

	m.rerunRunID = run.ID
	m.rerunChoice = true
	return m, nil
}

func (m Model) handleTrigger() (tea.Model, tea.Cmd) {
	// Find the workflow to trigger
	var wfID int64
	var wfName, wfPath string

	if m.focus == FocusSidebar {
		node := m.tree.SelectedNode()
		if node != nil && node.Kind == NodeWorkflow && node.Workflow != nil {
			wfID = node.Workflow.ID
			wfName = node.Workflow.Name
			wfPath = node.Workflow.Path
		}
	}

	// If no workflow from sidebar, try to infer from current run or selected run
	if wfID == 0 {
		var run *gh.WorkflowRun
		switch m.view {
		case ViewWorkflowRuns:
			run = m.runs.SelectedRun()
		case ViewJobs, ViewLogs:
			run = m.currentRun
		}
		if run != nil {
			wfID = run.WorkflowID
			wfName = run.Name
			wfPath = m.workflowPath(wfID)
		}
	}

	if wfID == 0 {
		return m, nil
	}

	m.trigger.Show(wfID, wfName)
	m.trigger.SetSize(m.width, m.height)

	if wfPath != "" {
		return m, m.fetchWorkflowInputs(wfID, wfPath)
	}
	// No path found, show dialog without inputs
	m.trigger.SetInputs(nil)
	return m, nil
}

func (m Model) handleViewWorkflowFile() (tea.Model, tea.Cmd) {
	var wfID int64
	var wfName, wfPath string

	// Try sidebar first
	if m.focus == FocusSidebar {
		node := m.tree.SelectedNode()
		if node != nil && node.Kind == NodeWorkflow && node.Workflow != nil {
			wfID = node.Workflow.ID
			wfName = node.Workflow.Name
			wfPath = node.Workflow.Path
		}
	}

	// Infer from run context
	if wfID == 0 {
		var run *gh.WorkflowRun
		switch m.view {
		case ViewWorkflowRuns:
			run = m.runs.SelectedRun()
		case ViewJobs, ViewLogs:
			run = m.currentRun
		}
		if run != nil {
			wfID = run.WorkflowID
			wfName = run.Name
			wfPath = m.workflowPath(wfID)
		}
	}

	if wfPath == "" {
		return m, nil
	}

	m.prevView = m.view
	m.view = ViewWorkflowFile
	m.focus = FocusMain
	m.logs.SetLoading(true)
	m.logs.SetFocused(true)
	m.tree.SetFocused(false)
	m.runs.SetFocused(false)
	m.graph.SetFocused(false)
	m.updateLayout()
	_ = wfID
	return m, m.fetchWorkflowFileContent(wfPath, wfName)
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
		case ViewLogs, ViewWorkflowFile:
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
		case ViewLogs, ViewWorkflowFile:
			m.logs.SetFocused(false)
		}
	}
	return m, nil
}

func (m Model) goBack() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewWorkflowFile:
		m.view = m.prevView
		m.focus = FocusMain
		m.logs.SetFocused(false)
		m.tree.SetFocused(false)
		switch m.prevView {
		case ViewWorkflowRuns:
			m.runs.SetFocused(true)
		case ViewJobs:
			m.graph.SetFocused(true)
		case ViewLogs:
			m.logs.SetFocused(true)
		}
		m.updateLayout()
		return m, nil
	case ViewLogs:
		// If viewing log content, go back to step view first
		if !m.logs.InStepView() && m.logs.BackToSteps() {
			return m, nil
		}
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
		m.currentAttempt = node.Run.RunAttempt
		m.currentRun = node.Run
		m.view = ViewJobs
		m.focus = FocusMain
		m.runs.SetFocused(false)
		m.tree.SetFocused(false)
		m.graph.SetLoading(true)
		m.updateGraphRunInfo()
		m.graph.SetFocused(true)
		m.updateLayout()
		return m, m.fetchJobsForAttempt(node.Run.ID, m.currentAttempt)
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
	case ViewLogs, ViewWorkflowFile:
		if m.focus == FocusSidebar {
			m.tree, cmd = m.tree.Update(msg)
		} else {
			m.logs, cmd = m.logs.Update(msg)
		}
	}
	return m, cmd
}

func (m Model) switchRepo(owner, repo string) (tea.Model, tea.Cmd) {
	m.client.SwitchRepo(owner, repo)
	m.repoOwner = owner
	m.repoName = repo

	// Full state reset
	m.view = ViewWorkflowRuns
	m.focus = FocusMain
	m.currentRun = nil
	m.currentJob = nil
	m.currentAttempt = 0
	m.err = nil
	m.statusMsg = ""
	m.showHelp = false
	m.confirmQuit = false
	m.rerunChoice = false
	m.pendingG = false
	m.workflows = nil
	m.yamlCache = make(map[string]map[string][]string)

	// Recreate sub-models
	runs := NewRunsModel()
	runs.SetFocused(true)
	m.runs = runs
	m.tree = NewTreeModel()
	m.graph = NewGraphModel()
	m.logs = NewLogsModel()
	m.filter = NewFilterModel()
	m.trigger = NewTriggerModel()

	m.updateLayout()

	return m, tea.Batch(
		m.fetchWorkflows(),
		m.fetchRuns(gh.RunFilter{}, true),
		m.tickCmd(),
	)
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
	errH := 0
	if m.err != nil || m.statusMsg != "" {
		errH = 1
	}

	switch m.view {
	case ViewWorkflowRuns:
		contentH := clamp(m.height-filterH-helpH-errH, 4, m.height)
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
		contentH := clamp(m.height-helpH-errH, 4, m.height)
		if m.sidebarVisible {
			sidebarW := clamp(m.width/4, 20, 35)
			mainW := clamp(m.width-sidebarW, 10, m.width)
			m.tree.SetSize(sidebarW, contentH)
			m.graph.SetSize(mainW, contentH)
		} else {
			m.graph.SetSize(m.width, contentH)
		}

	case ViewLogs, ViewWorkflowFile:
		contentH := clamp(m.height-helpH-errH, 4, m.height)
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

func (m Model) viewWithMode(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	// Disable mouse capture in log/file content view so users can select text
	if m.view == ViewWorkflowFile && !m.logs.Searching() {
		v.MouseMode = tea.MouseModeNone
	} else if m.view == ViewLogs && !m.logs.InStepView() && !m.logs.Searching() {
		v.MouseMode = tea.MouseModeNone
	} else {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return m.viewWithMode("Loading...")
	}

	// Error bar / status bar
	errBar := ""
	if m.err != nil {
		errBar = styleError.Width(m.width).Render(fmt.Sprintf("Error: %s", m.err))
	} else if m.statusMsg != "" {
		errBar = styleStatusBar.Width(m.width).Render(m.statusMsg)
	}

	// Confirm quit dialog
	if m.confirmQuit {
		return m.viewWithMode(m.confirmQuitView())
	}

	// Rerun choice dialog
	if m.rerunChoice {
		return m.viewWithMode(m.rerunChoiceView())
	}

	// Help overlay
	if m.showHelp {
		return m.viewWithMode(m.helpView())
	}

	// Repo picker overlay
	if m.repoPicker.Visible() {
		return m.viewWithMode(m.repoPicker.View(m.width, m.height))
	}

	// Trigger workflow overlay
	if m.trigger.Visible() {
		return m.viewWithMode(m.trigger.View(m.width, m.height))
	}

	var output string

	switch m.view {
	case ViewLogs, ViewWorkflowFile:
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
	return m.viewWithMode(truncateToFit(output, m.width, m.height))
}

func (m Model) helpBarView() string {
	repo := styleRepoIndicator.Render(m.repoOwner + "/" + m.repoName)
	extra := ""
	if m.view == ViewJobs && m.currentRun != nil && m.currentRun.RunAttempt > 1 {
		extra = "  [/]:attempt"
	}
	keys := styleHelpBar.Render("  ↑↓/jk:move  ←→/hl:expand  tab:pane  enter:select  esc/q:back  /:filter  r:refresh  R:rerun  T:trigger  w:workflow  b:sidebar  o:open  p:PR/branch  O:actions  ?:help" + extra)
	return repo + keys
}

func (m *Model) updateGraphRunInfo() {
	if m.currentRun == nil {
		return
	}
	runName := fmt.Sprintf("#%d %s", m.currentRun.Number, m.currentRun.Branch)
	if m.currentRun.RunAttempt > 1 {
		runName = fmt.Sprintf("#%d·%d %s", m.currentRun.Number, m.currentAttempt, m.currentRun.Branch)
	}
	m.graph.SetRunInfo(runName, m.currentAttempt, m.currentRun.RunAttempt)
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

func (m Model) rerunChoiceView() string {
	dialog := styleConfirmDialog.Render("Rerun: (a)ll  (f)ailed  (esc) cancel")
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
  click         Focus pane / select item
  scroll        Scroll content

Actions:
  /             Open filter bar / search logs
  r             Refresh data
  R             Rerun selected workflow run
  T             Trigger workflow (dispatch)
  w             View workflow file
  b             Toggle sidebar
  o             Open selected in browser
  p             Open PR or branch in browser
  O             Open actions page in browser
  S             Switch repository
  ?             Toggle help
  q             Quit

Jobs View:
  [ / ]         Previous / next run attempt

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
