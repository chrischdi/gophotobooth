package camera

import (
	"fmt"
	"io/ioutil"

	"github.com/micahwedemeyer/gphoto2go"
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
	return buf, fp.Name, nil
}
