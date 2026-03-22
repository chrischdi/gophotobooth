// Package buzzer defines the Buzzer interface for detecting a button press.
package buzzer

// Buzzer is the interface for a physical trigger button.
type Buzzer interface {
	// Wait blocks until the button is pressed.
	Wait() error
}
