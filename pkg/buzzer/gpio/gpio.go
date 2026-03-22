// Package gpio provides a Buzzer backed by a Raspberry Pi GPIO pin via periph.io.
package gpio

import (
	"fmt"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/host/v3"

	"github.com/chrischdi/gophotobooth/pkg/buzzer"
)

// Buzzer waits for a falling edge on a named GPIO pin.
type Buzzer struct {
	pin gpio.PinIO
}

var _ buzzer.Buzzer = (*Buzzer)(nil)

// New initialises periph.io and opens the named GPIO pin (e.g. "GPIO17").
func New(pinName string) (*Buzzer, error) {
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("gpio: host init: %w", err)
	}

	p := gpioreg.ByName(pinName)
	if p == nil {
		return nil, fmt.Errorf("gpio: pin %q not found", pinName)
	}

	if err := p.In(gpio.PullUp, gpio.FallingEdge); err != nil {
		return nil, fmt.Errorf("gpio: configure pin %q: %w", pinName, err)
	}

	return &Buzzer{pin: p}, nil
}

// Wait blocks until the GPIO pin detects a falling edge (button press).
func (b *Buzzer) Wait() error {
	b.pin.WaitForEdge(-1)
	return nil
}
