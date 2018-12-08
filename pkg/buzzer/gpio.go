package buzzer

import (
	"fmt"

	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
)

type GPIOBuzzer struct {
	pin gpio.PinIO
}

func NewGPIOBuzzer(gpioPin string) (Buzzer, error) {
	// gpio: load drivers
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("error initializing periph host: %v", err)
	}

	pin := gpioreg.ByName(gpioPin)
	if pin == nil {
		return nil, fmt.Errorf("gpio is not present")
	}
	// gpio: set it pin as input, with an internal pull down resistor
	if err := pin.In(gpio.PullDown, gpio.BothEdges); err != nil {
		return nil, fmt.Errorf("error setting pin %s as internal pull down: %v", gpioPin, err)
	}

	return &GPIOBuzzer{
		pin: pin,
	}, nil
}

func (b *GPIOBuzzer) Wait() error {
	b.pin.WaitForEdge(-1)
	return nil
}

func (b *GPIOBuzzer) Pressed() bool {
	return b.pin.Read() == gpio.High
}
