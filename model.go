package main

import (
	"context"
	"errors"
	"sort"
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

	screenState screen
	dialogState dialogState

	authScreen   authModel
	mainScreen   mainModel
	dialogScreen dialogModel

	width  int
	height int
}

func initialModel() model {
	return model{
		client:      nil,
		screenState: authScreen,
		authScreen: authModel{
			loadingSpinner: spinner.New(),
			state:          auth_loading,
		},
		dialogScreen: newDialogModel(0, 0, dialog_closed, nil),
		dialogState:  dialog_closed,
	}
}

type rerenderMsg bool

// create gist and store it in drafted gist collection
func (m *model) createGist(name string, visibility gistVisibility) []tea.Cmd {
	var cmds []tea.Cmd

	if name == "" {
		name = time.Now().String()
	}

	doc := document.NewDocument()
	id := uuid.New().String()
	doc.SetAll(map[string]any{
		"id":          id,
		"description": name,
		"status":      gist_status_drafted,
		"visibility":  visibility,
	})

	if err := storage.db.Insert(string(collectionDraftedGists), doc); err != nil {
		return cmds
	}

	// get the current items from gistList
	gistItems := m.mainScreen.gistList.Items()

	emptyList := []list.Item{}
	g := gist{
		id:        id,
		name:      name,
		status:    gist_status_drafted,
		visiblity: visibility,
	}

	gistItems = append(gistItems, &g)

	// fill the app gists map with empty list for better user experience
	m.mainScreen.gists[&g] = emptyList

	sort.Slice(gistItems, func(i, j int) bool {
		a := gistItems[i].(*gist)
		b := gistItems[j].(*gist)
		return a.name < b.name
	})

	// update gistList with the new slice
	gistCmd := m.mainScreen.gistList.SetItems(gistItems)
	// create an empty file list for the newly created gist item
	fileCmd := m.mainScreen.fileList.SetItems(emptyList)
	_, updateFileList := m.mainScreen.fileList.Update(nil)

	// select the newly created gist item immediately in the gist list
	for idx, item := range gistItems {
		gist, ok := item.(*gist)
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
	cmds = append(cmds, gistCmd, fileCmd, updateFileList, rerender)

	return cmds
}

func (m *model) deleteGist(g *gist) []tea.Cmd {
	var cmds []tea.Cmd
	if g.status == gist_status_published {
		_, err := m.client.Gists.Delete(context.Background(), g.id)
		if err != nil {
			log.Errorf("Could not delete gist:\n%w", err)
			return nil
		}
	} else {
		err := storage.db.Delete(query.NewQuery(string(collectionDraftedGists)).Where(query.Field("id").Eq(g.id)))
		if err != nil {
			log.Errorf("Could not delete draft gist:\n%w", err)
			return nil
		}
	}

	idx := m.mainScreen.gistList.Index()
	m.mainScreen.gistList.RemoveItem(idx)

	if len(m.mainScreen.gistList.Items()) > 0 {
		// shift focus back to the previous deleted gist item
		if idx > 0 {
			idx--
		}
		m.mainScreen.gistList.Select(idx)
		gItem := m.mainScreen.gistList.SelectedItem()
		selectedGist, ok := gItem.(*gist)
		if !ok {
			log.Errorf("Could not assert selectedGist to type gist, got %T", selectedGist)
			return nil
		}
		cmd := m.mainScreen.fileList.SetItems(m.mainScreen.gists[selectedGist])
		cmds = append(cmds, cmd)
		_, updateList := m.mainScreen.fileList.Update(nil)
		cmds = append(cmds, updateList)
	}

	cmds = append(cmds, func() tea.Msg {
		return rerenderMsg(true)
	})

	return cmds
}

// create gist and store it in drafted file collection
func (m *model) createFile(title string, gist *gist) []tea.Cmd {
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

	if title == "" {
		f.title = time.Now().String()
	}

	gistFiles := m.mainScreen.gists[gist]
	for _, item := range gistFiles {
		file, _ := item.(file)
		if file.title == title {
			cmds = append(cmds, func() tea.Msg {
				return errMsg{err: errors.New("Filename should be unique")}
			})
			return cmds
		}
	}

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
				f.gistId = response.GetID()
				f.rawUrl = file.GetRawURL()
				f.updatedAt = response.GetUpdatedAt().In(time.Local).String()
				f.draft = false
				break
			}
		}
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

	m.mainScreen.gists[gist] = append(m.mainScreen.gists[gist], f)

	// update the file list with the new list
	fileCmd := m.mainScreen.fileList.SetItems(m.mainScreen.gists[gist])
	_, updateFileList := m.mainScreen.fileList.Update(nil)

	// trigger rerender
	rerender := func() tea.Msg {
		return rerenderMsg(true)
	}
	cmds = append(cmds, fileCmd, updateFileList, rerender)

	return cmds
}

