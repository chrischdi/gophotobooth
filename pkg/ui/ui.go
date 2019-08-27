package ui

import (
	"fmt"
	"image"
	"time"

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
	Publish(img image.Image) error
	// Background starts the ui
	Background() error
	// Error shows the given error for the given amount of time
	Error(err error, duration time.Duration)
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
