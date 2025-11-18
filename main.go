package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	api "github.com/oo-developer/tinymq/pkg"
	"github.com/oo-developer/tinymq/src/application"
	"github.com/oo-developer/tinymq/src/config"
	"github.com/oo-developer/tinymq/src/user"
)

func main() {
	configFile := flag.String("config", "server_config.json", "Path to config file")
	addUser := flag.String("add-user", "", "AddMessage user")
	removeUser := flag.String("rm-user", "", "RemoveMessage user")
	flag.Parse()

	if configFile == nil || *configFile == "" {
		fmt.Printf("[ERROR] You have to specify a config file with --config\n")
		os.Exit(1)
	}
	configuration := config.Load(*configFile)
	if addUser != nil && *addUser != "" {
		users := user.NewUserService(configuration)
		err := users.AddUser(*addUser)
		users.Shutdown()
		if err != nil {
			fmt.Printf("[ERROR] adding user: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("[OK] Added user: %s\n", *addUser)
		os.Exit(0)
	}
	if removeUser != nil && *removeUser != "" {
		users := user.NewUserService(configuration)
		err := users.RemoveUserByName(*removeUser)
		users.Shutdown()
		if err != nil {
			fmt.Printf("[ERROR] removing user: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("[OK] Removed user: %s\n", *removeUser)
		os.Exit(0)
	}
	initialize(configuration)
	app := application.NewApplication(configuration)
	app.Start()
}

func initialize(configuration *config.Config) {
	if _, err := os.Stat(configuration.Crypto.PrivateKeyFile); os.IsNotExist(err) {
		fmt.Printf("[OK] PrivateKeyFile does not exist: %s\n", configuration.Crypto.PrivateKeyFile)
		fmt.Printf("[OK] Generating new key pair ...\n")

		publicKey, privateKey, err := api.GenerateKyberKeyPair()
		if err != nil {
			fmt.Printf("[ERROR] Error generating new key pair: %s\n", err)
			os.Exit(1)
		}
		publicKeyPem, err := api.EncodeKyberPublicKeyPEM(publicKey)
		if err != nil {
			fmt.Printf("[ERROR] Error encoding public key: %s\n", err)
			os.Exit(1)
		}
		privateKeyPem, err := api.EncodeKyberPrivateKeyPEM(privateKey)
		if err != nil {
			fmt.Printf("[ERROR] Error encoding private key: %s\n", err)
			os.Exit(1)
		}
		folder := filepath.Dir(configuration.Crypto.PrivateKeyFile)
		err = os.MkdirAll(folder, 0755)
		if err != nil {
			fmt.Printf("[ERROR] Error creating directory: %s\n", err)
		}
		err = os.WriteFile(configuration.Crypto.PrivateKeyFile, privateKeyPem, 0600)
		if err != nil {
			fmt.Printf("[ERROR] Error creating private key file: %s\n", err)
		}
		err = os.WriteFile(configuration.Crypto.PublicKeyFile, publicKeyPem, 0600)
		if err != nil {
			fmt.Printf("[ERROR] Error creating public key file: %s\n", err)
		}
		fmt.Printf("[OK] Created private key file: %s\n", configuration.Crypto.PrivateKeyFile)
		fmt.Printf("[OK] Created public key file: %s\n", configuration.Crypto.PublicKeyFile)
	}
}
