package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v74/github"
)

type dialogModel struct {
	client *github.Client
	width  int
	height int
	state  dialogStateChangeMsg
	form   *huh.Form
}

type dialogSubmitMsg struct {
	state    dialogState
	gistName string
	value    string
}

func newDialogModel(width, height int, s dialogStateChangeMsg, client *github.Client) dialogModel {
	m := dialogModel{
		client: client,
		width:  width,
		height: height,
		state:  s,
	}

	var actionType string
	if s.state == dialog_create_gist {
		actionType = "Gist"
	} else {
		actionType = "File"
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Placeholder(fmt.Sprintf("Enter %s name", actionType)).Key("value").Lines(1).WithWidth(60),
			huh.NewConfirm().
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
	var cmds []tea.Cmd
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
		cmds = append(cmds, cmd)
	}

	if m.form.State == huh.StateCompleted {
		cmds = append(cmds, func() tea.Msg {
			return dialogSubmitMsg{
				state:    m.state.state,
				gistName: m.state.gistName,
				value:    m.form.GetString("value"),
			}
		})
		// prevent the form firing up again thousands of time when submitting
		m.form.State = huh.StateAborted
	}

	return m, tea.Batch(cmds...)
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
