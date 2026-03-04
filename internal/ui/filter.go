package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

const (
	filterBranch = iota
	filterActor
	filterStatus
	filterEvent
	filterCount
)

type FilterAppliedMsg struct {
	Filter gh.RunFilter
}

type FilterCancelledMsg struct{}

type FilterModel struct {
	inputs  []textinput.Model
	active  int
	visible bool
	width   int
}

func NewFilterModel() FilterModel {
	inputs := make([]textinput.Model, filterCount)

	branch := textinput.New()
	branch.Placeholder = "branch"
	branch.CharLimit = 50
	branch.SetWidth(15)
	inputs[filterBranch] = branch

	actor := textinput.New()
	actor.Placeholder = "author"
	actor.CharLimit = 50
	actor.SetWidth(15)
	inputs[filterActor] = actor

	status := textinput.New()
	status.Placeholder = "status"
	status.CharLimit = 20
	status.SetWidth(15)
	inputs[filterStatus] = status

	event := textinput.New()
	event.Placeholder = "event"
	event.CharLimit = 20
	event.SetWidth(15)
	inputs[filterEvent] = event

	return FilterModel{inputs: inputs}
}

func (m *FilterModel) Show() {
	m.visible = true
	m.active = 0
	m.inputs[m.active].Focus()
}

func (m *FilterModel) Hide() {
	m.visible = false
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

func (m *FilterModel) Clear() {
	for i := range m.inputs {
		m.inputs[i].SetValue("")
	}
}

func (m FilterModel) Visible() bool {
	return m.visible
}

func (m *FilterModel) SetSize(width int) {
	m.width = width
	inputW := (width - 20) / filterCount
	if inputW < 10 {
		inputW = 10
	}
	for i := range m.inputs {
		m.inputs[i].SetWidth(inputW)
	}
}

func (m FilterModel) CurrentFilter(workflowID int64) gh.RunFilter {
	return gh.RunFilter{
		WorkflowID: workflowID,
		Branch:     m.inputs[filterBranch].Value(),
		Actor:      m.inputs[filterActor].Value(),
		Status:     m.inputs[filterStatus].Value(),
		Event:      m.inputs[filterEvent].Value(),
	}
}

func (m FilterModel) HasActiveFilter() bool {
	for _, input := range m.inputs {
		if input.Value() != "" {
			return true
		}
	}
	return false
}

func (m FilterModel) Update(msg tea.Msg) (FilterModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			filter := gh.RunFilter{
				Branch: m.inputs[filterBranch].Value(),
				Actor:  m.inputs[filterActor].Value(),
				Status: m.inputs[filterStatus].Value(),
				Event:  m.inputs[filterEvent].Value(),
			}
			m.Hide()
			return m, func() tea.Msg { return FilterAppliedMsg{Filter: filter} }
		case "esc":
			m.Hide()
			return m, func() tea.Msg { return FilterCancelledMsg{} }
		case "tab", "shift+tab":
			m.inputs[m.active].Blur()
			if msg.String() == "tab" {
				m.active = (m.active + 1) % filterCount
			} else {
				m.active = (m.active - 1 + filterCount) % filterCount
			}
			m.inputs[m.active].Focus()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.inputs[m.active], cmd = m.inputs[m.active].Update(msg)
	return m, cmd
}

func (m FilterModel) View() string {
	if !m.visible && !m.HasActiveFilter() {
		return ""
	}

	labelStyle := lipgloss.NewStyle().Foreground(colorMuted)

	parts := []string{}
	labels := []string{"branch:", "author:", "status:", "event:"}
	for i, input := range m.inputs {
		if m.visible {
			parts = append(parts, labelStyle.Render(labels[i])+" "+input.View())
		} else if input.Value() != "" {
			parts = append(parts, labelStyle.Render(labels[i])+" "+input.Value())
		}
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, joinWithSpaces(parts)...)
	return styleFilterBar.Width(m.width).Render(content)
}

func joinWithSpaces(parts []string) []string {
	if len(parts) == 0 {
		return nil
	}
	result := make([]string, 0, len(parts)*2-1)
	for i, p := range parts {
		result = append(result, p)
		if i < len(parts)-1 {
			result = append(result, "  ")
		}
	}
	return result
}
