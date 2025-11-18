package storage

import (
	"sync"
	"time"

	api "github.com/oo-developer/tinymq/pkg"
	"github.com/oo-developer/tinymq/src/common"
	"github.com/oo-developer/tinymq/src/config"
	log "github.com/oo-developer/tinymq/src/logging"
	"github.com/vmihailenco/msgpack/v5"
	"go.etcd.io/bbolt"
)

type storage struct {
	config               *config.Config
	db                   *bbolt.DB
	messageAddChannel    chan *api.Message
	messageRemoveChannel chan string
	messageCache         map[string]*api.Message
	mu                   sync.RWMutex
}

func NewStorage(config *config.Config) common.StorageService {
	s := &storage{
		config:               config,
		messageAddChannel:    make(chan *api.Message, 10),
		messageRemoveChannel: make(chan string, 10),
		messageCache:         make(map[string]*api.Message),
	}

	return s
}

func (s *storage) Start() {
	db, err := bbolt.Open(s.config.Storage.DbFile, 0600, &bbolt.Options{
		Timeout:         1 * time.Second,
		NoGrowSync:      true,
		NoFreelistSync:  true,
		FreelistType:    bbolt.FreelistArrayType,
		InitialMmapSize: 10 * 1024 * 1024,
	})
	if err != nil {
		log.Fatal(err)
	}
	s.db = db
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("users"))
		if err != nil {
			log.Fatal(err)
		}
		_, err = tx.CreateBucketIfNotExists([]byte("messages"))
		if err != nil {
			log.Fatal(err)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			select {
			case msg := <-s.messageAddChannel:
				s.mu.Lock()
				s.messageCache[msg.Topic] = msg
				s.mu.Unlock()
			case topic := <-s.messageRemoveChannel:
				s.removeMessage(topic)
			}
		}
	}()

	ticker := time.NewTicker(20 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.addMessages()
			}
		}
	}()

	log.Info("StorageService started")
}

func (s *storage) RemoveMessageChannel() chan string {
	return s.messageRemoveChannel
}

func (s *storage) AddMessageChannel() chan *api.Message {
	return s.messageAddChannel
}

func (s *storage) Shutdown() {
	s.addMessages()
	log.Info("StorageService shut down")
}

func (s *storage) addMessages() {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.Update(func(tx *bbolt.Tx) error {
		for _, msg := range s.messageCache {
			messageBucket := tx.Bucket([]byte("messages"))
			value, _ := msgpack.Marshal(msg)
			err := messageBucket.Put([]byte(msg.Topic), value)
			if err != nil {
				return err
			}
			delete(s.messageCache, msg.Topic)
		}
		return nil
	})
	if err != nil {
		log.Errorf("Error adding messages to storage: %v", err)
	}
}

func (s *storage) removeMessage(topic string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.Update(func(tx *bbolt.Tx) error {
		messageBucket := tx.Bucket([]byte("messages"))
		err := messageBucket.Delete([]byte(topic))
		return err
	})
	if err != nil {
		log.Errorf("Error removing message from storage: %v", err)
	}
}

func (s *storage) GetAllMessages() []*api.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	messages := make([]*api.Message, 0)
	err := s.db.View(func(tx *bbolt.Tx) error {
		messageBucket := tx.Bucket([]byte("messages"))
		err := messageBucket.ForEach(func(k, v []byte) error {
			msg := &api.Message{}
			err := msgpack.Unmarshal(v, msg)
			if err != nil {
				return err
			}
			messages = append(messages, msg)
			return nil
		})
		return err
	})

	if err != nil {
		log.Errorf("Error getting all messages: %v", err)
	}
	return messages
}
