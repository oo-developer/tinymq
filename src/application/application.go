package application

import (
	"os"
	"os/signal"
	"sync"

	"github.com/oo-developer/tinymq/src/broker"
	"github.com/oo-developer/tinymq/src/common"
	"github.com/oo-developer/tinymq/src/config"
	"github.com/oo-developer/tinymq/src/logging"
	log "github.com/oo-developer/tinymq/src/logging"
	"github.com/oo-developer/tinymq/src/storage"
	"github.com/oo-developer/tinymq/src/transport"
	"github.com/oo-developer/tinymq/src/user"
)

type application struct {
	wait             sync.WaitGroup
	config           *config.Config
	loggingService   common.Service
	brokerService    common.BrokerService
	transportService common.Service
	userService      common.UserService
	storage          common.StorageService
}

func NewApplication(config *config.Config) common.Service {
	app := &application{
		config: config,
	}
	app.loggingService = logging.NewLoggingService(app.config.Logging.Format, app.config.Logging.Output, app.config.Logging.Level)
	app.storage = storage.NewStorage(app.config)
	app.brokerService = broker.NewBrokerService(app.storage)
	app.userService = user.NewUserService(app.config)
	app.transportService = transport.NewTransportService(app.config, app.brokerService, app.userService)
	return app
}

func (a *application) Start() {
	a.wait.Add(1)
	a.loggingService.Start()
	a.storage.Start()
	a.userService.Start()
	a.brokerService.Start()
	a.transportService.Start()
	log.Info("Application started")
	a.handleInterrupt()
	a.wait.Wait()
}

func (a *application) Shutdown() {
	a.transportService.Shutdown()
	a.brokerService.Shutdown()
	a.userService.Shutdown()
	a.storage.Shutdown()
	a.loggingService.Shutdown()
	log.Info("Application shut down")
}

func (a *application) handleInterrupt() {
	hook := make(chan os.Signal, 1)
	signal.Notify(hook, os.Interrupt)
	go func(hook chan os.Signal, app *application) {

		for {
			sig := <-hook
			log.Infof("Signal received: '%s'", sig)
			app.Shutdown()
			os.Exit(0)
		}
	}(hook, a)
}
