package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/google/go-github/v74/github"
)

type authState int

const (
	auth_loading authState = iota
	auth_prompt_secrets
)

type authModel struct {
	loadingSpinner spinner.Model
	state          authState
	mux            *http.ServeMux
	form           *huh.Form
	width          int
	height         int

	authCtx context.Context
}

type authSuccessMsg struct {
	client *github.Client
}

type needSecretsMsg struct{}

func (m *authModel) authenticate() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		client := github.NewClient(nil).WithAuthToken(cfg.AccessToken)
		user, _, err := client.Users.Get(ctx, "")
		if user == nil {
			return showInfo(err.Error(), info_error)
		}
		return authSuccessMsg{client}
	}
}

func (m authModel) Init() tea.Cmd {
	if cfg.AccessToken == "" {
		return func() tea.Msg { return needSecretsMsg{} }
	}
	return tea.Batch(m.loadingSpinner.Tick, m.authenticate())
}

func (m authModel) View() string {
	switch m.state {
	case auth_prompt_secrets:
		if m.form != nil {
			return m.form.View()
		}
		return "Input required"
	case auth_loading:
		return fmt.Sprintf("%s Loading...", m.loadingSpinner.View())
	default:
		return "this should not happen"
	}
}

func (m authModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
		if m.state == auth_prompt_secrets && m.form != nil {
			var f tea.Model
			f, cmd = m.form.Update(msg)
			if form, ok := f.(*huh.Form); ok {
				m.form = form
			}
			cmds = append(cmds, cmd)

			if m.form.State == huh.StateCompleted {
				return m, func() tea.Msg { return authSuccessMsg{} }
			}
			return m, tea.Batch(cmds...)
		}
	case authSuccessMsg:
		return m, func() tea.Msg {
			return msg
		}
	case needSecretsMsg:
		m.state = auth_prompt_secrets
		m.form = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Github Personal Token").
					Value(&cfg.AccessToken).
					Key("access_token").
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("client id required")
						}
						return nil
					}),
			),
		)
		if m.width > 0 && m.height > 0 {
			var f tea.Model
			f, _ = m.form.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			if form, ok := f.(*huh.Form); ok {
				m.form = form
			}
		}
		return m, m.form.Init()
	case infoMsg:
		if msg.variant == info_error {
			log.Errorln(msg.msg)
		}
		return m, nil
	default:
		if m.state == auth_prompt_secrets && m.form != nil {
			var f tea.Model
			f, cmd = m.form.Update(msg)
			if form, ok := f.(*huh.Form); ok {
				m.form = form
			}
			cmds = append(cmds, cmd)

			if m.form.State == huh.StateCompleted {
				m.state = auth_loading
				access_token := m.form.GetString("access_token")

				if err := cfg.set("AccessToken", access_token); err != nil {
					panic(err)
				}

				return m, tea.Batch(m.loadingSpinner.Tick, m.authenticate())
			}
		} else {
			m.loadingSpinner, cmd = m.loadingSpinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}
