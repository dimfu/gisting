package main

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v74/github"
	"github.com/google/uuid"
	editor "github.com/ionut-t/goeditor/adapter-bubbletea"
	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
	"golang.org/x/oauth2"
)

type pane int

const MAX_PANE = 3

const (
	PANE_GISTS pane = iota
	PANE_FILES
	PANE_EDITOR
)

type mainModel struct {
	gists map[gist][]list.Item

	keymap   Keymap
	shutdown chan os.Signal

	client *github.Client

	width  int
	height int

	// tui area
	gistList list.Model
	fileList list.Model
	editor   editor.Model
	help     help.Model

	currentPane pane

	FilesStyle  FilesBaseStyle
	GistsStyle  GistsBaseStyle
	EditorStyle EditorBaseStyle
}

func newMainModel(shutdown chan os.Signal, client *github.Client) mainModel {
	defaultStyle := DefaultStyles()
	m := mainModel{
		gists:       map[gist][]list.Item{},
		client:      client,
		shutdown:    shutdown,
		keymap:      DefaultKeymap,
		help:        help.New(),
		currentPane: PANE_GISTS,
		GistsStyle:  defaultStyle.Gists.Focused,
		FilesStyle:  defaultStyle.Files.Blurred,
	}

	if err := m.getGists(); err != nil {
		panic(fmt.Sprintf("Could not get gists on initial start up: \n%v", err))
	}

	// populate gist list
	var firstgist *gist
	gistList := []list.Item{}

	// sort gist alphabetically
	sortedGists := slices.Collect(maps.Keys(m.gists))
	slices.SortFunc(sortedGists, func(a, b gist) int {
		return strings.Compare(a.name, b.name)
	})

	for _, g := range sortedGists {
		if firstgist == nil {
			firstgist = &g
		}
		gistList = append(gistList, gist{id: g.id, name: g.name, status: g.status})
	}

	m.gistList = newGistList(gistList, m.GistsStyle)
	m.fileList = newFileList(m.gists[*firstgist], m.FilesStyle)

	// dont care about the width and height because we set it inside the tea.WindowSizeMsg
	textEditor := editor.New(0, 0)
	textEditor.ShowMessages(true)
	textEditor.SetCursorBlinkMode(true)

	// ensure the editor is initialized using the correct language from the selected first file
	firstFile := m.gists[*firstgist][0]
	f, ok := firstFile.(file)
	if !ok {
		panic(fmt.Sprintf("Cannot assert firstFile to type file, got %T", f))
	}
	alias := m.getEditorLanguage(f)
	textEditor.SetLanguage(alias, "nord")

	var defaultEditorTheme = editor.Theme{
		NormalModeStyle:        lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255")),
		InsertModeStyle:        lipgloss.NewStyle().Background(lipgloss.Color("26")).Foreground(lipgloss.Color("255")),
		VisualModeStyle:        lipgloss.NewStyle().Background(lipgloss.Color("127")).Foreground(lipgloss.Color("255")),
		CommandModeStyle:       lipgloss.NewStyle().Background(lipgloss.Color("208")).Foreground(lipgloss.Color("255")),
		CommandLineStyle:       lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("255")),
		StatusLineStyle:        lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")),
		MessageStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color("34")),
		ErrorStyle:             lipgloss.NewStyle().Foreground(lipgloss.Color("208")),
		LineNumberStyle:        lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Width(4).Align(lipgloss.Right),
		CurrentLineNumberStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(4).Align(lipgloss.Right),
		SelectionStyle:         lipgloss.NewStyle().Background(lipgloss.Color("237")),
		HighlightYankStyle:     lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0")).Bold(true),
		PlaceholderStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}

	textEditor.WithTheme(defaultEditorTheme)

	m.editor = textEditor

	return m
}

func (m *mainModel) next() {
	m.currentPane = (m.currentPane + 1) % MAX_PANE
}

func (m *mainModel) previous() {
	m.currentPane--
	if m.currentPane < 0 {
		m.currentPane = MAX_PANE - 1
	}
}

