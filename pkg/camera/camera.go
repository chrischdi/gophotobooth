package camera

import (
	"bytes"
	"fmt"
	"image"
	"io/ioutil"
	"path"

	"github.com/disintegration/imaging"
)

type Options struct {
}

// Camera represents the implementation of a backend which creates photos
type Camera interface {
	// trigger is the subcommand which creates and returns the photo
	Trigger() ([]byte, string, error)
}

type cameraImpl struct{}

func NewCamera(name string, options Options) (Camera, error) {
	switch name {
	case "dslr":
		return NewDSLR(), nil
	case "dummy":
		return &DummyCamera{}, nil
	}
	return nil, fmt.Errorf("camera '%s' not found", name)
}

func HelpString() string {
	return "dslr,dummy"
}

func TriggerCamera(c Camera, directory string) (image.Image, error) {
	b, name, err := c.Trigger()
	if err != nil {
		return nil, fmt.Errorf("error c.trigger: %v", err)
	}

	p := path.Join(directory, name)

	err = ioutil.WriteFile(p, b, 0644)
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(b)
	img, err := imaging.Decode(reader)
	if err != nil {
		return nil, err
	}

	return img, nil
}
