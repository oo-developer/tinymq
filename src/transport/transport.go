package transport

import (
	"errors"
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
	privateKey      *api.KyberPrivateKey
	publicKey       *api.KyberPublicKey
	publicKeyPem    []byte
	securityEnabled bool
	listenerCommand net.Listener
	listenerPublish net.Listener
}

func NewTransportService(config *config.Config, b common.BrokerService, u common.UserService) common.Service {

	privateKey, err := api.LoadKyberPrivateKeyFile(config.Crypto.PrivateKeyFile)
	if err != nil {
		log.Fatal(err.Error())
	}
	publicKey, err := api.LoadKyberPublicKeyFile(config.Crypto.PublicKeyFile)
	if err != nil {
		log.Fatal(err.Error())
	}
	publicKeyPem, err := os.ReadFile(config.Crypto.PublicKeyFile)
	if err != nil {
		log.Fatal(err.Error())
	}
	return &transport{
		config:          &config.Transport,
		broker:          b,
		users:           u,
		privateKey:      privateKey,
		publicKey:       publicKey,
		publicKeyPem:    publicKeyPem,
		securityEnabled: false,
	}
}

func (s *transport) Start() {
	var err error

	os.Remove(s.config.AddressCommand)
	os.Remove(s.config.AddressPublish)

	s.listenerCommand, err = net.Listen(s.config.Network, s.config.AddressCommand)
	if err != nil {
		log.Fatal("failed to create listenerCommand: %w", err)
	}
	log.Infof("transport listening on %s", s.listenerCommand.Addr())
	s.listenerPublish, err = net.Listen(s.config.Network, s.config.AddressPublish)
	if err != nil {
		log.Fatalf("failed to create publish listenerCommand: %v", err)
	}
	log.Infof("transport listening on publish %s", s.listenerPublish.Addr())
	go func() {
		for {
			conn, err := s.listenerCommand.Accept()
			if errors.Is(err, net.ErrClosed) {
				log.Infof("Connection closed!")
				break
			}
			go s.handleConnectionCommand(conn)
		}
	}()
	go func() {
		for {
			connChannel, err := s.listenerPublish.Accept()
			if err != nil {
				log.Infof("Accept error: %v", err)
				continue
			}
			go s.handleConnectionPublish(connChannel)
		}
	}()
	log.Info("Transport started")
}

func (s *transport) cleanupUnixSocket() {
	if s.config.Network == "unix" {
		_ = os.Remove(s.config.AddressCommand)
		_ = os.Remove(s.config.AddressPublish)
	}
}

func (s *transport) Shutdown() {
	s.listenerCommand.Close()
	s.cleanupUnixSocket()
	log.Infof("Transport shut down")
}

func (s *transport) handleConnectionCommand(conn net.Conn) {
	defer conn.Close()
	log.Infof("New connection from '%s'", conn.RemoteAddr().String())

	// CONNECT
	noCipher := api.NewNoCipher()
	msg, err := api.Receive(conn, noCipher)
	if err != nil {
		log.Errorf("Error receiving CONNECT: %v", err)
		return
	}
	if msg.Type != api.TypeConnect {
		log.Errorf("Error receiving CONNECT (%d): %v", msg.Type, msg)
		return
	}
	connectAckMsg := &api.Message{
		Type:     api.TypeConnectAck,
		Payload:  s.publicKeyPem,
		ClientId: msg.ClientId,
	}
	err = connectAckMsg.Send(conn, noCipher)
	if err != nil {
		log.Errorf("Error sending CONNECT_ACK: %v", err)
	}

	// AUTHENTICATE
	handshakeCipher := api.NewKyberCipher(s.privateKey, nil)
	msg, err = api.Receive(conn, handshakeCipher)
	if err != nil {
		log.Errorf("Failed to decode AUZTHENTICATE message: %v", err)
		return
	}
	if msg.Type != api.TypeAuthenticate {
		log.Errorf("Expected AUTHENTICATE, got %v", msg.Type)
		return
	}
	userName := string(msg.Payload)
	user, ok := s.users.LookupUserByName(userName)
	if !ok {
		log.Warnf("User %s not found", userName)
		return
	}
	log.Infof("New connection from '%s' for user '%s'", conn.RemoteAddr(), user.Name())
	clientId := msg.ClientId

	s.broker.RegisterClient(clientId, user)
	defer s.broker.UnregisterClient(clientId)
	handshakeCipher = api.NewKyberCipher(s.privateKey, user.PublicKey())

	authAck := &api.Message{
		Type:     api.TypeAuthenticateAck,
		ClientId: clientId,
		Payload:  []byte(s.config.AddressPublish),
	}
	if err := authAck.Send(conn, handshakeCipher); err != nil {
		log.Errorf("Failed to send AUTHENTICATE_ACK: %v", err)
		return
	}
	log.Infof("Client %s connected", clientId)

	// SESSION KEY
	msg, err = api.Receive(conn, handshakeCipher)
	if err != nil {
		log.Errorf("Failed to receive SESSION_KEY: %v", err)
		return
	}
	if msg.Type != api.TypeSessionKey {
		log.Errorf("Error receiving SESSION_KEY (%d): %v", msg.Type, msg)
		return
	}
	transportCipher, err := api.RecoverCHaCha20Cipher(s.privateKey, msg.Payload)
	if err != nil {
		log.Errorf("Error recovering CHA-20-CIPHER: %v", err)
		return
	}
	transportCipher.Enable(true)
	sessionKeyAck := &api.Message{
		Type:     api.TypeSessionKeyAck,
		ClientId: clientId,
	}
	err = sessionKeyAck.Send(conn, handshakeCipher)
	if err != nil {
		log.Errorf("Failed to send SESSION_KEY_ACK: %v", err)
		return
	}

	for {
		msg, err := api.Receive(conn, transportCipher)
		if err != nil {
			log.Errorf("Client %s receive error: %v", clientId, err)
			continue
		}
		if !s.handleMessage(clientId, conn, transportCipher, msg) {
			break
		}
	}
	log.Infof("Client %s disconnected", clientId)
}

