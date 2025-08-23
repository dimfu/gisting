package main

import (
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	authScreen screen = iota
	mainScreen
)

type model struct {
	shutdown chan os.Signal

	screenState screen

	authScreen authModel
	mainScreen mainModel

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
	}
	return "no view defined for this state"
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Store the viewport size in the main model
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.shutdown <- syscall.SIGTERM
			return m, tea.Quit
		}

	case authSuccessMsg:
		model := newMainModel(m.shutdown, msg.client)
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
	}

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}
