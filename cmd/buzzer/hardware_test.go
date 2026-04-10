//go:build hardware

package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestHardware_GpioBuzzerWait requires a real Raspberry Pi with a physical
// button wired to GPIO17 (or the pin configured via the BUZZER_PIN env var).
//
// Run with:
//
//	go test -tags hardware -run TestHardware_GpioBuzzerWait ./cmd/buzzer/ -v
func TestHardware_GpioBuzzerWait(t *testing.T) {
	pin := os.Getenv("BUZZER_PIN")
	if pin == "" {
		pin = "GPIO17"
	}

	b, err := newGPIOButton(pin)
	if err != nil {
		t.Skipf("failed to initialise GPIO pin %q (no hardware?): %v", pin, err)
	}
	defer b.line.Close() //nolint:errcheck

	fmt.Fprintf(os.Stderr, "\n>>> MANUAL ACTION REQUIRED: press the button on pin %s within 30s <<<\n", pin)

	done := make(chan error, 1)
	go func() { done <- b.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait() returned error: %v", err)
		}
		t.Log("Button press detected — PASS")
	case <-time.After(30 * time.Second):
		t.Skip("Timed out waiting for button press — skipped")
	}
}
