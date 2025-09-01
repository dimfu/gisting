package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"time"

	"golang.org/x/oauth2"

	tea "github.com/charmbracelet/bubbletea"
	gg "github.com/google/go-github/v74/github"
	"golang.org/x/oauth2/github"
)

type authCallbackResult struct {
	error error
}

type authManager struct {
	token        *oauth2.Token
	config       oauth2.Config
	callbackChan chan authCallbackResult
}

func (a *authManager) init(clientId, clientSecret string) {
	a.config = oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		Endpoint:     github.Endpoint,
		Scopes:       []string{"gist"},
	}
	a.token = new(oauth2.Token)
	// just need 1 buffered callback result channel because we immediately return after an error occurred
	a.callbackChan = make(chan authCallbackResult, 1)
}

func (a *authManager) exchangeToken(ctx context.Context, code string) error {
	newTok, err := a.config.Exchange(ctx, code)
	if err != nil {
		return err
	}
	// set expiry time to zero to make it permanently authenticated
	newTok.Expiry = time.Time{}

	// replace old token with a new one
	a.token = newTok

	b, err := json.Marshal(newTok)
	if err != nil {
		return errors.New("Cannot json marshal new token")
	}

	if err := os.WriteFile(path.Join(cfgPath, "config.json"), b, 0644); err != nil {
		return errors.New("Cannot write new token to config file")
	}

	return nil
}

type authCodeMsg string

func (a *authManager) authenticate() tea.Cmd {
	return func() tea.Msg {
		if a.token != nil && a.token.AccessToken != "" {
			ctx := context.Background()
			tokenSource := a.config.TokenSource(ctx, a.token)
			client := gg.NewClient(oauth2.NewClient(ctx, tokenSource))
			user, _, err := client.Users.Get(ctx, "")
			if user == nil {
				return errMsg{err: err}
			}

			return authSuccessMsg{client}
		}
		return authCodeMsg(a.config.AuthCodeURL("state", oauth2.AccessTypeOffline))
	}
}

func (a *authManager) waitForCallback() tea.Cmd {
	return func() tea.Msg {
		select {
		case result := <-a.callbackChan:
			if result.error != nil {
				return errMsg{err: result.error}
			}
			ctx := context.Background()
			tokenSource := a.config.TokenSource(ctx, a.token)
			client := gg.NewClient(oauth2.NewClient(ctx, tokenSource))
			return authSuccessMsg{client}
		case <-time.After(5 * time.Minute):
			return errMsg{err: errors.New("Authentication timeout - no callback received")}
		}
	}
}

func (a *authManager) close() {
	close(a.callbackChan)
}
