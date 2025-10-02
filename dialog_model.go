package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/help"
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
	styles Styles
}

type dialogSubmitMsg struct {
	state          dialogState
	gistName       string
	value          string
	gistVisibility gistVisibility
}

func (m dialogModel) dialogTheme() *huh.Theme {
	var t huh.Theme

	t.Form.Base = lipgloss.NewStyle()
	t.Focused.Title = m.styles.Dialog.FocusedTitle
	t.Blurred.Title = m.styles.Dialog.BlurredTitle
	t.Group.Base = lipgloss.NewStyle()
	t.FieldSeparator = lipgloss.NewStyle().SetString("\n\n")

	// Focused styles.
	t.Focused.Base = m.styles.Dialog.Base
	t.Focused.Card = t.Focused.Base
	t.Focused.ErrorIndicator = lipgloss.NewStyle().SetString(" *")
	t.Focused.ErrorMessage = lipgloss.NewStyle().SetString(" *")
	t.Focused.SelectSelector = lipgloss.NewStyle().SetString("> ")
	t.Focused.NextIndicator = lipgloss.NewStyle().MarginLeft(1).SetString("→")
	t.Focused.PrevIndicator = lipgloss.NewStyle().MarginRight(1).SetString("←")
	t.Focused.MultiSelectSelector = lipgloss.NewStyle().SetString("> ")
	t.Focused.SelectedPrefix = lipgloss.NewStyle().SetString("[•] ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().SetString("[ ] ")
	t.Focused.FocusedButton = m.styles.Dialog.FocusedButton
	t.Focused.BlurredButton = m.styles.Dialog.BlurredButton
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	t.Focused.UnselectedOption = m.styles.Dialog.UnselectedOption

	t.Help = help.New().Styles

	// Blurred styles.
	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	return &t
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
			Placeholder(fmt.Sprintf("Enter %s name", actionType)).Value(&value).Key("value").WithWidth(60).WithTheme(m.dialogTheme()),
	}

	// show option to create public or private gist
	if m.state == dialog_create && actionType == "Gist" {
		s := huh.NewSelect[gistVisibility]().Title("Gist Visiblity").Options(
			huh.NewOption("Public", gist_public),
			huh.NewOption("Secret", gist_secret),
		).Key("visibility").WithTheme(m.dialogTheme())
		fields = append(fields, s)
	}

	fields = append(fields, huh.NewConfirm().
		Affirmative(affirmStr).
		Key("confirm").
		Negative("Cancel").Value(&d).WithTheme(m.dialogTheme()))

	return huh.NewForm(
		huh.NewGroup(fields...),
	)
}

func (m *dialogModel) formDelete() *huh.Form {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().Title("Are you sure?").Affirmative("Confirm").Negative("Cancel").Key("confirm").WithTheme(m.dialogTheme()),
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
	defaultStyles := DefaultStyles(cfg)
	m := dialogModel{
		width:  width,
		height: height,
		state:  state,
		styles: defaultStyles,
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
	styledContainer := m.styles.Dialog.Container.Render(formView)
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		styledContainer,
	)
}
