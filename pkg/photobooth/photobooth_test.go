package photobooth

import (
	"fmt"
	"testing"

	"github.com/chrischdi/gophotobooth/pkg/buzzer"
	"github.com/chrischdi/gophotobooth/pkg/camera"
	"github.com/chrischdi/gophotobooth/pkg/ui"
)

type AfterXErrorCamera struct {
	X int
}

func (c *AfterXErrorCamera) Trigger() ([]byte, string, error) {
	if c.X == 0 {
		return nil, "", fmt.Errorf("error")
	}
	c.X = c.X - 1
	return nil, "", nil
}

type AlwaysPressedBuzzer struct {
}

func (b *AlwaysPressedBuzzer) Wait() error {
	return nil
}

func (b *AlwaysPressedBuzzer) Pressed() bool {
	return true
}

type AfterXErrorWaitBuzzer struct {
	xPressed bool
	xWait    int
}

func (b *AfterXErrorWaitBuzzer) Wait() error {
	fmt.Printf("AfterXErrorWaitBuzzer Wait xWait: %d", b.xWait)
	if b.xWait == 0 {
		return fmt.Errorf("error")
	}
	b.xWait = b.xWait - 1
	return nil
}

func (b *AfterXErrorWaitBuzzer) Pressed() bool {
	return b.xPressed
}

type DummyUI struct {
	countdownErrAfter int
	publishErrAfter   int
}

func (ui *DummyUI) Countdown() error {
	if ui.countdownErrAfter == 0 {
		return fmt.Errorf("err")
	}
	ui.countdownErrAfter = ui.countdownErrAfter - 1
	return nil
}
func (ui *DummyUI) Publish(string) error {
	if ui.publishErrAfter == 0 {
		return fmt.Errorf("err")
	}
	ui.publishErrAfter = ui.publishErrAfter - 1
	return nil
}

func (ui *DummyUI) Background() error {
	return nil
}

func TestPhotobooth_Loop(t *testing.T) {
	type fields struct {
		buzzer buzzer.Buzzer
		camera camera.Camera
		ui     ui.UI
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			"error camera",
			fields{
				buzzer: &AlwaysPressedBuzzer{},
				camera: &AfterXErrorCamera{1},
				ui:     &DummyUI{-1, -1},
			},
			true,
		},
		{
			"error ui countdown",
			fields{
				buzzer: &AlwaysPressedBuzzer{},
				camera: &AfterXErrorCamera{-1},
				ui:     &DummyUI{1, -1},
			},
			true,
		},
		{
			"error ui trigger",
			fields{
				buzzer: &AlwaysPressedBuzzer{},
				camera: &AfterXErrorCamera{-1},
				ui:     &DummyUI{-1, 1},
			},
			true,
		},
		{
			"error buzzer wait",
			fields{
				buzzer: &AfterXErrorWaitBuzzer{false, 1},
				camera: &AfterXErrorCamera{-1},
				ui:     &DummyUI{-1, -1},
			},
			true,
		},
		// {
		// 	"error camera",
		// 	fields{
		// 		buzzer: &AfterXErrorWaitBuzzer{-1},
		// 		camera: &AfterXErrorCamera{1},
		// 		ui:     &DummyUI{-1, -1},
		// 	},
		// 	true,
		// },
		// {
		// 	"error ui countdown",
		// 	fields{
		// 		buzzer: &AfterXErrorWaitBuzzer{-1},
		// 		camera: &AfterXErrorCamera{-1},
		// 		ui:     &DummyUI{1, -1},
		// 	},
		// 	true,
		// },
		// {
		// 	"error ui trigger",
		// 	fields{
		// 		buzzer: &AfterXErrorWaitBuzzer{-1},
		// 		camera: &AfterXErrorCamera{-1},
		// 		ui:     &DummyUI{-1, 1},
		// 	},
		// 	true,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := &Photobooth{
				Buz: tt.fields.buzzer,
				Cam: tt.fields.camera,
				Gui: tt.fields.ui,
			}
			if err := pb.Run(); (err != nil) != tt.wantErr {
				t.Errorf("Photobooth.Loop() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
