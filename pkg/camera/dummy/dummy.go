// Package dummy provides a Camera implementation that generates a timestamp image.
// It requires no hardware and is useful for development and testing.
package dummy

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/chrischdi/gophotobooth/pkg/camera"
)

const (
	width  = 1280
	height = 720
)

var (
	bgColor        = color.RGBA{30, 30, 30, 255}
	labelColor     = color.RGBA{80, 80, 80, 255}
	timestampColor = color.RGBA{220, 220, 220, 255}
)

// Camera generates a JPEG image with the current timestamp rendered on it.
// It fulfills the camera.Camera interface without any hardware.
type Camera struct{}

var _ camera.Camera = (*Camera)(nil)

// TakePhoto returns a 1280x720 JPEG image with the timestamp printed in the center.
func (c *Camera) TakePhoto() (*camera.Photo, error) {
	now := time.Now()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	ttf, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}

	// Large face for the timestamp.
	largeFace, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size: 96,
		DPI:  72,
	})
	if err != nil {
		return nil, err
	}
	defer largeFace.Close() //nolint:errcheck

	// Small face for the "DUMMY" label.
	smallFace, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size: 28,
		DPI:  72,
	})
	if err != nil {
		return nil, err
	}
	defer smallFace.Close() //nolint:errcheck

	// Draw "DUMMY" label near top-center.
	label := "DUMMY"
	drawCenteredText(img, smallFace, label, height/4, labelColor)

	// Draw date line centered.
	dateLine := now.Format("2006-01-02")
	drawCenteredText(img, largeFace, dateLine, height/2-10, timestampColor)

	// Draw time line below date.
	timeLine := now.Format("15:04:05")
	metrics := largeFace.Metrics()
	lineHeight := (metrics.Ascent + metrics.Descent).Ceil()
	drawCenteredText(img, largeFace, timeLine, height/2-10+lineHeight+8, timestampColor)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}

	return &camera.Photo{
		Bytes:    buf.Bytes(),
		Filename: now.Format("2006-01-02T15-04-05") + ".jpg",
	}, nil
}

// drawCenteredText draws text horizontally centered at the given y baseline.
func drawCenteredText(img draw.Image, face font.Face, text string, y int, col color.Color) {
	advance := font.MeasureString(face, text)
	x := (width - advance.Ceil()) / 2
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(text)
}
