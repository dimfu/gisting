package main

import (
	"context"
	"fmt"
	"io"
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
	listStyleBlurred = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("237")).Margin(0, 2, 0, 2)

	listStyleFocused = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("215")).Margin(0, 2, 0, 2)

	// editor styles
	focusedBorderStyle = lipgloss.NewStyle().Margin(0, 2, 0, 0)

	blurredBorderStyle = lipgloss.NewStyle().Margin(0, 2, 0, 0)

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

type giststruct struct {
	name string
	id   string
}

func (i giststruct) FilterValue() string { return i.name }

type model struct {
	gists map[giststruct][]list.Item

	keymap  keymap
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
}

type keymap struct {
	left  key.Binding
	right key.Binding
	quit  key.Binding
}

func (k keymap) ShortHelp() []key.Binding {
	return []key.Binding{k.left, k.right, k.quit}
}

func (k keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.left, k.right},
		{k.quit},
	}
}

type item struct {
	title     string `clover:"title"`
	desc      string `clover:"desc"`
	rawUrl    string `clover:"rawUrl"`
	updatedAt string `clover:"updatedAt"`

	stale bool
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

func newList(items []list.Item) list.Model {
	delegate := list.NewDefaultDelegate()

	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(gruvboxYellow).
		Background(gruvboxBg).
		Bold(true)

	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(gruvboxFg).
		Background(gruvboxBg)

	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(gruvboxFg)

	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.
		Foreground(gruvboxGray)

	l := list.New(items, delegate, 0, 0)
	l.Title = "My Gists"
	l.Styles.Title = lipgloss.NewStyle().
		Width(35).
		Foreground(gruvboxBlue).
		Background(gruvboxBg).
		Bold(true).
		Padding(0, 1)

	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	return l
}

func newModel(githubclient *github.Client, closech chan os.Signal) model {
	m := model{
		gists:   map[giststruct][]list.Item{},
		github:  githubclient,
		closeCh: closech,
		keymap: keymap{
			left: key.NewBinding(
				key.WithKeys("ctrl+h"),
				key.WithHelp("ctrl+h", "left pane"),
			),
			right: key.NewBinding(
				key.WithKeys("ctrl+l"),
				key.WithHelp("ctrl+l", "right pane"),
			),
			quit: key.NewBinding(
				key.WithKeys("ctrl+c"),
				key.WithHelp("ctrl+c", "quit"),
			),
		},
		help:        help.New(),
		currentPane: PANE_GISTS,
	}

	if err := m.getGists(); err != nil {
		panic("Could not get gists on initial start up")
	}

	// populate gist list
	var firstgist *giststruct
	gistItems := []list.Item{}
	for gist := range m.gists {
		if firstgist == nil {
			firstgist = &gist
		}
		gistItems = append(gistItems, item{title: gist.name, desc: gist.id})
	}
	m.gistList = newList(gistItems)
	m.fileList = newList(m.gists[*firstgist])

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

func contentFromRawUrl(it item) (string, error) {
	var content string

	existing, err := storage.db.Query(string(collectionGistContent)).
		Where(clover.Field("rawUrl").Eq(it.rawUrl)).
		FindFirst()
	if err != nil {
		logs = append(logs, err.Error())
		return "", err
	}

	if it.stale || existing == nil {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(it.rawUrl)
		if err != nil {
			logs = append(logs, err.Error())
			return "", err
		}
		defer resp.Body.Close()

		contentBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logs = append(logs, err.Error())
			return "", err
		}
		content = string(contentBytes)
		logs = append(logs, fmt.Sprintf("stale %s", content))

		if existing == nil {
			existing = clover.NewDocument()
			existing.Set("rawUrl", it.rawUrl)
		}

		existing.SetAll(map[string]any{
			"title":     it.title,
			"desc":      it.desc,
			"rawUrl":    it.rawUrl,
			"updatedAt": it.updatedAt,
			"content":   content,
		})

		if err := storage.db.Save(string(collectionGistContent), existing); err != nil {
			logs = append(logs, err.Error())
			return "", err
		}
	} else {
		if val, ok := existing.Get("content").(string); ok {
			content = val
		}
	}

	return content, nil
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
	for _, gist := range gists {
		items := []list.Item{}
		for _, f := range gist.GetFiles() {
			i := item{
				title:     f.GetFilename(),
				desc:      gist.GetDescription(),
				rawUrl:    f.GetRawURL(),
				updatedAt: gist.GetUpdatedAt().String(),
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
				// TODO: should update the content of this gist
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
		g := giststruct{
			name: gist.GetDescription(),
			id:   gist.GetID(),
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

// tui lifecycles
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
			return m, nil
		case "ctrl+l":
			if m.currentPane != PANE_FILES {
				m.next()
				return m, nil
			}
		}

		switch m.currentPane {
		case PANE_GISTS:
			switch msg.String() {
			case "up", "down", "j", "k":
				m.gistList, cmd = m.gistList.Update(msg)
				cmds = append(cmds, cmd)
				if selected := m.gistList.SelectedItem(); selected != nil {
					if it, ok := selected.(item); ok {
						for gist, files := range m.gists {
							if gist.id == it.desc {
								m.fileList = newList(files)
								fileListWidth := m.width * 20 / 100
								m.fileList.SetWidth(fileListWidth)
								m.fileList.SetHeight(m.height - listStyleBlurred.GetVerticalFrameSize() - 1)
								break
							}
						}
					}
				}
			case "enter":
				m.next()
			default:
				m.gistList, cmd = m.gistList.Update(msg)
				cmds = append(cmds, cmd)
			}

		case PANE_FILES:
			switch msg.String() {
			case "up", "down", "j", "k":
				m.fileList, cmd = m.fileList.Update(msg)
				cmds = append(cmds, cmd)
			case "enter", "ctrl+l":
				if selected := m.fileList.SelectedItem(); selected != nil {
					if fileItem, ok := selected.(item); ok {
						content, err := contentFromRawUrl(fileItem)
						if err == nil {
							m.editor.SetContent(content)
							m.next()
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
				m.fileList, cmd = m.fileList.Update(msg)
				cmds = append(cmds, cmd)
			}

		case PANE_EDITOR:
			m.editor.Focus()
			editorModel, cmd := m.editor.Update(msg)
			cmds = append(cmds, cmd)
			m.editor = editorModel.(editor.Model)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		gistListWidth := m.width * 20 / 100
		fileListWidth := m.width * 20 / 100
		editorWidth := (m.width * 60 / 100) - 10

		m.gistList.SetWidth(gistListWidth)
		m.gistList.SetHeight(m.height - listStyleBlurred.GetVerticalFrameSize() - 1)

		m.fileList.SetWidth(fileListWidth)
		m.fileList.SetHeight(m.height - listStyleBlurred.GetVerticalFrameSize() - 1)

		m.editor.SetSize(editorWidth, m.height-focusedBorderStyle.GetVerticalFrameSize()-1)
	default:
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	var gistList, fileList, editor string

	// Style components based on current pane focus
	switch m.currentPane {
	case PANE_GISTS:
		gistList = listStyleFocused.Render(m.gistList.View())
		fileList = listStyleBlurred.Render(m.fileList.View())
		editor = blurredBorderStyle.Render(m.editor.View())
	case PANE_FILES:
		gistList = listStyleBlurred.Render(m.gistList.View())
		fileList = listStyleFocused.Render(m.fileList.View())
		editor = blurredBorderStyle.Render(m.editor.View())
	case PANE_EDITOR:
		gistList = listStyleBlurred.Render(m.gistList.View())
		fileList = listStyleBlurred.Render(m.fileList.View())
		editor = focusedBorderStyle.Render(m.editor.View())
	}

	helpView := lipgloss.NewStyle().MarginLeft(2).Render(m.help.View(m.keymap))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.JoinHorizontal(
			lipgloss.Top,
			gistList,
			fileList,
			editor,
		),
		helpView,
	)
}
