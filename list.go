package main

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type gistStatus int64

const (
	gist_status_drafted gistStatus = iota
	gist_status_published
)

type gist struct {
	id     string     `clover:"id"`
	name   string     `clover:"name"`
	status gistStatus `clover:"status"`
}

func (f gist) FilterValue() string {
	return f.name
}

type gistsDelegate struct {
	list.DefaultDelegate
	styles GistsBaseStyle
}

func (d gistsDelegate) Height() int {
	return 1
}

// Spacing is the number of lines to insert between folder items.
func (d gistsDelegate) Spacing() int {
	return 0
}

func (d gistsDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func newGistList(items []list.Item, styles GistsBaseStyle) list.Model {
	l := list.New(items, gistsDelegate{styles: styles}, 0, 0)
	l.Title = "Gists                               "
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = styles.Title
	l.Styles.TitleBar = styles.TitleBar
	l.Styles.NoItems = styles.NoItems
	return l
}

// Render renders a folder list item.
func (d gistsDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	f, ok := item.(gist)
	if !ok {
		return
	}
	fmt.Fprint(w, "  ")
	if index == m.Index() {
		if f.status == gist_status_drafted {
			fmt.Fprint(w, d.styles.Selected.Render("→ "+f.name+" (Draft)"))
		} else {
			fmt.Fprint(w, d.styles.Selected.Render("→ "+f.name))
		}
		return
	}
	if f.status == gist_status_drafted {
		fmt.Fprint(w, d.styles.Unselected.Render("→ "+f.name+" (Draft)"))
	} else {
		fmt.Fprint(w, d.styles.Unselected.Render("→ "+f.name))
	}
}

type filesDelegate struct {
	list.DefaultDelegate
	styles FilesBaseStyle
}

// Height is the number of lines the snippet list item takes up.
func (d filesDelegate) Height() int {
	return 2
}

// Spacing is the number of lines to insert between list items.
func (d filesDelegate) Spacing() int {
	return 1
}

// Update is called when the list is updated.
// We use this to update the snippet code view.
func (d filesDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.DefaultDelegate.Update(msg, m)
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
	l := list.New(items, filesDelegate{styles: styles}, 0, 0)
	l.Title = "Files"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = styles.Title
	l.Styles.TitleBar = styles.TitleBar
	l.Styles.NoItems = styles.NoItems
	return l
}
