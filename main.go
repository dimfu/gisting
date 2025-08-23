package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
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
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	defer storage.db.Close()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	p := tea.NewProgram(initialModel(shutdown), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Println(err)
		shutdown <- syscall.SIGTERM
	}

	<-shutdown
	auth.close()

	// think something smart than ts :skull:
	for _, s := range logs {
		log.Println(s)
	}
}