func (s *transport) handleMessage(clientID string, conn net.Conn, cipher api.Cipher, msg *api.Message) bool {
	switch msg.Type {
	case api.TypePublish:
		s.broker.Publish(msg.Properties, msg.Topic, msg.Payload, clientID)
		connAck := &api.Message{
			Type:     api.TypePublishAck,
			ClientId: clientID,
		}
		if err := connAck.Send(conn, cipher); err != nil {
			log.Errorf("Failed to send PublishAck message: %v", err)
			return true
		}
	case api.TypeSubscribe:
		subscriptionId, err := s.broker.Subscribe(clientID, msg.Topic)
		if err != nil {
			log.Errorf("Subscribe error for client %s: %v", clientID, err)
			return true
		}
		connAck := &api.Message{
			Type:           api.TypeSubscribeAck,
			ClientId:       clientID,
			SubscriptionId: subscriptionId,
		}
		if err := connAck.Send(conn, cipher); err != nil {
			log.Errorf("Failed to send SubscribeAck message: %v", err)
			return true
		}
	case api.TypeUnsubscribe:
		if err := s.broker.Unsubscribe(clientID, msg.Topic, msg.SubscriptionId); err != nil {
			log.Errorf("Unsubscribe error for client %s: %v", clientID, err)
		}
		connAck := &api.Message{
			Type:     api.TypeUnsubscribeAck,
			ClientId: clientID,
		}
		if err := connAck.Send(conn, cipher); err != nil {
			log.Errorf("Failed to send UnsubscribeAck message: %v", err)
			return true
		}
	case api.TypePing:
		connAck := &api.Message{
			Type:     api.TypePong,
			ClientId: clientID,
		}
		if err := connAck.Send(conn, cipher); err != nil {
			log.Errorf("Failed to send Pong message: %v", err)
			return true
		}
	case api.TypeDisconnect:
		log.Infof("Client %s requested disconnect", clientID)
		return false
	default:
		log.Infof("Unknown message type from client '%s': %v", clientID, msg.Type)
	}
	return true
}

func (s *transport) handleConnectionPublish(conn net.Conn) {
	log.Infof("New publish connection from %s", conn.RemoteAddr())

	// CONNECT
	noCipher := api.NewNoCipher()
	msg, err := api.Receive(conn, noCipher)
	if err != nil {
		log.Errorf("Error receiving CONNECT: %v", err)
		return
	}
	if msg.Type != api.TypeConnect {
		log.Errorf("Error receiving CONNECT (%d): %v", msg.Type, msg)
		return
	}
	connectAckMsg := &api.Message{
		Type:     api.TypeConnectAck,
		Payload:  s.publicKeyPem,
		ClientId: msg.ClientId,
	}
	err = connectAckMsg.Send(conn, noCipher)
	if err != nil {
		log.Errorf("Error sending CONNECT_ACK: %v", err)
	}

	// AUTHENTICATE
	handshakeCipher := api.NewKyberCipher(s.privateKey, nil)
	msg, err = api.Receive(conn, handshakeCipher)
	if err != nil {
		log.Infof("Failed to decode connect message: %v", err)
		return
	}
	if msg.Type != api.TypeAuthenticate {
		log.Infof("Expected AUTHENTICATE, got %v", msg.Type)
		return
	}
	userName := string(msg.Payload)
	user, ok := s.users.LookupUserByName(userName)
	if !ok {
		log.Warnf("User '%s' not found", userName)
		return
	}
	log.Infof("New publish connection from '%s' for user '%s'", conn.RemoteAddr().Network(), user.Name())
	clientID := msg.ClientId
	if clientID == "" {
		log.Warnf("Empty client ID")
		return
	}
	handshakeCipher = api.NewKyberCipher(s.privateKey, user.PublicKey())
	connAck := &api.Message{
		Type:     api.TypeAuthenticateAck,
		ClientId: clientID,
		Payload:  []byte(s.config.AddressPublish),
	}
	if err := connAck.Send(conn, handshakeCipher); err != nil {
		log.Errorf("Failed to send message: %v", err)
		return
	}

	client := s.broker.Client(clientID)
	if client == nil {
		log.Warnf("Broker client '%s' not found", clientID)
	}

	// SESSION KEY
	msg, err = api.Receive(conn, handshakeCipher)
	if err != nil {
		log.Errorf("Failed to receive SESSION_KEY: %v", err)
		return
	}
	if msg.Type != api.TypeSessionKey {
		log.Errorf("Error receiving SESSION_KEY (%d): %v", msg.Type, msg)
		return
	}
	transportCipher, err := api.RecoverCHaCha20Cipher(s.privateKey, msg.Payload)
	if err != nil {
		log.Errorf("Error recovering CHA-20-CIPHER: %v", err)
		return
	}
	transportCipher.Enable(true)
	sessionKeyAck := &api.Message{
		Type:     api.TypeSessionKeyAck,
		ClientId: clientID,
	}
	err = sessionKeyAck.Send(conn, handshakeCipher)
	if err != nil {
		log.Errorf("Failed to send SESSION_KEY_ACK: %v", err)
		return
	}

	go func() {
		for msg := range client.MessageChan() {
			err = msg.Send(conn, transportCipher)
			if err != nil {
				log.Errorf("Failed publish message: %v", err)
			}
		}
	}()
	log.Infof("Client '%s' connected to publish", clientID)
}
