package option

import "fmt"

type Option interface {
	GetChildByName(string) (Option, error)
	GetName() string
	GetValue() interface{}
	SetValue(interface{}) error
}

type impl struct {
	name  string
	value interface{}
	path  string
}

func (o *impl) GetName() string {
	return o.name
}

func (o *impl) GetValue() interface{} {
	return o.value
}

func (o *impl) SetValue(value interface{}) error {
	return fmt.Errorf("unimplemented")
}
