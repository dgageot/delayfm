package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/dgageot/delayfm/player"
	"github.com/dgageot/delayfm/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	initialDelay := flag.Duration("delay", 0, "initial playback delay (e.g. 10s)")
	flag.Parse()

	p := player.New(*initialDelay)
	defer p.Stop()

	program := tea.NewProgram(ui.New(p))

	_, err := program.Run()
	return err
}
