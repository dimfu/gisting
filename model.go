package main

import (
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/google/go-github/v74/github"
	"github.com/google/uuid"
	"github.com/ostafen/clover/v2/document"
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
)

type model struct {
	client   *github.Client
	shutdown chan os.Signal

	screenState screen

	dialogState dialogStateChangeMsg

	authScreen   authModel
	mainScreen   mainModel
	dialogScreen dialogModel

	width  int
	height int
}

func initialModel(shutdown chan os.Signal) model {
	mux := http.NewServeMux()
	return model{
		client:      nil,
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
		dialogScreen: newDialogModel(0, 0, dialogStateChangeMsg{state: dialog_create_gist, gistName: ""}, nil),
		dialogState:  dialogStateChangeMsg{state: dialog_create_gist, gistName: ""},
	}
}

type rerenderMsg bool

// create gist and store it in drafted gist collection
func (m *model) createGist(name string) []tea.Cmd {
	var cmds []tea.Cmd

	doc := document.NewDocument()
	id := uuid.New().String()
	doc.SetAll(map[string]any{
		"id":          id,
		"description": name,
		"status":      gist_status_drafted,
	})
	if err := storage.db.Insert(string(collectionDraftedGists), doc); err != nil {
		return cmds
	}

	// get the current items from gistList
	items := m.mainScreen.gistList.Items()

	// TODO: should this be re ordered alphabetically?

	items = append(items, gist{
		id:     id,
		name:   name,
		status: gist_status_drafted,
	})

	// update gistList with the new slice
	cmd := m.mainScreen.gistList.SetItems(items)

	// trigger rerender
	rerender := func() tea.Msg {
		return rerenderMsg(true)
	}
	cmds = append(cmds, cmd, rerender)

	return cmds
}

// create gist and store it in drafted file collection
func (m *model) createFile() tea.Cmd {
	return func() tea.Msg {
		return nil
	}
}

// since github dont allow us to upload file one by one, upload all drafted files for a gist at once
func (m *model) uploadGist() tea.Cmd {
	return func() tea.Msg {
		return nil
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
			if m.dialogState.state == dialog_opened {
				var updated tea.Model
				updated, cmd = m.dialogScreen.form.Update(msg)
				m.dialogScreen.form = updated.(*huh.Form)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			} else {
				// recreate with current viewport dimension to prevent jankiness
				m.dialogScreen = newDialogModel(m.width, m.height, dialogStateChangeMsg{
					state:    m.dialogState.state,
					gistName: m.dialogState.gistName,
				}, m.client)
				m.dialogState = dialogStateChangeMsg{state: dialog_opened, gistName: ""}
				cmds = append(cmds, m.dialogScreen.Init())
				m.screenState = dialogScreen
				return m, tea.Batch(cmds...)
			}
		case "esc":
			if m.screenState == dialogScreen {
				m.screenState = mainScreen
			}
		}

	case authSuccessMsg:
		m.client = msg.client
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

	case dialogStateChangeMsg:
		m.dialogState = msg

	case dialogSubmitMsg:
		logs = append(logs, msg)
		// revert back to the dialog state that we are at when triggering the dialog
		m.dialogState = dialogStateChangeMsg{state: msg.state, gistName: msg.gistName}
		m.screenState = mainScreen

		if msg.state == dialog_create_gist {
			cmds = append(cmds, m.createGist(msg.value)...)
		} else {
			cmds = append(cmds, m.createFile())
		}

	case rerenderMsg:
		newMainScreen, newCmd := m.mainScreen.Update(msg)
		mainModel, ok := newMainScreen.(mainModel)
		if !ok {
			panic("could not perform authModel assertion")
		}
		m.mainScreen = mainModel
		cmd = newCmd
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
