package buzzer

import "time"

type TrueBuzzer struct {
	Timer time.Duration
}

func (b *TrueBuzzer) Wait() error {
	time.Sleep(b.Timer)
	return nil
}

func (b *TrueBuzzer) Pressed() bool {
	time.Sleep(time.Second)
	return true
}