func (m *model) upload(pane pane) []tea.Cmd {
	var cmds []tea.Cmd
	gItem := m.mainScreen.gistList.SelectedItem()
	g, ok := gItem.(*gist)
	if !ok {
		log.Errorf("Cannot assert gist to type gist, got %T\n", g)
		return nil
	}

	if pane == PANE_FILES {
		return cmds
	}

	var public bool
	if g.visiblity == gist_public {
		public = true
	}

	gist := github.Gist{
		Public:      &public,
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
	var (
		newGistId    string
		newUpdatedAt time.Time
	)
	for _, respFile := range response.GetFiles() {
		for i, dbFile := range files {
			if dbFile.title == respFile.GetFilename() {
				q := query.NewQuery(string(collectionGistContent)).
					Where(query.Field("id").Eq(dbFile.id))
				newGistId = response.GetID()
				newUpdatedAt = response.GetUpdatedAt().In(time.Local)
				updates := map[string]any{
					"id":        dbFile.id,
					"gistId":    response.GetID(),
					"content":   respFile.GetContent(),
					"updatedAt": newUpdatedAt.String(),
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
		g.status = gist_status_published
		g.id = newGistId
		g.updatedAt = newUpdatedAt

		m.mainScreen.gists[g] = updatedItems
		cmd := m.mainScreen.gistList.SetItem(m.mainScreen.gistList.Index(), g)
		cmds = append(cmds, cmd)
		m.mainScreen.gistList.Select(m.mainScreen.gistList.Index())
	} else {
		m.mainScreen.gists[g] = updatedItems
	}

	// update the file list so that we have the latest data
	cmd := m.mainScreen.fileList.SetItems(updatedItems)
	cmds = append(cmds, cmd)
	_, updatedFileList := m.mainScreen.fileList.Update(nil)
	cmds = append(cmds, updatedFileList)

	cmds = append(cmds, func() tea.Msg {
		return rerenderMsg(true)
	})
	return cmds
}

func (m *model) deleteFile(g *gist) tea.Cmd {
	f, ok := m.mainScreen.fileList.SelectedItem().(file)
	if !ok || f.gistId != g.id {
		log.Errorf("Cannot get the selected file")
		return nil
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
			log.Errorf("Could not delete gist file %q from Github\n%w", f.title, err)
			return nil
		}

	}

	err := storage.db.Delete(query.NewQuery(string(collectionGistContent)).Where(query.Field("id").Eq(f.id)))
	if err != nil {
		log.Errorf("Could not delete file %q from collection\n", f.title)
		return nil
	}

	// remove deleted file & update the gist file list with the new one
	idx := m.mainScreen.fileList.Index()
	m.mainScreen.fileList.RemoveItem(idx)
	m.mainScreen.gists[g] = m.mainScreen.fileList.Items()

	// shift focus back to the previous deleted gist item
	if len(m.mainScreen.fileList.Items()) > 0 {
		if idx > 0 {
			idx--
		}
		m.mainScreen.fileList.Select(idx)
	}

	_, cmd := m.mainScreen.fileList.Update(nil)
	return cmd
}

func (m *model) rename(pane pane, newValue string) []tea.Cmd {
	var cmds []tea.Cmd
	gItem := m.mainScreen.gistList.SelectedItem()
	selectedGist, ok := gItem.(*gist)
	if !ok {
		log.Errorf("Could not assert selectedGist to type gist, got %T", selectedGist)
		return nil
	}

	gist := github.Gist{
		Files: map[github.GistFilename]github.GistFile{},
	}

	var selectedFile file
	if pane == PANE_GISTS {
		gist.Description = &newValue
	} else {
		fItem := m.mainScreen.fileList.SelectedItem()
		f, ok := fItem.(file)
		if !ok {
			log.Errorf("Could not assert f to type file, got %T", f)
			return nil
		}
		selectedFile = f
		gist.Files[github.GistFilename(selectedFile.title)] = github.GistFile{
			Filename: &newValue,
		}
	}

	var response *github.Gist
	if selectedGist.status == gist_status_published {
		r, _, err := m.client.Gists.Edit(context.Background(), selectedGist.id, &gist)
		if err != nil {
			log.Errorf("Error renaming gist with id %q\n%w", selectedGist.id, err)
			return nil
		}
		response = r
	}

	if pane == PANE_GISTS {
		if selectedGist.status == gist_status_drafted {
			q := query.NewQuery(string(collectionDraftedGists)).Where(query.Field("id").Eq(selectedGist.id))
			updates := map[string]any{}
			updates["description"] = newValue
			if err := storage.db.Update(q, updates); err != nil {
				log.Errorf("Could not update renamed gist file %q\n%w", newValue, err)
				return nil
			}
			selectedGist.name = newValue
		} else {
			selectedGist.name = response.GetDescription()
		}

		idx := m.mainScreen.gistList.Index()
		m.mainScreen.gistList.SetItem(idx, selectedGist)

		cmds = append(cmds, m.mainScreen.gistList.SetItem(idx, selectedGist))
	} else {
		q := query.NewQuery(string(collectionGistContent)).Where(query.Field("id").Eq(selectedFile.id))
		updates := map[string]any{}
		updates["title"] = newValue
		selectedFile.title = newValue

		if selectedGist.status == gist_status_published {
			for _, file := range response.GetFiles() {
				if file.GetFilename() == newValue {
					updates["rawUrl"] = file.GetRawURL()
					updates["updatedAt"] = response.GetUpdatedAt().In(time.Local).String()
				}
				break
			}
		}

		if err := storage.db.Update(q, updates); err != nil {
			log.Errorf("Could not update renamed gist file %q\n%w", newValue, err)
			return nil
		}

		fileIdx := m.mainScreen.fileList.Index()
		m.mainScreen.fileList.SetItem(fileIdx, selectedFile)

		files := m.mainScreen.gists[selectedGist]
		if fileIdx >= 0 && fileIdx < len(files) {
			files[fileIdx] = selectedFile
		}
		m.mainScreen.gists[selectedGist] = files

		cmds = append(cmds, m.mainScreen.fileList.SetItem(fileIdx, selectedFile))
	}

	cmds = append(cmds, func() tea.Msg {
		return rerenderMsg(true)
	})

	return cmds
}

// prevent dialog from popping up when we press dialog key map by relaying current keystroke to the current dialog form instead
func (m *model) allowDialogKeystroke(msg tea.Msg) tea.Cmd {
	updated, cmd := m.dialogScreen.form.Update(msg)
	m.dialogScreen.form = updated.(*huh.Form)
	return cmd
}

// prevent dialog from popping up by sending whatever msg we currently have to the main screen dialog
func (m *model) disableDialogPopup(msg tea.Msg) tea.Cmd {
	updatedMainScreen, cmd := m.mainScreen.Update(msg)
	m.mainScreen = updatedMainScreen.(mainModel)
	return cmd
}

func (m *model) closeDialog() {
	m.dialogState = dialog_closed
	m.screenState = mainScreen
}

func (m *model) reInitDialog(msg tea.Msg, formType formType) tea.Cmd {
	if m.dialogState == dialog_opened {
		return m.allowDialogKeystroke(msg)
	} else if m.dialogState == dialog_disabled {
		return m.disableDialogPopup(msg)
	}

	m.dialogScreen = newDialogModel(m.width, m.height, m.dialogState, m.client)

	// change the dialog state to opened for the main model.go
	m.dialogState = dialog_opened

	var actionType string
	switch m.mainScreen.currentPane {
	case PANE_GISTS:
		actionType = "Gist"
	case PANE_FILES:
		actionType = "File"
	}

	switch formType {
	case form_type_create:
		m.dialogScreen.state = dialog_create
		m.dialogScreen.form = m.dialogScreen.formInput(actionType, "")
	case form_type_rename:
		m.dialogScreen.state = dialog_rename
		var value string
		switch m.mainScreen.currentPane {
		case PANE_GISTS:
			item := m.mainScreen.gistList.SelectedItem()
			gist, ok := item.(*gist)
			if !ok {
				log.Errorf("Could not assert gist to type gist, got %T", gist)
				return nil
			}
			value = gist.name
		case PANE_FILES:
			item := m.mainScreen.fileList.SelectedItem()
			file, ok := item.(file)
			if !ok {
				log.Errorf("Could not assert file to type file, got %T", file)
				return nil
			}
			value = file.title
		default:
			return nil
		}
		m.dialogScreen.form = m.dialogScreen.formInput(actionType, value)
	case form_type_delete:
		m.dialogScreen.state = dialog_delete
		m.dialogScreen.form = m.dialogScreen.formDelete()
	}

	m.dialogScreen.form.WithShowHelp(true)

	// change the screen state to the dialog model
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
		if m.screenState == mainScreen || m.screenState == dialogScreen {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "u":
				if m.mainScreen.currentPane != PANE_EDITOR {
					cmds = append(cmds, m.upload(m.mainScreen.currentPane)...)
					return m, tea.Batch(cmds...)
				}
			case "a":
				return m, m.reInitDialog(msg, form_type_create)
			case "r":
				return m, m.reInitDialog(msg, form_type_rename)
			case "d":
				return m, m.reInitDialog(msg, form_type_delete)
			case "esc":
				if m.screenState == dialogScreen {
					m.closeDialog()
				}
			}
		}

	case errMsg:
		log.Errorln(msg.err.Error())
		return m, nil

	case authSuccessMsg:
		m.client = msg.client
		model := newMainModel(msg.client)
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

	case dialogSubmitMsg:
		selectedGist := m.mainScreen.gistList.SelectedItem()
		gist, ok := selectedGist.(*gist)
		if !ok {
			log.Error("Could not get selected gist on dialogCreateSubmitMsg")
			return m, nil
		}

		state := msg.state
		pane := m.mainScreen.currentPane

		switch state {
		case dialog_create:
			if pane == PANE_GISTS {
				cmds = append(cmds, m.createGist(msg.value, msg.gistVisibility)...)
			} else {
				cmds = append(cmds, m.createFile(msg.value, gist)...)
			}
			break
		case dialog_delete:
			if pane == PANE_GISTS {
				cmds = append(cmds, m.deleteGist(gist)...)
			} else {
				cmds = append(cmds, m.deleteFile(gist))
			}
			break
		case dialog_rename:
			cmds = append(cmds, m.rename(pane, msg.value)...)
			break
		default:
			log.Errorf("Unrecognized dialog state %q\n", state)
			return m, nil
		}

		cmds = append(cmds, m.mainScreen.updateActivePane(msg)...)
		m.closeDialog()
		return m, tea.Batch(cmds...)
	case dialogCancelled:
		cmds = append(cmds, m.mainScreen.updateActivePane(msg)...)
		m.closeDialog()
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
