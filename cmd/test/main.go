package main

import (
	"fmt"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

var lastTrigger time.Time

func main() {
	line, err := gpiocdev.RequestLine("gpiochip0", 17,
		gpiocdev.WithBiasDisabled,
		gpiocdev.WithFallingEdge,
		gpiocdev.WithEventHandler(func(evt gpiocdev.LineEvent) {
			if time.Since(lastTrigger) < 300*time.Millisecond {
				return
			}
			lastTrigger = time.Now()
			triggerAction()
		}),
	)
	if err != nil {
		panic(fmt.Sprintf("request line: %v", err))
	}
	defer line.Close()

	fmt.Println("Waiting for button press...")
	select {}
}

func triggerAction() {
	fmt.Printf("Button pressed! %s\n", time.Now().Format(time.RFC3339Nano))
}
