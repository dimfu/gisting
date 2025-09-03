package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v74/github"
)

type dialogState int

const (
	dialog_pane_gist dialogState = iota
	dialog_pane_file
	dialog_opened
	dialog_delete
	dialog_disabled
)

type dialogModel struct {
	client *github.Client
	width  int
	height int
	state  dialogState
	form   *huh.Form
}

type dialogCreateSubmitMsg struct {
	state    dialogState
	gistName string
	value    string
}

func (m *dialogModel) formCreate(actionType string) *huh.Form {
	d := true
	return huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Placeholder(fmt.Sprintf("Enter %s name", actionType)).Key("value").Lines(1).WithWidth(60),
			huh.NewConfirm().
				Affirmative("Create").
				Key("confirm").
				Negative("Cancel").Value(&d),
		),
	)
}

func (m *dialogModel) formDelete() *huh.Form {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().Title("Are you sure?").Affirmative("Confirm").Negative("Cancel").Key("confirm"),
		),
	)
	return form
}

type formType int

const (
	form_type_create formType = iota
	form_type_delete
)

func newDialogModel(width, height int, state dialogState, client *github.Client, formType formType) dialogModel {
	m := dialogModel{
		client: client,
		width:  width,
		height: height,
		state:  state,
	}

	var actionType string
	switch state {
	case dialog_pane_gist:
		actionType = "Gist"
	case dialog_pane_file:
		actionType = "File"
	}

	switch formType {
	case form_type_create:
		m.form = m.formCreate(actionType)
	case form_type_delete:
		m.form = m.formDelete()
	}

	m.form.WithShowHelp(false)

	return m
}

func (m dialogModel) Init() tea.Cmd {
	return m.form.Init()
}

type dialogCancelled bool

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
		isAffirm := m.form.GetBool("confirm")
		if isAffirm {
			cmds = append(cmds, func() tea.Msg {
				return dialogCreateSubmitMsg{
					state: m.state,
					value: m.form.GetString("value"),
				}
			})
		} else {
			cmds = append(cmds, func() tea.Msg {
				return dialogCancelled(true)
			})
		}
		// prevent the form from firing again
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
