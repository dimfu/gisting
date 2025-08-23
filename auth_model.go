package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
)

type authState int

const (
	auth_loading authState = iota
	auth_wait_oauth
	auth_success
)

type errMsg struct{ err error }

type authModel struct {
	shutdown       chan os.Signal
	loadingSpinner spinner.Model
	state          authState
	authCodeUrl    authCodeMsg
	mux            *http.ServeMux
	server         *http.Server
}

type authSuccessMsg struct {
	client *github.Client
}

func (m authModel) runAuthServer() tea.Cmd {
	return func() tea.Msg {
		httpClient := &http.Client{Timeout: 10 * time.Second}
		ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
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
			if err := auth.exchangeToken(ctx, code); err != nil {
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

func (m authModel) Init() tea.Cmd {
	return tea.Batch(m.loadingSpinner.Tick, m.runAuthServer(), auth.authenticate())
}

func (m authModel) View() string {
	switch m.state {
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

func (s authModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case authSuccessMsg:
		s.state = auth_success
		return s, func() tea.Msg {
			return msg
		}
	case authCodeMsg:
		s.state = auth_wait_oauth
		s.authCodeUrl = msg
		return s, auth.waitForCallback()
	case errMsg:
		return s, nil
	default:
		s.loadingSpinner, cmd = s.loadingSpinner.Update(msg)
	}
	cmds = append(cmds, cmd)
	return s, tea.Batch(cmds...)
}
