package ui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

// Matches GitHub Actions log timestamp: "2024-01-15T10:30:45.1234567Z "
var timestampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z `)

// Matches ##[group]<name> lines with optional timestamp prefix
var groupRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z )?##\[group\](.+)$`)

// Matches ##[endgroup] lines with optional timestamp prefix
var endgroupRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z )?##\[endgroup\]\s*$`)

type logSection struct {
	Name string // text after "##[group]"
	Line int    // 0-based line number in processed content
}

type LogsModel struct {
	viewport       viewport.Model
	jobName        string
	rawContent     string // original log content
	loading        bool
	focused        bool
	showTimestamps bool
	searching      bool
	searchInput    textinput.Model
	searchTerm     string
	matchLines     []int // line numbers with matches
	matchIdx       int   // current match index
	width          int
	height         int
	ready          bool
	steps          []gh.JobStep // live step status for in-progress jobs
	jobStatus      string       // "in_progress", "completed", etc.
	showingSteps   bool         // true = step list view, false = log content view
	stepCursor     int          // highlighted step (0-based)
	stepOffset     int          // scroll offset for long step lists
	sections       []logSection // parsed group sections from log content
	scrollToStep   int          // step index to scroll to when content loads (-1 = none)
}

func NewLogsModel() LogsModel {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = 100
	return LogsModel{
		showTimestamps: true,
		searchInput:    ti,
		scrollToStep:   -1,
	}
}

func (m *LogsModel) SetContent(content, jobName string) {
	isRefresh := m.jobName == jobName && !m.loading
	m.jobName = jobName
	m.loading = false
	m.showingSteps = false
	m.jobStatus = "completed"

	if isRefresh {
		// Preserve scroll position and search on poll refresh
		yOff := m.viewport.YOffset()
		m.rawContent = content
		m.applyContent()
		m.viewport.SetYOffset(yOff)
	} else {
		m.rawContent = content
		m.searchTerm = ""
		m.matchLines = nil
		m.matchIdx = 0
		m.searchInput.SetValue("")
		m.applyContent()

		if m.scrollToStep >= 0 {
			targetLine := m.lineForStep(m.scrollToStep)
			m.viewport.SetYOffset(targetLine)
			m.scrollToStep = -1
		} else {
			m.viewport.GotoBottom()
		}
	}
}

// SetSteps updates the step status display. Used for both in-progress and completed jobs.
func (m *LogsModel) SetSteps(steps []gh.JobStep, jobName, status string) {
	m.jobName = jobName
	m.jobStatus = status
	m.steps = steps
	m.loading = false
	m.showingSteps = true

	// Preserve cursor if refreshing the same job
	if m.stepCursor >= len(steps) {
		m.stepCursor = 0
		m.stepOffset = 0
	}
}

// InStepView returns true when the step list is being shown (not log content).
func (m *LogsModel) InStepView() bool {
	return m.showingSteps
}

// BackToSteps transitions from log content view back to step view.
// Returns false if there are no steps to go back to.
func (m *LogsModel) BackToSteps() bool {
	if len(m.steps) == 0 {
		return false
	}
	m.showingSteps = true
	return true
}

// StepCursor returns the current step cursor index.
func (m *LogsModel) StepCursor() int {
	return m.stepCursor
}

// SetScrollToStep records which step to scroll to when content loads.
func (m *LogsModel) SetScrollToStep(idx int) {
	m.scrollToStep = idx
}

// lineForStep finds the line number for a step by matching against section names.
func (m *LogsModel) lineForStep(stepIdx int) int {
	if len(m.sections) == 0 || stepIdx < 0 || stepIdx >= len(m.steps) {
		return 0
	}

	stepName := m.steps[stepIdx].Name

	// Exact match
	for _, sec := range m.sections {
		if sec.Name == stepName {
			return sec.Line
		}
	}

	// Substring match (GitHub wraps names like "Run actions/checkout@v4")
	for _, sec := range m.sections {
		if strings.Contains(sec.Name, stepName) || strings.Contains(stepName, sec.Name) {
			return sec.Line
		}
	}

	// Positional fallback
	if stepIdx < len(m.sections) {
		return m.sections[stepIdx].Line
	}

	return 0
}

