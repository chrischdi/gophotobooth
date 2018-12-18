package gtk

import (
	"bytes"
	"fmt"
	"image"

	"github.com/disintegration/imaging"

	"time"

	"github.com/rs/zerolog/log"

	"github.com/gotk3/gotk3/glib"

	"github.com/gotk3/gotk3/gdk"

	"github.com/gotk3/gotk3/gtk"
)

const (
	Title = "Photobox"
)

type GTK struct {
	Timer    int
	overscan int
	window   *gtk.Window
	content  struct {
		overlay        *gtk.Overlay
		image          *gtk.Image
		countdownLabel *gtk.Label
	}
}

func (ui *GTK) Countdown() error {
	log.Info().Msg("countdown start")
	for i := ui.Timer; i > 0; i-- {
		log.Debug().Int("countdown", i).Msg("countdown")
		_, err := glib.IdleAdd(gtkSetCountdownLabel, ui.content.countdownLabel, i)
		if err != nil {
			log.Error().Err(err).Msg("error on idleAdd for countdownLabel")
		}
		time.Sleep(time.Second)
	}
	_, err := glib.IdleAdd(gtkSetCountdownLabel, ui.content.countdownLabel, "Action!")
	if err != nil {
		log.Error().Err(err).Msg("error on idleAdd for countdownLabel")
	}
	return nil
}

func (ui *GTK) Publish(img image.Image) error {
	width, height := ui.window.GetSize()

	resized := imaging.Fill(img, width, height, imaging.Center, imaging.Box)

	log.Debug().Int("dx", img.Bounds().Dx()).Int("dy", img.Bounds().Dy()).Msg("img.Bounds")
	log.Debug().Int("dx", resized.Bounds().Dx()).Int("dy", resized.Bounds().Dy()).Msg("resized.Bounds")

	// write jpeg to buffer
	var buf bytes.Buffer
	err := imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(95))
	if err != nil {
		return err
	}

	// load buffer to pixbuf
	loader, err := gdk.PixbufLoaderNewWithType("jpeg")
	if err != nil {
		return err
	}
	_, err = loader.Write(buf.Bytes())
	if err != nil {
		return err
	}
	pb, err := loader.GetPixbuf()
	if err != nil {
		return err
	}

	// Publish the pixbuf
	_, err = glib.IdleAdd(gtkSetImage, ui, pb)
	if err != nil {
		log.Error().Err(err).Msg("error on idleAdd for image")
	}
	log.Debug().Msg("publish image done")
	return nil
}

func (ui *GTK) Background() error {
	var err error
	// initialize gtk
	gtk.Init(nil)

	// create window
	ui.window, err = createWindow(Title, 1280, 800)
	if err != nil {
		return fmt.Errorf("error creating gtk.Window: %v", err)
	}

	// create window content
	ui.content.image, ui.content.overlay, ui.content.countdownLabel, err = createContent()
	if err != nil {
		return fmt.Errorf("error creating content: %v", err)
	}
	ui.content.image.SetSizeRequest(ui.window.GetAllocatedWidth(), ui.window.GetAllocatedHeight())
	ui.window.Add(ui.content.overlay)

	ui.window.ShowAll()

	go ui.background()
	return nil
}

func (ui *GTK) background() {
	gtk.Main()
	panic("gtk.Main did return")
}

func createWindow(title string, width, height int) (*gtk.Window, error) {
	w, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return nil, fmt.Errorf("error creating gtk.Window: %v", err)
	}
	w.SetTitle(title)
	w.SetPosition(gtk.WIN_POS_CENTER)
	w.SetDefaultSize(width, height)

	// set to full screen
	w.Fullscreen()
	return w, nil
}

func createContent() (*gtk.Image, *gtk.Overlay, *gtk.Label, error) {
	o, err := gtk.OverlayNew()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error creating gtk.Overlay: %v", err)
	}
	o.SetHExpand(true)
	o.SetVExpand(true)

	i, err := gtk.ImageNew()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error creating gtk.Image: %v", err)
	}
	i.SetHExpand(false)
	i.SetVExpand(false)
	o.Add(i)

	l, err := gtk.LabelNew("")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error creating gtk.Label: %v", err)
	}
	// set position
	l.SetHAlign(gtk.ALIGN_CENTER)
	l.SetVAlign(gtk.ALIGN_CENTER)

	o.AddOverlay(l)

	if err != nil {
		return nil, nil, nil, fmt.Errorf("error creating draw handler: %v", err)
	}

	return i, o, l, nil
}

func gtkSetImage(ui *GTK, pixbuf *gdk.Pixbuf) {
	log.Debug().Msg("gtkSetImage: setFromPixBuf")
	log.Debug().Int("width", ui.content.image.GetAllocatedWidth()).Int("height", ui.content.image.GetAllocatedHeight()).Msg("img allocatedSize before")
	ui.content.image.SetFromPixbuf(pixbuf)
	log.Debug().Int("width", ui.content.image.GetAllocatedWidth()).Int("height", ui.content.image.GetAllocatedHeight()).Msg("img allocatedSize after")
	log.Debug().Msg("gtkSetImage: setCountdownLabel")
	gtkSetCountdownLabel(ui.content.countdownLabel, "")
	log.Debug().Msg("gtkSetImage: queueDraw")
	ui.content.overlay.QueueDraw()
	log.Debug().Msg("gtkSetImage: end")
}

func gtkSetCountdownLabel(label *gtk.Label, i interface{}) {
	var tpl string
	switch i.(type) {
	case int:
		tpl = "<span font_desc='Tahoma 120' color='#f44248'>%d</span>"
	case string:
		tpl = "<span font_desc='Tahoma 120' color='#f44248'>%s</span>"
	default:
		tpl = "<span font_desc='Tahoma 120' color='#f44248'>%v</span>"
	}
	label.SetMarkup(fmt.Sprintf(tpl, i))

	label.QueueDraw()
}

func NewGTK(timer, overscan int) (*GTK, error) {
	if timer < 0 {
		return nil, fmt.Errorf("invalid value %d for timer", timer)
	}

	if overscan < 0 {
		return nil, fmt.Errorf("invalid value %d for overscan", overscan)
	}

	log.Debug().Int("overscan", overscan).Msg("option gtk")
	log.Debug().Int("timer", timer).Msg("option gtk")
	ui := &GTK{
		Timer:    timer,
		overscan: overscan,
	}
	return ui, nil
}
