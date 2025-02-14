package gcpsvc

import (
	"net/url"

	"oss.nandlabs.io/golly/textutils"
)

func ExtractKey(url *url.URL) (key string) {
	if url.Host != "" {
		key = key + url.Host
	}

	if url.Path != "" {
		key = key + textutils.ForwardSlashStr + url.Path
	}

	return
}