// IsJobInProgress returns true if the current job hasn't completed yet.
func (m *LogsModel) IsJobInProgress() bool {
	return m.jobStatus == "in_progress" || m.jobStatus == "queued" || m.jobStatus == "waiting" || m.jobStatus == "pending"
}

// stepStatusText returns styled status text for a step.
func stepStatusText(step gh.JobStep) string {
	switch step.Status {
	case "in_progress":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("running")
	case "completed":
		switch step.Conclusion {
		case "success":
			return lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Render("done")
		case "failure":
			return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("failed")
		case "skipped":
			return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("skipped")
		default:
			return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(step.Conclusion)
		}
	case "queued", "pending":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("waiting")
	}
	return ""
}

func (m *LogsModel) scrollStepsToVisible() {
	innerH := m.height - 6 // border(2) + header(2) + info line + blank line
	if innerH < 1 {
		innerH = 1
	}
	if m.stepOffset < 0 {
		m.stepOffset = 0
	}
	if m.stepCursor < m.stepOffset {
		m.stepOffset = m.stepCursor
	}
	if m.stepCursor >= m.stepOffset+innerH {
		m.stepOffset = m.stepCursor - innerH + 1
	}
}

func (m LogsModel) updateStepView(msg tea.Msg) (LogsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.stepCursor < len(m.steps)-1 {
				m.stepCursor++
				m.scrollStepsToVisible()
			}
		case "k", "up":
			if m.stepCursor > 0 {
				m.stepCursor--
				m.scrollStepsToVisible()
			}
		case "home":
			m.stepCursor = 0
			m.stepOffset = 0
		case "end":
			m.stepCursor = len(m.steps) - 1
			m.scrollStepsToVisible()
		}
	case tea.MouseWheelMsg:
		m.handleStepScroll(msg.Button)
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.handleStepClick(msg.Y)
		}
	}
	return m, nil
}

func (m *LogsModel) handleStepScroll(button tea.MouseButton) {
	delta := 3
	innerH := m.height - 6
	if innerH < 1 {
		innerH = 1
	}
	switch button {
	case tea.MouseWheelUp:
		m.stepOffset -= delta
		if m.stepOffset < 0 {
			m.stepOffset = 0
		}
	case tea.MouseWheelDown:
		maxOffset := len(m.steps) - innerH
		if maxOffset < 0 {
			maxOffset = 0
		}
		m.stepOffset += delta
		if m.stepOffset > maxOffset {
			m.stepOffset = maxOffset
		}
	}
}

func (m *LogsModel) handleStepClick(absY int) {
	// border(1) + title(1) + separator(1) + info(1) + blank(1) = 5 lines of header
	relY := absY - 5
	if relY < 0 {
		return
	}
	idx := m.stepOffset + relY
	if idx >= 0 && idx < len(m.steps) {
		m.stepCursor = idx
	}
}

func (m LogsModel) renderStepView(innerW int) string {
	var info string
	if m.IsJobInProgress() {
		info = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Italic(true).
			Render("  in progress")
	} else {
		info = lipgloss.NewStyle().Foreground(colorMuted).Italic(true).
			Render("  press enter to view logs")
	}

	var lines []string
	for i, step := range m.steps {
		icon := StatusIcon(step.Status, step.Conclusion)
		name := step.Name
		st := stepStatusText(step)
		var text string
		if st != "" {
			text = fmt.Sprintf("  %s %s  %s", icon, name, st)
		} else {
			text = fmt.Sprintf("  %s %s", icon, name)
		}

		if i == m.stepCursor && m.focused {
			// Highlight selected step
			lines = append(lines, styleGraphNodeSelected.Render(icon+" "+name+func() string {
				if st != "" {
					return "  " + st
				}
				return ""
			}()))
		} else {
			lines = append(lines, text)
		}
	}

	// Apply scroll offset
	innerH := m.height - 6 // border(2) + header(2) + info + blank
	if innerH < 1 {
		innerH = 1
	}
	end := m.stepOffset + innerH
	if end > len(lines) {
		end = len(lines)
	}
	start := m.stepOffset
	if start > len(lines) {
		start = len(lines)
	}
	visibleLines := lines[start:end]

	parts := []string{info, ""}
	parts = append(parts, visibleLines...)
	return strings.Join(parts, "\n")
}

