package photobooth

import (
	"fmt"
	"image"
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

	Options PhotoboothOptions
}

type PhotoboothOptions struct {
	Buzzer string
	Camera string
	Gui    string
}

// Run starts the gui and loops for photos
func (pb *Photobooth) Run() error {
	var err error

	pb.timer = time.AfterFunc(pb.AutoPictureTimer, pb.autoPicture)

	err = pb.Gui.Background()
	if err != nil {
		return fmt.Errorf("gui background error: %v", err)
	}

	for {
		err = pb.ResetCamera()
		if err == nil {
			break
		}
		pb.Gui.Error(fmt.Errorf("* ist die Kamera eingeschaltet?\n\n%v", err), time.Second*3)
	}

	for {
		if pb.Buz.Pressed() {
			if err := pb.TriggerWorkflow(); err != nil {
				pb.Gui.Error(err, time.Second*3)
				if err := pb.ResetCamera(); err != nil {
					pb.Gui.Error(err, time.Second*3)
				}
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
	err := pb.TriggerWorkflow()
	if err != nil {
		pb.Gui.Error(err, time.Second*3)
		if err := pb.ResetCamera(); err != nil {
			pb.Gui.Error(err, time.Second*3)
		}
	}
}

func (pb *Photobooth) TriggerWorkflow() error {
	pb.mutex.Lock()
	defer pb.mutex.Unlock()

	pb.timer.Reset(pb.AutoPictureTimer)

	var (
		err   error
		photo image.Image
	)

	err = pb.Gui.Countdown()
	if err != nil {
		return fmt.Errorf("gui countdown error: %v", err)
	}
	photo, err = pb.triggerCamera()
	if err != nil {
		return fmt.Errorf("* ist die Kamera eingeschaltet?\n* ist der Raum zu Dunkel?\n\ncam trigger error: %v", err)
	}
	err = pb.Gui.Publish(photo)
	if err != nil {
		return fmt.Errorf("gui publish error: %v", err)
	}
	return nil
}

func (pb *Photobooth) ResetCamera() error {
	if pb.Cam != nil {
		pb.Cam.Free()
	}
	var err error
	pb.Cam, err = camera.NewCamera(pb.Options.Camera, camera.Options{})
	return err
}

func (pb *Photobooth) triggerCamera() (image.Image, error) {
	err := fmt.Errorf("none")
	var photo image.Image
	for i := 0; i < 3; i++ {
		photo, err = camera.TriggerCamera(pb.Cam, pb.Directory)
		if err != nil {
			log.Warn().Err(err).Int("retry", i).Msg("error triggering camera")
			continue
		}
		return photo, err
	}
	return nil, err
}
