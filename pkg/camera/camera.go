package camera

import (
	"fmt"
	"io/ioutil"
	"path"
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

func TriggerCamera(c Camera, directory string) (string, error) {
	b, name, err := c.Trigger()
	if err != nil {
		return "", fmt.Errorf("error c.trigger: %v", err)
	}

	p := path.Join(directory, name)

	err = ioutil.WriteFile(p, b, 0644)
	if err != nil {
		return "", err
	}

	return p, nil
}
