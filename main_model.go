package main

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v74/github"
	"github.com/google/uuid"
	editor "github.com/ionut-t/goeditor/adapter-bubbletea"
	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
	"golang.design/x/clipboard"
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
	gists  map[*gist][]list.Item
	client *github.Client

	currentPane pane
	width       int
	height      int
	infoMsg     *infoMsg

	// tui area
	gistList list.Model
	fileList list.Model
	editor   editor.Model
	help     help.Model
	keymap   Keymap

	FilesStyle FilesBaseStyle
	GistsStyle GistsBaseStyle
}

func newMainModel(client *github.Client) mainModel {
	defaultStyle := DefaultStyles()
	m := mainModel{
		gists:       map[*gist][]list.Item{},
		client:      client,
		keymap:      DefaultKeymap,
		help:        help.New(),
		currentPane: PANE_GISTS,
		GistsStyle:  defaultStyle.Gists.Focused,
		FilesStyle:  defaultStyle.Files.Blurred,
		infoMsg:     nil,
	}

	if err := m.getGists(); err != nil {
		panic(fmt.Sprintf("Could not get gists on initial start up: \n%v", err))
	}

	// populate gist list
	var firstgist *gist
	gistList := []list.Item{}

	// sort gist alphabetically
	sortedGists := slices.Collect(maps.Keys(m.gists))
	slices.SortFunc(sortedGists, func(a, b *gist) int {
		return strings.Compare(a.name, b.name)
	})

	for _, g := range sortedGists {
		if firstgist == nil {
			firstgist = g
		}
		gistList = append(gistList, g)
	}

	m.gistList = newGistList(gistList, m.GistsStyle)
	m.fileList = newFileList(m.gists[firstgist], m.FilesStyle)

	// dont care about the width and height because we set it inside the tea.WindowSizeMsg
	textEditor := editor.New(0, 0)
	textEditor.ShowMessages(true)
	textEditor.SetCursorBlinkMode(true)

	textEditor.DisableVimMode(true)
	if withVimMotion {
		textEditor.DisableVimMode(false)
	}

	// ensure the editor is initialized using the correct language from the selected first file
	firstFile := m.gists[firstgist][0]
	f, ok := firstFile.(file)
	if !ok {
		panic(fmt.Sprintf("Cannot assert firstFile to type file, got %T", f))
	}

	var defaultEditorTheme = editor.Theme{
		NormalModeStyle:        lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255")),
		InsertModeStyle:        lipgloss.NewStyle().Background(lipgloss.Color("26")).Foreground(lipgloss.Color("255")),
		VisualModeStyle:        lipgloss.NewStyle().Background(lipgloss.Color("127")).Foreground(lipgloss.Color("255")),
		CommandModeStyle:       lipgloss.NewStyle().Background(lipgloss.Color("208")).Foreground(lipgloss.Color("255")),
		CommandLineStyle:       lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("255")),
		MessageStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color("34")),
		ErrorStyle:             lipgloss.NewStyle().Foreground(lipgloss.Color("208")),
		LineNumberStyle:        lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Width(4).Align(lipgloss.Right),
		CurrentLineNumberStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(4).Align(lipgloss.Right),
		SelectionStyle:         lipgloss.NewStyle().Background(lipgloss.Color("237")),
		HighlightYankStyle:     lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0")).Bold(true),
		PlaceholderStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}

	if withVimMotion {
		defaultEditorTheme.StatusLineStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255"))
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
					rawUrl:    existing.Get("rawUrl").(string),
					updatedAt: existing.Get("updatedAt").(string),
					draft:     existing.Get("draft").(bool),
				}

				// only get field content if they are not empty or else the program will be upset lol
				if c, ok := existing.Get("content").(string); ok {
					i.content = c
				}
			}

			items = append(items, i)
		}

		visibility := gist_secret
		if g.GetPublic() {
			visibility = gist_public
		}

		g := gist{
			name:      g.GetDescription(),
			id:        g.GetID(),
			status:    gist_status_published,
			updatedAt: g.GetUpdatedAt().Time.In(time.Local),
			visiblity: visibility,
		}
		m.gists[&g] = items
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
		visibility := doc.Get("visibility").(int64)
		g := gist{
			id:        gistId,
			name:      doc.Get("description").(string),
			status:    gistStatus(statusInt),
			visiblity: gistVisibility(visibility),
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
		m.gists[&g] = items
	}
	return nil
}

