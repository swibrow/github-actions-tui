package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

// TriggerSubmitMsg is sent when the user submits the trigger form.
type TriggerSubmitMsg struct {
	WorkflowID int64
	Ref        string
	Inputs     map[string]interface{}
}

// TriggerCancelledMsg is sent when the user cancels the trigger dialog.
type TriggerCancelledMsg struct{}

// WorkflowInputsMsg carries fetched workflow inputs for the trigger dialog.
type WorkflowInputsMsg struct {
	WorkflowID int64
	Inputs     []gh.WorkflowInput
	Err        error
}

// TriggerModel is the overlay dialog for triggering a workflow_dispatch event.
type TriggerModel struct {
	visible    bool
	loading    bool
	workflowID int64
	wfName     string
	inputs     []gh.WorkflowInput
	refInput   textinput.Model
	fieldInputs []textinput.Model
	active     int // 0 = ref, 1..N = workflow inputs
	width      int
	height     int
}

func NewTriggerModel() TriggerModel {
	ref := textinput.New()
	ref.Placeholder = "main"
	ref.CharLimit = 100
	ref.SetWidth(40)
	return TriggerModel{refInput: ref}
}

func (m *TriggerModel) Show(workflowID int64, wfName string) {
	m.visible = true
	m.loading = true
	m.workflowID = workflowID
	m.wfName = wfName
	m.inputs = nil
	m.fieldInputs = nil
	m.active = 0
	m.refInput.SetValue("")
	m.refInput.Focus()
}

func (m *TriggerModel) SetInputs(inputs []gh.WorkflowInput) {
	m.loading = false
	m.inputs = inputs
	m.fieldInputs = make([]textinput.Model, len(inputs))
	for i, inp := range inputs {
		ti := textinput.New()
		if inp.Default != "" {
			ti.Placeholder = inp.Default
		}
		if inp.Type == "boolean" {
			ti.Placeholder = "true/false"
		}
		if len(inp.Options) > 0 {
			ti.Placeholder = strings.Join(inp.Options, "|")
		}
		ti.CharLimit = 200
		ti.SetWidth(40)
		m.fieldInputs[i] = ti
	}
}

func (m *TriggerModel) Hide() {
	m.visible = false
	m.refInput.Blur()
	for i := range m.fieldInputs {
		m.fieldInputs[i].Blur()
	}
}

func (m TriggerModel) Visible() bool {
	return m.visible
}

func (m *TriggerModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m TriggerModel) totalFields() int {
	return 1 + len(m.fieldInputs) // ref + workflow inputs
}

func (m TriggerModel) Update(msg tea.Msg) (TriggerModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.loading {
				return m, nil
			}
			ref := m.refInput.Value()
			if ref == "" {
				ref = m.refInput.Placeholder
			}
			inputs := make(map[string]interface{})
			for i, inp := range m.inputs {
				val := m.fieldInputs[i].Value()
				if val == "" {
					val = inp.Default
				}
				if val != "" {
					inputs[inp.Name] = val
				}
			}
			m.Hide()
			return m, func() tea.Msg {
				return TriggerSubmitMsg{
					WorkflowID: m.workflowID,
					Ref:        ref,
					Inputs:     inputs,
				}
			}

		case "esc":
			m.Hide()
			return m, func() tea.Msg { return TriggerCancelledMsg{} }

		case "tab", "shift+tab":
			total := m.totalFields()
			if total <= 1 && m.loading {
				return m, nil
			}
			// Blur current
			if m.active == 0 {
				m.refInput.Blur()
			} else {
				m.fieldInputs[m.active-1].Blur()
			}
			// Move
			if msg.String() == "tab" {
				m.active = (m.active + 1) % total
			} else {
				m.active = (m.active - 1 + total) % total
			}
			// Focus new
			if m.active == 0 {
				m.refInput.Focus()
			} else {
				m.fieldInputs[m.active-1].Focus()
			}
			return m, nil
		}
	}

	// Update active input
	var cmd tea.Cmd
	if m.active == 0 {
		m.refInput, cmd = m.refInput.Update(msg)
	} else if m.active-1 < len(m.fieldInputs) {
		m.fieldInputs[m.active-1], cmd = m.fieldInputs[m.active-1].Update(msg)
	}
	return m, cmd
}

func (m TriggerModel) View(width, height int) string {
	title := styleTitle.Render(fmt.Sprintf("Trigger: %s", m.wfName))

	var content strings.Builder
	content.WriteString(title + "\n\n")

	if m.loading {
		content.WriteString(styleLoading.Render("Loading inputs..."))
	} else {
		labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
		requiredStyle := lipgloss.NewStyle().Foreground(colorFailure)

		content.WriteString(labelStyle.Render("ref: ") + m.refInput.View() + "\n")

		for i, inp := range m.inputs {
			label := inp.Name
			if inp.Required {
				label += requiredStyle.Render("*")
			}
			content.WriteString("\n" + labelStyle.Render(label+": ") + m.fieldInputs[i].View())
			if inp.Description != "" {
				content.WriteString("\n  " + lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render(inp.Description))
			}
		}

		content.WriteString("\n\n" + labelStyle.Render("tab: next field  enter: trigger  esc: cancel"))
	}

	overlay := stylePickerOverlay.
		Width(clamp(width-10, 40, 60)).
		Render(content.String())

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		overlay)
}
