package gcpsvc

import "oss.nandlabs.io/golly/managers"

type Config struct {
	ProjectId string
}

var Manager = managers.NewItemManager[Config]()
