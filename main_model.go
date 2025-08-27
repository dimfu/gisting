package main

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	editor "github.com/ionut-t/goeditor/adapter-bubbletea"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v74/github"
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

	keymap       Keymap
	shutdown     chan os.Signal
	githubClient *github.Client

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

func newMainModel(shutdown chan os.Signal, githubClient *github.Client) mainModel {
	defaultStyle := DefaultStyles()
	m := mainModel{
		gists:        map[gist][]list.Item{},
		githubClient: githubClient,
		shutdown:     shutdown,
		keymap:       DefaultKeymap,
		help:         help.New(),
		currentPane:  PANE_GISTS,
		GistsStyle:   defaultStyle.Gists.Focused,
		FilesStyle:   defaultStyle.Files.Blurred,
	}

	if err := m.getGists(); err != nil {
		panic(fmt.Sprintf("Could not get gists on initial start up: %v", err))
	}

	// populate gist list
	var firstgist *gist
	gistFiles := []list.Item{}

	// sort gist alphabetically
	sortedGists := slices.Collect(maps.Keys(m.gists))
	slices.SortFunc(sortedGists, func(a, b gist) int {
		return strings.Compare(a.name, b.name)
	})

	for _, g := range sortedGists {
		if firstgist == nil {
			firstgist = &g
		}
		gistFiles = append(gistFiles, gist{id: g.id, name: g.name})
	}

	m.gistList = newGistList(gistFiles, m.GistsStyle)
	m.fileList = newFileList(m.gists[*firstgist], m.FilesStyle)

	// dont care about the width and height because we set it inside the tea.WindowSizeMsg
	textEditor := editor.New(0, 0)
	textEditor.ShowMessages(true)
	textEditor.SetCursorBlinkMode(true)
	textEditor.SetLanguage("go", "nord")

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

func (m *mainModel) getGists() error {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	gists, _, err := m.githubClient.Gists.List(ctx, "", &github.GistListOptions{
		ListOptions: github.ListOptions{
			// TODO: should we handle per page pagination or not?
			PerPage: 100,
		},
	})
	if err != nil {
		return err
	}

	docs := []*document.Document{}
	for _, g := range gists {
		items := []list.Item{}
		for _, f := range g.GetFiles() {
			i := file{
				title:     f.GetFilename(),
				desc:      g.GetDescription(),
				rawUrl:    f.GetRawURL(),
				updatedAt: g.GetUpdatedAt().String(),
				stale:     false,
			}

			existing, err := storage.db.FindFirst(
				query.NewQuery(string(collectionGistContent)).Where(query.Field("rawUrl").Eq(i.rawUrl)),
			)
			if err != nil {
				continue
			}

			if existing != nil {
				// if existing data is unchanged (based on the date time) skip db operations since there is nothing to change
				existingUA, _ := existing.Get("updatedAt").(string)
				if existingUA >= i.updatedAt {
					items = append(items, i)
					continue
				}
				i.stale = true
				existing.Set("updatedAt", i.updatedAt)
				if err := storage.db.Save(string(collectionGists), existing); err != nil {
					return fmt.Errorf("failed to update gist: %w", err)
				}
			} else {
				doc := document.NewDocument()
				doc.SetAll(map[string]any{
					"title":     i.title,
					"desc":      i.desc,
					"rawUrl":    i.rawUrl,
					"updatedAt": i.updatedAt,
				})
				docs = append(docs, doc)
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
			query.NewQuery(string(collectionDraftedFiles)).Where(query.Field("gist_id").Eq(gistId)),
		)

		if err != nil {
			return err
		}

		items := []list.Item{}
		for _, doc := range fileDocs {
			i := file{
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

	// insert new gist records into the collectiion
	if len(docs) > 0 {
		if err := storage.db.Insert(string(collectionGists), docs...); err != nil {
			return err
		}
	}

	return nil
}

type updateEditorContent string

func (m *mainModel) loadSelectedFile() tea.Cmd {
	li := m.fileList.SelectedItem()
	if li == nil {
		return nil // no command
	}

	f, _ := li.(file)
	content, err := f.getContent()
	if err != nil {
		return func() tea.Msg {
			return errMsg{err: err}
		}
	}

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
		}

	case updateEditorContent:
		m.editor.SetContent(string(msg))
		editorModel, cmd := m.editor.Update(msg)
		cmds = append(cmds, cmd)
		m.editor = editorModel.(editor.Model)
		cmds = append(cmds, m.updateActivePane(msg)...)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.currentPane != PANE_EDITOR {
				m.shutdown <- syscall.SIGTERM
				return m, tea.Quit
			}
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
				logs = append(logs, m.gistList.SelectedItem())
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

				cmds = append(cmds, m.updateActivePane(msg)...)
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

type dialogStateChangeMsg struct {
	state    dialogState
	gistName string
}

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
		cmds = append(cmds, func() tea.Msg {
			return dialogStateChangeMsg{
				state:    dialog_create_gist,
				gistName: "",
			}
		})
	case PANE_FILES:
		m.GistsStyle = DefaultStyles().Gists.Blurred
		m.FilesStyle = DefaultStyles().Files.Focused
		m.EditorStyle = DefaultStyles().Editor.Blurred
		m.fileList, cmd = m.fileList.Update(msg)
		cmds = append(cmds, cmd)
		cmds = append(cmds, func() tea.Msg {
			selectedItem := m.gistList.SelectedItem()
			selectedGist, ok := selectedItem.(gist)
			if !ok {
				return nil
			}
			return dialogStateChangeMsg{
				state:    dialog_create_file,
				gistName: selectedGist.name,
			}
		})
	case PANE_EDITOR:
		m.GistsStyle = DefaultStyles().Gists.Blurred
		m.FilesStyle = DefaultStyles().Files.Blurred
		m.EditorStyle = DefaultStyles().Editor.Focused
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
