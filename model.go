package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
)

var (
	docStyle    = lipgloss.NewStyle().Margin(1, 2)
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
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238"))

	blurredBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.HiddenBorder())
)

func newTextarea() textarea.Model {
	t := textarea.New()
	t.Prompt = ""
	t.Placeholder = "Type something"
	t.ShowLineNumbers = true
	t.Cursor.Style = cursorStyle
	t.FocusedStyle.Placeholder = focusedPlaceholderStyle
	t.BlurredStyle.Placeholder = placeholderStyle
	t.FocusedStyle.CursorLine = cursorLineStyle
	t.FocusedStyle.Base = focusedBorderStyle
	t.BlurredStyle.Base = blurredBorderStyle
	t.FocusedStyle.EndOfBuffer = endOfBufferStyle
	t.BlurredStyle.EndOfBuffer = endOfBufferStyle
	t.KeyMap.DeleteWordBackward.SetEnabled(false)
	t.KeyMap.LineNext = key.NewBinding(key.WithKeys("down"))
	t.KeyMap.LinePrevious = key.NewBinding(key.WithKeys("up"))
	t.Blur()
	return t
}

type model struct {
	keymap  keymap
	closeCh chan os.Signal
	github  *github.Client

	terminalWidth  int
	terminalHeight int

	// tui area
	list     list.Model
	textarea textarea.Model
}

type item struct {
	title, desc, content string
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
				key.WithKeys("esc", "ctrl+c"),
				key.WithHelp("esc", "quit"),
			),
		},
	}

	items, err := m.populateList()
	if err != nil {
		panic("Could not populate gists on initial start up")
	}

	m.list = list.New(items, list.NewDefaultDelegate(), 0, 0)
	m.textarea = newTextarea()

	return m
}

func contentFromRawUrl(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	contentBytes, _ := io.ReadAll(resp.Body)
	content := string(contentBytes)
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
	for _, gist := range gists {
		for _, f := range gist.GetFiles() {
			// note: had to extract gist content with the raw url because f.GetContent()
			// doesn't return anything for some reason...
			content, err := contentFromRawUrl(httpClient, f.GetRawURL())
			if err != nil {
				continue
			}
			items = append(items,
				item{title: f.GetFilename(), desc: gist.GetDescription(), content: content},
			)
		}
	}
	return items, nil
}

// tui lifecycles
func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.closeCh <- syscall.SIGTERM
			return m, tea.Quit
		case "ctrl+z":
			return m, tea.Suspend
		case "esc":
			if m.textarea.Focused() {
				m.textarea.Blur()
				return m, nil
			}
		case "m":
			if !m.textarea.Focused() {
				cmd = m.textarea.Focus()
				cmds = append(cmds, cmd)
			}
		case "enter":
			if !m.textarea.Focused() {
				if selected := m.list.SelectedItem(); selected != nil {
					if it, ok := selected.(item); ok {
						m.textarea.SetValue(it.content)
						cmd = m.textarea.Focus()
						cmds = append(cmds, cmd)
					}
				}
			}
		}
	case tea.WindowSizeMsg:
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height

		h, v := docStyle.GetFrameSize()
		availableWidth := msg.Width - h
		leftWidth := availableWidth / 2
		rightWidth := availableWidth - leftWidth

		// Set sizes for both components
		m.list.SetSize(leftWidth, msg.Height-v)
		m.textarea.SetWidth(rightWidth)
		m.textarea.SetHeight(msg.Height - v)
	}

	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	// Get the current terminal size
	width, height := m.getTerminalSize()

	// Calculate 50% width for each side (accounting for margins/padding)
	h, v := docStyle.GetFrameSize()
	availableWidth := width - h
	leftWidth := availableWidth / 2
	rightWidth := availableWidth - leftWidth // Handle odd widths

	// Set the list size to use half the available space
	m.list.SetSize(leftWidth, height-v)

	// Set the textarea size explicitly
	m.textarea.SetWidth(rightWidth)
	m.textarea.SetHeight(height - v)

	left := docStyle.Render(m.list.View())
	right := m.textarea.View()

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m model) getTerminalSize() (int, int) {
	return m.terminalWidth, m.terminalHeight
}
