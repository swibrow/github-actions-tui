package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type LogsModel struct {
	viewport viewport.Model
	jobName  string
	loading  bool
	width    int
	height   int
	ready    bool
}

func NewLogsModel() LogsModel {
	return LogsModel{}
}

func (m *LogsModel) SetContent(content, jobName string) {
	m.jobName = jobName
	m.loading = false
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m *LogsModel) SetLoading(loading bool) {
	m.loading = loading
}

func (m *LogsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	headerH := 2
	footerH := 1
	vpH := height - headerH - footerH
	if vpH < 1 {
		vpH = 1
	}
	if !m.ready {
		m.viewport = viewport.New(width, vpH)
		m.viewport.MouseWheelEnabled = true
		m.ready = true
	} else {
		m.viewport.Width = width
		m.viewport.Height = vpH
	}
}

func (m LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m LogsModel) View() string {
	if m.loading {
		return styleLoading.Render("Loading logs...")
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).
		Render(fmt.Sprintf("Logs: %s", m.jobName))
	separator := lipgloss.NewStyle().Foreground(colorBorder).
		Render(strings.Repeat("─", m.width))

	pct := m.viewport.ScrollPercent() * 100
	footer := lipgloss.NewStyle().Foreground(colorMuted).
		Render(fmt.Sprintf("%.0f%% │ esc: back │ j/k: scroll", pct))

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		separator,
		m.viewport.View(),
		footer,
	)
}
