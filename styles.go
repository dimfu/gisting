package main

import "github.com/charmbracelet/lipgloss"

type FilesStyle struct {
	Focused FilesBaseStyle
	Blurred FilesBaseStyle
}

type GistsStyle struct {
	Focused GistsBaseStyle
	Blurred GistsBaseStyle
}

type EditorStyle struct {
	Focused EditorBaseStyle
	Blurred EditorBaseStyle
}

type FilesBaseStyle struct {
	Base               lipgloss.Style
	Title              lipgloss.Style
	TitleBar           lipgloss.Style
	SelectedSubtitle   lipgloss.Style
	UnselectedSubtitle lipgloss.Style
	SelectedTitle      lipgloss.Style
	UnselectedTitle    lipgloss.Style
	NoItems            lipgloss.Style
}

type GistsBaseStyle struct {
	Base       lipgloss.Style
	Title      lipgloss.Style
	TitleBar   lipgloss.Style
	Selected   lipgloss.Style
	Unselected lipgloss.Style
	NoItems    lipgloss.Style
}

type EditorBaseStyle struct {
	Base         lipgloss.Style
	Title        lipgloss.Style
	Separator    lipgloss.Style
	LineNumber   lipgloss.Style
	EmptyHint    lipgloss.Style
	EmptyHintKey lipgloss.Style
}

type Styles struct {
	Files  FilesStyle
	Gists  GistsStyle
	Editor EditorStyle
}

func DefaultStyles() Styles {
	white := lipgloss.Color("#ffffff")
	gray := lipgloss.Color("241")
	black := lipgloss.Color("235")
	// brightBlack := lipgloss.Color("#373b41")
	// green := lipgloss.Color("#527251")
	// brightGreen := lipgloss.Color("#bce1af")
	brightBlue := lipgloss.Color("#afbee1")
	blue := lipgloss.Color("#64708d")
	// red := lipgloss.Color("a46060")
	// brightRed := lipgloss.Color("#e49393")

	return Styles{
		Gists: GistsStyle{
			Focused: GistsBaseStyle{
				Base:       lipgloss.NewStyle().Width(40).Height(1),
				Title:      lipgloss.NewStyle().Padding(0, 1).Foreground(white),
				TitleBar:   lipgloss.NewStyle().Background(blue).Width(40).Margin(0, 0, 1, 0).Padding(0, 1).Height(1),
				Selected:   lipgloss.NewStyle().Foreground(brightBlue),
				Unselected: lipgloss.NewStyle().Foreground(gray),
				NoItems: lipgloss.NewStyle().
					UnsetBackground().
					Foreground(gray).
					Padding(0, 2),
			},
			Blurred: GistsBaseStyle{
				Base:       lipgloss.NewStyle().Width(40).Height(1),
				Title:      lipgloss.NewStyle().Padding(0, 1).Foreground(gray),
				TitleBar:   lipgloss.NewStyle().Background(black).Width(40).Margin(0, 0, 1, 0).Padding(0, 1).Height(1).Height(1),
				Selected:   lipgloss.NewStyle().Foreground(brightBlue),
				Unselected: lipgloss.NewStyle().Foreground(lipgloss.Color("237")),
				NoItems: lipgloss.NewStyle().
					UnsetBackground().
					Foreground(gray).
					Padding(0, 2),
			},
		},
		Files: FilesStyle{
			Focused: FilesBaseStyle{
				Base:               lipgloss.NewStyle().Width(25).Height(1),
				TitleBar:           lipgloss.NewStyle().Background(blue).Width(25).Margin(0, 1, 1, 1).Padding(0, 1).Foreground(white).Height(1),
				SelectedSubtitle:   lipgloss.NewStyle().Foreground(blue),
				UnselectedSubtitle: lipgloss.NewStyle().Foreground(lipgloss.Color("237")),
				SelectedTitle:      lipgloss.NewStyle().Foreground(brightBlue),
				UnselectedTitle:    lipgloss.NewStyle().Foreground(gray),
				NoItems: lipgloss.NewStyle().
					UnsetBackground().
					Foreground(gray).
					Padding(0, 2),
			},
			Blurred: FilesBaseStyle{
				Base:               lipgloss.NewStyle().Width(25).Height(1),
				TitleBar:           lipgloss.NewStyle().Background(black).Width(25).Margin(0, 1, 1, 1).Padding(0, 1).Foreground(gray).Height(1),
				SelectedSubtitle:   lipgloss.NewStyle().Foreground(blue),
				UnselectedSubtitle: lipgloss.NewStyle().Foreground(black),
				SelectedTitle:      lipgloss.NewStyle().Foreground(brightBlue),
				UnselectedTitle:    lipgloss.NewStyle().Foreground(lipgloss.Color("237")),
				NoItems: lipgloss.NewStyle().
					UnsetBackground().
					Foreground(gray).
					Padding(0, 2),
			},
		},
	}
}