func (m mainModel) getEditorLanguage(f file) string {
	// get the language alias from the title first
	lexer := lexers.Match(f.title)
	// if no extension exist, analyze the content itself
	if lexer == nil {
		lexer = lexers.Analyse(f.content)
	}
	// fallback to whatever the lexer wants (i dont give a shit)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	langName := lexer.Config().Name
	var alias string
	if len(lexer.Config().Aliases) > 0 {
		alias = lexer.Config().Aliases[0]
	} else {
		alias = langName
	}
	return alias
}

func (m *mainModel) getGists() error {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	gists, _, err := m.client.Gists.List(ctx, "", &github.GistListOptions{
		ListOptions: github.ListOptions{
			// TODO: should we handle per page pagination or not?
			PerPage: 100,
		},
	})
	if err != nil {
		return err
	}
	publishedGistRawUrls := []string{}

	// get the uploaded gists
	for _, g := range gists {
		items := []list.Item{}
		for _, f := range g.GetFiles() {
			existing, err := storage.db.FindFirst(
				query.NewQuery(string(collectionGistContent)).Where(query.Field("rawUrl").Eq(f.GetRawURL()).And(query.Field("draft").Eq(false))),
			)
			if err != nil {
				return fmt.Errorf("Error while finding gist content with raw url %s\n%v", f.GetRawURL(), err)
			}

			i := file{
				id:        uuid.New().String(),
				gistId:    g.GetID(),
				title:     f.GetFilename(),
				desc:      g.GetDescription(),
				rawUrl:    f.GetRawURL(),
				updatedAt: g.GetUpdatedAt().In(time.Local).String(),
				draft:     false,
			}

			publishedGistRawUrls = append(publishedGistRawUrls, i.rawUrl)

			if existing == nil {
				doc := document.NewDocument()
				doc.SetAll(map[string]any{
					"id":        i.id,
					"gistId":    i.gistId,
					"title":     i.title,
					"desc":      i.desc,
					"rawUrl":    i.rawUrl,
					"updatedAt": i.updatedAt,
					"draft":     i.draft,
				})
				err := storage.db.Save(string(collectionGistContent), doc)
				if err != nil {
					return fmt.Errorf(`failed to insert gist "%s": %w`, g.GetDescription(), err)
				}
				existing = doc
			} else {
				i = file{
					id:        existing.Get("id").(string),
					gistId:    existing.Get("gistId").(string),
					title:     existing.Get("title").(string),
					desc:      existing.Get("desc").(string),
					rawUrl:    existing.Get("rawUrl").(string),
					updatedAt: existing.Get("updatedAt").(string),
					draft:     existing.Get("draft").(bool),
					content:   existing.Get("content").(string),
				}
			}

			items = append(items, i)
		}
		g := gist{
			name:   g.GetDescription(),
			id:     g.GetID(),
			status: gist_status_published,
		}
		m.gists[g] = items
	}

	existingRecords, err := storage.db.FindAll(
		query.NewQuery(string(collectionGistContent)).Where(query.Field("draft").Eq(false)),
	)
	if err != nil {
		return err
	}

	// when file are being updated it became unused, because the rawUrl changes every file update
	for _, record := range existingRecords {
		rawUrl := record.Get("rawUrl").(string)
		if !slices.Contains(publishedGistRawUrls, rawUrl) {
			err := storage.db.Delete(query.NewQuery(string(collectionGistContent)).Where(query.Field("rawUrl").Eq(rawUrl)))
			if err != nil {
				return fmt.Errorf(`failed to delete orphaned gist file: %w`, err)
			}
		}
	}

	// get all the drafted gists
	draftedDocs, err := storage.db.FindAll(
		query.NewQuery(string(collectionDraftedGists)),
	)
	if err != nil {
		return err
	}

	for _, doc := range draftedDocs {
		statusInt := doc.Get("status").(int64)
		gistId := doc.Get("id").(string)
		g := gist{
			id:     gistId,
			name:   doc.Get("description").(string),
			status: gistStatus(statusInt),
		}
		fileDocs, err := storage.db.FindAll(
			query.NewQuery(string(collectionGistContent)).Where(query.Field("gistId").Eq(gistId).And(query.Field("draft").Eq(true))),
		)
		if err != nil {
			return err
		}
		items := []list.Item{}
		for _, doc := range fileDocs {
			i := file{
				id:        doc.Get("id").(string),
				gistId:    doc.Get("gistId").(string),
				title:     doc.Get("title").(string),
				rawUrl:    doc.Get("rawUrl").(string),
				stale:     doc.Get("stale").(bool),
				desc:      doc.Get("desc").(string),
				updatedAt: doc.Get("updatedAt").(string),
				content:   doc.Get("content").(string),
				draft:     doc.Get("draft").(bool),
			}
			items = append(items, i)
		}
		m.gists[g] = items
	}
	return nil
}

