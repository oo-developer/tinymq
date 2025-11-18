package main

import (
	"flag"
	"log"
	"time"

	tinymq "github.com/oo-developer/tinymq/pkg"
	testtools "github.com/oo-developer/tinymq/test"
)

const (
	TOPIC_TEST1 = "test/test1/#"
	TOPIC_TEST2 = "test/test2/value"
)

const totalCount = 100000

func main() {
	serverConfigFile := flag.String("server-config", "server_config.json", "Path to server config file")
	clientConfigFile := flag.String("client-config", "client_config.json", "Path to client config file")
	flag.Parse()

	server := testtools.StartServer(*serverConfigFile)

	clientConfig, err := tinymq.LoadConfig(*clientConfigFile)
	if err != nil {
		panic(err)
	}
	start := time.Now().UnixMilli()
	client, err := tinymq.NewClient(clientConfig)
	if err != nil {
		panic(err)
	}
	err = client.Connect()
	if err != nil {
		panic(err)
	}

	countReceived := 0
	client.Subscribe(TOPIC_TEST1, func(topic string, payload []byte) {
		countReceived++
		if countReceived%100000 == 0 {
			log.Printf("Received %d messages", countReceived)
		}
		if countReceived >= totalCount {
			end := time.Now().UnixMilli()
			span := end - start
			msPerMsg := float64(span) / float64(totalCount)
			log.Printf("Received %d messages in %.3f ms per message", countReceived, msPerMsg)
			log.Printf("Sent %d messages in %.2f s", countReceived, float64(end-start)/1000.0)
			client.Unsubscribe(TOPIC_TEST1)
			client.Disconnect()
		}
	})

	msg := "This is a short message."
	msg += msg
	log.Printf("Message size: %d ", len(msg))
	for ii := 0; ii <= totalCount; ii++ {
		client.Publish(TOPIC_TEST1, []byte(msg))
		if ii%100000 == 0 {
			log.Printf("Sent %d messages", ii)
		}
	}

	for {
		time.Sleep(10 * time.Second)
		if countReceived >= totalCount {
			server.Shutdown()
			break
		}
	}
}

/*
func startServer(configFile string) common.Service {
	configuration := config.Load(configFile)
	app := application.NewApplication(configuration)
	go func() {
		app.Start()
	}()
	time.Sleep(1 * time.Second)
	return app
}
*/
