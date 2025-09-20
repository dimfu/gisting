package main

import (
	"github.com/charmbracelet/lipgloss"
)

type FilesStyle struct {
	Focused FilesBaseStyle
	Blurred FilesBaseStyle
}

type GistsStyle struct {
	Focused GistsBaseStyle
	Blurred GistsBaseStyle
}

type DialogStyle struct {
	Container        lipgloss.Style
	Base             lipgloss.Style
	FocusedTitle     lipgloss.Style
	BlurredTitle     lipgloss.Style
	UnselectedOption lipgloss.Style
	FieldFocused     lipgloss.Style
	FocusedButton    lipgloss.Style
	BlurredButton    lipgloss.Style
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

type Styles struct {
	Files  FilesStyle
	Gists  GistsStyle
	Dialog DialogStyle
}

func DefaultStyles() Styles {
	white := lipgloss.Color("#ffffff") // white color
	gray := lipgloss.Color("241")      // gray color
	black := lipgloss.Color("235")     // background color
	// brightBlack := lipgloss.Color("#373b41")
	// green := lipgloss.Color("#527251")
	// brightGreen := lipgloss.Color("#bce1af")
	brightBlue := lipgloss.Color("#afbee1") // primary color
	blue := lipgloss.Color("#64708d")       // primary color subdued
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
		Dialog: DialogStyle{
			Container:        lipgloss.NewStyle().Align(lipgloss.Center, lipgloss.Center).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(gray)).Padding(1, 2),
			Base:             lipgloss.NewStyle().PaddingLeft(1).BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).BorderForeground(gray),
			FocusedTitle:     lipgloss.NewStyle().Foreground(white),
			BlurredTitle:     lipgloss.NewStyle().Foreground(gray),
			UnselectedOption: lipgloss.NewStyle().Foreground(gray),
			FieldFocused:     lipgloss.NewStyle().PaddingLeft(1).BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).Foreground(lipgloss.Color(gray)),
			FocusedButton:    lipgloss.NewStyle().Padding(0, 2).MarginRight(1).Foreground(lipgloss.Color("0")).Background(blue),
			BlurredButton:    lipgloss.NewStyle().Padding(0, 2).MarginRight(1).Foreground(lipgloss.Color("7")).Background(lipgloss.Color("0")),
		},
	}
}
