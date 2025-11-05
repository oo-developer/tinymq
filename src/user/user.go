package user

import (
	"crypto/rsa"

	api "github.com/oo-developer/tinymq/pkg"
	"github.com/oo-developer/tinymq/src/common"
	"github.com/oo-developer/tinymq/src/config"
	log "github.com/oo-developer/tinymq/src/logging"
)

type user struct {
	name      string
	publicKey *rsa.PublicKey
}

func (n *user) Name() string {
	return n.name
}

func (n *user) PublicKey() *rsa.PublicKey {
	return n.publicKey
}

type users struct {
	config *config.Config
	users  map[string]*user
}

func NewUserService(config *config.Config) common.UserService {
	u := &users{
		config: config,
		users:  make(map[string]*user),
	}
	return u
}

func (u *users) Start() {

	for _, configUser := range u.config.Users {
		publicKey, err := api.LoadPublicKey([]byte(configUser.PublicKey))
		if err != nil {
			log.Errorf("load public key failed for user '%s':  %s", configUser.Name, err.Error())
		} else {
			u.users[configUser.Name] = &user{
				name:      configUser.Name,
				publicKey: publicKey,
			}
		}
	}

	log.Info("UserService started")
}

func (u *users) Shutdown() {
	log.Info("UserService shut down")
}

func (u *users) LookupUserByName(name string) (common.User, bool) {
	user, ok := u.users[name]
	return user, ok
}
