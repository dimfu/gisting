package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

const maxWidth = 60

type dialogModel struct {
	width  int
	height int
	form   *huh.Form
}

func newDialogModel(width, height int) dialogModel {
	m := dialogModel{
		width:  width,
		height: height,
	}
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Placeholder("Enter your name").Lines(1),
			huh.NewConfirm().
				Key("done").
				Affirmative("Create").
				Negative("Cancel"),
		),
	)
	m.form.WithShowHelp(false)
	return m
}

func (m dialogModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m dialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}
	var cmds []tea.Cmd
	var cmd tea.Cmd
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
		cmds = append(cmds, cmd)
	}
	return m, cmd
}

func (m dialogModel) View() string {
	formView := m.form.View()

	containerStyle := lipgloss.NewStyle().
		Align(lipgloss.Center, lipgloss.Center).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("241")).
		Padding(1, 2)

	styledContainer := containerStyle.Render(formView)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		styledContainer,
	)
}
