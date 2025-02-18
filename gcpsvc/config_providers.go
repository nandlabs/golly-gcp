package gcpsvc

import (
	"google.golang.org/api/option"
	"oss.nandlabs.io/golly/managers"
)

type Config struct {
	Options   []option.ClientOption
	ProjectId string
	Region    string
}

func (c *Config) SetProjectId(projectId string) {
	c.ProjectId = projectId
}

func (c *Config) SetRegion(region string) {
	c.Region = region
}

func (c *Config) SetCredentialFile(filePath string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithCredentialsFile(filePath))
	return c.Options
}

func (c *Config) SetCredentialJSON(json []byte) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithCredentialsJSON(json))
	return c.Options
}

func (c *Config) SetEndpoint(endpoint string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithEndpoint(endpoint))
	return c.Options
}

func (c *Config) SetUserAgent(userAgent string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithUserAgent(userAgent))
	return c.Options
}

func (c *Config) SetQuotaProject(quotaProject string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithQuotaProject(quotaProject))
	return c.Options
}

func (c *Config) SetScopes(scopes ...string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithScopes(scopes...))
	return c.Options
}

func (c *Config) AddOption(opt option.ClientOption) {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, opt)
}

var Manager = managers.NewItemManager[Config]()
