package gcpsvc

import (
	"net/url"

	"oss.nandlabs.io/golly/textutils"
)

func GetConfig(url *url.URL, name string) (config *Config) {
	key := ""

	if url == nil {
		config = Manager.Get(name)
		return
	}
	if url.Host != "" {
		key = url.Host
	}

	config = Manager.Get(key)
	if config == nil {
		if url.Path != "" {
			key = key + textutils.ForwardSlashStr + url.Path
		}
		config = Manager.Get(key)

	}

	if config == nil {
		config = Manager.Get(name)
	}

	return
}
