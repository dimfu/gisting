package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"golang.org/x/oauth2"

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

func (a *authManager) authenticate(ctx context.Context, shutdown <-chan os.Signal) (*gg.Client, error) {
	if a.token != nil && a.token.AccessToken != "" {
		client := gg.NewClient(a.config.Client(ctx, a.token))
		user, _, err := client.Users.Get(ctx, "")
		if user == nil {
			return nil, fmt.Errorf("Error while retrieving Github user data: %v", err)
		}
		return client, err
	}

	// prompt the user to authenticate their github account if they're not authenticated yet
	authUrl := a.config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Printf("Visit the URL for the auth dialog: %v\n", authUrl)
	for {
		select {
		case result := <-a.callbackChan:
			if result.error != nil {
				return nil, result.error
			}
			return gg.NewClient(auth.config.Client(ctx, auth.token)), nil
		case <-shutdown:
			return nil, errors.New("Cancelled by the user")
		}
	}
}

func (a *authManager) close() {
	close(a.callbackChan)
}
