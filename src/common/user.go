package common

import "crypto/rsa"

type User interface {
	Name() string
	PublicKey() *rsa.PublicKey
}

type UserService interface {
	Service
	LookupUserByName(name string) (User, bool)
}
