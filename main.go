package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
	g "golang.org/x/oauth2/github"
)

var (
	token        oauth2.Token
	cfgPath      string
	clientId     = flag.String("cid", "", "github client id")
	clientSecret = flag.String("cs", "", "github client id")
)

func setup(t *oauth2.Token) error {
	cfgdir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	gistingpath := path.Join(cfgdir, "gisting")
	_, err = os.Stat(gistingpath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(gistingpath, 0755); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	cfgPath = path.Join(gistingpath, "config.json")
	_, err = os.Stat(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			f, err := os.Create(cfgPath)
			if err != nil {
				return err
			}
			defer f.Close()

			e := json.NewEncoder(f)
			e.SetIndent("", "  ")
			if err := e.Encode(&token); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	cfg, err := os.Open(cfgPath)
	if err != nil {
		return err
	}
	defer cfg.Close()

	decoder := json.NewDecoder(cfg)
	err = decoder.Decode(&t)
	if err != nil {
		return err
	}

	return nil
}

func init() {
	flag.Parse()
	if *clientId == "" || *clientSecret == "" {
		panic("Client ID and secret cannot be empty")
	}

	if err := setup(&token); err != nil {
		panic(err)
	}

	fmt.Println(token)
}

func main() {
	s := &http.Server{
		Addr: ":8080",
	}

	conf := oauth2.Config{
		ClientID:     *clientId,
		ClientSecret: *clientSecret,
		Endpoint:     g.Endpoint,
	}

	type callbackResult struct {
		err error
	}
	callbackRes := make(chan callbackResult, 1)

	// http client with oauth transport
	var client *github.Client

	// Use the custom HTTP client when requesting a token.
	httpClient := &http.Client{Timeout: 2 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)

	mux := &http.ServeMux{}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello World"))
	}))
	mux.Handle("/callback", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := callbackResult{}
		defer func() {
			callbackRes <- res
		}()
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Code not found", http.StatusBadRequest)
			return
		}
		tok, err := conf.Exchange(ctx, code)
		if err != nil {
			res.err = err
			http.Error(w, "Could not exchange code for token"+err.Error(), http.StatusInternalServerError)
			return
		}
		// make it zero so that it wont expire
		tok.Expiry = time.Time{}
		// replace initial token with the new one
		token = *tok
		b, err := json.Marshal(token)
		if err != nil {
			res.err = err
			http.Error(w, "Could not marshal json token", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(cfgPath, b, 0644); err != nil {
			res.err = err
			http.Error(w, "Could not write token to gisting config", http.StatusInternalServerError)
			return
		}

		client = github.NewClient(conf.Client(ctx, tok))
		user, _, err := client.Users.Get(ctx, "")
		if err != nil {
			res.err = err
			http.Error(w, "Could not get user "+err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Token: %+v\nUser Info: %s\n", tok, user.String())
	}))
	s.Handler = mux

	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	close := make(chan os.Signal, 1)
	signal.Notify(close, syscall.SIGINT, syscall.SIGTERM)

	// handle initial and persistent authentication
	if token.AccessToken != "" {
		client = github.NewClient(conf.Client(ctx, &token))
		user, _, err := client.Users.Get(ctx, "")
		if err != nil {
			log.Println(err)
			close <- syscall.SIGTERM
		}
		log.Printf("Welcome back %s", user.GetName())
		callbackRes <- callbackResult{err: nil}
	} else {
		url := conf.AuthCodeURL("state", oauth2.AccessTypeOffline)
		fmt.Printf("Visit the URL for the auth dialog: %v\n", url)
		res := <-callbackRes

		if res.err != nil {
			close <- syscall.SIGTERM
			log.Printf("Could not authorize user: %v\n", res.err)
		}
	}

	log.Println("Authentication succeeded")

	// main app
	p := tea.NewProgram(newModel(client, close), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Println(err)
		close <- syscall.SIGTERM
	}

	<-close

	// discard client
	_ = client

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		panic(err)
	}
}
