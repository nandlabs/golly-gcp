package provider

import "context"

type ConfigProvider interface {
	Get() (*SessionConfig, error)
}

var defaultConfigProvider = &defaultProvider{}

type defaultProvider struct{}

func (d *defaultProvider) Get() (sess *SessionConfig, err error) {
	var getSess *SessionConfig

	getSess = NewSessionConfig(context.Background())

	return getSess, nil
}

func GetDefault() ConfigProvider {
	return defaultConfigProvider
}
