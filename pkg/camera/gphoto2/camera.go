//go:build cgo

// Package gphoto2 provides a Camera implementation backed by libgphoto2.
// It requires CGO and libgphoto2 to be installed on the system.
package gphoto2

import (
	"bytes"
	"fmt"
	"io"
	"time"

	gp "github.com/chrischdi/gphoto2go"

	"github.com/chrischdi/gophotobooth/pkg/camera"
)

// Camera wraps a gphoto2 camera connection.
type Camera struct {
	cam gp.Camera
}

var _ camera.Camera = (*Camera)(nil)

// New opens the first available gphoto2 camera.
func New() (*Camera, error) {
	c := &Camera{}
	if err := c.cam.Init(); err != nil {
		return nil, fmt.Errorf("gphoto2: init: %w", err)
	}
	return c, nil
}

// Close releases the camera connection.
func (c *Camera) Close() {
	_ = c.cam.Exit()
}

// TakePhoto captures a photo via gphoto2 and returns the JPEG bytes.
func (c *Camera) TakePhoto() (*camera.Photo, error) {
	now := time.Now()

	filePath, err := c.cam.TriggerCaptureToFile()
	if err != nil {
		return nil, fmt.Errorf("gphoto2: capture: %w", err)
	}

	rc := c.cam.FileReader(filePath.Folder, filePath.Name)
	defer rc.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return nil, fmt.Errorf("gphoto2: read file: %w", err)
	}

	return &camera.Photo{
		Bytes:    buf.Bytes(),
		Filename: now.Format("2006-01-02T15-04-05") + ".jpg",
	}, nil
}
