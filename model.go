package main

import (
	"context"
	"errors"
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

type model struct {
	client *github.Client

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
		dialogScreen: newDialogModel(0, 0, dialog_pane_gist, nil, formCreate("File")),
		dialogState:  dialog_pane_gist,
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
			log.Errorln("could not assert item to type gist")
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
func (m *model) createFile(title string, gist gist) []tea.Cmd {
	var cmds []tea.Cmd

	id := uuid.New().String()
	f := file{
		id:     id,
		gistId: gist.id,
		title:  title,
		// had to add something to string or else github will complain that we're deleting a missing file from the current gist
		content:   "New File",
		desc:      "",
		rawUrl:    "",
		stale:     false,
		updatedAt: time.Now().In(time.Local).String(),
		draft:     true,
	}

	items := m.mainScreen.fileList.Items()

	if gist.status == gist_status_published {
		g := github.Gist{
			Description: &gist.name,
			Files:       map[github.GistFilename]github.GistFile{},
		}

		// add the new created file to the map
		g.Files[github.GistFilename(f.title)] = github.GistFile{Filename: &f.title, Content: &f.content}

		response, _, err := m.client.Gists.Edit(context.Background(), gist.id, &g)
		if err != nil {
			log.Errorf("Could not create gist file\n %v", err)
			return nil
		}

		for _, file := range response.Files {
			if file.GetFilename() == f.title {
				f.rawUrl = file.GetRawURL()
				f.updatedAt = response.GetUpdatedAt().In(time.Local).String()
				break
			}
		}

		f.draft = false
	}

	doc := document.NewDocument()
	doc.SetAll(map[string]any{
		"id":        f.id,
		"title":     f.title,
		"desc":      f.desc,
		"gistId":    f.gistId,
		"content":   f.content,
		"rawUrl":    f.rawUrl,
		"stale":     f.stale,
		"updatedAt": f.updatedAt,
		"draft":     f.draft,
	})
	if err := storage.db.Insert(string(collectionGistContent), doc); err != nil {
		log.Errorln(err.Error())
		return cmds
	}

	items = append(items, f)
	m.mainScreen.gists[gist] = items

	// update the file list with the new list
	cmd := m.mainScreen.fileList.SetItems(items)

	// trigger rerender
	rerender := func() tea.Msg {
		return rerenderMsg(true)
	}
	cmds = append(cmds, cmd, rerender)

	return cmds
}

func (m *model) upload(pane pane) []tea.Cmd {
	var cmds []tea.Cmd
	gItem := m.mainScreen.gistList.SelectedItem()
	g, ok := gItem.(gist)
	if !ok {
		log.Errorf("Cannot assert gist to type gist, got %T\n", g)
		return nil
	}
	switch pane {
	// when upload key is pressed on gist pane, it should upload all drafted files at once
	case PANE_GISTS:
		gist := github.Gist{
			Description: &g.name,
			Files:       map[github.GistFilename]github.GistFile{},
		}
		files := []file{}
		for _, item := range m.mainScreen.fileList.Items() {
			file, ok := item.(file)
			if !ok {
				log.Errorf("Cannot assert file to type file, got %T\n", file)
				return nil
			}
			gist.Files[github.GistFilename(file.title)] = github.GistFile{
				Filename: &file.title,
				Content:  &file.content,
			}
			files = append(files, file)
		}
		var response *github.Gist
		if g.status == gist_status_drafted {
			r, _, err := m.client.Gists.Create(context.Background(), &gist)
			if err != nil {
				log.Errorf("Could not create gist\n%w", err)
				return nil
			}
			response = r
			err = storage.db.Delete(query.NewQuery(string(collectionDraftedGists)).Where(query.Field("id").Eq(g.id)))
			if err != nil {
				log.Errorf("Could not delete draft gist %q\n%w", g.name, err)
				return nil
			}
		} else {
			r, _, err := m.client.Gists.Edit(context.Background(), g.id, &gist)
			if err != nil {
				log.Errorf("Could not update gist files\n%w", err)
				return nil
			}
			response = r
		}
		var newGistId string
		for _, respFile := range response.GetFiles() {
			for i, dbFile := range files {
				if dbFile.title == respFile.GetFilename() {
					q := query.NewQuery(string(collectionGistContent)).
						Where(query.Field("id").Eq(dbFile.id))
					newGistId = response.GetID()
					updates := map[string]any{
						"id":        dbFile.id,
						"gistId":    response.GetID(),
						"content":   respFile.GetContent(),
						"updatedAt": response.GetUpdatedAt().In(time.Local).String(),
						"draft":     false,
						"rawUrl":    respFile.GetRawURL(),
					}

					files[i].gistId = updates["gistId"].(string)
					files[i].content = updates["content"].(string)
					files[i].updatedAt = updates["updatedAt"].(string)
					files[i].draft = updates["draft"].(bool)
					files[i].rawUrl = updates["rawUrl"].(string)

					if err := storage.db.Update(q, updates); err != nil {
						log.Errorf(
							"Could not update gist file %q in the collection\n%v",
							respFile.GetFilename(), err,
						)
						return cmds
					}
					break
				}
			}
		}

		updatedItems := make([]list.Item, len(files))
		for idx, file := range files {
			updatedItems[idx] = file
		}

		if g.status == gist_status_drafted {
			newGist := g
			newGist.status = gist_status_published
			newGist.id = newGistId

			m.mainScreen.gists[newGist] = updatedItems
			delete(m.mainScreen.gists, g)

			cmd := m.mainScreen.gistList.SetItem(m.mainScreen.gistList.Index(), newGist)
			cmds = append(cmds, cmd)
			m.mainScreen.gistList.Select(m.mainScreen.gistList.Index())
		} else {
			m.mainScreen.gists[g] = updatedItems
		}

		// update the file list so that we have the latest data
		cmd := m.mainScreen.fileList.SetItems(updatedItems)
		cmds = append(cmds, cmd)

		break
	case PANE_FILES:
		break
	default:
		return cmds
	}
	cmds = append(cmds, func() tea.Msg {
		return rerenderMsg(true)
	})
	return cmds
}

func (m *model) deleteFile(g gist) error {
	f, ok := m.mainScreen.fileList.SelectedItem().(file)
	if !ok || f.gistId != g.id {
		return errors.New("Cannot get the selected file")
	}

	// uploaded file handling
	if !f.draft {
		gist := github.Gist{
			Files: map[github.GistFilename]github.GistFile{
				github.GistFilename(f.title): {},
			},
		}
		_, _, err := m.client.Gists.Edit(context.Background(), g.id, &gist)
		if err != nil {
			return fmt.Errorf("Could not delete gist file %q from Github\n%w", f.title, err)
		}

	}

	err := storage.db.Delete(query.NewQuery(string(collectionGistContent)).Where(query.Field("id").Eq(f.id)))
	if err != nil {
		return fmt.Errorf("Could not delete file %q from collection\n", f.title)
	}

	// remove deleted file & update the gist file list with the new one
	m.mainScreen.fileList.RemoveItem(m.mainScreen.fileList.Index())
	m.mainScreen.gists[g] = m.mainScreen.fileList.Items()

	return nil
}

// only use when the dialog initial render is janky
func (m *model) reInitDialog(form *huh.Form) tea.Cmd {
	m.dialogScreen = newDialogModel(m.width, m.height, m.dialogState, m.client, form)
	// change the mainscreen model to dialog model
	m.screenState = dialogScreen
	return m.dialogScreen.Init()
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
		case "ctrl+c":
			m.shutdown <- syscall.SIGTERM
			return m, tea.Quit

		case "ctrl+u":
			cmds = append(cmds, m.upload(m.mainScreen.currentPane)...)
			return m, tea.Batch(cmds...)
		case "a":
			// to enable other model to function properly i had to
			// relay the msg to the main screen model if the dialog is disabled
			if m.dialogState == dialog_disabled {
				updatedMainScreen, cmd := m.mainScreen.Update(msg)
				m.mainScreen = updatedMainScreen.(mainModel)
				return m, cmd
			}

			// handle "a" keystroke if dialog already open
			if m.dialogState == dialog_opened {
				var updated tea.Model
				updated, cmd = m.dialogScreen.form.Update(msg)
				m.dialogScreen.form = updated.(*huh.Form)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			} else {
				var form *huh.Form
				if m.dialogState == dialog_pane_gist {
					form = formCreate("Gist")
				} else if m.dialogState == dialog_pane_file {
					form = formCreate("File")
				}
				reinit := m.reInitDialog(form)
				m.dialogState = dialog_opened
				cmds = append(cmds, reinit)
				m.screenState = dialogScreen
				return m, tea.Batch(cmds...)
			}
		case "d":
			if m.dialogState == dialog_opened {
				var updated tea.Model
				updated, cmd = m.dialogScreen.form.Update(msg)
				m.dialogScreen.form = updated.(*huh.Form)
				cmds = append(cmds, cmd)
				updatedMainScreen, cmd := m.mainScreen.Update(msg)
				m.mainScreen = updatedMainScreen.(mainModel)
				return m, cmd
			}
			if m.dialogState != dialog_disabled {
				reinit := m.reInitDialog(formDelete())
				// change the dialog state to dialog_delete so that when we are submitting the dialog form
				// we can proceed to using delete condition instead of create
				m.dialogState = dialog_delete
				cmds = append(cmds, reinit)
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
		m.dialogState = dialogState(msg)

	case dialogCreateSubmitMsg:
		selectedGist := m.mainScreen.gistList.SelectedItem()
		gist, ok := selectedGist.(gist)
		if !ok {
			return m, nil
		}

		if msg.state == dialog_pane_gist {
			if m.dialogState == dialog_delete {
				log.Info("Deleting Gist")
			} else {
				cmds = append(cmds, m.createGist(msg.value)...)
			}
		} else {
			if m.dialogState == dialog_delete {
				if err := m.deleteFile(gist); err != nil {
					panic(err)
				}
			} else {
				cmds = append(cmds, m.createFile(msg.value, gist)...)
			}
		}
		cmds = append(cmds, m.mainScreen.updateActivePane(msg)...)
		m.screenState = mainScreen
		return m, tea.Batch(cmds...)

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
