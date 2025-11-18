package common

import api "github.com/oo-developer/tinymq/pkg"

type StorageService interface {
	Service
	GetAllMessages() []*api.Message
	AddMessageChannel() chan *api.Message
	RemoveMessageChannel() chan string
}
