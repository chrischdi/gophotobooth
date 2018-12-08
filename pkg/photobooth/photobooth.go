package photobooth

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/chrischdi/gophotobooth/pkg/buzzer"
	"github.com/chrischdi/gophotobooth/pkg/camera"
	"github.com/chrischdi/gophotobooth/pkg/ui"
)

// Photobooth represents a Photobooth
type Photobooth struct {
	Buz   buzzer.Buzzer
	Cam   camera.Camera
	Gui   ui.UI
	mutex sync.Mutex
	timer *time.Timer
	// Directore is the directory where to save photos
	Directory string
	// AutoPictureTimer is the timer which enforces taking a picture when reached
	AutoPictureTimer time.Duration
}

// Run starts the gui and loops for photos
func (pb *Photobooth) Run() error {
	var err error

	pb.timer = time.AfterFunc(pb.AutoPictureTimer, pb.autoPicture)

	err = pb.Gui.Background()
	if err != nil {
		return fmt.Errorf("gui background error: %v", err)
	}
	pb.Gui.Publish("/opt/background.jpg")

	for {
		if pb.Buz.Pressed() {
			err = pb.triggerWorkflow()
			if err != nil {
				return err
			}
			continue
		}
		err = pb.Buz.Wait()
		if err != nil {
			return fmt.Errorf("buzzer wait error: %v", err)
		}
	}
}

func (pb *Photobooth) autoPicture() {
	err := pb.triggerWorkflow()
	if err != nil {
		log.Error().Err(err).Msg("error triggering workflow")
	}
}

func (pb *Photobooth) triggerWorkflow() error {
	pb.mutex.Lock()
	defer pb.mutex.Unlock()

	pb.timer.Reset(pb.AutoPictureTimer)

	var (
		err   error
		photo string
	)

	err = pb.Gui.Countdown()
	if err != nil {
		return fmt.Errorf("gui countdown error: %v", err)
	}
	photo, err = pb.triggerCamera()
	if err != nil {
		return fmt.Errorf("cam trigger error: %v", err)
	}
	err = pb.Gui.Publish(photo)
	if err != nil {
		return fmt.Errorf("gui publish error: %v", err)
	}
	return nil
}

func (pb *Photobooth) triggerCamera() (string, error) {
	err := fmt.Errorf("none")
	var photo string
	for i := 0; i < 3; i++ {
		photo, err = camera.TriggerCamera(pb.Cam, pb.Directory)
		if err != nil {
			log.Warn().Err(err).Int("retry", i).Msg("error triggering camera")
			continue
		}
		return photo, err
	}
	return "", err
}