func (m *mainModel) saveFileContent(content string) []tea.Cmd {
	selectedGist := m.gistList.SelectedItem()
	if selectedGist == nil {
		log.Error("Could not get the selected gist data")
		return nil
	}
	g, ok := selectedGist.(gist)
	if !ok {
		log.Errorf("Could not assert g to type gist, got %T", g)
		return nil
	}
	selectedFile := m.fileList.SelectedItem()
	if selectedFile == nil {
		log.Errorln("Could not get the selected file data")
		return nil
	}
	f, ok := selectedFile.(file)
	if !ok {
		log.Errorf("Could not assert f to type file, got %T\n", f)
		return nil
	}

	updates := map[string]interface{}{
		"id":        f.id,
		"content":   content,
		"updatedAt": f.updatedAt,
	}

	if !f.draft {
		gist := github.Gist{
			Files: map[github.GistFilename]github.GistFile{
				github.GistFilename(f.title): {
					Content: &content,
				},
			},
		}
		updatedGist, _, err := m.client.Gists.Edit(context.Background(), g.id, &gist)
		if err != nil {
			log.Errorf("Could not update gist file %q from Github\n%w", f.title, err)
			return nil
		}

		// update the rawUrl because it changes every update (learned it the hard way)
		for _, file := range updatedGist.GetFiles() {
			if file.GetFilename() == f.title {
				updates["rawUrl"] = file.GetRawURL()
				log.Printf("Old: %s -> New: %s", f.rawUrl, file.GetRawURL())
				break
			}
		}

		updates["updatedAt"] = updatedGist.GetUpdatedAt().In(time.Local).String()
	} else {
		updates["rawUrl"] = ""
		updates["updatedAt"] = time.Now().In(time.Local).String()
	}

	q := query.NewQuery(string(collectionGistContent)).Where(query.Field("id").Eq(f.id))
	if err := storage.db.Update(q, updates); err != nil {
		log.Errorf("Could not update gist content %q\n%w", f.title, err)
		return nil
	}

	// update the file item with updated data
	updatedFile := file{
		id:        f.id,
		gistId:    f.gistId,
		title:     f.title,
		desc:      f.desc,
		rawUrl:    updates["rawUrl"].(string),
		content:   content,
		updatedAt: updates["updatedAt"].(string),
		stale:     false,
		draft:     f.draft,
	}

	var cmds []tea.Cmd

	idx := m.fileList.Index()
	m.gists[g][idx] = updatedFile

	cmds = append(cmds, m.fileList.SetItem(idx, updatedFile))
	cmds = append(cmds, func() tea.Msg { return rerenderMsg(true) })

	return cmds
}

type updateEditorContent string

func (m *mainModel) loadSelectedFile() tea.Cmd {
	li := m.fileList.SelectedItem()
	// if there is no item inside gist item, render empty content instead
	if li == nil {
		return func() tea.Msg {
			return updateEditorContent("")
		}
	}

	f, _ := li.(file)
	content, err := f.getContent()
	if err != nil {
		return func() tea.Msg {
			return errMsg{err: err}
		}
	}

	alias := m.getEditorLanguage(f)
	m.editor.SetLanguage(alias, "nord")

	return func() tea.Msg {
		return updateEditorContent(content)
	}
}