func (m *LogsModel) SetLoading(loading bool) {
	m.loading = loading
}

func (m *LogsModel) SetFocused(focused bool) {
	m.focused = focused
}

// applyContent rebuilds the viewport content based on timestamp, group, and search settings.
func (m *LogsModel) applyContent() {
	rawLines := strings.Split(m.rawContent, "\n")
	m.sections = nil
	var processed []string

	for _, line := range rawLines {
		// Drop ##[endgroup] lines entirely
		if endgroupRe.MatchString(line) {
			continue
		}

		// Handle ##[group]<name> lines
		if match := groupRe.FindStringSubmatch(line); match != nil {
			groupName := match[2]
			processedLine := groupName
			if m.showTimestamps && match[1] != "" {
				processedLine = match[1] + groupName
			}
			m.sections = append(m.sections, logSection{
				Name: groupName,
				Line: len(processed),
			})
			processed = append(processed, styleLogGroup.Render(processedLine))
			continue
		}

		// Normal lines: strip timestamps if toggled off
		if !m.showTimestamps {
			line = timestampRe.ReplaceAllString(line, "")
		}
		processed = append(processed, line)
	}

	// Search highlighting on processed lines
	if m.searchTerm != "" {
		m.matchLines = nil
		for i, line := range processed {
			if strings.Contains(strings.ToLower(line), strings.ToLower(m.searchTerm)) {
				m.matchLines = append(m.matchLines, i)
				processed[i] = highlightMatch(line, m.searchTerm)
			}
		}
	} else {
		m.matchLines = nil
	}

	m.viewport.SetContent(strings.Join(processed, "\n"))
}

func highlightMatch(line, term string) string {
	lower := strings.ToLower(line)
	lowerTerm := strings.ToLower(term)
	hlStyle := lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("0"))

	var result strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], lowerTerm)
		if idx < 0 {
			result.WriteString(line[pos:])
			break
		}
		result.WriteString(line[pos : pos+idx])
		result.WriteString(hlStyle.Render(line[pos+idx : pos+idx+len(term)]))
		pos += idx + len(term)
	}
	return result.String()
}

func (m *LogsModel) jumpToMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	if m.matchIdx < 0 {
		m.matchIdx = len(m.matchLines) - 1
	}
	if m.matchIdx >= len(m.matchLines) {
		m.matchIdx = 0
	}
	lineNum := m.matchLines[m.matchIdx]
	// Position the viewport so the match is visible
	m.viewport.SetYOffset(lineNum)
}

func (m *LogsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	innerW := width - 4  // border(2) + padding(2)
	headerH := 2         // title + separator
	footerH := 1         // scroll info
	searchH := 0
	if m.searching {
		searchH = 1
	}
	innerH := height - 2 // border(2)
	vpH := innerH - headerH - footerH - searchH
	if vpH < 1 {
		vpH = 1
	}
	if innerW < 10 {
		innerW = 10
	}
	if !m.ready {
		m.viewport = viewport.New(viewport.WithWidth(innerW), viewport.WithHeight(vpH))
		m.ready = true
	} else {
		m.viewport.SetWidth(innerW)
		m.viewport.SetHeight(vpH)
	}
}

func (m *LogsModel) Searching() bool {
	return m.searching
}

