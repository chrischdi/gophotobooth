package dummy

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTakePhoto_ReturnsValidJPEG(t *testing.T) {
	c := &Camera{}
	photo, err := c.TakePhoto()
	require.NoError(t, err)
	require.NotNil(t, photo)

	assert.NotEmpty(t, photo.Bytes, "image bytes must not be empty")

	img, format, err := image.Decode(bytes.NewReader(photo.Bytes))
	require.NoError(t, err, "image bytes must be decodable")
	assert.Equal(t, "jpeg", format)

	b := img.Bounds()
	assert.Equal(t, 1280, b.Dx(), "width must be 1280")
	assert.Equal(t, 720, b.Dy(), "height must be 720")
}

func TestTakePhoto_FilenameFormat(t *testing.T) {
	c := &Camera{}
	photo, err := c.TakePhoto()
	require.NoError(t, err)

	pattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.jpg$`)
	assert.Regexp(t, pattern, photo.Filename, "filename must match timestamp format")
}
