package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/aquilax/truncate"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dustin/go-humanize"
	"github.com/ostafen/clover/v2/query"
)

type gistStatus int64

const (
	gist_status_drafted gistStatus = iota
	gist_status_published
)

type gistVisibility int64

const (
	gist_public gistVisibility = iota
	gist_secret
)

type gist struct {
	id        string         `clover:"id"`
	name      string         `clover:"name"`
	status    gistStatus     `clover:"status"`
	visiblity gistVisibility `clover:"visibility"`
	updatedAt time.Time
}

func (f gist) FilterValue() string {
	return f.name
}

type gistsDelegate struct {
	styles GistsBaseStyle
}

func (d gistsDelegate) Height() int {
	return 2
}

func (d gistsDelegate) Spacing() int {
	return 1
}

func (d gistsDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func newGistList(items []list.Item, styles GistsBaseStyle) list.Model {
	l := list.New(items, gistsDelegate{styles: styles}, 45, 0)
	l.Title = "Gists                               " // THIS I STILL DONT KNOW HOW TO FIX LOL
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = styles.Title
	l.Styles.TitleBar = styles.TitleBar
	l.Styles.NoItems = styles.NoItems
	l.InfiniteScrolling = true
	return l
}

func (d gistsDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	g, ok := item.(*gist)
	if !ok {
		return
	}

	var label string

	if g.status == gist_status_drafted {
		// truncate *only* the name, then append (Draft)
		truncated := truncate.Truncate(g.name, 25, "...", truncate.PositionEnd)
		label = "→ " + truncated + " (Draft)"
	} else {
		truncated := truncate.Truncate(g.name, 30, "...", truncate.PositionEnd)
		label = "→ " + truncated
	}

	style := d.styles.Unselected
	if index == m.Index() {
		style = d.styles.Selected
	}

	attribute := d.styles.Unselected
	lastUpdated := fmt.Sprintf("Last updated: %s", humanize.Time(g.updatedAt))

	fmt.Fprint(w, "  "+style.Render(label)+"\n    "+attribute.Render(lastUpdated))
}

type file struct {
	id        string `clover:"id"`
	gistId    string `clover:"gistId"`
	title     string `clover:"title"`
	desc      string `clover:"desc"`
	rawUrl    string `clover:"rawUrl"`
	updatedAt string `clover:"updatedAt"`
	content   string `clover:"content"`
	draft     bool   `clover:"draft"`
	stale     bool
}

func (f file) Title() string       { return f.title }
func (f file) Description() string { return f.desc }
func (f file) FilterValue() string { return f.title }

func (f file) getContent() (string, error) {
	if f.draft {
		return f.content, nil
	}

	existing, err := storage.db.FindFirst(
		query.NewQuery(string(collectionGistContent)).Where(query.Field("rawUrl").Eq(f.rawUrl).And(query.Field("id").Eq(f.id))),
	)

	if err != nil {
		log.Errorln(err)
		return "", err
	}

	if existing == nil {
		log.Errorf("Could not find %q with id %q and rawUrl %q\n", f.title, f.id, f.rawUrl)
		return "", nil
	}

	var shouldFetch bool
	existingUA, _ := existing.Get("updatedAt").(string)
	shouldFetch = f.updatedAt > existingUA

	if _, ok := existing.Get("content").(string); !ok {
		shouldFetch = true
	}

	if shouldFetch {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(f.rawUrl)
		if err != nil {
			log.Errorf("Could not fetch file with raw url: %s", f.rawUrl)
			return "", err
		}
		defer resp.Body.Close()

		contentBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Errorf(err.Error())
			return "", err
		}

		content := string(contentBytes)

		existing.Set("content", content)

		if err := storage.db.Save(string(collectionGistContent), existing); err != nil {
			log.Errorf(err.Error())
			return "", err
		}

		return content, nil
	} else {
		if cachedContent, ok := existing.Get("content").(string); ok {
			return cachedContent, nil
		}
		return "", fmt.Errorf("no cached content available")
	}
}

type filesDelegate struct {
	styles FilesBaseStyle
}

func (d filesDelegate) Height() int {
	return 2
}

func (d filesDelegate) Spacing() int {
	return 0
}

func (d filesDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	// if there is no item inside gist item, render empty content instead
	if m.SelectedItem() == nil {
		return func() tea.Msg {
			return updateEditorContent{content: "", language: "text"}
		}
	}

	f, _ := m.SelectedItem().(file)
	content, err := f.getContent()
	if err != nil {
		return func() tea.Msg {
			return showInfo(err.Error(), info_error)
		}
	}

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

	return func() tea.Msg {
		return updateEditorContent{content: content, language: alias}
	}
}

func (d filesDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if item == nil {
		return
	}
	s, ok := item.(file)
	if !ok {
		return
	}

	var title string
	if index == m.Index() {
		title = d.styles.SelectedTitle.Render(s.Title())
	} else {
		title = d.styles.UnselectedTitle.Render(s.Title())
	}

	fmt.Fprintln(w, "  "+title)
}

func newFileList(items []list.Item, styles FilesBaseStyle) list.Model {
	l := list.New(items, filesDelegate{styles: styles}, 25, 0)
	l.Title = "Files"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = styles.Title
	l.Styles.TitleBar = styles.TitleBar
	l.Styles.NoItems = styles.NoItems
	l.InfiniteScrolling = true
	return l
}
