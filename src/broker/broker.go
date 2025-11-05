package broker

import (
	"fmt"
	"strings"
	"sync"

	"github.com/oo-developer/tinymq/pkg"
	"github.com/oo-developer/tinymq/src/common"
	log "github.com/oo-developer/tinymq/src/logging"
)

type clientInfo struct {
	id             string
	user           common.User
	messageChannel chan *api.Message
	subscriptions  map[string]bool
	mu             sync.RWMutex
}

func (c *clientInfo) Id() string {
	return c.id
}

func (c *clientInfo) MessageChan() <-chan *api.Message {
	return c.messageChannel
}

// broker manages message routing
type broker struct {
	clients       map[string]*clientInfo
	subscriptions map[string]map[string]*clientInfo // topic -> clientID -> clientInfo
	mu            sync.RWMutex
}

func NewBrokerService() common.BrokerService {
	return &broker{
		clients:       make(map[string]*clientInfo),
		subscriptions: make(map[string]map[string]*clientInfo),
	}
}

func (b *broker) Start() {
	log.Info("BrokerService started")
}

func (b *broker) Shutdown() {
	log.Info("BrokerService shut down")
}

func (b *broker) RegisterClient(clientID string, user common.User) common.BrokerClient {
	b.mu.Lock()
	defer b.mu.Unlock()

	client := &clientInfo{
		id:             clientID,
		user:           user,
		messageChannel: make(chan *api.Message, 100000),
		subscriptions:  make(map[string]bool),
	}

	b.clients[clientID] = client
	log.Infof("Client registered: %s", clientID)
	return client
}

func (b *broker) UnregisterClient(clientID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	client, exists := b.clients[clientID]
	if !exists {
		return
	}
	for topic := range client.subscriptions {
		if subscribers, ok := b.subscriptions[topic]; ok {
			delete(subscribers, clientID)
			if len(subscribers) == 0 {
				delete(b.subscriptions, topic)
			}
		}
	}
	close(client.messageChannel)
	delete(b.clients, clientID)
	log.Infof("Client unregistered: %s", clientID)
}

func (b *broker) Client(clientId string) common.BrokerClient {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if client, exists := b.clients[clientId]; exists {
		return client
	}
	return nil
}

// Subscribe adds a subscription for a clientInfo
func (b *broker) Subscribe(clientID, topic string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	client, exists := b.clients[clientID]
	if !exists {
		return fmt.Errorf("Client not found: %s", clientID)
	}

	client.mu.Lock()
	client.subscriptions[topic] = true
	client.mu.Unlock()

	if _, ok := b.subscriptions[topic]; !ok {
		b.subscriptions[topic] = make(map[string]*clientInfo)
	}
	b.subscriptions[topic][clientID] = client

	log.Infof("Client %s subscribed to topic: %s", clientID, topic)
	return nil
}

// Unsubscribe removes a subscription
func (b *broker) Unsubscribe(clientID, topic string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	client, exists := b.clients[clientID]
	if !exists {
		return fmt.Errorf("Client not found: %s", clientID)
	}

	client.mu.Lock()
	delete(client.subscriptions, topic)
	client.mu.Unlock()

	if subscribers, ok := b.subscriptions[topic]; ok {
		delete(subscribers, clientID)
		if len(subscribers) == 0 {
			delete(b.subscriptions, topic)
		}
	}

	log.Infof("Client %s unsubscribed from topic: %s", clientID, topic)
	return nil
}

func (b *broker) Publish(topic string, payload []byte, publisherID string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	matches := b.findMatchingTopics(topic)
	msg := &api.Message{
		Type:     api.TypeMessage,
		Topic:    topic,
		Payload:  payload,
		ClientId: publisherID,
	}
	sentCount := 0
	for matchTopic := range matches {
		if subscribers, ok := b.subscriptions[matchTopic]; ok {
			for _, client := range subscribers {
				client.messageChannel <- msg
				sentCount++
			}
		}
	}
	log.Debugf("Published to topic %s: %d subscribers received message", topic, sentCount)
}

func (b *broker) findMatchingTopics(publishedTopic string) map[string]bool {
	matches := make(map[string]bool)
	for subTopic := range b.subscriptions {
		if b.topicMatches(subTopic, publishedTopic) {
			matches[subTopic] = true
		}
	}
	return matches
}

// topicMatches checks if a subscription topic matches a published topic
// Supports MQTT-style wildcards:
// - "+" matches a single level: "sensor/+/temp" matches "sensor/room1/temp"
// - "#" matches multiple levels: "sensor/#" matches "sensor/room1/temp"
func (b *broker) topicMatches(subTopic, pubTopic string) bool {
	// Exact match
	if subTopic == pubTopic {
		return true
	}

	// Split topics into levels
	subLevels := strings.Split(subTopic, "/")
	pubLevels := strings.Split(pubTopic, "/")

	// Check for multi-level wildcard "#"
	// Must be at the end and alone in its level
	if len(subLevels) > 0 && subLevels[len(subLevels)-1] == "#" {
		// "#" alone matches everything
		if len(subLevels) == 1 {
			return true
		}
		// Match all levels before "#"
		if len(pubLevels) < len(subLevels)-1 {
			return false
		}
		for i := 0; i < len(subLevels)-1; i++ {
			if subLevels[i] != pubLevels[i] && subLevels[i] != "+" {
				return false
			}
		}
		return true
	}

	// For non-# wildcards, level count must match exactly
	if len(subLevels) != len(pubLevels) {
		return false
	}

	// Check each level with "+" wildcard support
	for i := 0; i < len(subLevels); i++ {
		if subLevels[i] == "+" {
			// "+" matches any single level
			continue
		}
		if subLevels[i] != pubLevels[i] {
			return false
		}
	}

	return true
}

// GetClientCount returns the number of connected clients
func (b *broker) GetClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
