package transport

import (
	"crypto/rsa"
	"io"
	"net"
	"os"

	"github.com/oo-developer/tinymq/pkg"
	"github.com/oo-developer/tinymq/src/common"
	"github.com/oo-developer/tinymq/src/config"
	log "github.com/oo-developer/tinymq/src/logging"
)

type transport struct {
	config          *config.Transport
	broker          common.BrokerService
	users           common.UserService
	privateKey      *rsa.PrivateKey
	securityEnabled bool
	listener        net.Listener
	listenerChannel net.Listener
}

func NewTransportService(config *config.Config, b common.BrokerService, u common.UserService) common.Service {

	privateKey, err := api.RsaLoadPrivateKeyFile(config.Transport.PrivateKeyFile)
	if err != nil {
		log.Fatal(err.Error())
	}
	return &transport{
		config:          &config.Transport,
		broker:          b,
		users:           u,
		privateKey:      privateKey,
		securityEnabled: false,
	}
}

func (s *transport) Start() {
	var err error
	s.listener, err = net.Listen(s.config.Network, s.config.Address)
	if err != nil {
		log.Fatal("failed to create listener: %w", err)
	}
	log.Infof("transport listening on %s", s.listener.Addr())
	s.listenerChannel, err = net.Listen(s.config.Network, s.config.AddressChannel)
	if err != nil {
		log.Fatal("failed to create listener channel: %w", err)
	}
	log.Infof("transport listening on channel %s", s.listenerChannel.Addr())
	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				log.Infof("Accept error: %v", err)
				continue
			}
			go s.handleConnection(conn)
		}
	}()
	go func() {
		for {
			connChannel, err := s.listenerChannel.Accept()
			if err != nil {
				log.Infof("Accept error: %v", err)
				continue
			}
			go s.handleConnectionChannel(connChannel)
		}
	}()
	log.Info("Transport started")
}

func (s *transport) cleanupUnixSocket() {
	if s.config.Network == "unix" {
		_ = os.Remove(s.config.Address)
		_ = os.Remove(s.config.AddressChannel)
	}
}

func (s *transport) Shutdown() {
	s.listener.Close()
	s.cleanupUnixSocket()
	log.Infof("Transport shut down")
}

func (s *transport) handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Infof("New connection from '%s'", conn.RemoteAddr())

	cypher := api.NewRsaCypher(s.privateKey, nil)
	msg, err := api.Receive(conn, cypher)
	if err != nil {
		log.Infof("Failed to decode connect message: %v", err)
		return
	}
	if msg.Type != api.TypeConnect {
		log.Infof("Expected CONNECT, got %v", msg.Type)
		return
	}
	userName := string(msg.Payload)
	user, ok := s.users.LookupUserByName(userName)
	if !ok {
		log.Warnf("User %s not found", userName)
		return
	}
	log.Infof("New connection from '%s' for user '%s'", conn.RemoteAddr(), user.Name())
	clientID := msg.ClientId
	if clientID == "" {
		log.Warnf("Empty client ID")
		return
	}

	s.broker.RegisterClient(clientID, user)
	defer s.broker.UnregisterClient(clientID)
	cypher = api.NewRsaCypher(s.privateKey, user.PublicKey())

	connAck := &api.Message{
		Type:     api.TypeConnAck,
		ClientId: clientID,
		Payload:  []byte(s.config.AddressChannel),
	}
	if err := connAck.Send(conn, cypher); err != nil {
		log.Errorf("Failed to send message: %v", err)
		return
	}
	log.Infof("Client %s connected", clientID)

	cypher.Enable(s.securityEnabled)
	for {
		msg, err := api.Receive(conn, cypher)
		if err != nil {
			if err != io.EOF {
				log.Errorf("Client %s decode error: %v", clientID, err)
			}
			break
		}
		if !s.handleMessage(clientID, conn, cypher, msg) {
			break
		}
	}
	log.Infof("Client %s disconnected", clientID)
}