func (m LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	if m.searching {
		return m.updateSearch(msg)
	}
	if m.showingSteps {
		return m.updateStepView(msg)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m LogsModel) updateSearch(msg tea.Msg) (LogsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.searching = false
			m.recalcViewportHeight()
			return m, nil
		case "esc":
			m.searching = false
			m.searchInput.SetValue("")
			m.searchTerm = ""
			m.matchLines = nil
			m.matchIdx = 0
			m.applyContent()
			m.recalcViewportHeight()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)

	// Live search: update highlights as you type
	newTerm := m.searchInput.Value()
	if newTerm != m.searchTerm {
		m.searchTerm = newTerm
		m.matchIdx = 0
		m.applyContent()
		if len(m.matchLines) > 0 {
			m.jumpToMatch()
		}
	}

	return m, cmd
}

func (m *LogsModel) recalcViewportHeight() {
	headerH := 2
	footerH := 1
	searchH := 0
	if m.searching {
		searchH = 1
	}
	innerH := m.height - 2
	vpH := innerH - headerH - footerH - searchH
	if vpH < 1 {
		vpH = 1
	}
	m.viewport.SetHeight(vpH)
}

// StartSearch opens the search input.
func (m *LogsModel) StartSearch() {
	m.searching = true
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	m.recalcViewportHeight()
}

// ToggleTimestamps toggles timestamp visibility.
func (m *LogsModel) ToggleTimestamps() {
	m.showTimestamps = !m.showTimestamps
	// Remember scroll position
	yOff := m.viewport.YOffset()
	m.applyContent()
	m.viewport.SetYOffset(yOff)
}

// NextMatch jumps to the next search match.
func (m *LogsModel) NextMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	m.matchIdx++
	m.jumpToMatch()
}

// PrevMatch jumps to the previous search match.
func (m *LogsModel) PrevMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	m.matchIdx--
	m.jumpToMatch()
}

func (m LogsModel) View() string {
	style := styleMainBlurred
	if m.focused {
		style = styleMainFocused
	}

	innerW := m.width - 4
	if innerW < 10 {
		innerW = 10
	}

	var content string
	if m.loading {
		header := styleTitle.Render(fmt.Sprintf("Logs: %s", m.jobName))
		content = header + "\n" + styleLoading.Render("  Loading logs...")
	} else if m.showingSteps {
		header := styleTitle.Render(fmt.Sprintf("Steps: %s", m.jobName))
		separator := lipgloss.NewStyle().Foreground(colorBorder).
			Render(strings.Repeat("─", innerW))
		stepContent := m.renderStepView(innerW)
		content = lipgloss.JoinVertical(lipgloss.Left, header, separator, stepContent)
	} else {
		content = m.renderLogView(innerW)
	}

	innerH := m.height - 2
	if innerH < 1 {
		innerH = 1
	}
	lines := strings.Split(content, "\n")
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	content = strings.Join(lines, "\n")

	return style.Width(m.width).Height(m.height).Render(content)
}

func (m LogsModel) renderLogView(innerW int) string {
	header := styleTitle.Render(fmt.Sprintf("Logs: %s", m.jobName))
	separator := lipgloss.NewStyle().Foreground(colorBorder).
		Render(strings.Repeat("─", innerW))

	// Footer with search info
	pct := m.viewport.ScrollPercent() * 100
	footerParts := fmt.Sprintf("%.0f%%", pct)
	if m.searchTerm != "" {
		if len(m.matchLines) > 0 {
			footerParts += fmt.Sprintf(" │ match %d/%d", m.matchIdx+1, len(m.matchLines))
		} else {
			footerParts += " │ no matches"
		}
	}
	if !m.showTimestamps {
		footerParts += " │ timestamps off"
	}
	footer := lipgloss.NewStyle().Foreground(colorMuted).Render(footerParts)

	parts := []string{header, separator, m.viewport.View(), footer}
	if m.searching {
		searchBar := lipgloss.NewStyle().Foreground(colorPrimary).Render("/") + m.searchInput.View()
		parts = append(parts, searchBar)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
