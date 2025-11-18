package testtools

import (
	"time"

	"github.com/oo-developer/tinymq/src/application"
	"github.com/oo-developer/tinymq/src/common"
	"github.com/oo-developer/tinymq/src/config"
)

func StartServer(configFile string) common.Service {
	configuration := config.Load(configFile)
	app := application.NewApplication(configuration)
	go func() {
		app.Start()
	}()
	time.Sleep(2 * time.Second)
	return app
}
