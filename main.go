package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
)

var (
	// TODO: should put cfgPath inside config.json later
	cfgPath string

	auth    = new(authManager)
	storage = new(store)

	clientId     = flag.String("cid", "", "github client id")
	clientSecret = flag.String("cs", "", "github client id")

	drop = flag.Bool("drop", false, "drop collections at start up")

	logs = []any{}
)

func init() {
	flag.Parse()
	if *clientId == "" || *clientSecret == "" {
		flag.Usage()
		os.Exit(1)
	}

	auth.init(*clientId, *clientSecret)

	// initiate setup the database and the config file
	if err := setup(auth.token); err != nil {
		panic(err)
	}
}

func main() {
	defer storage.db.Close()
	s := &http.Server{
		Addr: ":8080",
	}

	// http client with oauth transport
	var client *github.Client

	httpClient := &http.Client{Timeout: 2 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)

	mux := &http.ServeMux{}
	mux.Handle("/callback", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			return
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
	s.Handler = mux

	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	client, err := auth.authenticate(ctx, shutdown)
	if err != nil {
		log.Printf("Authentication failed: %s\n", err.Error())
		shutdown <- syscall.SIGTERM
	}

	if client != nil {
		log.Println("Authentication succeeded")
		p := tea.NewProgram(newModel(client, shutdown), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			log.Println(err)
			shutdown <- syscall.SIGTERM
		}
	}

	<-shutdown

	// think something smart than ts :skull:
	for _, s := range logs {
		log.Println(s)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		panic(err)
	}
}
