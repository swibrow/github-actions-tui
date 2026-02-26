package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

// GraphTier groups parallel jobs into a single tier.
type GraphTier struct {
	Label string // e.g., "Tier 1 (parallel)" or "Tier 2 (needs: build, test)"
	Jobs  []gh.WorkflowJob
}

// GraphModel displays jobs grouped by dependency tiers.
type GraphModel struct {
	tiers         []GraphTier
	jobs          []gh.WorkflowJob
	flat          []int // flat index -> (tierIdx, jobIdx) encoded as tierIdx*1000+jobIdx
	cursor        int
	offset        int
	focused       bool
	loading       bool
	width         int
	height        int
	runName       string
	currentAttempt int
	totalAttempts  int
}

func NewGraphModel() GraphModel {
	return GraphModel{}
}

// SetJobs arranges jobs into tiers based on dependency information.
func (m *GraphModel) SetJobs(jobs []gh.WorkflowJob, deps map[string][]string, runName string) {
	m.jobs = jobs
	m.runName = runName
	m.loading = false

	if deps == nil {
		deps = gh.InferJobDependencies(jobs)
	}

	m.tiers = buildTiers(jobs, deps)
	m.buildFlat()

	if len(m.flat) > 0 {
		m.cursor = 0
		m.offset = 0
	}
}

// buildTiers performs a topological sort of jobs into tiers.
func buildTiers(jobs []gh.WorkflowJob, deps map[string][]string) []GraphTier {
	if len(jobs) == 0 {
		return nil
	}

	// Build name -> job lookup
	jobMap := make(map[string]*gh.WorkflowJob, len(jobs))
	for i := range jobs {
		jobMap[jobs[i].Name] = &jobs[i]
	}

	// Track which tier each job belongs to
	tierOf := make(map[string]int, len(jobs))
	assigned := make(map[string]bool, len(jobs))

	// Iteratively assign tiers: a job goes in tier max(deps)+1
	changed := true
	for changed {
		changed = false
		for _, j := range jobs {
			if assigned[j.Name] {
				continue
			}
			needs := deps[j.Name]
			if len(needs) == 0 {
				tierOf[j.Name] = 0
				assigned[j.Name] = true
				changed = true
				continue
			}
			// Check if all deps are assigned
			allAssigned := true
			maxTier := 0
			for _, dep := range needs {
				if !assigned[dep] {
					allAssigned = false
					break
				}
				if tierOf[dep] >= maxTier {
					maxTier = tierOf[dep] + 1
				}
			}
			if allAssigned {
				tierOf[j.Name] = maxTier
				assigned[j.Name] = true
				changed = true
			}
		}
	}

	// Assign any unresolved jobs to tier 0
	for _, j := range jobs {
		if !assigned[j.Name] {
			tierOf[j.Name] = 0
		}
	}

	// Group into tiers
	maxTier := 0
	for _, t := range tierOf {
		if t > maxTier {
			maxTier = t
		}
	}

	tiers := make([]GraphTier, maxTier+1)
	for i := range tiers {
		tiers[i].Label = fmt.Sprintf("Tier %d", i+1)
	}
	for _, j := range jobs {
		t := tierOf[j.Name]
		tiers[t].Jobs = append(tiers[t].Jobs, j)
	}

	// Build labels with dependency info
	for i := range tiers {
		if i == 0 {
			if len(tiers[i].Jobs) > 1 {
				tiers[i].Label = "Tier 1 (parallel)"
			} else {
				tiers[i].Label = "Tier 1"
			}
		} else {
			// Find what this tier needs
			needsSet := make(map[string]bool)
			for _, j := range tiers[i].Jobs {
				for _, dep := range deps[j.Name] {
					needsSet[dep] = true
				}
			}
			if len(needsSet) > 0 {
				needsList := make([]string, 0, len(needsSet))
				for dep := range needsSet {
					needsList = append(needsList, dep)
				}
				tiers[i].Label = fmt.Sprintf("Tier %d (needs: %s)", i+1, strings.Join(needsList, ", "))
			} else if len(tiers[i].Jobs) > 1 {
				tiers[i].Label = fmt.Sprintf("Tier %d (parallel)", i+1)
			}
		}
	}

	return tiers
}

func (m *GraphModel) buildFlat() {
	m.flat = nil
	for ti, tier := range m.tiers {
		for ji := range tier.Jobs {
			m.flat = append(m.flat, ti*1000+ji)
		}
	}
}

