package config

import (
	"encoding/json"
	"log"
	"os"
)

type Transport struct {
	Network        string `json:"network"`
	Address        string `json:"address"`
	AddressChannel string `json:"addressChannel"`
	PrivateKeyFile string `json:"privateKeyFile"`
}

type User struct {
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
}

type Logging struct {
	Level  string `json:"level"`
	Output string `json:"output"`
	Format string `json:"format"`
}

type Config struct {
	Transport Transport `json:"transport"`
	Users     []User    `json:"users"`
	Logging   Logging   `json:"logging"`
}

func Load(fileName string) *Config {
	data, err := os.ReadFile(fileName)
	if err != nil {
		log.Fatalf("Loading config from %s: %v", fileName, err)
	}
	config := &Config{}
	err = json.Unmarshal(data, config)
	if err != nil {
		log.Fatalf("Unmarshal config from %s: %v", fileName, err)
	}
	return config
}
