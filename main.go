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
	cfg *config
	log = logrus.New()

	auth    = new(authManager)
	storage = new(store)

	drop = flag.Bool("drop", false, "drop collections at start up")
)

func init() {
	// initiate setup the database, config and auth manager
	flag.Parse()
	auth.init()
	if err := setup(); err != nil {
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
