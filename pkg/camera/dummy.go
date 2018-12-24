package camera

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"
)

type DummyCamera struct {
	// directory to write files to
	Directory string
}

func (c *DummyCamera) Trigger() ([]byte, string, error) {
	f, err := ioutil.TempFile(c.Directory, "gophotobooth_*.jpg")
	if err != nil {
		return nil, "", fmt.Errorf("error creating file: %v", err)
	}
	f.Close()

	cmd := exec.Command("convert", "-size", "4496x3000", "xc:green", "-font", "Cantarell-Bold", "-pointsize", "120", "-fill", "black", "-annotate", "+120+120", fmt.Sprintf("\"\n\n\ngophotobooth\n%s\"", time.Now()), f.Name())
	err = cmd.Run()
	if err != nil {
		return nil, "", fmt.Errorf("Command finished with error: %v", err)
	}

	b, err := ioutil.ReadFile(f.Name())
	if err != nil {
		return nil, "", fmt.Errorf("error reading tmp file: %v", err)
	}

	defer os.Remove(f.Name())

	parts := strings.Split(f.Name(), "/")

	return b, parts[len(parts)-1], nil
}