func (s *transport) handleMessage(clientID string, conn net.Conn, cypher api.Cypher, msg *api.Message) bool {
	switch msg.Type {
	case api.TypePublish:
		s.broker.Publish(msg.Topic, msg.Payload, clientID)
		connAck := &api.Message{
			Type:     api.TypePublishAck,
			ClientId: clientID,
		}
		if err := connAck.Send(conn, cypher); err != nil {
			log.Errorf("Failed to send PublishAck message: %v", err)
			return true
		}
	case api.TypeSubscribe:
		if err := s.broker.Subscribe(clientID, msg.Topic); err != nil {
			log.Errorf("Subscribe error for client %s: %v", clientID, err)
			return true
		}
		connAck := &api.Message{
			Type:     api.TypeSubscribeAck,
			ClientId: clientID,
		}
		if err := connAck.Send(conn, cypher); err != nil {
			log.Errorf("Failed to send SubscribeAck message: %v", err)
			return true
		}
	case api.TypeUnsubscribe:
		if err := s.broker.Unsubscribe(clientID, msg.Topic); err != nil {
			log.Errorf("Unsubscribe error for client %s: %v", clientID, err)
		}
		connAck := &api.Message{
			Type:     api.TypeUnsubscribeAck,
			ClientId: clientID,
		}
		if err := connAck.Send(conn, cypher); err != nil {
			log.Errorf("Failed to send UnsubscribeAck message: %v", err)
			return true
		}
	case api.TypePing:
		connAck := &api.Message{
			Type:     api.TypePong,
			ClientId: clientID,
		}
		if err := connAck.Send(conn, cypher); err != nil {
			log.Errorf("Failed to send Pong message: %v", err)
			return true
		}
	case api.TypeDisconnect:
		log.Infof("Client %s requested disconnect", clientID)
		return false
	default:
		log.Infof("Unknown message type from client %s: %v", clientID, msg.Type)
	}
	return true
}

func (s *transport) handleConnectionChannel(conn net.Conn) {
	log.Infof("New channel connection from %s", conn.RemoteAddr())

	cypher := api.NewRsaCypher(s.privateKey, nil)
	msg, err := api.Receive(conn, cypher)
	if err != nil {
		log.Infof("Failed to decode connect message: %v", err)
		return
	}
	if msg.Type != api.TypeConnect {
		log.Infof("Expected CONNECT, got %v", msg.Type)
		return
	}
	userName := string(msg.Payload)
	user, ok := s.users.LookupUserByName(userName)
	if !ok {
		log.Warnf("User %s not found", userName)
		return
	}
	log.Infof("New channel connection from '%s' for user '%s'", conn.RemoteAddr(), user.Name())
	clientID := msg.ClientId
	if clientID == "" {
		log.Warnf("Empty client ID")
		return
	}
	cypher = api.NewRsaCypher(s.privateKey, user.PublicKey())

	connAck := &api.Message{
		Type:     api.TypeConnAck,
		ClientId: clientID,
		Payload:  []byte(s.config.AddressChannel),
	}
	if err := connAck.Send(conn, cypher); err != nil {
		log.Errorf("Failed to send message: %v", err)
		return
	}

	client := s.broker.Client(clientID)
	if client == nil {
		log.Warnf("Broker client %s not found", clientID)
	}
	cypher.Enable(s.securityEnabled)
	go func() {
		for msg := range client.MessageChan() {
			//msg := <-client.MessageChan()
			err = msg.Send(conn, cypher)
			if err != nil {
				log.Errorf("Failed publish message: %v", err)
			}

			/*
				msg, err = api.Receive(conn, cypher)
				if err != nil {
					log.Errorf("Failed to receive Ack: %v", err)
					continue
				}
				if msg.Type != api.TypeMessageAck {
					log.Infof("Expected Ack, got %v", msg.Type)
				}

			*/
		}
	}()
	log.Infof("Client %s connected to channel", clientID)
}
