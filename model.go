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

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v74/github"
	"github.com/ostafen/clover"
	"golang.org/x/oauth2"
)

var (
	listStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).Margin(0, 2, 0, 2)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	cursorLineStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("57")).
			Foreground(lipgloss.Color("230"))

	placeholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238"))

	endOfBufferStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("235"))

	focusedPlaceholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("99"))

	focusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("238")).Margin(0, 2, 0, 0)

	blurredBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.HiddenBorder()).Margin(0, 2, 0, 0)
)

type model struct {
	keymap  keymap
	closeCh chan os.Signal
	github  *github.Client

	width  int
	height int

	// tui area
	list   list.Model
	editor editor.Model
}

type item struct {
	title     string `clover:"title"`
	desc      string `clover:"desc"`
	rawUrl    string `clover:"rawUrl"`
	updatedAt string `clover:"updatedAt"`

	// to indicate if the gist is just updated or not
	stale bool
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type keymap = struct {
	next, prev, add, remove, quit key.Binding
}

func newModel(githubclient *github.Client, closech chan os.Signal) model {
	m := model{
		github:  githubclient,
		closeCh: closech,
		keymap: keymap{
			next: key.NewBinding(
				key.WithKeys("tab"),
				key.WithHelp("tab", "next"),
			),
			prev: key.NewBinding(
				key.WithKeys("shift+tab"),
				key.WithHelp("shift+tab", "prev"),
			),
			quit: key.NewBinding(
				key.WithKeys("ctrl+c"),
				key.WithHelp("ctrl+c", "quit"),
			),
		},
	}

	items, err := m.populateList()
	if err != nil {
		panic("Could not populate gists on initial start up")
	}

	m.list = list.New(items, list.NewDefaultDelegate(), 0, 0)

	// dont care about the width and height because we set it inside the tea.WindowSizeMsg
	textEditor := editor.New(0, 0)
	textEditor.ShowMessages(true)
	textEditor.SetCursorBlinkMode(true)
	textEditor.SetLanguage("go", "catppuccin-mocha")

	t := []editor.Theme{}

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

func (m *model) populateList() ([]list.Item, error) {
	items := []list.Item{}
	httpClient := &http.Client{Timeout: 5 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	gists, _, err := m.github.Gists.List(ctx, "", &github.GistListOptions{
		ListOptions: github.ListOptions{
			// TODO: should we handle per page pagination or not?
			PerPage: 100,
		},
	})
	if err != nil {
		return items, err
	}

	docs := []*clover.Document{}
	for _, gist := range gists {
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
					return items, fmt.Errorf("failed to update gist: %w", err)
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

			// append to items array for the list user interface
			items = append(items, i)
		}
	}

	// insert new gist records into the collectiion
	if len(docs) > 0 {
		if err := storage.db.Insert(string(collectionGists), docs...); err != nil {
			return items, err
		}
	}

	return items, nil
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
		if m.editor.IsFocused() {
			m.editor.Blur()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if !m.editor.IsFocused() {
				m.closeCh <- syscall.SIGTERM
				return m, tea.Quit
			}
		case "ctrl+h":
			m.editor.Blur()
		case "ctrl+z":
			return m, tea.Suspend
		case "enter", "ctrl+l":
			if !m.editor.IsFocused() {
				if selected := m.list.SelectedItem(); selected != nil {
					if it, ok := selected.(item); ok {
						content, err := contentFromRawUrl(it)
						if err == nil {
							m.editor.SetContent(content)
							m.editor.Focus()
						}
					}
				}
			}
		}

		// navigation handler for the list
		if m.editor.IsFocused() {
			editorModel, cmd := m.editor.Update(msg)
			cmds = append(cmds, cmd)
			m.editor = editorModel.(editor.Model)
		} else {
			switch msg.String() {
			case "up", "down", "j", "k":
				m.list, cmd = m.list.Update(msg)
				cmds = append(cmds, cmd)
			default:
				m.list, cmd = m.list.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		listWidth := m.width * 20 / 100
		editorWidth := (m.width * 80 / 100) - 10

		m.list.SetWidth(listWidth)
		m.list.SetHeight(m.height - listStyle.GetVerticalFrameSize())

		m.editor.SetSize(editorWidth, m.height-focusedBorderStyle.GetVerticalFrameSize())

	default:
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		listStyle.Render(m.list.View()),
		focusedBorderStyle.Render(m.editor.View()),
	)
}
