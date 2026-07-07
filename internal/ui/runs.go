package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

// Card grid geometry.
const (
	cardMinW = 26 // minimum card outer width before dropping a column
	cardH    = 6  // card outer height (rounded border 2 + 4 content lines)
	cardGapX = 1  // horizontal gap between cards
	cardGapY = 1  // vertical gap between card rows
)

type RunsModel struct {
	runs      []gh.WorkflowRun
	cursor    int
	rowOffset int // first visible card-row
	focused   bool
	loading   bool
	width     int
	height    int
	title     string

	cols  int // columns in the grid (recomputed on resize)
	cardW int // card outer width
}

func NewRunsModel() RunsModel {
	return RunsModel{title: "Workflow Runs", cols: 1, cardW: cardMinW}
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
	if resetCursor {
		m.cursor = 0
		m.rowOffset = 0
	}
	if m.cursor >= len(runs) {
		m.cursor = len(runs) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.scrollToVisible()
}

func (m RunsModel) SelectedRun() *gh.WorkflowRun {
	if m.cursor < 0 || m.cursor >= len(m.runs) {
		return nil
	}
	r := m.runs[m.cursor]
	return &r
}

func (m *RunsModel) SetFocused(focused bool) {
	m.focused = focused
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
	m.computeGrid()
	m.scrollToVisible()
}

// computeGrid sizes the card columns to fill the available inner width.
func (m *RunsModel) computeGrid() {
	innerW := paneInnerWidth(m.width)
	cols := (innerW + cardGapX) / (cardMinW + cardGapX)
	if cols < 1 {
		cols = 1
	}
	// Expand cards to consume the leftover width evenly.
	m.cols = cols
	m.cardW = (innerW - cardGapX*(cols-1)) / cols
	if m.cardW < cardMinW/2 {
		m.cardW = cardMinW / 2
	}
}

func (m *RunsModel) visibleRows() int {
	innerH := paneInnerHeight(m.height)
	rows := (innerH + cardGapY) / (cardH + cardGapY)
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *RunsModel) scrollToVisible() {
	if m.cols < 1 {
		m.cols = 1
	}
	row := m.cursor / m.cols
	vis := m.visibleRows()
	if row < m.rowOffset {
		m.rowOffset = row
	}
	if row >= m.rowOffset+vis {
		m.rowOffset = row - vis + 1
	}
	if m.rowOffset < 0 {
		m.rowOffset = 0
	}
}

func (m RunsModel) Update(msg tea.Msg) (RunsModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.rowOffset--
		case tea.MouseWheelDown:
			m.rowOffset++
		}
		m.clampOffset()
		return m, nil
	case tea.KeyMsg:
		if len(m.runs) == 0 {
			return m, nil
		}
		switch msg.String() {
		case "k", "up":
			m.cursor -= m.cols
		case "j", "down":
			m.cursor += m.cols
		case "h", "left":
			m.cursor--
		case "l", "right":
			m.cursor++
		case "home":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.runs) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= len(m.runs) {
			m.cursor = len(m.runs) - 1
		}
		m.scrollToVisible()
	}
	return m, nil
}

func (m *RunsModel) clampOffset() {
	totalRows := 0
	if m.cols > 0 {
		totalRows = (len(m.runs) + m.cols - 1) / m.cols
	}
	maxOffset := totalRows - m.visibleRows()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.rowOffset > maxOffset {
		m.rowOffset = maxOffset
	}
	if m.rowOffset < 0 {
		m.rowOffset = 0
	}
}

func (m RunsModel) View() string {
	titleText := fmt.Sprintf("%s (%d)", m.title, len(m.runs))
	if m.loading && len(m.runs) > 0 {
		titleText += "  ⟳ refreshing…"
	}

	innerH := paneInnerHeight(m.height)
	var content string

	switch {
	case m.loading && len(m.runs) == 0:
		content = styleLoading.Render("  Loading runs...")
	case len(m.runs) == 0:
		content = styleLoading.Render("  No runs found")
	default:
		content = m.renderGrid()
	}

	lines := strings.Split(content, "\n")
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	content = strings.Join(lines, "\n")

	return paneFrame(content, m.width, m.height, m.focused, "⚡", titleText)
}

// renderGrid lays the run cards out in a responsive grid.
func (m RunsModel) renderGrid() string {
	vis := m.visibleRows()
	gapCol := strings.Repeat(" ", cardGapX)

	var rowBlocks []string
	for r := m.rowOffset; r < m.rowOffset+vis; r++ {
		start := r * m.cols
		if start >= len(m.runs) {
			break
		}
		var cards []string
		for c := 0; c < m.cols; c++ {
			idx := start + c
			if idx < len(m.runs) {
				cards = append(cards, m.renderCard(m.runs[idx], idx == m.cursor))
			} else {
				// Blank placeholder keeps the row height/alignment stable.
				cards = append(cards, lipgloss.NewStyle().Width(m.cardW).Height(cardH).Render(""))
			}
			if c < m.cols-1 {
				cards = append(cards, gapCol)
			}
		}
		rowBlocks = append(rowBlocks, lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	}

	// Join card-rows with a blank gap line between them.
	var out []string
	for i, rb := range rowBlocks {
		if i > 0 {
			for g := 0; g < cardGapY; g++ {
				out = append(out, "")
			}
		}
		out = append(out, rb)
	}
	return strings.Join(out, "\n")
}

// renderCard renders a single run as a bordered card.
func (m RunsModel) renderCard(r gh.WorkflowRun, selected bool) string {
	style := styleCard
	if selected {
		style = styleCardSelected
	}
	ciw := m.cardW - 4 // border(2) + padding(2)
	if ciw < 6 {
		ciw = 6
	}

	num := fmt.Sprintf("#%d", r.Number)
	if r.RunAttempt > 1 {
		num = fmt.Sprintf("#%d·%d", r.Number, r.RunAttempt)
	}
	icon := StatusIcon(r.Status, r.Conclusion)

	titleFg := colorText
	if selected {
		titleFg = lipgloss.Color("#ffffff")
	}
	title := icon + " " + lipgloss.NewStyle().Bold(true).Foreground(titleFg).
		Render(truncate(num+" "+r.Name, ciw-2))

	branch := lipgloss.NewStyle().Foreground(colorTeal).Render(truncate(r.Branch, ciw))

	sha := ""
	if len(r.HeadSHA) >= 7 {
		sha = r.HeadSHA[:7]
	}
	meta := strings.TrimPrefix(sha+" · "+r.Event, " · ")
	metaLine := lipgloss.NewStyle().Foreground(colorMuted).Render(truncate(meta, ciw))

	// Bottom line: actor + age on the left, duration tag on the right.
	left := r.Actor
	if age := relativeTime(r.CreatedAt); age != "" {
		if left != "" {
			left += " · " + age
		} else {
			left = age
		}
	}
	dur := formatDuration(r.Duration)
	durTag := ""
	if dur != "" {
		durTag = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(dur)
	}
	bottom := padBetween(
		lipgloss.NewStyle().Foreground(colorMuted).Render(truncate(left, ciw-lipgloss.Width(durTag)-1)),
		durTag, ciw)

	body := lipgloss.JoinVertical(lipgloss.Left, title, branch, metaLine, bottom)
	return style.Width(m.cardW).Height(cardH).Render(body)
}

// padBetween left-justifies left and right-justifies right within width.
func padBetween(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
