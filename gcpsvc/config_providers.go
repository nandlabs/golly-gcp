package gcpsvc

import (
	"google.golang.org/api/option"
	"oss.nandlabs.io/golly/managers"
)

// Config holds GCP client options and project/location info.
type Config struct {
	Options   []option.ClientOption
	ProjectId string
	Location  string
}

// SetProjectId sets the GCP project ID.
func (c *Config) SetProjectId(projectId string) {
	c.ProjectId = projectId
}

// SetRegion sets the GCP region/location.
func (c *Config) SetRegion(region string) {
	c.Location = region
}

// Deprecated: SetCredentialFile uses a deprecated API with potential security risks.
// Use SetAuthCredentialFile instead.
func (c *Config) SetCredentialFile(filePath string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithCredentialsFile(filePath)) //nolint:staticcheck
	return c.Options
}

// Deprecated: SetCredentialJSON uses a deprecated API with potential security risks.
// Use SetAuthCredentialJSON instead.
func (c *Config) SetCredentialJSON(json []byte) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithCredentialsJSON(json)) //nolint:staticcheck
	return c.Options
}

// SetAuthCredentialFile adds a typed credentials file option and returns the updated options.
// credType specifies the credential type (e.g. option.ServiceAccount, option.AuthorizedUser).
func (c *Config) SetAuthCredentialFile(credType option.CredentialsType, filePath string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithAuthCredentialsFile(credType, filePath))
	return c.Options
}

// SetAuthCredentialJSON adds a typed credentials JSON option and returns the updated options.
// credType specifies the credential type (e.g. option.ServiceAccount, option.AuthorizedUser).
func (c *Config) SetAuthCredentialJSON(credType option.CredentialsType, json []byte) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithAuthCredentialsJSON(credType, json))
	return c.Options
}

// SetEndpoint adds an endpoint option and returns the updated options.
func (c *Config) SetEndpoint(endpoint string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithEndpoint(endpoint))
	return c.Options
}

// SetUserAgent adds a user agent option and returns the updated options.
func (c *Config) SetUserAgent(userAgent string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithUserAgent(userAgent))
	return c.Options
}

// SetQuotaProject adds a quota project option and returns the updated options.
func (c *Config) SetQuotaProject(quotaProject string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithQuotaProject(quotaProject))
	return c.Options
}

// SetScopes adds scopes options and returns the updated options.
func (c *Config) SetScopes(scopes ...string) []option.ClientOption {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, option.WithScopes(scopes...))
	return c.Options
}

// AddOption appends a custom client option to the options slice.
func (c *Config) AddOption(opt option.ClientOption) {
	if c.Options == nil {
		c.Options = make([]option.ClientOption, 0)
	}
	c.Options = append(c.Options, opt)
}

// Manager is an item manager for Config instances.
var Manager = managers.NewItemManager[*Config]()
