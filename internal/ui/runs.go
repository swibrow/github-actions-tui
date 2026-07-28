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

// RunsLayout selects how the runs pane arranges its items.
type RunsLayout int

const (
	LayoutGrid RunsLayout = iota // kanban-style cards
	LayoutList                   // one row per run
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
	layout    RunsLayout

	cols  int // columns in the grid (recomputed on resize)
	cardW int // card outer width
}

func NewRunsModel() RunsModel {
	return RunsModel{title: "Workflow Runs", cols: 1, cardW: cardMinW}
}

func (m RunsModel) Layout() RunsLayout {
	return m.layout
}

func (m *RunsModel) SetLayout(layout RunsLayout) {
	m.layout = layout
	m.computeGrid()
	m.scrollToVisible()
}

func (m *RunsModel) ToggleLayout() {
	if m.layout == LayoutGrid {
		m.SetLayout(LayoutList)
		return
	}
	m.SetLayout(LayoutGrid)
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
// The list layout is a single column, so navigation arithmetic that steps by
// m.cols works unchanged in both layouts.
func (m *RunsModel) computeGrid() {
	innerW := paneInnerWidth(m.width)
	if m.layout == LayoutList {
		m.cols = 1
		m.cardW = innerW
		return
	}
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
	if m.layout == LayoutList {
		rows := innerH - 1 // column header line
		if rows < 1 {
			rows = 1
		}
		return rows
	}
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
	case m.layout == LayoutList:
		content = m.renderList()
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

// Column indices into the cell slice built by runCells.
const (
	colIcon = iota
	colNum
	colName
	colBranch
	colSHA
	colEvent
	colActor
	colAge
	colDur
)

const listColGap = 1

// listCol describes one column of the list layout. Columns with flex > 0 share
// the width left over after the fixed columns; dropOrder marks a column as
// droppable when the pane is too narrow, lowest order dropped first (0 = never).
type listCol struct {
	title     string
	width     int // fixed width, ignored when flex > 0
	flex      int // relative share of the leftover width
	min       int // floor for flexible columns
	dropOrder int
}

// runsListCols must stay index-aligned with the col* constants.
var runsListCols = []listCol{
	colIcon:   {title: " ", width: 1},
	colNum:    {title: "#", width: 7, dropOrder: 7},
	colName:   {title: "Workflow", flex: 2, min: 8},
	colBranch: {title: "Branch", flex: 3, min: 8, dropOrder: 6},
	colSHA:    {title: "SHA", width: 7, dropOrder: 3},
	colEvent:  {title: "Event", width: 12, dropOrder: 2},
	colActor:  {title: "Actor", width: 14, dropOrder: 1},
	colAge:    {title: "Age", width: 8, dropOrder: 4},
	colDur:    {title: "Dur", width: 7, dropOrder: 5},
}

// runsListCellStyles colors individual cells of unselected list rows.
var runsListCellStyles = map[int]lipgloss.Style{
	colBranch: lipgloss.NewStyle().Foreground(colorTeal),
	colSHA:    lipgloss.NewStyle().Foreground(colorMuted),
	colEvent:  lipgloss.NewStyle().Foreground(colorMuted),
	colActor:  lipgloss.NewStyle().Foreground(colorMuted),
	colAge:    lipgloss.NewStyle().Foreground(colorMuted),
	colDur:    lipgloss.NewStyle().Foreground(colorPrimary),
}

// listLayout picks the columns that fit in width and returns their indices
// alongside the final width of each.
func listLayout(width int) (idx []int, widths []int) {
	visible := make([]bool, len(runsListCols))
	for i := range visible {
		visible[i] = true
	}

	base := func() int {
		total, n := 0, 0
		for i, c := range runsListCols {
			if !visible[i] {
				continue
			}
			n++
			if c.flex > 0 {
				total += c.min
			} else {
				total += c.width
			}
		}
		if n > 1 {
			total += listColGap * (n - 1)
		}
		return total
	}

	for base() > width {
		drop := -1
		for i, c := range runsListCols {
			if visible[i] && c.dropOrder > 0 &&
				(drop == -1 || c.dropOrder < runsListCols[drop].dropOrder) {
				drop = i
			}
		}
		if drop == -1 {
			break
		}
		visible[drop] = false
	}

	for i := range runsListCols {
		if visible[i] {
			idx = append(idx, i)
		}
	}
	widths = make([]int, len(idx))
	totalFlex := 0
	for n, i := range idx {
		c := runsListCols[i]
		if c.flex > 0 {
			widths[n] = c.min
			totalFlex += c.flex
		} else {
			widths[n] = c.width
		}
	}

	// Too narrow even for the columns that can never be dropped: shrink the
	// flexible ones below their floor rather than overflow the pane.
	for deficit := base() - width; deficit > 0; {
		shrunk := false
		for n, i := range idx {
			if runsListCols[i].flex == 0 || widths[n] <= 1 {
				continue
			}
			take := min(deficit, widths[n]-1)
			widths[n] -= take
			deficit -= take
			shrunk = true
			if deficit <= 0 {
				break
			}
		}
		if !shrunk {
			break
		}
	}

	// Hand the leftover width to the flexible columns, proportionally.
	if slack := width - base(); slack > 0 && totalFlex > 0 {
		given := 0
		last := -1
		for n, i := range idx {
			c := runsListCols[i]
			if c.flex == 0 {
				continue
			}
			share := slack * c.flex / totalFlex
			widths[n] += share
			given += share
			last = n
		}
		if last >= 0 {
			widths[last] += slack - given // rounding remainder
		}
	}

	// Pane too narrow even for the mandatory columns at their floor: drop from
	// the right until what is left fits, keeping the status icon last.
	for len(idx) > 1 && listTotal(widths) > width {
		idx = idx[:len(idx)-1]
		widths = widths[:len(widths)-1]
	}
	if len(widths) == 1 && widths[0] > width {
		widths[0] = max(width, 1)
	}

	return idx, widths
}

// listTotal is the rendered width of a set of columns, gaps included.
func listTotal(widths []int) int {
	total := 0
	for _, w := range widths {
		total += w
	}
	if len(widths) > 1 {
		total += listColGap * (len(widths) - 1)
	}
	return total
}

// runCells returns the plain-text cell values for a run, index-aligned with
// the col* constants. The status icon is rendered separately so it can be
// colored without breaking width math.
func runCells(r gh.WorkflowRun) []string {
	num := fmt.Sprintf("#%d", r.Number)
	if r.RunAttempt > 1 {
		num = fmt.Sprintf("#%d·%d", r.Number, r.RunAttempt)
	}
	sha := ""
	if len(r.HeadSHA) >= 7 {
		sha = r.HeadSHA[:7]
	}
	cells := make([]string, len(runsListCols))
	cells[colIcon] = StatusIconPlain(r.Status, r.Conclusion)
	cells[colNum] = num
	cells[colName] = r.Name
	cells[colBranch] = r.Branch
	cells[colSHA] = sha
	cells[colEvent] = r.Event
	cells[colActor] = r.Actor
	cells[colAge] = relativeTime(r.CreatedAt)
	cells[colDur] = formatDuration(r.Duration)
	return cells
}

// renderList lays the runs out as one row per run with a column header.
func (m RunsModel) renderList() string {
	innerW := paneInnerWidth(m.width)
	idx, widths := listLayout(innerW)
	gap := strings.Repeat(" ", listColGap)

	var head []string
	for n, i := range idx {
		head = append(head, padRight(truncate(runsListCols[i].title, widths[n]), widths[n]))
	}
	lines := []string{styleListHeader.Render(strings.Join(head, gap))}

	vis := m.visibleRows()
	for row := m.rowOffset; row < m.rowOffset+vis && row < len(m.runs); row++ {
		r := m.runs[row]
		cells := runCells(r)
		selected := row == m.cursor && m.focused

		var out []string
		for n, i := range idx {
			var cell string
			switch {
			case i == colIcon:
				// The icon is a single multi-byte rune; byte-slicing it in
				// truncate would corrupt it, so pad it directly.
				icon := cells[i]
				if !selected {
					icon = StatusIcon(r.Status, r.Conclusion)
				}
				cell = padRight(icon, widths[n])
			case selected:
				// Keep the row plain so the highlight background is unbroken.
				cell = padRight(truncate(cells[i], widths[n]), widths[n])
			default:
				cell = padRight(truncate(cells[i], widths[n]), widths[n])
				if st, ok := runsListCellStyles[i]; ok {
					cell = st.Render(cell)
				} else {
					cell = styleListRow.Render(cell)
				}
			}
			out = append(out, cell)
		}
		line := strings.Join(out, gap)
		if selected {
			line = styleListRowSelected.Width(innerW).Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// padRight pads s with spaces to exactly w display cells.
func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
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
