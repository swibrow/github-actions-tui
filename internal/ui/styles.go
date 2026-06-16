package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme — a cohesive "Tokyo Night"-inspired palette. True-color hex values
// degrade gracefully on 256-color terminals via lipgloss.
var (
	colorBg        = lipgloss.Color("#1a1b26") // base background
	colorText      = lipgloss.Color("#c0caf5") // primary foreground
	colorPrimary   = lipgloss.Color("#7aa2f7") // blue — focus / accents
	colorAccent    = lipgloss.Color("#bb9af7") // purple — titles
	colorTeal      = lipgloss.Color("#2ac3de") // teal — secondary accent
	colorSuccess   = lipgloss.Color("#9ece6a") // green
	colorFailure   = lipgloss.Color("#f7768e") // red
	colorRunning   = lipgloss.Color("#7dcfff") // cyan — in progress
	colorQueued    = lipgloss.Color("#e0af68") // amber — queued
	colorCancelled = lipgloss.Color("#565f89") // slate — cancelled/skipped
	colorMuted     = lipgloss.Color("#787c99") // muted text
	colorBorder    = lipgloss.Color("#3d59a1") // blurred border / dividers
	colorFocused   = lipgloss.Color("#7dcfff") // focused border (bright cyan)
	colorSelBg     = lipgloss.Color("#2d3f76") // selection background
)

var (
	styleMainFocused = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(colorFocused).
				Padding(0, 1)

	styleMainBlurred = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder).
				Padding(0, 1)

	styleFilterBar = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorTeal).
			Padding(0, 1)

	styleHelpBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	styleError = lipgloss.NewStyle().
			Foreground(colorBg).
			Background(colorFailure).
			Bold(true).
			Padding(0, 1)

	styleLoading = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	styleConfirmDialog = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(1, 3).
				Bold(true)

	styleTreeNode = lipgloss.NewStyle().
			Foreground(colorText)

	styleTreeNodeSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#ffffff")).
				Background(colorSelBg)

	styleGraphTier = lipgloss.NewStyle().
			Foreground(colorTeal).
			Bold(true)

	styleGraphNode = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(colorText)

	styleGraphNodeSelected = lipgloss.NewStyle().
				PaddingLeft(2).
				Bold(true).
				Foreground(lipgloss.Color("#ffffff")).
				Background(colorSelBg)

	styleLogGroup = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	stylePickerOverlay = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(1, 2)

	stylePickerSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#ffffff")).
				Background(colorSelBg)

	stylePickerMatch = lipgloss.NewStyle().
				Foreground(colorTeal).
				Bold(true).
				Underline(true)

	stylePickerDesc = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	stylePickerPrivate = lipgloss.NewStyle().
				Foreground(colorQueued)

	styleRepoIndicator = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorBg).
				Background(colorPrimary).
				Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorBg).
			Background(colorSuccess).
			Bold(true).
			Padding(0, 1)

	// Help-bar key/label styles for colorful, legible key hints.
	styleHelpKey = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	styleHelpLabel = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleHelpSep = lipgloss.NewStyle().
			Foreground(colorBorder)
)

// paneInnerWidth is the usable content width inside a pane (border + padding).
func paneInnerWidth(width int) int {
	w := width - 4 // left/right border(2) + horizontal padding(2)
	if w < 1 {
		w = 1
	}
	return w
}

// paneInnerHeight is the usable content height inside a pane (top/bottom border).
func paneInnerHeight(height int) int {
	h := height - 2 // top/bottom border; vertical padding is 0
	if h < 1 {
		h = 1
	}
	return h
}

// paneFrame draws content inside a bordered pane and splices a bold, colored
// title into the top border line — a "window title" look. width/height are the
// pane's outer dimensions; content must already be sized to fit the interior.
func paneFrame(content string, width, height int, focused bool, icon, title string) string {
	style := styleMainBlurred
	if focused {
		style = styleMainFocused
	}
	box := style.Width(width).Height(height).Render(content)
	return spliceBorderTitle(box, width, focused, icon, title)
}

// spliceBorderTitle rebuilds the top border line of a rendered box, embedding a
// styled label like  ┏━ ⚡ Workflow Runs ━━━━━┓.
func spliceBorderTitle(box string, width int, focused bool, icon, title string) string {
	if width < 8 {
		return box
	}
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}

	lc, rc, hor := "╭", "╮", "─"
	bcol := colorBorder
	lfg := colorTeal
	if focused {
		lc, rc, hor = "┏", "┓", "━"
		bcol = colorFocused
		lfg = lipgloss.Color("#ffffff")
	}
	borderStyle := lipgloss.NewStyle().Foreground(bcol)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lfg)

	label := strings.TrimSpace(icon + " " + title)
	plain := " " + label + " "
	maxLabel := width - 5 // corners(2) + lead(1) + trailing fill(>=1) + breathing room
	if w := lipgloss.Width(plain); w > maxLabel {
		plain = truncate(plain, maxLabel) + " "
	}

	lead := 1
	fill := width - 3 - lipgloss.Width(plain) // corners(2)+lead(1)
	if fill < 0 {
		fill = 0
	}
	lines[0] = borderStyle.Render(lc+strings.Repeat(hor, lead)) +
		labelStyle.Render(plain) +
		borderStyle.Render(strings.Repeat(hor, fill)+rc)
	return strings.Join(lines, "\n")
}

// helpHint renders a "key label" pair for the footer help bar.
func helpHint(k, label string) string {
	return styleHelpKey.Render(k) + " " + styleHelpLabel.Render(label)
}

// joinHints joins help hints with a dim separator.
func joinHints(hints ...string) string {
	sep := styleHelpSep.Render(" · ")
	return strings.Join(hints, sep)
}

// StatusIcon returns a styled (ANSI-colored) status icon.
// Use StatusIconPlain for contexts where ANSI codes break width measurement (e.g. bubbles table cells).
func StatusIcon(status, conclusion string) string {
	if status == "completed" {
		switch conclusion {
		case "success":
			return lipgloss.NewStyle().Foreground(colorSuccess).Render("✓")
		case "failure":
			return lipgloss.NewStyle().Foreground(colorFailure).Render("✗")
		case "cancelled":
			return lipgloss.NewStyle().Foreground(colorCancelled).Render("⊘")
		case "skipped":
			return lipgloss.NewStyle().Foreground(colorCancelled).Render("⊘")
		default:
			return lipgloss.NewStyle().Foreground(colorCancelled).Render("⊘")
		}
	}
	switch status {
	case "in_progress":
		return lipgloss.NewStyle().Foreground(colorRunning).Render("●")
	case "queued", "waiting", "pending":
		return lipgloss.NewStyle().Foreground(colorQueued).Render("◌")
	default:
		return lipgloss.NewStyle().Foreground(colorMuted).Render("·")
	}
}

// StatusIconPlain returns a plain (unstyled) status icon character.
// Safe for use in bubbles table cells where runewidth.Truncate is not ANSI-aware.
func StatusIconPlain(status, conclusion string) string {
	if status == "completed" {
		switch conclusion {
		case "success":
			return "✓"
		case "failure":
			return "✗"
		case "cancelled", "skipped":
			return "⊘"
		default:
			return "⊘"
		}
	}
	switch status {
	case "in_progress":
		return "●"
	case "queued", "waiting", "pending":
		return "◌"
	default:
		return "·"
	}
}
