package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.Color("39")  // blue
	colorSuccess   = lipgloss.Color("34")  // green
	colorFailure   = lipgloss.Color("196") // red
	colorRunning   = lipgloss.Color("39")  // blue
	colorQueued    = lipgloss.Color("226") // yellow
	colorCancelled = lipgloss.Color("245") // gray
	colorMuted     = lipgloss.Color("245")
	colorBorder    = lipgloss.Color("240")
	colorFocused   = lipgloss.Color("39")

	styleSidebarFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFocused).
				Padding(0, 1)

	styleSidebarBlurred = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder).
				Padding(0, 1)

	styleMainFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFocused).
				Padding(0, 1)

	styleMainBlurred = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder).
				Padding(0, 1)

	styleFilterBar = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	styleHelpBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(colorFailure).
			Padding(0, 1)

	styleLoading = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	styleConfirmDialog = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(1, 3).
				Bold(true)

	styleTreeNode = lipgloss.NewStyle()

	styleTreeNodeSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary)

	styleGraphTier = lipgloss.NewStyle().
			Foreground(colorMuted).
			Bold(true)

	styleGraphNode = lipgloss.NewStyle().
			PaddingLeft(2)

	styleGraphNodeSelected = lipgloss.NewStyle().
				PaddingLeft(2).
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(colorPrimary)

	styleLogGroup = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	stylePickerOverlay = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(1, 2)

	stylePickerSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(colorPrimary)

	stylePickerMatch = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true).
				Underline(true)

	stylePickerDesc = lipgloss.NewStyle().
				Foreground(colorMuted).
				Italic(true)

	stylePickerPrivate = lipgloss.NewStyle().
				Foreground(colorQueued)

	styleRepoIndicator = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary)
)

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
