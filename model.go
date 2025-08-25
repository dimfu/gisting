package main

import (
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

type screen int

const (
	authScreen screen = iota
	mainScreen
	dialogScreen
)

type dialogState int

const (
	dialog_create_gist dialogState = iota
	dialog_create_file
	dialog_opened
	dialog_closed
)

type model struct {
	shutdown chan os.Signal

	screenState screen

	dialogState dialogState

	authScreen   authModel
	mainScreen   mainModel
	dialogScreen dialogModel

	width  int
	height int
}

func initialModel(shutdown chan os.Signal) model {
	mux := http.NewServeMux()
	return model{
		shutdown:    shutdown,
		screenState: authScreen,
		authScreen: authModel{
			loadingSpinner: spinner.New(),
			state:          auth_loading,
			shutdown:       shutdown,
			mux:            mux,
			server: &http.Server{
				Addr:         ":8080",
				Handler:      mux,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
			},
		},
		dialogScreen: newDialogModel(0, 0),
		dialogState:  dialog_create_file,
	}
}

func (m model) Init() tea.Cmd {
	return m.authScreen.Init()
}

func (m model) View() string {
	switch m.screenState {
	case authScreen:
		return m.authScreen.View()
	case mainScreen:
		return m.mainScreen.View()
	case dialogScreen:
		return m.dialogScreen.View()
	}
	return "no view defined for this state"
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.shutdown <- syscall.SIGTERM
			return m, tea.Quit
		case "a":
			// handle "a" keystroke if dialog already open
			if m.dialogState == dialog_opened {
				var updated tea.Model
				updated, cmd = m.dialogScreen.form.Update(msg)
				m.dialogScreen.form = updated.(*huh.Form)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			} else {
				m.dialogState = dialog_opened
				// recreate with current viewport dimension to prevent jankiness
				m.dialogScreen = newDialogModel(m.width, m.height)
				cmds = append(cmds, m.dialogScreen.Init())
				m.screenState = dialogScreen
				return m, tea.Batch(cmds...)
			}
		case "esc":
			if m.screenState == dialogScreen {
				m.screenState = mainScreen
				m.dialogState = dialog_closed
			}
		}

	case authSuccessMsg:
		model := newMainModel(m.shutdown, msg.client, &m.dialogState)
		m.mainScreen = model
		cmds = append(cmds, model.Init())
		m.screenState = mainScreen

		// CRITICAL: Send the current viewport size to the new main screen immediately
		if m.width > 0 && m.height > 0 {
			newMainScreen, newCmd := m.mainScreen.Update(tea.WindowSizeMsg{
				Width:  m.width,
				Height: m.height,
			})
			m.mainScreen = newMainScreen.(mainModel)
			cmds = append(cmds, newCmd)
		}
		return m, tea.Batch(cmds...)
	}

	switch m.screenState {
	case authScreen:
		newAuthScreen, newCmd := m.authScreen.Update(msg)
		authModel, ok := newAuthScreen.(authModel)
		if !ok {
			panic("could not perform authModel assertion")
		}
		m.authScreen = authModel
		cmd = newCmd

	case mainScreen:
		newMainScreen, newCmd := m.mainScreen.Update(msg)
		mainModel, ok := newMainScreen.(mainModel)
		if !ok {
			panic("could not perform authModel assertion")
		}
		m.mainScreen = mainModel
		cmd = newCmd
	case dialogScreen:
		newDialogModel, newCmd := m.dialogScreen.Update(msg)
		dialogModel, ok := newDialogModel.(dialogModel)
		if !ok {
			panic("could not perform dialogScreen assertion")
		}
		m.dialogScreen = dialogModel
		cmd = newCmd
	}

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}
