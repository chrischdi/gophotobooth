package camera

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rwcarlsen/goexif/exif"

	"github.com/chrischdi/gphoto2go"
)

// config vars for Nikon
var (
	// /main/capturesettings/focusmode2
	// Label: Focus Mode 2
	// Readonly: 0
	// Type: RADIO
	// Current: MF (selection)
	// Choice: 0 AF-S
	// Choice: 1 AF-C
	// Choice: 2 AF-A
	// Choice: 3 MF (fixed)
	// Choice: 4 MF (selection)
	autofocusEnableNikon = option{
		"focusmode2",
		"AF-A",
	}
	autofocusDisableNikon = option{
		"focusmode2",
		"MF (selection)",
	}

	autofocusDisableCanon = option{
		"focusmode",
		"One Shot",
	}
	// /main/actions/autofocusdrive
	// Label: Drive Nikon DSLR Autofocus
	// Readonly: 0
	// Type: TOGGLE
	// Current: 0
	autofocusDrive = option{
		"autofocusdrive",
		1,
	}

	// /main/capturesettings/shutterspeed
	// Label: Shutter Speed
	// Readonly: 0
	// Type: RADIO
	// Current: 0.6250s
	// Choice: 0 0.0002s
	// ...
	// Choice: 51 30.0000s
	// Choice: 52 Time
	// Choice: 53 Bulb
	shutterspeedOption = "shutterspeed"
)

type option struct {
	name  string
	value interface{}
}

type RadioOption struct {
	min     int
	max     int
	current int
	values  []string
}

type DSLR struct {
	dslr         gphoto2go.Camera
	shutterspeed *RadioOption
	mutex        sync.Mutex
	dateSet      bool
}

func NewRadioOption(root *gphoto2go.CameraWidget, name string) (*RadioOption, error) {
	opt := RadioOption{}
	var err error

	widget, err := root.GetChildrenByName(name)
	if err != nil {
		return nil, err
	}
	opt.min = 0
	opt.max = widget.CountChoices() - 1

	for i := 0; i <= opt.max; i++ {
		val, err := widget.GetChoice(i)
		if err != nil {
			return nil, err
		}
		opt.values = append(opt.values, val)
	}

	currentVal, err := widget.GetValue()
	if err != nil {
		return nil, err
	}

	for i := 0; i <= opt.max; i++ {
		if opt.values[i] == currentVal {
			opt.current = i
			break
		}
	}

	return &opt, nil
}

func NewDSLR() (Camera, error) {
	dslr := DSLR{
		dslr: gphoto2go.Camera{},
	}
	dslr.dslr.Init()
	var err error
	var rootWidget *gphoto2go.CameraWidget

	if rootWidget, err = dslr.dslr.GetConfig(); err != nil {
		return nil, fmt.Errorf("error getting dslr config: %v", err)
	}

	if dslr.shutterspeed, err = NewRadioOption(rootWidget, "shutterspeed"); err != nil {
		return nil, fmt.Errorf("error getting shutterspeed config: %v", err)
	}

	return &dslr, nil
}

func (c *DSLR) Trigger() ([]byte, string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
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

	if !c.dateSet {
		dateString := d.Format("2 Jan 2006 15:04:05")
		log.Info().Msgf("Setting system date to: %s", dateString)
		args := []string{"date", "--set", dateString}
		err := exec.Command("sudo", args...).Run()
		if err != nil {
			log.Error().Err(err).Msg("error setting date")
		}
	}

	return buf, fmt.Sprintf("%s.jpg", d.Format("2006-01-02T15-04-05")), nil
}

func (c *DSLR) getWidget(name string) (rootWidget, widget *gphoto2go.CameraWidget, err error) {
	if rootWidget, err = c.dslr.GetConfig(); err != nil {
		log.Warn().Err(err).Msg("error getting dslr config")
		return
	}

	if widget, err = rootWidget.GetChildrenByName(name); err != nil {
		log.Warn().Err(err).Msgf("error getting %s config", name)
	}
	return rootWidget, widget, nil

}

// func (c *DSLR)

func logErr(err error) error {
	if err != nil {
		log.Error().Err(err).Msg("err")
		return err
	}
	return nil
}

func (c *DSLR) Focus() error {
	return c.focusCanon()
}

func (c *DSLR) focusNikon() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	log.Info().Msg("enabling autofocus mdoe")
	if err := c.getAndCommitValue(autofocusEnableNikon.name, autofocusEnableNikon.value); err != nil {
		return logErr(err)
	}

	log.Info().Msg("doing autofocus")
	if err := c.getAndCommitValue(autofocusDrive.name, autofocusDrive.value); err != nil {
		return logErr(err)
	}

	log.Info().Msg("disabling autofocus")
	// use retry because autofocus may take time
	if err := retry(func() error { return c.getAndCommitValue(autofocusDisableNikon.name, autofocusDisableNikon.value) }, 10); err != nil {
		return logErr(err)
	}
	return nil
}

func (c *DSLR) focusCanon() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	log.Info().Msg("enabling oneshot mode")
	if err := c.getAndCommitValue(autofocusDisableCanon.name, autofocusDisableCanon.value); err != nil {
		return logErr(err)
	}

	log.Info().Msg("doing autofocus")
	if err := c.getAndCommitValue(autofocusDrive.name, autofocusDrive.value); err != nil {
		return logErr(err)
	}

	return nil
}

func retry(fn func() error, n int) error {
	err := fn()
	for i := 1; i < n; i++ {
		time.Sleep(time.Second)
		if err == nil {
			return err
		}
		err = fn()
	}
	return err
}

func (c *DSLR) ShutterspeedInc() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.shutterspeedUpdate(c.shutterspeed.current - 1)
}

func (c *DSLR) getAndCommitValue(option string, value interface{}) error {
	root, widget, err := c.getWidget(option)
	if err != nil {
		return logErr(err)
	}
	if err := widget.SetValue(value); err != nil {
		return logErr(err)
	}
	if err := c.commitSettings(root); err != nil {
		return logErr(err)
	}
	return nil
}

func (c *DSLR) getAndSetChoice(option string, value int) error {
	root, widget, err := c.getWidget(option)
	if err != nil {
		return err
	}
	defer root.Free()

	val, err := widget.GetChoice(value)
	if err != nil {
		return err
	}

	log.Info().Msgf("setting value %s='%s'", option, val)

	if err := widget.SetValue(val); err != nil {
		return err
	}

	if err := c.commitSettings(root); err != nil {
		return err
	}
	return nil
}

func (c *DSLR) ShutterspeedDec() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.shutterspeedUpdate(c.shutterspeed.current + 1)
}

func (c *DSLR) shutterspeedUpdate(newVal int) error {
	if newVal < c.shutterspeed.min || newVal > c.shutterspeed.max {
		return fmt.Errorf("unable to update, newVal < min || newVal > max")
	}

	root, widget, err := c.getWidget(shutterspeedOption)
	if err != nil {
		return err
	}
	defer root.Free()

	val, err := widget.GetChoice(newVal)
	if err != nil {
		return err
	}

	if err := widget.SetValue(val); err != nil {
		return err
	}

	if err := c.commitSettings(root); err != nil {
		return err
	}
	c.shutterspeed.current = newVal
	return nil
}

func (c *DSLR) Free() {
	c.dslr.Exit()
}

func (c *DSLR) commitSettings(root *gphoto2go.CameraWidget) error {
	return c.dslr.SetConfig(root)
}