func (m *mainModel) saveFileContent(content string) []tea.Cmd {
	var cmds []tea.Cmd
	selectedGist := m.gistList.SelectedItem()
	if selectedGist == nil {
		log.Error("could not get the selected gist data")
		cmds = append(cmds, showInfo("could not get selected gist data", info_error))
		return cmds
	}
	g, _ := selectedGist.(*gist)
	selectedFile := m.fileList.SelectedItem()
	if selectedFile == nil {
		log.Errorln("could not get the selected file data")
		cmds = append(cmds, showInfo("could not get selected file data", info_error))
		return cmds
	}

	f, _ := selectedFile.(file)

	updates := map[string]interface{}{
		"id":        f.id,
		"content":   content,
		"updatedAt": f.updatedAt,
	}

	var updateTime time.Time

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
			log.Errorf("could not update gist file %q from Github\n%w", f.title, err)
			cmds = append(cmds, showInfo("could not update gist from github", info_error))
			return cmds
		}

		// update the rawUrl because it changes every update (learned it the hard way)
		for _, file := range updatedGist.GetFiles() {
			if file.GetFilename() == f.title {
				updates["rawUrl"] = file.GetRawURL()
				// log.Printf("Old: %s -> New: %s", f.rawUrl, file.GetRawURL())
				break
			}
		}
		updateTime = updatedGist.GetUpdatedAt().In(time.Local)
		updates["updatedAt"] = updateTime.String()
	} else {
		updates["rawUrl"] = ""
		updateTime = time.Now().In(time.Local)
		updates["updatedAt"] = updateTime.String()
	}

	q := query.NewQuery(string(collectionGistContent)).Where(query.Field("id").Eq(f.id))
	if err := storage.db.Update(q, updates); err != nil {
		log.Errorf("could not gist content on db %q\n%w", f.title, err)
		cmds = append(cmds, showInfo("could not gist content on db", info_error))
		return cmds
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

	idx := m.fileList.Index()
	g.updatedAt = updateTime
	m.gists[g][idx] = updatedFile

	cmds = append(cmds, m.fileList.SetItem(idx, updatedFile))
	cmds = append(cmds, showInfo("gist content saved", info_default))

	return cmds
}

func (m *mainModel) copyToClipboard() tea.Cmd {
	selectedItem := m.fileList.SelectedItem()
	f, ok := selectedItem.(file)
	if !ok {
		return showInfo("no file selected", info_default)
	}
	clipboard.Write(clipboard.FmtText, []byte(f.content))
	return showInfo("content copied to clipboard", info_default)
}

type updateEditorContent struct {
	content  string
	language string
}

func (m mainModel) Init() tea.Cmd {
	_, initFileList := m.fileList.Update(nil)
	return tea.Batch(initFileList, m.editor.CursorBlink())
}

