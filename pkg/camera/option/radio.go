package option

import "fmt"

type RadioOption struct {
	impl

	min     int
	max     int
	current int
}

func (o *RadioOption) Increment() error {
	return o.update(o.current + 1)
}

func (o *RadioOption) Decrement() error {
	return o.update(o.current - 1)
}

func (o *RadioOption) update(newVal int) error {
	if newVal < o.min || newVal > o.max {
		return fmt.Errorf("unable to update, newVal < min || newVal > max")
	}
	if err := o.SetValue(newVal); err != nil {
		return fmt.Errorf("error incrementing: %v", err)
	}
	o.current = o.current + 1
	return nil
}
