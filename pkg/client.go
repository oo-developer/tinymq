package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"sync"
)

// MessageHandler is called when a message is received
type MessageHandler func(topic string, payload []byte)

// Client represents a broker client
type Client struct {
	clientID        string
	config          *Config
	conn            net.Conn
	connChannel     net.Conn
	cypher          Cypher
	securityEnabled bool
	messageHandler  MessageHandler
	done            chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
}

// Config holds client configuration
type Config struct {
	ClientID             string `json:"clientId"`
	Network              string `json:"network"`
	Address              string `json:"address"`
	User                 string `json:"user"`
	ClientPrivateKeyFile string `json:"clientPrivateKeyFile"`
	ServerPublicKey      string `json:"serverPublicKey"`
}

func LoadConfig(configFile string) (*Config, error) {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	err = json.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	return config, nil
}

// NewClient creates a new client instance
func NewClient(config *Config) (*Client, error) {
	conn, err := net.Dial(config.Network, config.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	client := &Client{
		clientID:        config.ClientID,
		config:          config,
		conn:            conn,
		securityEnabled: false,
		done:            make(chan struct{}),
	}
	serverPublicKey, err := LoadPublicKey([]byte(config.ServerPublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to load server public key: %w", err)
	}
	clientPrivateKey, err := RsaLoadPrivateKeyFile(config.ClientPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client private key: %w", err)
	}
	client.cypher = NewRsaCypher(clientPrivateKey, serverPublicKey)
	return client, nil
}

func (c *Client) Connect() error {
	var err error
	c.conn, err = net.Dial(c.config.Network, c.config.Address)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	// Send CONNECT message
	connectMsg := &Message{
		Type:     TypeConnect,
		Payload:  []byte(c.config.User),
		ClientId: c.config.ClientID,
	}
	if err := connectMsg.Send(c.conn, c.cypher); err != nil {
		return fmt.Errorf("failed to send connect: %w", err)
	}

	msg, err := Receive(c.conn, c.cypher)
	if err != nil {
		return fmt.Errorf("failed to receive connack: %w", err)
	}
	if msg.Type != TypeConnAck {
		return fmt.Errorf("expected CONNACK, got %v", msg.Type)
	}
	channelAddress := string(msg.Payload)
	err = c.connectChannel(channelAddress)
	if err != nil {
		return fmt.Errorf("failed to connect channel: %w", err)
	}
	log.Printf("Client %s connected", c.config.ClientID)
	c.cypher.Enable(c.securityEnabled)
	return nil
}

func (c *Client) connectChannel(channelAddress string) error {
	var err error
	c.connChannel, err = net.Dial(c.config.Network, channelAddress)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	// Send CONNECT message
	connectMsg := &Message{
		Type:     TypeConnect,
		Payload:  []byte(c.config.User),
		ClientId: c.config.ClientID,
	}
	if err := connectMsg.Send(c.connChannel, c.cypher); err != nil {
		return fmt.Errorf("failed to send connect: %w", err)
	}

	msg, err := Receive(c.connChannel, c.cypher)
	if err != nil {
		return fmt.Errorf("failed to receive connack: %w", err)
	}
	if msg.Type != TypeConnAck {
		return fmt.Errorf("expected CONNACK, got %v", msg.Type)
	}
	// END CONNECT

	c.cypher.Enable(c.securityEnabled)
	go c.receiveChannelLoop()
	log.Printf("Client %s connected to channel", c.config.ClientID)
	return nil
}

// SetMessageHandler sets the handler for received messages
func (c *Client) SetMessageHandler(handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageHandler = handler
}

// Subscribe subscribes to a topic
func (c *Client) Subscribe(topic string) error {
	msg := &Message{
		Type:     TypeSubscribe,
		Topic:    topic,
		ClientId: c.clientID,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := msg.Send(c.conn, c.cypher); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	if _, err := Receive(c.conn, c.cypher); err != nil {
		return fmt.Errorf("failed to receive SubscribeAck: %w", err)
	}
	log.Printf("Subscribed to topic: %s", topic)
	return nil
}

// Unsubscribe unsubscribes from a topic
func (c *Client) Unsubscribe(topic string) error {
	msg := &Message{
		Type:     TypeUnsubscribe,
		Topic:    topic,
		ClientId: c.clientID,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := msg.Send(c.conn, c.cypher); err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}
	if _, err := Receive(c.conn, c.cypher); err != nil {
		return fmt.Errorf("failed to receive UnsubscribeAck: %w", err)
	}
	log.Printf("Unsubscribed from topic: %s", topic)
	return nil
}

// Publish publishes a message to a topic
func (c *Client) Publish(topic string, payload []byte) error {
	msg := &Message{
		Type:     TypePublish,
		Topic:    topic,
		Payload:  payload,
		ClientId: c.clientID,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := msg.Send(c.conn, c.cypher); err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}
	if _, err := Receive(c.conn, c.cypher); err != nil {
		return fmt.Errorf("failed to receive PublishAck: %w", err)
	}

	return nil
}

func (c *Client) receiveLoop() {
	for {
		msg, err := Receive(c.conn, c.cypher)
		if err != nil {
			if err != io.EOF {
				log.Printf("Receive error: %v", err)
			}
			return
		}
		c.handleMessage(msg)
	}
}

func (c *Client) receiveChannelLoop() {
	for {
		msg, err := Receive(c.connChannel, c.cypher)
		if err != nil {
			if err != io.EOF {
				log.Printf("Receive error: %v", err)
			}
			return
		}
		c.handleChannelMessage(msg)
	}
}

func (c *Client) handleMessage(msg *Message) {
	switch msg.Type {
	case TypeMessage:
		c.mu.Lock()
		handler := c.messageHandler
		c.mu.Unlock()
		if handler != nil {
			handler(msg.Topic, msg.Payload)
		}
	case TypePong:
		// Handle pong
	default:
		log.Printf("Unhandled message type: %v", msg.Type)
	}
}

func (c *Client) handleChannelMessage(msg *Message) {
	switch msg.Type {
	case TypeMessage:
		c.mu.Lock()
		defer c.mu.Unlock()
		handler := c.messageHandler
		if handler != nil {
			handler(msg.Topic, msg.Payload)
		}
		/*
			ackMsg := &Message{
				Type:     TypeMessageAck,
				ClientId: c.clientID,
			}
			if err := ackMsg.Send(c.connChannel, c.cypher); err != nil {
				log.Printf("Failed to ack message: %v", err)
			}
		*/
	case TypePong:
		// Handle pong
	default:
		log.Printf("Unhandled channel message type: %v", msg.Type)
	}
}

// Disconnect closes the connection
func (c *Client) Disconnect() error {
	// Send disconnect message
	disconnectMsg := &Message{
		Type:     TypeDisconnect,
		ClientId: c.clientID,
	}

	disconnectMsg.Send(c.conn, c.cypher)
	if _, err := Receive(c.conn, c.cypher); err != nil {
		return fmt.Errorf("failed to receive PublishAck: %w", err)
	}
	close(c.done)
	c.conn.Close()
	c.wg.Wait()

	log.Printf("Client %s disconnected", c.clientID)
	return nil
}
