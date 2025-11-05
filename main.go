package main

import (
	"flag"

	"github.com/oo-developer/tinymq/src/application"
)

func main() {
	configFile := flag.String("config", "server_config.json", "Path to config file")
	flag.Parse()

	app := application.NewApplication(*configFile)
	app.Start()
}
