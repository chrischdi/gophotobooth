package option

type ToggleOption struct {
	impl
}

func (o *ToggleOption) Toggle() error {
	return o.SetValue(1)
}
