package gcpsvc

import (
	"google.golang.org/api/option"
	"oss.nandlabs.io/golly/managers"
)

type Config struct {
	ProjectId string
	Options   []option.ClientOption
}

var Manager = managers.NewItemManager[Config]()
