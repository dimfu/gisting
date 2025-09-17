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
	dialog_opened dialogState = iota
	dialog_closed
	dialog_delete
	dialog_create
	dialog_rename
	dialog_disabled
)

type dialogModel struct {
	width  int
	height int
	state  dialogState
	form   *huh.Form
}

type dialogSubmitMsg struct {
	state          dialogState
	gistName       string
	value          string
	gistVisibility gistVisibility
}

func (m *dialogModel) formInput(actionType, value string) *huh.Form {
	d := true
	var affirmStr string

	switch m.state {
	case dialog_create:
		affirmStr = "Create"
	case dialog_rename:
		affirmStr = "Rename"
	default:
		affirmStr = "Submit"
	}

	fields := []huh.Field{
		huh.NewInput().
			Placeholder(fmt.Sprintf("Enter %s name", actionType)).Value(&value).Key("value").WithWidth(60),
	}

	// show option to create public or private gist
	if m.state == dialog_create && actionType == "Gist" {
		s := huh.NewSelect[gistVisibility]().Title("Gist Visiblity").Options(
			huh.NewOption("Public", gist_public),
			huh.NewOption("Secret", gist_secret),
		).Key("visibility")
		fields = append(fields, s)
	}

	fields = append(fields, huh.NewConfirm().
		Affirmative(affirmStr).
		Key("confirm").
		Negative("Cancel").Value(&d))

	return huh.NewForm(
		huh.NewGroup(fields...),
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
	form_type_rename
)

func newDialogModel(width, height int, state dialogState, client *github.Client) dialogModel {
	m := dialogModel{
		width:  width,
		height: height,
		state:  state,
	}
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
			msg := dialogSubmitMsg{
				state: m.state,
				value: m.form.GetString("value"),
			}

			if m.state == dialog_create {
				visGet := m.form.Get("visibility")
				visibility, _ := visGet.(gistVisibility)
				msg.gistVisibility = visibility
			}

			cmds = append(cmds, func() tea.Msg {
				return msg
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
