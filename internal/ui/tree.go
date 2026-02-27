package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

// TreeNodeKind identifies the type of tree node.
type TreeNodeKind int

const (
	NodeWorkflow TreeNodeKind = iota
	NodeRun
	NodeJob
)

// TreeNode represents a single node in the tree sidebar.
type TreeNode struct {
	Kind     TreeNodeKind
	Label    string
	Workflow *gh.Workflow
	Run      *gh.WorkflowRun
	Job      *gh.WorkflowJob
	Expanded bool
	Children []*TreeNode
	Depth    int
}

// TreeModel is the hierarchical tree sidebar replacing the flat workflow list.
type TreeModel struct {
	roots   []*TreeNode
	flat    []*TreeNode // flattened visible nodes
	cursor  int
	offset  int
	focused bool
	width   int
	height  int
	loading map[int64]bool // workflow IDs currently loading
}

// RunsForTreeMsg is sent when runs are fetched for a specific workflow in the tree.
type RunsForTreeMsg struct {
	WorkflowID int64
	Runs       []gh.WorkflowRun
	Err        error
}

func NewTreeModel() TreeModel {
	return TreeModel{
		loading: make(map[int64]bool),
	}
}

func (m *TreeModel) SetWorkflows(workflows []gh.Workflow) {
	m.roots = make([]*TreeNode, 0, len(workflows)+1)
	// "All Workflows" pseudo-node at top
	m.roots = append(m.roots, &TreeNode{
		Kind:  NodeWorkflow,
		Label: "All Workflows",
		Depth: 0,
	})
	for i := range workflows {
		w := workflows[i]
		m.roots = append(m.roots, &TreeNode{
			Kind:     NodeWorkflow,
			Label:    w.Name,
			Workflow: &w,
			Depth:    0,
		})
	}
	m.flatten()
}

// SetRunsForWorkflow populates runs under a workflow node.
func (m *TreeModel) SetRunsForWorkflow(workflowID int64, runs []gh.WorkflowRun) {
	delete(m.loading, workflowID)
	for _, root := range m.roots {
		if root.Workflow != nil && root.Workflow.ID == workflowID {
			root.Children = make([]*TreeNode, 0, len(runs))
			for i := range runs {
				r := runs[i]
				icon := StatusIconPlain(r.Status, r.Conclusion)
				label := fmt.Sprintf("%s #%d %s", icon, r.Number, r.Branch)
				if r.RunAttempt > 1 {
					label = fmt.Sprintf("%s #%d·%d %s", icon, r.Number, r.RunAttempt, r.Branch)
				}
				root.Children = append(root.Children, &TreeNode{
					Kind:  NodeRun,
					Label: label,
					Run:   &r,
					Depth: 1,
				})
			}
			break
		}
	}
	m.flatten()
}

func (m *TreeModel) flatten() {
	m.flat = nil
	for _, root := range m.roots {
		m.flat = append(m.flat, root)
		if root.Expanded {
			for _, child := range root.Children {
				m.flat = append(m.flat, child)
				if child.Expanded {
					m.flat = append(m.flat, child.Children...)
				}
			}
		}
	}
	// Clamp cursor
	if m.cursor >= len(m.flat) {
		m.cursor = len(m.flat) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// SelectedNode returns the currently selected tree node.
func (m TreeModel) SelectedNode() *TreeNode {
	if m.cursor < 0 || m.cursor >= len(m.flat) {
		return nil
	}
	return m.flat[m.cursor]
}

// SelectedWorkflow returns the workflow ID and name based on current selection.
// If a run or job is selected, returns the parent workflow info.
func (m TreeModel) SelectedWorkflow() (int64, string) {
	node := m.SelectedNode()
	if node == nil {
		return 0, ""
	}
	switch node.Kind {
	case NodeWorkflow:
		if node.Workflow != nil {
			return node.Workflow.ID, node.Workflow.Name
		}
	case NodeRun:
		if node.Run != nil {
			// Find parent workflow
			for _, root := range m.roots {
				if root.Workflow != nil && root.Workflow.ID == node.Run.WorkflowID {
					return root.Workflow.ID, root.Workflow.Name
				}
			}
		}
	}
	return 0, ""
}

func (m *TreeModel) SetFocused(focused bool) {
	m.focused = focused
}

func (m *TreeModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *TreeModel) scrollToVisible() {
	innerH := m.height - 4 // border + title
	if innerH < 1 {
		innerH = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+innerH {
		m.offset = m.cursor - innerH + 1
	}
}

func (m TreeModel) Update(msg tea.Msg) (TreeModel, tea.Cmd) {
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

// ToggleExpand toggles the expand/collapse state of the current node.
// Returns true if the node was expanded (needs data fetch), along with the node.
func (m *TreeModel) ToggleExpand() (expanded bool, node *TreeNode) {
	n := m.SelectedNode()
	if n == nil {
		return false, nil
	}
	if n.Kind == NodeWorkflow {
		n.Expanded = !n.Expanded
		m.flatten()
		return n.Expanded, n
	}
	if n.Kind == NodeRun {
		n.Expanded = !n.Expanded
		m.flatten()
		return n.Expanded, n
	}
	return false, n
}

// CollapseParent collapses the parent of the current node and moves the cursor to it.
func (m *TreeModel) CollapseParent() {
	n := m.SelectedNode()
	if n == nil {
		return
	}
	// Find parent node in roots
	for i, root := range m.roots {
		if n.Kind == NodeRun && root.Expanded {
			for _, child := range root.Children {
				if child == n {
					root.Expanded = false
					m.flatten()
					// Move cursor to the parent workflow
					for fi, fn := range m.flat {
						if fn == root {
							m.cursor = fi
							break
						}
					}
					_ = i
					return
				}
			}
		}
	}
}

func (m *TreeModel) IsLoading(workflowID int64) bool {
	return m.loading[workflowID]
}

func (m *TreeModel) SetLoading(workflowID int64) {
	m.loading[workflowID] = true
}

func (m TreeModel) View() string {
	style := styleSidebarBlurred
	if m.focused {
		style = styleSidebarFocused
	}

	title := styleTitle.Render("Workflows") + "\n"

	innerH := m.height - 4 // border + title
	if innerH < 1 {
		innerH = 1
	}

	var lines []string
	for i, node := range m.flat {
		if i < m.offset {
			continue
		}
		if len(lines) >= innerH {
			break
		}

		indent := strings.Repeat("  ", node.Depth)
		prefix := ""
		switch node.Kind {
		case NodeWorkflow:
			if node.Workflow == nil {
				// "All Workflows" node — no expand arrow
				prefix = "  "
			} else if node.Expanded {
				prefix = "▼ "
			} else {
				prefix = "▶ "
			}
		case NodeRun:
			if node.Expanded {
				prefix = "▼ "
			} else {
				prefix = "  "
			}
		default:
			prefix = "  "
		}

		text := indent + prefix + node.Label
		maxW := m.width - 4 // border + padding
		if maxW > 0 {
			text = truncate(text, maxW)
		}

		if i == m.cursor && m.focused {
			lines = append(lines, styleTreeNodeSelected.Render(text))
		} else {
			lines = append(lines, styleTreeNode.Render(text))
		}
	}

	// Pad to fill
	for len(lines) < innerH {
		lines = append(lines, "")
	}

	content := title + strings.Join(lines, "\n")

	return style.Width(m.width - 2).Height(m.height - 2).Render(content)
}

