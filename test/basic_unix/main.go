package main

import (
	"flag"
	"log"
	"strconv"
	"sync"
	"time"

	tinymq "github.com/oo-developer/tinymq/pkg"
	"github.com/oo-developer/tinymq/src/application"
)

const (
	TOPIC_TEST1 = "test/test1/#"
	TOPIC_TEST2 = "test/test2/value"
)

func main() {
	serverConfigFile := flag.String("server-config", "server_config.json", "Path to server config file")
	clientConfigFile := flag.String("client-config", "client_config.json", "Path to client config file")
	flag.Parse()

	startServer(*serverConfigFile)

	clientConfig, err := tinymq.LoadConfig(*clientConfigFile)
	if err != nil {
		panic(err)
	}
	client, err := tinymq.NewClient(clientConfig)
	if err != nil {
		panic(err)
	}
	err = client.Connect()
	if err != nil {
		panic(err)
	}

	countRecived := 0
	client.SetMessageHandler(func(topic string, payload []byte) {
		countRecived++
		if countRecived%10 == 0 {
			log.Printf("Received %d messages", countRecived)
		}
	})
	client.Subscribe(TOPIC_TEST1)
	client.Subscribe(TOPIC_TEST2)

	for ii := 0; ii < 1000000; ii++ {
		client.Publish(TOPIC_TEST1, []byte(strconv.Itoa(ii)))
		if ii%1000 == 0 {
			log.Printf("Sent %d messages", ii)
		}
	}

	for {
		time.Sleep(10 * time.Second)
	}
}

func startServer(configFile string) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		app := application.NewApplication(configFile)
		wg.Done()
		app.Start()
	}()
	wg.Wait()
	time.Sleep(1 * time.Second)
}
