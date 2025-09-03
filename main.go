package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"
)

var (
	// TODO: should put cfgPath inside config.json later
	cfgPath string
	log     = logrus.New()

	auth    = new(authManager)
	storage = new(store)

	clientId     = flag.String("cid", "", "github client id")
	clientSecret = flag.String("cs", "", "github client id")

	drop = flag.Bool("drop", false, "drop collections at start up")
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
	f, err := initLogger()
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	p := tea.NewProgram(initialModel(shutdown), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Println(err)
		shutdown <- syscall.SIGTERM
	}

	<-shutdown
	auth.close()
}
