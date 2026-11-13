package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines keyboard shortcuts for navigating, filtering, previewing, and managing sessions.
type KeyMap struct {
	Up                 key.Binding
	Down               key.Binding
	PageUp             key.Binding
	PageDown           key.Binding
	Top                key.Binding
	Bottom             key.Binding
	Search             key.Binding
	ToggleSubs         key.Binding
	Collapse           key.Binding
	TogglePreviewFocus key.Binding
	Rename             key.Binding
	TogglePin          key.Binding
	ToggleArchive      key.Binding
	ToggleShowArchived key.Binding
	Help               key.Binding
	Confirm            key.Binding
	Clear              key.Binding
	Quit               key.Binding
}

// DefaultKeyMap returns the default keyboard bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("PgUp", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("PgDn", "page down"),
		),
		Top: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("g", "jump to top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("G", "jump to bottom"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search sessions"),
		),
		ToggleSubs: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("l", "expand subsessions"),
		),
		Collapse: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h", "collapse subsessions"),
		),
		TogglePreviewFocus: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("Tab", "focus preview"),
		),
		Rename: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "rename session"),
		),
		TogglePin: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pin / unpin"),
		),
		ToggleArchive: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "archive / restore"),
		),
		ToggleShowArchived: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "show / hide archived"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "keybindings"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("Enter", "resume session"),
		),
		Clear: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("Esc", "clear / back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
