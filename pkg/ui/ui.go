package ui

import (
	"fmt"

	"github.com/chrischdi/gophotobooth/pkg/ui/cli"
	"github.com/chrischdi/gophotobooth/pkg/ui/gtk"
)

type Options struct {
	// Timer is the time in seconds to wait until taking a photo
	Timer int
	// Overscan is the overscan added to the image size to fill the screen
	Overscan int
}

// UI represents the user interface
type UI interface {
	// Countdown shows the countdown till a photo gets taken
	Countdown() error
	// Publish makes a photo visible to the user
	Publish(string) error
	// Background starts the ui
	Background() error
}

func NewUI(name string, options Options) (UI, error) {
	switch name {
	case "gtk":
		return gtk.NewGTK(options.Timer, options.Overscan)
	case "cli":
		return &cli.CLI{
			Timer: options.Timer,
		}, nil
	}
	return nil, fmt.Errorf("ui '%s' not found", name)
}

func HelpString() string {
	return "cli,gtk"
}
