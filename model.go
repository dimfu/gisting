package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	editor "github.com/ionut-t/goeditor/adapter-bubbletea"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v74/github"
	"github.com/ostafen/clover"
	"golang.org/x/oauth2"
)

var (
	// editor styles
	focusedBorderStyle = lipgloss.NewStyle().Margin(0, 2, 0, 0).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("215"))

	blurredBorderStyle = lipgloss.NewStyle().Margin(0, 2, 0, 0).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("237"))

	normalModeStyle = lipgloss.NewStyle().Background(lipgloss.Color("#3C3836")).Foreground(lipgloss.Color("255"))
	insertModeStyle = lipgloss.NewStyle().Background(lipgloss.Color("26")).Foreground(lipgloss.Color("255"))
	visualModeStyle = lipgloss.NewStyle().Background(lipgloss.Color("127")).Foreground(lipgloss.Color("255"))
	statusLineStyle = lipgloss.NewStyle().Background(lipgloss.Color("#3c3836"))
	lineNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3c3836")).PaddingLeft(2)
)

var (
	bindings = []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev")),
		key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
)

var (
	gruvboxBg     = lipgloss.Color("#282828")
	gruvboxFg     = lipgloss.Color("#ebdbb2")
	gruvboxGray   = lipgloss.Color("#928374")
	gruvboxYellow = lipgloss.Color("#fabd2f")
	gruvboxBlue   = lipgloss.Color("#83a598")
	gruvboxGreen  = lipgloss.Color("#b8bb26")
	gruvboxRed    = lipgloss.Color("#fb4934")
)

type pane int

const MAX_PANE = 3

const (
	PANE_GISTS pane = iota
	PANE_FILES
	PANE_EDITOR
)

