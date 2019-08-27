package main

import (
	"fmt"
	"os"
	"time"

	"github.com/chrischdi/gophotobooth/pkg/api"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/chrischdi/gophotobooth/pkg/camera"

	"github.com/spf13/pflag"

	"github.com/chrischdi/gophotobooth/pkg/buzzer"
	"github.com/chrischdi/gophotobooth/pkg/photobooth"
	"github.com/chrischdi/gophotobooth/pkg/ui"
)

var gui = pflag.String("gui", "", "the graphical user interface to use. (one of "+ui.HelpString()+")")
var buz = pflag.String("buzzer", "", "the buzzer driver to use. (one of "+buzzer.HelpString()+")")
var cam = pflag.String("camera", "", "the camera driver to use. (one of "+camera.HelpString()+")")
var directory = pflag.StringP("directory", "d", "/mnt", "directory where to save the pictures")
var timer = pflag.Int("timer", 3, "the number of seconds before taking a picture")
var verbose = pflag.BoolP("verbose", "v", false, "toggle for verbose logging")
var buzzerGPIOPin = pflag.String("buzzer.gpio.pin", "6", "the gpio pin to use for the gpio buzzer")
var guiOverscan = pflag.Int("gui.overscan", 55, "the overscan to keep fullscreen")
var autoPictureTimer = pflag.Duration("auto-picture-timer", time.Minute*29, "timer to enforce taking a picture to prevent flashlight or camera to turn off")
var adminUITimeout = pflag.Duration("admin-ui-timeout", time.Minute*10, "duration until the admin ui gets disabled")

var port = pflag.IntP("http.port", "p", 8080, "port to serve the given directory")

func errExit(err error) {
	if err != nil {
		pflag.PrintDefaults()
		log.Fatal().Err(err).Msg("error")
	}
}

func buildPhotobooth() (*photobooth.Photobooth, *api.CameraAPI, error) {
	if *gui == "" {
		return nil, nil, fmt.Errorf("parameter `gui` is not set")
	}
	if *buz == "" {
		return nil, nil, fmt.Errorf("parameter `buzzer` is not set")
	}
	if *cam == "" {
		return nil, nil, fmt.Errorf("parameter `camera` is not set")
	}

	pb := photobooth.Photobooth{
		Directory:        *directory,
		AutoPictureTimer: *autoPictureTimer,
		Options: photobooth.PhotoboothOptions{
			*buz,
			*cam,
			*gui,
		},
	}
	var err error

	pb.Gui, err = ui.NewUI(*gui, ui.Options{Timer: *timer, Overscan: *guiOverscan})
	if err != nil {
		return nil, nil, err
	}

	pb.Buz, err = buzzer.NewBuzzer(*buz, buzzer.Options{GPIOPin: *buzzerGPIOPin})
	if err != nil {
		return nil, nil, err
	}

	api := api.NewCameraAPI(&pb, *directory)

	return &pb, api, nil
}

func main() {
	pflag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	pb, api, err := buildPhotobooth()
	errExit(err)

	go api.Serve(fmt.Sprintf(":%d", *port), *adminUITimeout)

	err = pb.Run()
	if err != nil {
		log.Fatal().Err(err).Msg("fatal error during Run")
	}
}
