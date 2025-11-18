package config

import (
	"encoding/json"
	"log"
	"os"
)

type Transport struct {
	Network        string `json:"network"`
	AddressCommand string `json:"addressCommand"`
	AddressPublish string `json:"addressPublish"`
}

type Logging struct {
	Level  string `json:"level"`
	Output string `json:"output"`
	Format string `json:"format"`
}

type crypto struct {
	PrivateKeyFile string `json:"privateKeyFile"`
	PublicKeyFile  string `json:"publicKeyFile"`
}

type storage struct {
	DbFile string `json:"dbFile"`
}

type users struct {
	DataBaseFile string `json:"databaseFile"`
}

type Config struct {
	Transport Transport `json:"transport"`
	Logging   Logging   `json:"logging"`
	Users     users     `json:"users"`
	Storage   storage   `json:"storage"`
	Crypto    crypto    `json:"crypto"`
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
