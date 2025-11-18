package broker

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/oo-developer/tinymq/pkg"
	"github.com/oo-developer/tinymq/src/common"
	log "github.com/oo-developer/tinymq/src/logging"
)

type subscription struct {
	id       string
	clientId string
	topic    string
}

type clientInfo struct {
	id             string
	user           common.User
	messageChannel chan *api.Message
	mutex          sync.RWMutex
}

func (c *clientInfo) Id() string {
	return c.id
}

func (c *clientInfo) MessageChan() <-chan *api.Message {
	return c.messageChannel
}

// broker manages message routing
type broker struct {
	clients        map[string]*clientInfo
	subscriptions  map[string]map[string]*subscription
	matchCache     map[string][]string
	messages       map[string]*api.Message
	storage        common.StorageService
	publishChannel chan *api.Message
	mu             sync.RWMutex
}

func NewBrokerService(storage common.StorageService) common.BrokerService {
	b := &broker{
		clients:        make(map[string]*clientInfo),
		subscriptions:  make(map[string]map[string]*subscription),
		matchCache:     make(map[string][]string),
		messages:       make(map[string]*api.Message),
		storage:        storage,
		publishChannel: make(chan *api.Message, 100000),
	}
	return b
}

func (b *broker) Start() {
	// Publish go func pool
	for ii := 0; ii < 10; ii++ {
		go func() {
			for {
				msg := <-b.publishChannel
				b.publish(msg)
			}
		}()
	}
	// Load persistent message
	for _, msg := range b.storage.GetAllMessages() {
		b.messages[msg.Topic] = msg
	}
	log.Info("BrokerService started")
}

func (b *broker) Shutdown() {
	log.Info("BrokerService shut down")
}

func (b *broker) RegisterClient(clientId string, user common.User) common.BrokerClient {
	b.mu.Lock()
	defer b.mu.Unlock()

	client := &clientInfo{
		id:             uuid.NewString(),
		user:           user,
		messageChannel: make(chan *api.Message, 1000),
	}

	b.clients[clientId] = client
	log.Infof("Client registered: %s", clientId)
	return client
}

func (b *broker) UnregisterClient(clientId string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	client, exists := b.clients[clientId]
	if !exists {
		return
	}

	for _, topic := range b.subscriptions {
		toDelete := make([]*subscription, 0)
		for _, sub := range topic {
			if sub.clientId == clientId {
				toDelete = append(toDelete, sub)
			}
		}
		for _, sub := range toDelete {
			delete(topic, sub.id)
		}
	}

	close(client.messageChannel)
	delete(b.clients, clientId)
	log.Infof("Client unregistered: %s", clientId)
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
func (b *broker) Subscribe(clientID, topic string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	client, exists := b.clients[clientID]
	if !exists {
		return "", fmt.Errorf("Client not found: %s", clientID)
	}

	client.mutex.Lock()
	defer client.mutex.Unlock()
	sub := &subscription{
		id:       uuid.NewString(),
		clientId: clientID,
		topic:    topic,
	}

	if _, ok := b.subscriptions[topic]; !ok {
		b.subscriptions[topic] = make(map[string]*subscription)
	}
	b.subscriptions[topic][sub.id] = sub
	// Send retained messages
	for _, msg := range b.messages {
		if b.topicMatches(msg.Topic, topic) {
			msgCopy := *msg
			msgCopy.SubscriptionId = sub.id
			client.messageChannel <- &msgCopy
		}
	}

	log.Infof("Client %s subscribed to topic: %s", clientID, topic)
	return sub.id, nil
}

// Unsubscribe removes a subscription
func (b *broker) Unsubscribe(clientID, topic string, subscriptionId string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	client, exists := b.clients[clientID]
	if !exists {
		return fmt.Errorf("Client not found: %s", clientID)
	}

	client.mutex.Lock()
	defer client.mutex.Unlock()

	for _, topicEntry := range b.subscriptions {
		toDelete := make([]*subscription, 0)
		for _, sub := range topicEntry {
			if sub.topic == topic && sub.id == subscriptionId {
				toDelete = append(toDelete, sub)
			}
		}
		for _, sub := range toDelete {
			delete(topicEntry, sub.id)
		}
		if len(topicEntry) == 0 {
			delete(b.subscriptions, topic)
			break
		}
	}

	log.Infof("Client %s unsubscribed from topic: %s", clientID, topic)
	return nil
}

func (b *broker) Publish(properties api.MessageProperty, topic string, payload []byte, publisherID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	msg := &api.Message{
		Properties: properties,
		Type:       api.TypeMessage,
		Topic:      topic,
		Payload:    payload,
		ClientId:   publisherID,
	}
	if msg.Payload == nil || len(msg.Payload) == 0 {
		delete(b.messages, topic)
		b.storage.RemoveMessageChannel() <- msg.Topic
		return
	}
	if msg.IsRetained() {
		b.messages[topic] = msg
	}
	if msg.IsPersistent() {
		b.messages[topic] = msg
		b.storage.AddMessageChannel() <- msg
	}
	b.publishChannel <- msg
}

func (b *broker) publish(msg *api.Message) {
	matches := b.findMatchingTopics(msg.Topic)
	for _, match := range matches {
		for _, subs := range b.subscriptions[match] {
			if client, ok := b.clients[subs.clientId]; ok {
				msgCopy := *msg
				msgCopy.SubscriptionId = subs.id
				client.messageChannel <- &msgCopy
			}
		}
	}
}

func (b *broker) findMatchingTopics(publishedTopic string) []string {
	if _, ok := b.matchCache[publishedTopic]; !ok {
		b.matchCache[publishedTopic] = make([]string, 0)
		for subTopic := range b.subscriptions {
			if b.topicMatches(subTopic, publishedTopic) {
				b.matchCache[publishedTopic] = append(b.matchCache[publishedTopic], subTopic)
			}
		}
	}
	return b.matchCache[publishedTopic]
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
