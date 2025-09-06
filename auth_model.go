package main

import (
	"context"
	"errors"
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
	auth_wait_oauth
	auth_success
)

type errMsg struct{ err error }

type authModel struct {
	loadingSpinner spinner.Model
	state          authState
	authCodeUrl    authCodeMsg
	mux            *http.ServeMux
	server         *http.Server
	form           *huh.Form
	width          int
	height         int

	authCtx    context.Context
	authCancel context.CancelFunc
}

type authSuccessMsg struct {
	client *github.Client
}

func (m authModel) runAuthServer() tea.Cmd {
	return func() tea.Msg {
		m.mux.Handle("/callback", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var cbErr error
			defer func() {
				auth.callbackChan <- authCallbackResult{error: cbErr}
			}()
			code := r.URL.Query().Get("code")
			if code == "" {
				cbErr = errors.New("Could not get the oauth code")
				http.Error(w, "Code not found", http.StatusBadRequest)
				return
			}
			if err := auth.exchangeToken(context.Background(), code); err != nil {
				cbErr = err
				http.Error(w, "Error while exchanging auth token "+err.Error(), http.StatusInternalServerError)
			}
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `
			<html>
				<body>
					<h2>Authentication Successful!</h2>
					<p>You can now close this window and return to the application.</p>
				</body>
			</html>
		`)
		}))
		go func() {
			if err := m.server.ListenAndServe(); err != nil {
				if errors.Is(err, http.ErrServerClosed) {
					return
				}
			}
		}()
		return nil
	}
}

type needSecretsMsg struct{}

func (m authModel) Init() tea.Cmd {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return func() tea.Msg { return needSecretsMsg{} }
	}
	auth.token = &cfg.Token
	return tea.Batch(m.loadingSpinner.Tick, m.runAuthServer(), auth.authenticate())
}

func (m authModel) View() string {
	switch m.state {
	case auth_prompt_secrets:
		if m.form != nil {
			return m.form.View()
		}
		return "Input required"
	case auth_loading:
		return fmt.Sprintf("%s Authenticating...", m.loadingSpinner.View())
	case auth_wait_oauth:
		// prompt the user to authenticate their github account if they're not authenticated yet
		return fmt.Sprintf("Visit the URL for the auth dialog: %v\n", m.authCodeUrl)
	case auth_success:
		// shutdown the http server since its not needed anymore after authentication
		if err := m.server.Shutdown(context.Background()); err != nil {
			panic(err)
		}
		return fmt.Sprintf("Authenticated")
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
			if err := m.server.Shutdown(context.Background()); err != nil {
				panic(err)
			}
			m.authCancel()
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
		m.state = auth_success
		return m, func() tea.Msg {
			return msg
		}
	case needSecretsMsg:
		m.state = auth_prompt_secrets
		m.form = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("GitHub Client ID").
					Value(&cfg.ClientID).
					Key("clientId").
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("client id required")
						}
						return nil
					}),
				huh.NewInput().
					Title("GitHub Client Secret").
					Value(&cfg.ClientSecret).
					Key("clientSecret").
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("client secret required")
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
	case authCodeMsg:
		m.state = auth_wait_oauth
		m.authCodeUrl = msg
		return m, auth.waitForCallback(m.authCtx)
	case errMsg:
		log.Errorln(msg)
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
				clientSecret := m.form.GetString("clientSecret")
				clientId := m.form.GetString("clientId")

				if err := cfg.set("ClientSecret", clientSecret); err != nil {
					panic(err)
				}
				if err := cfg.set("ClientID", clientId); err != nil {
					panic(err)
				}

				auth.config.ClientSecret = clientSecret
				auth.config.ClientID = clientId

				return m, tea.Batch(m.loadingSpinner.Tick, m.runAuthServer(), auth.authenticate())
			}
		} else {
			m.loadingSpinner, cmd = m.loadingSpinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}