func (m mainModel) Init() tea.Cmd {
	return tea.Batch(m.loadSelectedFile(), m.editor.CursorBlink())
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case rerenderMsg:
		cmds = append(cmds, m.updateActivePane(msg)...)
		return m, tea.Batch(cmds...)

	case editor.SaveMsg:
		if m.currentPane == PANE_EDITOR {
			m.editor.Blur()
			m.previous()

			cmds = append(cmds, m.saveFileContent(string(msg))...)
			cmds = append(cmds, m.updateActivePane(msg)...)
			return m, tea.Batch(cmds...)
		}

	case updateEditorContent:
		m.editor.SetContent(string(msg))
		editorModel, cmd := m.editor.Update(msg)
		cmds = append(cmds, cmd)
		m.editor = editorModel.(editor.Model)
		cmds = append(cmds, m.updateActivePane(msg)...)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+h":
			m.previous()
			return m, tea.Batch(m.updateActivePane(msg)...)
		case "ctrl+l":
			if m.currentPane != PANE_FILES {
				m.next()
				return m, tea.Batch(m.updateActivePane(msg)...)
			}
		}

		switch m.currentPane {
		case PANE_GISTS:
			switch msg.String() {
			case "up", "down", "j", "k":
				m.gistList, cmd = m.gistList.Update(msg)
				cmds = append(cmds, cmd)
				if selectedGist, ok := m.gistList.SelectedItem().(gist); ok {
					for gist, files := range m.gists {
						if gist.id == selectedGist.id {
							items := make([]list.Item, len(files))
							for i, item := range files {
								items[i] = item
							}

							cmd = m.fileList.SetItems(items)
							m.fileList.Select(0)
							cmds = append(cmds, cmd)

							// update editor content on gist changes by using the first selected item in list
							cmd = m.loadSelectedFile()
							cmds = append(cmds, cmd)

							break
						}
					}
				}
			case "enter":
				m.next()
			default:
			}

		case PANE_FILES:
			switch msg.String() {
			case "up", "down", "j", "k":
				m.fileList, cmd = m.fileList.Update(msg)
				cmds = append(cmds, cmd)
				// update editor content on file changes
				cmd = m.loadSelectedFile()
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			case "enter", "ctrl+l":
				m.next()
				return m, func() tea.Msg {
					return tea.KeyMsg{
						Type:  tea.KeyRunes,
						Runes: []rune{},
					}
				}
			default:
			}

		case PANE_EDITOR:
			m.editor.Focus()
			editorModel, cmd := m.editor.Update(msg)
			cmds = append(cmds, cmd)
			m.editor = editorModel.(editor.Model)
			cmds = append(cmds, m.updateActivePane(msg)...)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 1

		gv, _ := m.GistsStyle.Base.GetFrameSize()
		m.gistList.SetSize(m.width, m.height)

		fv, _ := m.FilesStyle.Base.GetFrameSize()
		m.fileList.SetSize(m.width, m.height)

		m.editor.SetSize(m.width-fv-gv-67, m.height+1)
	default:
	}

	return m, tea.Batch(cmds...)
}

type dialogStateChangeMsg dialogState

func (m *mainModel) updateActivePane(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch m.currentPane {
	case PANE_GISTS:
		m.GistsStyle = DefaultStyles().Gists.Focused
		m.FilesStyle = DefaultStyles().Files.Blurred
		m.EditorStyle = DefaultStyles().Editor.Blurred
		m.gistList, cmd = m.gistList.Update(msg)
		cmds = append(cmds, cmd)
	case PANE_FILES:
		m.GistsStyle = DefaultStyles().Gists.Blurred
		m.FilesStyle = DefaultStyles().Files.Focused
		m.EditorStyle = DefaultStyles().Editor.Blurred
		m.fileList, cmd = m.fileList.Update(msg)
		cmds = append(cmds, cmd)
	case PANE_EDITOR:
		m.GistsStyle = DefaultStyles().Gists.Blurred
		m.FilesStyle = DefaultStyles().Files.Blurred
		m.EditorStyle = DefaultStyles().Editor.Focused
		cmds = append(cmds, func() tea.Msg {
			return dialogStateChangeMsg(dialog_disabled)
		})
	}

	m.fileList.Styles.TitleBar = m.FilesStyle.TitleBar
	m.fileList.Styles.Title = m.FilesStyle.Title

	m.gistList.Styles.TitleBar = m.GistsStyle.TitleBar
	m.gistList.Styles.Title = m.GistsStyle.Title

	return cmds
}

func (m mainModel) View() string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.JoinHorizontal(
				lipgloss.Left,
				m.gistList.View(),
				m.fileList.View(),
			),
			lipgloss.NewStyle().Render(m.help.View(m.keymap)),
		),
		m.editor.View(),
	)
}