// SelectedJob returns the currently selected job.
func (m GraphModel) SelectedJob() *gh.WorkflowJob {
	if m.cursor < 0 || m.cursor >= len(m.flat) {
		return nil
	}
	idx := m.flat[m.cursor]
	ti, ji := idx/1000, idx%1000
	if ti < len(m.tiers) && ji < len(m.tiers[ti].Jobs) {
		j := m.tiers[ti].Jobs[ji]
		return &j
	}
	return nil
}

func (m *GraphModel) SetRunInfo(runName string, currentAttempt, totalAttempts int) {
	m.runName = runName
	m.currentAttempt = currentAttempt
	m.totalAttempts = totalAttempts
}

func (m *GraphModel) SetFocused(focused bool) {
	m.focused = focused
}

func (m *GraphModel) SetLoading(loading bool) {
	m.loading = loading
}

func (m *GraphModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *GraphModel) scrollToVisible() {
	innerH := m.height - 4
	if innerH < 1 {
		innerH = 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+innerH {
		m.offset = m.cursor - innerH + 1
	}
}

func (m GraphModel) Update(msg tea.Msg) (GraphModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.flat)-1 {
				m.cursor++
				m.scrollToVisible()
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.scrollToVisible()
			}
		case "home":
			m.cursor = 0
			m.offset = 0
		case "end":
			m.cursor = len(m.flat) - 1
			m.scrollToVisible()
		}
	}
	return m, nil
}

func (m GraphModel) View() string {
	style := styleMainBlurred
	if m.focused {
		style = styleMainFocused
	}

	title := styleTitle.Render(fmt.Sprintf("Jobs: %s", m.runName)) + "\n"
	if m.totalAttempts > 1 {
		title += styleHelpBar.Render(fmt.Sprintf("  ← [ attempt %d/%d ] →", m.currentAttempt, m.totalAttempts)) + "\n"
	}

	if m.loading {
		content := title + styleLoading.Render("  Loading jobs...")
		return m.renderBox(style, content)
	}

	if len(m.flat) == 0 {
		content := title + styleLoading.Render("  No jobs found")
		return m.renderBox(style, content)
	}

	// Inner content width: panel width minus border(2) and padding(2)
	innerW := m.width - 4
	if innerW < 20 {
		innerW = 20
	}
	// Job line: "  " (node indent) + icon(1) + " " + name + " " + duration
	durW := 10
	indentW := 2 // styleGraphNode PaddingLeft
	nameW := innerW - indentW - 2 - durW // 2 = icon + space after icon
	if nameW < 10 {
		nameW = 10
	}

	// Build all lines and track which line the cursor is on
	var allLines []string
	cursorLine := 0
	flatIdx := 0
	for _, tier := range m.tiers {
		allLines = append(allLines, styleGraphTier.Render(tier.Label))

		for _, job := range tier.Jobs {
			icon := StatusIcon(job.Status, job.Conclusion)
			name := truncate(job.Name, nameW)
			dur := formatDuration(job.Duration)
			padding := nameW - len(name)
			if padding < 0 {
				padding = 0
			}
			text := icon + " " + name + strings.Repeat(" ", padding) + " " + dur

			if flatIdx == m.cursor {
				cursorLine = len(allLines)
			}
			if flatIdx == m.cursor && m.focused {
				allLines = append(allLines, styleGraphNodeSelected.Render(text))
			} else {
				allLines = append(allLines, styleGraphNode.Render(text))
			}
			flatIdx++
		}
		allLines = append(allLines, "") // blank line between tiers
	}

	// Scroll to keep cursor visible
	innerH := m.height - 4 // border + title
	if m.totalAttempts > 1 {
		innerH-- // attempt hint line
	}
	if innerH < 1 {
		innerH = 1
	}
	scrollOffset := 0
	if cursorLine >= innerH {
		scrollOffset = cursorLine - innerH + 1
	}
	// Clamp
	maxOffset := len(allLines) - innerH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if scrollOffset > maxOffset {
		scrollOffset = maxOffset
	}

	end := scrollOffset + innerH
	if end > len(allLines) {
		end = len(allLines)
	}
	visibleLines := allLines[scrollOffset:end]

	content := title + strings.Join(visibleLines, "\n")
	return m.renderBox(style, content)
}

func (m GraphModel) renderBox(style lipgloss.Style, content string) string {
	lines := strings.Split(content, "\n")
	innerH := m.height - 2
	if innerH < 1 {
		innerH = 1
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	content = strings.Join(lines, "\n")
	return style.Width(m.width - 2).Height(m.height - 2).Render(content)
}
