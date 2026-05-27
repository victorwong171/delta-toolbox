package config

import (
	"github.com/spf13/viper"
)

type Loader[O any] interface {
	Load() (O, error)
}

type file[O any] struct {
	name  string
	path  string
	_type string
}

func NewLoader[O any](name, path, _type string) Loader[O] {
	return &file[O]{
		name:  name,
		path:  path,
		_type: _type,
	}
}

func (f file[O]) Load() (O, error) {
	v := viper.New()
	v.SetConfigName(f.name)
	v.SetConfigType(f._type)
	v.AddConfigPath(f.path)
	if err := v.ReadInConfig(); err != nil {
		return *new(O), err
	}
	var config O
	if err := v.Unmarshal(&config); err != nil {
		return *new(O), err
	}
	return config, nil
}
