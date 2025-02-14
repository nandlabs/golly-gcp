package gcpsvc

import (
	"google.golang.org/api/option"
	"oss.nandlabs.io/golly/managers"
)

type Config struct {
	ProjectId string
	Options   []option.ClientOption
}

func (c *Config) SetCredentialFile(filePath string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithCredentialsFile(filePath))
	return c.Options
}

var Manager = managers.NewItemManager[Config]()
