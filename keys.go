package main

import "github.com/charmbracelet/bubbles/key"

type Keymap struct {
	Navigate key.Binding
	Create   key.Binding
	Upload   key.Binding
	Delete   key.Binding
	Rename   key.Binding
	Copy     key.Binding
	Left     key.Binding
	Right    key.Binding
	Quit     key.Binding
	Help     key.Binding
}

func (k Keymap) ShortHelp() []key.Binding {
	return []key.Binding{k.Navigate, k.Create, k.Upload, k.Quit, k.Help}
}

func (k Keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Navigate, k.Left, k.Right},
		{k.Create, k.Upload, k.Delete},
		{k.Rename, k.Copy, k.Help},
		{k.Quit},
	}
}

var DefaultKeymap = Keymap{
	Navigate: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "navigate"),
	),
	Create: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "create"),
	),
	Left: key.NewBinding(
		key.WithKeys("ctrl+h"),
		key.WithHelp("ctrl+h", "left pane"),
	),
	Right: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "right pane"),
	),
	Upload: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "upload"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename"),
	),
	Copy: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "copy content"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
}
