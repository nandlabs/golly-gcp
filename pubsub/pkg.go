package pubsub

import (
	"oss.nandlabs.io/golly/l3"
	"oss.nandlabs.io/golly/messaging"
)

var logger = l3.Get()

func init() {
	provider := &Provider{}
	messagingManager := messaging.GetManager()
	messagingManager.Register(provider)
}
