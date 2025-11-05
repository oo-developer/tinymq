package common

import (
	"github.com/oo-developer/tinymq/pkg"
)

type BrokerClient interface {
	Id() string
	MessageChan() <-chan *api.Message
}

type BrokerService interface {
	Service
	RegisterClient(clientID string, user User) BrokerClient
	UnregisterClient(clientID string)
	Client(clientId string) BrokerClient
	Subscribe(clientID, topic string) error
	Unsubscribe(clientID, topic string) error
	Publish(topic string, payload []byte, publisherID string)
}
