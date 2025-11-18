package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/google/uuid"
)

// MessageHandler is called when a message is received
type MessageHandler func(topic string, payload []byte)

type Subscription struct {
	Id      string
	Topic   string
	Handler MessageHandler
}

// Client represents a broker client
type Client struct {
	clientId         string
	config           *Config
	connCommand      net.Conn
	connPublish      net.Conn
	clientPrivateKey *KyberPrivateKey
	noCipher         Cipher
	handshakeCipher  Cipher
	transportCipher  Cipher
	kemCipherText    []byte
	securityEnabled  bool
	subscriptions    map[string]*Subscription
	messageChannel   chan *Message
	done             chan struct{}
	wg               sync.WaitGroup
	mu               sync.RWMutex
}

// Config holds client configuration
type Config struct {
	Network              string `json:"network"`
	Address              string `json:"address"`
	User                 string `json:"user"`
	ClientPrivateKeyFile string `json:"clientPrivateKeyFile"`
	ServerPublicKeyFile  string `json:"serverPublicKeyFile"`
}

func LoadConfig(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	err = json.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// NewClient creates a new client instance
func NewClient(config *Config) (*Client, error) {
	client := &Client{
		clientId:        uuid.NewString(),
		config:          config,
		securityEnabled: false,
		done:            make(chan struct{}),
		subscriptions:   make(map[string]*Subscription),
		messageChannel:  make(chan *Message, 10000),
	}
	clientPrivateKey, err := LoadKyberPrivateKeyFile(config.ClientPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client private key: %w", err)
	}
	client.clientPrivateKey = clientPrivateKey
	client.noCipher = NewNoCipher()
	client.handlePublishMessage()
	return client, nil
}

func (c *Client) Connect() error {
	var err error
	c.connCommand, err = net.Dial(c.config.Network, c.config.Address)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Send CONNECT (receive server public key)
	connectMsg := &Message{
		Type:    TypeConnect,
		Payload: []byte("wuff"),
	}
	if err := connectMsg.Send(c.connCommand, c.noCipher); err != nil {
		return fmt.Errorf("failed to send CONNECT message: %w", err)
	}
	msg, err := Receive(c.connCommand, c.noCipher)
	if err != nil {
		return fmt.Errorf("failed to receive CONNECT_ACK: %w", err)
	}
	if msg.Type != TypeConnectAck {
		return fmt.Errorf("failed to receive CONNECT_ACK")
	}
	serverPublicKey, err := LoadKyberPublicKey(msg.Payload)
	if err != nil {
		return fmt.Errorf("failed to load kyber public key: %w", err)
	}
	c.handshakeCipher = NewKyberCipher(c.clientPrivateKey, serverPublicKey)

	// Send AUTHENTICATE message
	authMsg := &Message{
		Type:     TypeAuthenticate,
		Payload:  []byte(c.config.User),
		ClientId: c.clientId,
	}
	if err = authMsg.Send(c.connCommand, c.handshakeCipher); err != nil {
		return fmt.Errorf("failed to send connect: %w", err)
	}
	msg, err = Receive(c.connCommand, c.handshakeCipher)
	if err != nil {
		return fmt.Errorf("failed to receive AUTHENTICATE_ACK: %w", err)
	}
	if msg.Type != TypeAuthenticateAck {
		return fmt.Errorf("expected AUTHENTICATE_ACK, got %v", msg.Type)
	}
	channelAddress := string(msg.Payload)

	// Send SESSION_KEY message
	transportCipher, kemCipherText, err := EstablishChCha20Cipher(serverPublicKey.key)
	if err != nil {
		return fmt.Errorf("failed to establish chaCha20 cipher: %w", err)
	}
	c.transportCipher = transportCipher
	c.kemCipherText = kemCipherText
	c.transportCipher.Enable(true)
	sessionKeyMsg := &Message{
		Type:     TypeSessionKey,
		Payload:  []byte(kemCipherText),
		ClientId: c.clientId,
	}
	if err = sessionKeyMsg.Send(c.connCommand, c.handshakeCipher); err != nil {
		return fmt.Errorf("failed to send SESSION_KEY: %w", err)
	}
	msg, err = Receive(c.connCommand, c.handshakeCipher)
	if err != nil {
		return fmt.Errorf("failed to receive SESSION_KEY_ACK: %w", err)
	}
	if msg.Type != TypeSessionKeyAck {
		return fmt.Errorf("expected SESSION_KEY_ACK, got %v", msg.Type)
	}

	// Connect to publish socket
	err = c.connectPublishSocket(channelAddress)
	if err != nil {
		return fmt.Errorf("failed to connect publish socket: %w", err)
	}
	log.Printf("Client %s connected", c.clientId)
	return nil
}

func (c *Client) connectPublishSocket(address string) error {
	var err error
	c.connPublish, err = net.Dial(c.config.Network, address)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Send CONNECT (receive server public key)
	connectMsg := &Message{
		Type:    TypeConnect,
		Payload: []byte("wuff"),
	}
	if err := connectMsg.Send(c.connPublish, c.noCipher); err != nil {
		return fmt.Errorf("failed to send CONNECT message: %w", err)
	}
	msg, err := Receive(c.connPublish, c.noCipher)
	if err != nil {
		return fmt.Errorf("failed to receive CONNECT_ACK: %w", err)
	}
	if msg.Type != TypeConnectAck {
		return fmt.Errorf("failed to receive CONNECT_ACK")
	}

	// Send AUTHENTICATE message
	authMsg := &Message{
		Type:     TypeAuthenticate,
		Payload:  []byte(c.config.User),
		ClientId: c.clientId,
	}
	if err := authMsg.Send(c.connPublish, c.handshakeCipher); err != nil {
		return fmt.Errorf("failed to send AUTHENTICATE: %w", err)
	}
	msg, err = Receive(c.connPublish, c.handshakeCipher)
	if err != nil {
		return fmt.Errorf("failed to receive AUTHENTICATE_ACK: %w", err)
	}
	if msg.Type != TypeAuthenticateAck {
		return fmt.Errorf("expected AUTHENTICATE_ACK, got %v", msg.Type)
	}

	// Send SESSION_KEY message
	sessionKeyMsg := &Message{
		Type:     TypeSessionKey,
		Payload:  c.kemCipherText,
		ClientId: c.clientId,
	}
	if err = sessionKeyMsg.Send(c.connPublish, c.handshakeCipher); err != nil {
		return fmt.Errorf("failed to send SESSION_KEY: %w", err)
	}
	msg, err = Receive(c.connPublish, c.handshakeCipher)
	if err != nil {
		return fmt.Errorf("failed to receive SESSION_KEY_ACK: %w", err)
	}
	if msg.Type != TypeSessionKeyAck {
		return fmt.Errorf("expected SESSION_KEY_ACK, got %v", msg.Type)
	}

	// END CONNECT

	go c.receivePublishLoop()
	log.Printf("Client %s connected to publish socket", c.clientId)
	return nil
}

// Subscribe subscribes to a topic
func (c *Client) Subscribe(topic string, handler MessageHandler) error {
	msg := &Message{
		Type:     TypeSubscribe,
		Topic:    topic,
		ClientId: c.clientId,
	}

	if err := msg.Send(c.connCommand, c.transportCipher); err != nil {
		return fmt.Errorf("failed to SUBSCRIBE: %w", err)
	}
	msgAck, err := Receive(c.connCommand, c.transportCipher)
	if err != nil {
		return fmt.Errorf("failed to receive SUBSCRIBE_ACK: %w", err)
	}
	sub := &Subscription{
		Id:      msgAck.SubscriptionId,
		Topic:   topic,
		Handler: handler,
	}
	c.mu.Lock()
	c.subscriptions[sub.Id] = sub
	c.mu.Unlock()

	log.Printf("Subscribed to topic: %s", topic)
	return nil
}

// Unsubscribe unsubscribes from a topic
func (c *Client) Unsubscribe(topic string) error {
	c.mu.Lock()
	toDelete := make([]string, 0)
	for subId, sub := range c.subscriptions {
		if sub.Topic == topic {
			toDelete = append(toDelete, subId)
		}
	}
	for _, id := range toDelete {
		delete(c.subscriptions, id)
	}
	c.mu.Unlock()
	for _, id := range toDelete {
		msg := &Message{
			Type:           TypeUnsubscribe,
			Topic:          topic,
			ClientId:       c.clientId,
			SubscriptionId: id,
		}
		if err := msg.Send(c.connCommand, c.transportCipher); err != nil {
			return fmt.Errorf("failed to UNSUBSCRIBE: %w", err)
		}
		if _, err := Receive(c.connCommand, c.transportCipher); err != nil {
			return fmt.Errorf("failed to receive UNSUBSCRIBE_ACK: %w", err)
		}
	}
	return nil
}

// Publish publishes a message to a topic
func (c *Client) Publish(topic string, payload []byte, properties ...MessageProperty) error {

	var combinedProperties MessageProperty = 0
	for _, prop := range properties {
		combinedProperties |= prop
	}
	msg := &Message{
		Type:       TypePublish,
		Topic:      topic,
		Payload:    payload,
		Properties: combinedProperties,
		ClientId:   c.clientId,
	}

	if err := msg.Send(c.connCommand, c.transportCipher); err != nil {
		return fmt.Errorf("failed to PUBLISH: %w", err)
	}
	if _, err := Receive(c.connCommand, c.transportCipher); err != nil {
		return fmt.Errorf("failed to receive PUBLISH_ACK: %w", err)
	}

	return nil
}

func (c *Client) receivePublishLoop() {

	for {
		msg, err := Receive(c.connPublish, c.transportCipher)
		if err != nil {
			log.Printf("Receive error: %v", err)
			return
		}
		c.messageChannel <- msg
	}
}

func (c *Client) handlePublishMessage() {
	go func() {
		for {
			msg := <-c.messageChannel
			switch msg.Type {
			case TypeMessage:
				c.mu.RLock()
				if sub, ok := c.subscriptions[msg.SubscriptionId]; ok {
					c.mu.RUnlock()
					sub.Handler(msg.Topic, msg.Payload)
				} else {
					c.mu.RUnlock()
				}
			default:
				log.Printf("Unhandled publish message type: %v", msg.Type)
			}
		}
	}()
}

// Disconnect closes the connection
func (c *Client) Disconnect() error {
	// Send disconnect message
	disconnectMsg := &Message{
		Type:     TypeDisconnect,
		ClientId: c.clientId,
	}

	disconnectMsg.Send(c.connCommand, c.transportCipher)
	if _, err := Receive(c.connCommand, c.transportCipher); err != nil {
		return fmt.Errorf("failed to receive PublishAck: %w", err)
	}
	c.connCommand.Close()
	c.connPublish.Close()

	log.Printf("Client %s disconnected", c.clientId)
	return nil
}
