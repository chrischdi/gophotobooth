// Package camera defines the Camera interface for taking photos.
package camera

// Photo holds the result of a captured photo.
type Photo struct {
	// Bytes contains the raw JPEG image data.
	Bytes []byte
	// Filename is the suggested filename, e.g. "2026-03-22T15-04-05.jpg".
	Filename string
}

// Camera is the interface for capturing photos.
type Camera interface {
	// TakePhoto captures a photo and returns the result.
	TakePhoto() (*Photo, error)
}
