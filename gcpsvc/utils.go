package gcpsvc

import (
	"net/url"

	"oss.nandlabs.io/golly/textutils"
)

// GetConfig resolves a Config for the given URL using a 3-step resolution:
// 1. Try url.Host
// 2. Try url.Host + "/" + url.Path
// 3. Fallback to the service name
// Returns nil if no config is registered.
func GetConfig(u *url.URL, name string) (config *Config) {
	if u == nil {
		config = Manager.Get(name)
		return
	}

	key := ""
	if u.Host != "" {
		key = u.Host
	}

	config = Manager.Get(key)
	if config == nil && u.Path != "" {
		key = key + textutils.ForwardSlashStr + u.Path
		config = Manager.Get(key)
	}

	if config == nil {
		config = Manager.Get(name)
	}

	return
}
