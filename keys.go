package main

import "github.com/charmbracelet/bubbles/key"

type Keymap struct {
	Left  key.Binding
	Right key.Binding
	Quit  key.Binding
}

func (k Keymap) ShortHelp() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Quit}
}

func (k Keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right},
		{k.Quit},
	}
}

var DefaultKeymap = Keymap{
	Left: key.NewBinding(
		key.WithKeys("ctrl+h"),
		key.WithHelp("ctrl+h", "left pane"),
	),
	Right: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "right pane"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
}
