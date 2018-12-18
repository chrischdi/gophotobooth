package cli

import (
	"image"
	"time"

	"github.com/rs/zerolog/log"
)

type CLI struct {
	Timer int
}

func (ui *CLI) Countdown() error {
	for i := ui.Timer; i > 0; i-- {
		log.Log().Int("i", i).Msg("UI: countdown timer")
		time.Sleep(time.Second)
	}
	log.Log().Msg("UI: action")
	return nil
}

func (ui *CLI) Publish(img image.Image) error {
	log.Log().Msg("UI: presenting picture")
	return nil
}

func (ui *CLI) Background() error {
	return nil
}
