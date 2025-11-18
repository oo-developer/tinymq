package user

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	api "github.com/oo-developer/tinymq/pkg"
	"github.com/oo-developer/tinymq/src/common"
	"github.com/oo-developer/tinymq/src/config"
	log "github.com/oo-developer/tinymq/src/logging"
)

type user struct {
	NameString   string `json:"name"`
	Admin        bool   `json:"admin"`
	PublicKeyPem string `json:"publicKeyPem"`
	publicKey    api.KyberPublicKey
}

func (n *user) Name() string {
	return n.NameString
}

func (n *user) IsAdmin() bool {
	return n.Admin
}

func (n *user) PublicKey() *api.KyberPublicKey {
	return &n.publicKey
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
	u.load()
	log.Info("UserService started")
}

func (u *users) load() {
	_, err := os.Stat(u.config.Users.DataBaseFile)
	if os.IsNotExist(err) {
		u.save()
	}

	data, err := os.ReadFile(u.config.Users.DataBaseFile)
	if err != nil {
		log.Errorf("read user data file '%s' failed: %s", u.config.Users.DataBaseFile, err.Error())
		os.Exit(1)
	}
	userList := make([]*user, 0)
	err = json.Unmarshal(data, &userList)
	if err != nil {
		log.Errorf("unmarshal user data file '%s' failed: %s", u.config.Users.DataBaseFile, err.Error())
		os.Exit(1)
	}
	for _, entry := range userList {
		u.users[entry.Name()] = entry
		publicKey, err := api.LoadKyberPublicKey([]byte(entry.PublicKeyPem))
		if err != nil {
			log.Errorf("load public key failed for user '%s': %s", entry.Name(), err.Error())
		} else {
			entry.publicKey = *publicKey
		}
	}
}

func (u *users) save() {
	directory := filepath.Dir(u.config.Users.DataBaseFile)
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		err := os.MkdirAll(directory, 0750)
		if err != nil {
			log.Errorf("Failed to create directory '%s': %s", directory, err.Error())
			os.Exit(1)
		}
	}
	userList := make([]*user, 0)
	for _, user := range u.users {
		userList = append(userList, user)
	}
	jsonList, err := json.MarshalIndent(userList, "", "  ")
	if err != nil {
		log.Errorf("Failed to marshal users: %s", err.Error())
		os.Exit(1)
	}
	err = os.WriteFile(u.config.Users.DataBaseFile, jsonList, 0640)
	if err != nil {
		log.Errorf("Failed to save users: %s", err.Error())
		os.Exit(1)
	}
}

func (u *users) Shutdown() {
	u.save()
	log.Info("UserService shut down")
}

func (u *users) LookupUserByName(name string) (common.User, bool) {
	user, ok := u.users[name]
	return user, ok
}

func (u *users) AddUser(userName string) error {
	if _, ok := u.users[userName]; ok {
		return fmt.Errorf("user '%s' already exists", userName)
	}
	publicKey, privateKey, err := api.GenerateKyberKeyPair()
	if err != nil {
		return err
	}
	publicKeyBytes, err := api.EncodeKyberPublicKeyPEM(publicKey)
	if err != nil {
		return err
	}
	privateKeyBytes, err := api.EncodeKyberPrivateKeyPEM(privateKey)
	if err != nil {
		return err
	}
	err = os.WriteFile(fmt.Sprintf("%s_private_key.pem", userName), privateKeyBytes, 0600)
	if err != nil {
		return err
	}
	userEntry := &user{
		NameString:   userName,
		PublicKeyPem: string(publicKeyBytes),
		publicKey:    *publicKey,
	}
	u.users[userName] = userEntry
	u.save()
	return nil
}

func (u *users) RemoveUserByName(userName string) error {
	if _, ok := u.users[userName]; !ok {
		return fmt.Errorf("user '%s' does not exists", userName)
	}
	delete(u.users, userName)
	u.save()
	return nil
}
