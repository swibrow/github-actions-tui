package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

type workflowItem struct {
	id   int64
	name string
}

func (i workflowItem) FilterValue() string { return i.name }

type workflowDelegate struct{}

func (d workflowDelegate) Height() int                             { return 1 }
func (d workflowDelegate) Spacing() int                            { return 0 }
func (d workflowDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d workflowDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(workflowItem)
	if !ok {
		return
	}
	cursor := "  "
	style := lipgloss.NewStyle()
	if index == m.Index() {
		cursor = "> "
		style = style.Bold(true).Foreground(colorPrimary)
	}
	fmt.Fprint(w, style.Render(cursor+item.name))
}

type SidebarModel struct {
	list    list.Model
	focused bool
	width   int
	height  int
}

func NewSidebarModel() SidebarModel {
	l := list.New([]list.Item{}, workflowDelegate{}, 0, 0)
	l.Title = "Workflows"
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.Styles.Title = styleTitle
	l.DisableQuitKeybindings()

	return SidebarModel{list: l}
}

func (m *SidebarModel) SetWorkflows(workflows []gh.Workflow) {
	items := make([]list.Item, 0, len(workflows)+1)
	items = append(items, workflowItem{id: 0, name: "All Workflows"})
	for _, w := range workflows {
		items = append(items, workflowItem{id: w.ID, name: w.Name})
	}
	m.list.SetItems(items)
}

func (m SidebarModel) SelectedWorkflow() (int64, string) {
	item, ok := m.list.SelectedItem().(workflowItem)
	if !ok {
		return 0, ""
	}
	return item.id, item.name
}

func (m *SidebarModel) SetFocused(focused bool) {
	m.focused = focused
}

func (m *SidebarModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.list.SetSize(width-2, height-2) // account for border+padding
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m SidebarModel) View() string {
	style := styleSidebarBlurred
	if m.focused {
		style = styleSidebarFocused
	}

	content := m.list.View()
	// Pad content to fill the panel
	lines := strings.Split(content, "\n")
	innerH := m.height - 2 // border
	if innerH < 1 {
		innerH = 1
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	content = strings.Join(lines[:innerH], "\n")

	return style.Width(m.width - 2).Height(m.height - 2).Render(content)
}
