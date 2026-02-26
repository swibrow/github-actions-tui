package ui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up            key.Binding
	Down          key.Binding
	Left          key.Binding
	Right         key.Binding
	SwitchPane    key.Binding
	Top           key.Binding
	Bottom        key.Binding
	Enter         key.Binding
	Back          key.Binding
	Filter        key.Binding
	Refresh       key.Binding
	Help          key.Binding
	Quit          key.Binding
	ToggleSidebar key.Binding
	SwitchRepo    key.Binding
	OpenBrowser   key.Binding
	PrevAttempt   key.Binding
	NextAttempt   key.Binding
}

var Keys = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("↑/k", "up/down"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("↓/j", ""),
	),
	Left: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("←/h", "panes"),
	),
	Right: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("→/l", ""),
	),
	SwitchPane: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch pane"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("gg/G", "top/bottom"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("", ""),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	ToggleSidebar: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "toggle sidebar"),
	),
	SwitchRepo: key.NewBinding(
		key.WithKeys("S"),
		key.WithHelp("S", "switch repo"),
	),
	OpenBrowser: key.NewBinding(
		key.WithKeys("O"),
		key.WithHelp("O", "open in browser"),
	),
	PrevAttempt: key.NewBinding(
		key.WithKeys("["),
		key.WithHelp("[/]", "prev/next attempt"),
	),
	NextAttempt: key.NewBinding(
		key.WithKeys("]"),
		key.WithHelp("", ""),
	),
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Left, k.Enter, k.Back, k.Filter, k.Refresh, k.ToggleSidebar, k.SwitchRepo, k.Help, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Top, k.Bottom, k.Enter, k.Back},
		{k.Filter, k.Refresh, k.Help, k.Quit},
	}
}