type model struct {
	gists map[gist][]list.Item

	keymap  Keymap
	closeCh chan os.Signal
	github  *github.Client

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

func newGistList(items []list.Item, styles GistsBaseStyle) list.Model {
	l := list.New(items, gistsDelegate{styles: styles}, 0, 0)
	l.Title = "My Gists"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = styles.Title
	l.Styles.TitleBar = styles.TitleBar
	return l
}

func newFileList(items []list.Item, styles FilesBaseStyle) list.Model {
	l := list.New(items, filesDelegate{styles: styles}, 0, 0)
	l.Title = "Files"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = styles.Title
	l.Styles.TitleBar = styles.TitleBar
	return l
}

func newModel(githubclient *github.Client, closech chan os.Signal) model {
	defaultStyle := DefaultStyles()
	m := model{
		gists:       map[gist][]list.Item{},
		github:      githubclient,
		closeCh:     closech,
		keymap:      DefaultKeymap,
		help:        help.New(),
		currentPane: PANE_GISTS,
		GistsStyle:  defaultStyle.Gists.Focused,
		FilesStyle:  defaultStyle.Files.Blurred,
	}

	if err := m.getGists(); err != nil {
		panic("Could not get gists on initial start up")
	}

	// populate gist list
	var firstgist *gist
	gistFiles := []list.Item{}
	for g := range m.gists {
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
	textEditor.SetLanguage("go", "gruvbox")
	textEditor.HideStatusLine(true)
	textEditor.WithTheme(editor.Theme{
		NormalModeStyle: normalModeStyle,
		InsertModeStyle: insertModeStyle,
		VisualModeStyle: visualModeStyle,
		LineNumberStyle: lineNumberStyle,
	})

	m.editor = textEditor

	return m
}

func (m *model) next() {
	m.currentPane = (m.currentPane + 1) % MAX_PANE
}

func (m *model) previous() {
	m.currentPane--
	if m.currentPane < 0 {
		m.currentPane = MAX_PANE - 1
	}
}

func (m *model) getGists() error {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	gists, _, err := m.github.Gists.List(ctx, "", &github.GistListOptions{
		ListOptions: github.ListOptions{
			// TODO: should we handle per page pagination or not?
			PerPage: 100,
		},
	})
	if err != nil {
		return err
	}

	docs := []*clover.Document{}
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

			existing, err := storage.db.Query(string(collectionGists)).Where(clover.Field("rawUrl").Eq(i.rawUrl)).FindFirst()
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
				doc := clover.NewDocument()
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
			name: g.GetDescription(),
			id:   g.GetID(),
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

func (m model) Init() tea.Cmd {
	return m.editor.CursorBlink()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case editor.SaveMsg:
		if m.currentPane == PANE_EDITOR {
			m.editor.Blur()
			m.previous()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.currentPane != PANE_EDITOR {
				m.closeCh <- syscall.SIGTERM
				return m, tea.Quit
			}
		case "ctrl+z":
			return m, tea.Suspend
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
				if selected := m.gistList.SelectedItem(); selected != nil {
					if selectedGist, ok := selected.(gist); ok {
						for gist, files := range m.gists {
							if gist.id == selectedGist.id {
								items := make([]list.Item, len(files))
								for i, item := range files {
									logs = append(logs, item)
									items[i] = item
								}
								return m, m.fileList.SetItems(items)
							}
						}
					}
				}
			case "enter":
				m.next()
			default:
				cmds = append(cmds, m.updateActivePane(msg)...)
			}

		case PANE_FILES:
			switch msg.String() {
			case "up", "down", "j", "k":
				m.fileList, cmd = m.fileList.Update(msg)
				cmds = append(cmds, cmd)
			case "enter", "ctrl+l":
				if selected := m.fileList.SelectedItem(); selected != nil {
					if f, ok := selected.(file); ok {
						content, err := f.content()
						if err == nil {
							m.editor.SetContent(content)
							m.next()
							// hack to rerender the whole app and show the editor's content
							return m, func() tea.Msg {
								return tea.KeyMsg{
									Type:  tea.KeyRunes,
									Runes: []rune{},
								}
							}
						}
					}
				}
			default:
				cmds = append(cmds, m.updateActivePane(msg)...)
			}

		case PANE_EDITOR:
			m.GistsStyle = DefaultStyles().Gists.Blurred
			m.EditorStyle = DefaultStyles().Editor.Focused
			m.FilesStyle = DefaultStyles().Files.Blurred

			m.editor.Focus()

			editorModel, cmd := m.editor.Update(msg)
			cmds = append(cmds, cmd)
			m.editor = editorModel.(editor.Model)
			cmds = append(cmds, m.updateActivePane(msg)...)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4

		gistListWidth := m.width * 20 / 100
		fileListWidth := m.width * 20 / 100
		editorWidth := (m.width * 60 / 100)

		logs = append(logs, m.width)

		m.gistList.SetWidth(gistListWidth)
		m.gistList.SetHeight(m.height)

		m.fileList.SetWidth(fileListWidth)
		m.fileList.SetHeight(m.height)

		m.editor.SetSize(editorWidth, m.height-focusedBorderStyle.GetVerticalFrameSize()-1)
	default:
	}

	return m, tea.Batch(cmds...)
}

func (m *model) updateActivePane(msg tea.Msg) []tea.Cmd {
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
		m.editor.Focus()
		editorModel, cmd := m.editor.Update(msg)
		cmds = append(cmds, cmd)
		m.editor = editorModel.(editor.Model)
	}

	m.gistList.Styles.TitleBar = m.GistsStyle.TitleBar
	m.gistList.Styles.Title = m.GistsStyle.Title

	m.fileList.Styles.TitleBar = m.FilesStyle.TitleBar
	m.fileList.Styles.Title = m.FilesStyle.Title

	return cmds
}

func (m model) View() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.GistsStyle.Base.Render(m.gistList.View()),
			m.FilesStyle.Base.Render(m.fileList.View()),
			blurredBorderStyle.Render(m.editor.View()),
		),
		lipgloss.NewStyle().MarginLeft(2).Render(m.help.View(m.keymap)),
	)
}