func (m *mainModel) resetListHeight() {
	if m.help.ShowAll {
		m.gistList.SetSize(45, m.height-2)
		m.fileList.SetSize(45, m.height-2)
	} else {
		m.gistList.SetSize(45, m.height)
		m.fileList.SetSize(45, m.height)
	}
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case editor.SaveMsg:
		if m.currentPane == PANE_EDITOR {
			m.editor.Blur()
			m.previous()

			cmds = append(cmds, m.saveFileContent(string(msg))...)
			cmds = append(cmds, m.updateActivePane(msg)...)
			return m, tea.Batch(cmds...)
		}

	case updateEditorContent:
		m.editor.SetContent(string(msg.content))
		m.editor.SetLanguage(msg.language, "nord")
		editorModel, cmd := m.editor.Update(msg)
		cmds = append(cmds, cmd)
		m.editor = editorModel.(editor.Model)
		cmds = append(cmds, m.updateActivePane(msg)...)

	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			m.help.ShowAll = !m.help.ShowAll
			m.resetListHeight()
		case "ctrl+h":
			m.previous()
			return m, tea.Batch(m.updateActivePane(msg)...)
		case "ctrl+l", "tab":
			// skip tab navigation while editor is on INSERT mode or VISUAL
			if m.currentPane == PANE_EDITOR && !m.editor.IsNormalMode() {
				editorModel, cmd := m.editor.Update(msg)
				cmds = append(cmds, cmd)
				m.editor = editorModel.(editor.Model)
				return m, tea.Batch(cmds...)
			}
			m.next()
			// hack: send keypress cmd to trigger cursor blink
			if m.currentPane == PANE_EDITOR {
				m.editor.Focus()
				return m, func() tea.Msg {
					return tea.KeyMsg{
						Type:  tea.KeyRunes,
						Runes: []rune{},
					}
				}
			}
			return m, tea.Batch(m.updateActivePane(msg)...)
		}

		switch m.currentPane {
		case PANE_GISTS:
			switch msg.String() {
			case "up", "down", "j", "k":
				m.gistList, cmd = m.gistList.Update(msg)
				cmds = append(cmds, cmd)
				if selectedGist, ok := m.gistList.SelectedItem().(*gist); ok {
					for gist, _ := range m.gists {
						if gist.id == selectedGist.id {
							m.fileList.Select(0)
							cmds = append(cmds, m.fileList.SetItems(m.gists[gist]))
							m.fileList.SetSize(20, m.height)
							_, updateFileList := m.fileList.Update(nil)
							cmds = append(cmds, updateFileList)
							break
						}
					}
				}
			case "enter":
				m.next()
				return m, tea.Batch(m.updateActivePane(msg)...)
			default:
			}

		case PANE_FILES:
			switch msg.String() {
			case "up", "down", "j", "k":
				m.fileList, cmd = m.fileList.Update(msg)
				cmds = append(cmds, cmd)

			case "y":
				if m.currentPane != PANE_EDITOR {
					cmds = append(cmds, m.copyToClipboard())
					cmds = append(cmds, m.updateActivePane(msg)...)
					return m, tea.Batch(cmds...)
				}

			// same thing here, trigger cursor blink on editor on select
			case "enter":
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
			switch msg.String() {
			case "ctrl+s":
				m.editor.Blur()
				m.previous()

				msg := m.editor.GetCurrentContent()

				cmds = append(cmds, m.saveFileContent(string(msg))...)
				cmds = append(cmds, m.updateActivePane(msg)...)
				return m, tea.Batch(cmds...)
			}
			m.editor.Focus()
			editorModel, cmd := m.editor.Update(msg)
			cmds = append(cmds, cmd)
			m.editor = editorModel.(editor.Model)
			cmds = append(cmds, m.updateActivePane(msg)...)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 2

		gv, _ := m.GistsStyle.Base.GetFrameSize()
		m.gistList.SetSize(45, m.height)

		fv, _ := m.FilesStyle.Base.GetFrameSize()
		m.fileList.SetSize(20, m.height)

		m.resetListHeight()

		m.editor.SetSize(m.width-fv-gv, m.height+2)
	default:
	}

	return m, tea.Batch(cmds...)
}

type dialogStateChangeMsg dialogState

func (m *mainModel) updateActivePane(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.help.ShowAll = false
	m.resetListHeight()

	switch m.currentPane {
	case PANE_GISTS:
		m.GistsStyle = DefaultStyles().Gists.Focused
		m.FilesStyle = DefaultStyles().Files.Blurred
		m.editor.Blur()

		editorModel, updateEditorModel := m.editor.Update(msg)
		cmds = append(cmds, updateEditorModel)
		m.editor = editorModel.(editor.Model)

		m.gistList, cmd = m.gistList.Update(msg)
		cmds = append(cmds, cmd)
		cmds = append(cmds, func() tea.Msg {
			return dialogStateChangeMsg(dialog_closed)
		})
	case PANE_FILES:
		m.GistsStyle = DefaultStyles().Gists.Blurred
		m.FilesStyle = DefaultStyles().Files.Focused
		m.editor.Blur()

		editorModel, updateEditorModel := m.editor.Update(msg)
		cmds = append(cmds, updateEditorModel)
		m.editor = editorModel.(editor.Model)

		cmds = append(cmds, func() tea.Msg {
			return dialogStateChangeMsg(dialog_closed)
		})
	case PANE_EDITOR:
		m.GistsStyle = DefaultStyles().Gists.Blurred
		m.FilesStyle = DefaultStyles().Files.Blurred
		m.editor.Focus()
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
	var infoView string
	if m.infoMsg != nil && len(m.infoMsg.msg) > 0 {
		var str string
		switch m.infoMsg.variant {
		case info_default:
			str = fmt.Sprintf("\uea74  %s", m.infoMsg.msg)
		case info_error:
			str = fmt.Sprintf("\ue654  %s", m.infoMsg.msg)
		}
		infoView = DefaultStyles().InfoLabel.Render(str)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.JoinHorizontal(
				lipgloss.Left,
				m.gistList.View(),
				m.fileList.View(),
			),
			infoView,
			lipgloss.NewStyle().Render(m.help.View(m.keymap)),
		),
		m.editor.View(),
	)
}
