package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	gh "github.com/swibrow/github-actions-tui/internal/github"
)

// Messages emitted by the repo picker.
type RepoSelectedMsg struct {
	Owner string
	Repo  string
}

type RepoCancelledMsg struct{}

type RepoSearchTriggerMsg struct {
	Query string
}

// RepoPickerModel is a full-screen overlay for switching repos.
type RepoPickerModel struct {
	input     textinput.Model
	repos     []gh.Repository // all loaded repos
	matches   []fuzzy.Match   // current fuzzy matches
	cursor    int
	offset    int
	visible   bool
	loading   bool
	searching bool // API search in flight
	width     int
	height    int
	lastQuery string
}

// repoSource adapts []gh.Repository to the fuzzy.Source interface.
type repoSource []gh.Repository

func (s repoSource) String(i int) string {
	return s[i].FullName
}

func (s repoSource) Len() int {
	return len(s)
}

func NewRepoPickerModel() RepoPickerModel {
	ti := textinput.New()
	ti.Placeholder = "Search repos..."
	ti.CharLimit = 100
	ti.SetWidth(40)
	return RepoPickerModel{input: ti}
}

func (m *RepoPickerModel) Show() {
	m.visible = true
	m.loading = true
	m.searching = false
	m.cursor = 0
	m.offset = 0
	m.repos = nil
	m.matches = nil
	m.lastQuery = ""
	m.input.SetValue("")
	m.input.Focus()
}

func (m *RepoPickerModel) Hide() {
	m.visible = false
	m.input.Blur()
}

func (m RepoPickerModel) Visible() bool {
	return m.visible
}

func (m *RepoPickerModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	inputW := w - 12
	if inputW < 20 {
		inputW = 20
	}
	if inputW > 60 {
		inputW = 60
	}
	m.input.SetWidth(inputW)
}

// AddRepos merges new repos into the list, deduplicating by FullName.
func (m *RepoPickerModel) AddRepos(repos []gh.Repository) {
	seen := make(map[string]bool, len(m.repos))
	for _, r := range m.repos {
		seen[r.FullName] = true
	}
	for _, r := range repos {
		if !seen[r.FullName] {
			m.repos = append(m.repos, r)
			seen[r.FullName] = true
		}
	}
	m.loading = false
	m.filterRepos()
}

// SetSearchResults adds search results, deduplicating.
func (m *RepoPickerModel) SetSearchResults(repos []gh.Repository) {
	m.searching = false
	m.AddRepos(repos)
}

func (m *RepoPickerModel) filterRepos() {
	query := m.input.Value()
	if query == "" {
		// Show all repos as matches (no highlighting)
		m.matches = make([]fuzzy.Match, len(m.repos))
		for i := range m.repos {
			m.matches[i] = fuzzy.Match{
				Str:   m.repos[i].FullName,
				Index: i,
			}
		}
	} else {
		m.matches = fuzzy.FindFrom(query, repoSource(m.repos))
	}
	m.cursor = 0
	m.offset = 0
}

func (m RepoPickerModel) InputValue() string {
	return m.input.Value()
}

func (m RepoPickerModel) selectedRepo() *gh.Repository {
	if len(m.matches) == 0 || m.cursor >= len(m.matches) {
		return nil
	}
	idx := m.matches[m.cursor].Index
	if idx >= len(m.repos) {
		return nil
	}
	return &m.repos[idx]
}

func (m RepoPickerModel) Update(msg tea.Msg) (RepoPickerModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			r := m.selectedRepo()
			if r != nil {
				m.Hide()
				return m, func() tea.Msg {
					return RepoSelectedMsg{Owner: r.Owner, Repo: r.Name}
				}
			}
			return m, nil
		case "esc":
			m.Hide()
			return m, func() tea.Msg { return RepoCancelledMsg{} }
		case "up", "ctrl+k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
			return m, nil
		case "down", "ctrl+j":
			if m.cursor < len(m.matches)-1 {
				m.cursor++
				maxVisible := m.maxVisible()
				if m.cursor >= m.offset+maxVisible {
					m.offset = m.cursor - maxVisible + 1
				}
			}
			return m, nil
		}

		// Pass to text input
		prevValue := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)

		if m.input.Value() != prevValue {
			m.filterRepos()
			// Check if we should trigger API search
			query := m.input.Value()
			if len(query) >= 3 && len(m.matches) < 5 && query != m.lastQuery {
				m.lastQuery = query
				return m, tea.Batch(cmd, m.searchTriggerCmd(query))
			}
		}
		return m, cmd
	}

	return m, nil
}

