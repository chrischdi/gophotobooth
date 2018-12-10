package camera

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/rs/zerolog/log"

	"github.com/micahwedemeyer/gphoto2go"
	"github.com/rwcarlsen/goexif/exif"
)

type DSLR struct {
	dslr gphoto2go.Camera
}

func NewDSLR() Camera {
	dslr := DSLR{
		dslr: gphoto2go.Camera{},
	}
	dslr.dslr.Init()
	return &dslr
}

func (c *DSLR) Trigger() ([]byte, string, error) {
	fp, ierr := c.dslr.TriggerCaptureToFile()
	if ierr < 0 {
		return nil, "", fmt.Errorf("TriggerCaptureToFile: %v", gphoto2go.CameraResultToString(ierr))
	}
	cameraFileReader := c.dslr.FileReader(fp.Folder, fp.Name)
	defer cameraFileReader.Close()

	buf, err := ioutil.ReadAll(cameraFileReader)
	if err != nil {
		return nil, "", fmt.Errorf("Error on ioutil ReadAll")
	}

	r := bytes.NewReader(buf)
	x, err := exif.Decode(r)
	if err != nil {
		log.Error().Err(err).Msg("error on exif.Decode")
		// fallback to name from trigger
		return buf, fp.Name, nil
	}

	d, err := x.DateTime()
	if err != nil {
		log.Error().Err(err).Msg("error extracting DateTime from exif")
		// fallback to name from trigger
		return buf, fp.Name, nil
	}

	return buf, fmt.Sprintf("%s.jpg", d.Format("2006-01-02T15-04-05")), nil
}
