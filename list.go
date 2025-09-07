package main

import (
	"fmt"
	"io"
	"time"

	"github.com/aquilax/truncate"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dustin/go-humanize"
)

type gistStatus int64

const (
	gist_status_drafted gistStatus = iota
	gist_status_published
)

type gist struct {
	id        string     `clover:"id"`
	name      string     `clover:"name"`
	status    gistStatus `clover:"status"`
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

// Spacing is the number of lines to insert between folder items.
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
	return l
}

// Render renders a folder list item.
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
	return nil
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
	return l
}