func (m RepoPickerModel) searchTriggerCmd(query string) tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(_ time.Time) tea.Msg {
		return RepoSearchTriggerMsg{Query: query}
	})
}

func (m RepoPickerModel) maxVisible() int {
	// Overlay height minus borders, padding, title, input, status line
	v := m.height - 12
	if v < 3 {
		v = 3
	}
	if v > 20 {
		v = 20
	}
	return v
}

func (m RepoPickerModel) View(width, height int) string {
	overlayW := width - 10
	if overlayW < 40 {
		overlayW = 40
	}
	if overlayW > 80 {
		overlayW = 80
	}

	// Content width inside the overlay box (minus border + padding)
	contentW := overlayW - 6

	var lines []string
	lines = append(lines, styleTitle.Render("Switch Repository"))
	lines = append(lines, "")
	lines = append(lines, m.input.View())
	lines = append(lines, "")

	if m.loading && len(m.repos) == 0 {
		lines = append(lines, styleLoading.Render("Loading repos..."))
	} else if len(m.matches) == 0 {
		lines = append(lines, styleLoading.Render("No matches"))
	} else {
		maxVis := m.maxVisible()
		end := m.offset + maxVis
		if end > len(m.matches) {
			end = len(m.matches)
		}

		for i := m.offset; i < end; i++ {
			match := m.matches[i]
			repo := m.repos[match.Index]
			selected := i == m.cursor

			name := repo.FullName
			tag := ""
			if repo.Private {
				tag = " [private]"
			}

			// Truncate name to fit (leave room for cursor prefix + tag)
			maxName := contentW - 4 - len(tag)
			if maxName < 10 {
				maxName = 10
			}
			if len(name) > maxName {
				name = name[:maxName-3] + "..."
			}

			if selected {
				// Selected line: plain text with selected style, no nested ANSI
				padded := fmt.Sprintf(" > %-*s", contentW-4, name+tag)
				lines = append(lines, stylePickerSelected.Render(padded))
			} else {
				// Unselected line: fuzzy highlight on the name, tag appended separately
				highlighted := renderFuzzyHighlight(name, match.MatchedIndexes)
				if tag != "" {
					highlighted += stylePickerPrivate.Render(tag)
				}
				lines = append(lines, "   "+highlighted)
			}

			// Description for selected item
			if selected && repo.Description != "" {
				desc := repo.Description
				maxDesc := contentW - 6
				if maxDesc > 0 && len(desc) > maxDesc {
					desc = desc[:maxDesc-3] + "..."
				}
				lines = append(lines, "   "+stylePickerDesc.Render(desc))
			}
		}

		lines = append(lines, "")
		countStr := fmt.Sprintf("%d/%d repos", len(m.matches), len(m.repos))
		if m.searching {
			countStr += " (searching...)"
		}
		lines = append(lines, styleLoading.Render(countStr))
	}

	lines = append(lines, "")
	lines = append(lines, styleHelpBar.Render("up/down:navigate  enter:select  esc:cancel"))

	content := strings.Join(lines, "\n")
	box := stylePickerOverlay.Width(overlayW).Render(content)
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		box)
}

// renderFuzzyHighlight renders a string with fuzzy-matched character indices
// highlighted. Each character is styled independently — no nesting.
func renderFuzzyHighlight(s string, matchedIndexes []int) string {
	if len(matchedIndexes) == 0 {
		return s
	}

	matchSet := make(map[int]bool, len(matchedIndexes))
	for _, idx := range matchedIndexes {
		matchSet[idx] = true
	}

	var b strings.Builder
	for i, ch := range s {
		if matchSet[i] {
			b.WriteString(stylePickerMatch.Render(string(ch)))
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}
