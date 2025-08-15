package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var (
	clientId     = flag.String("cid", "", "github client id")
	clientSecret = flag.String("cs", "", "github client id")
)

func init() {
	flag.Parse()
	if *clientId == "" || *clientSecret == "" {
		panic("Client ID and secret cannot be empty")
	}
}

func main() {
	s := &http.Server{
		Addr: ":8080",
	}

	conf := oauth2.Config{
		ClientID:     *clientId,
		ClientSecret: *clientSecret,
		Endpoint:     github.Endpoint,
	}

	type callbackResult struct {
		err error
	}
	callbackRes := make(chan callbackResult, 1)

	// http client with oauth transport
	var client *http.Client

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
		client = conf.Client(ctx, tok)
		resp, err := client.Get("https://api.github.com/user")
		if err != nil {
			res.err = err
			http.Error(w, "Failed to get user info: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			res.err = err
			http.Error(w, "Failed to read response: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Token: %+v\nUser Info: %s\n", tok, body)
	}))
	s.Handler = mux

	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	close := make(chan os.Signal, 1)
	signal.Notify(close, syscall.SIGINT, syscall.SIGTERM)

	url := conf.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Printf("Visit the URL for the auth dialog: %v\n", url)
	res := <-callbackRes

	if res.err != nil {
		close <- syscall.SIGTERM
		log.Printf("Could not authorize user: %v\n", res.err)
	} else {
		log.Println("Authentication succeeded")
		// do something with tui, with http.Client with oauth transport
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
