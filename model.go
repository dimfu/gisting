package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/google/go-github/v74/github"
	"github.com/google/uuid"
	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
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
	gistItems := m.mainScreen.gistList.Items()

	emptyList := []list.Item{}
	g := gist{
		id:     id,
		name:   name,
		status: gist_status_drafted,
	}
	gistItems = append(gistItems, g)

	// fill the app gists map with empty list for better user experience
	m.mainScreen.gists[g] = emptyList

	sort.Slice(gistItems, func(i, j int) bool {
		a := gistItems[i].(gist)
		b := gistItems[j].(gist)
		return a.name < b.name
	})

	// update gistList with the new slice
	cmd := m.mainScreen.gistList.SetItems(gistItems)
	cmds = append(cmds, cmd)

	// create an empty file list for the newly created gist item
	cmd = m.mainScreen.fileList.SetItems(emptyList)
	cmds = append(cmds, cmd)

	// select the newly created gist item immediately in the gist list
	for idx, item := range gistItems {
		gist, ok := item.(gist)
		if !ok {
			logs = append(logs, "could not assert item to type gist")
			return cmds
		}
		if gist.id == id {
			m.mainScreen.gistList.Select(idx)
			break
		}
	}

	// trigger rerender
	rerender := func() tea.Msg {
		return rerenderMsg(true)
	}
	cmds = append(cmds, rerender)

	return cmds
}

// create gist and store it in drafted file collection
func (m *model) createFile(title, gistId string) []tea.Cmd {
	var cmds []tea.Cmd

	gistDoc, err := storage.db.FindFirst(
		query.NewQuery(string(collectionDraftedGists)).Where(query.Field("id").Eq(gistId)),
	)
	if err != nil {
		logs = append(logs, fmt.Sprintf("Could not get gist document with id %s", gistId))
		return cmds
	}

	var (
		currGistMapItem gist
		foundGist       bool
	)

	draftedGistId := gistDoc.Get("id").(string)
	for gist := range m.mainScreen.gists {
		if gist.id == draftedGistId {
			currGistMapItem = gist
			foundGist = true
			break
		}
	}

	if !foundGist {
		logs = append(logs, fmt.Sprintf("Could not find gist inside the main app map\n"))
		return cmds
	}

	doc := document.NewDocument()
	id := uuid.New().String()
	doc.SetAll(map[string]any{
		"id":        id, // id used only for the database ops
		"title":     title,
		"desc":      "",
		"gist_id":   gistId,
		"content":   "",
		"rawUrl":    "",
		"stale":     false,
		"updatedAt": time.Now().String(),
		"draft":     true,
	})

	items := m.mainScreen.fileList.Items()
	f := file{
		title:     title,
		content:   "",
		desc:      "",
		rawUrl:    "",
		stale:     false,
		updatedAt: time.Now().String(),
		draft:     true,
	}
	items = append(items, f)
	m.mainScreen.gists[currGistMapItem] = items

	if err := storage.db.Insert(string(collectionDraftedFiles), doc); err != nil {
		logs = append(logs, err)
		return cmds
	}

	// update the file list with the new list
	cmd := m.mainScreen.fileList.SetItems(items)

	// trigger rerender
	rerender := func() tea.Msg {
		return rerenderMsg(true)
	}
	cmds = append(cmds, cmd, rerender)

	return cmds
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
			selectedItem := m.mainScreen.gistList.SelectedItem()
			gist, ok := selectedItem.(gist)
			if !ok {
				return m, nil
			}
			cmds = append(cmds, m.createFile(msg.value, gist.id)...)
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
