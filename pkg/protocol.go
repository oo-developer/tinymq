package api

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type MessageType byte
type MessageBehaviour byte

const (
	TypeMessage MessageType = iota
	TypeMessageAck
	TypeConnect
	TypeConnAck
	TypePublish
	TypePublishAck
	TypeSubscribe
	TypeSubscribeAck
	TypeUnsubscribe
	TypeUnsubscribeAck
	TypePing
	TypePong
	TypeDisconnect
)

const (
	MsgRetained   MessageBehaviour = 1
	MsgPersistent MessageBehaviour = 2
)

type RawMessage struct {
}

type Message struct {
	Type     MessageType
	Topic    string
	Payload  []byte
	ClientId string
}

func (m *Message) Send(w io.Writer, cypher Cypher) error {
	buffer := bytes.Buffer{}
	err := m.encode(&buffer)
	if err != nil {
		return err
	}
	payloadData := buffer.Bytes()
	encryptedData, err := cypher.Encrypt(payloadData)
	if err != nil {
		return fmt.Errorf("encrypt failed: %v", err)
	}
	if err := binary.Write(w, binary.BigEndian, uint16(len(encryptedData))); err != nil {
		return fmt.Errorf("failed to write data length: %w", err)
	}
	if _, err := w.Write(encryptedData); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	return nil
}

func Receive(r io.Reader, cypher Cypher) (*Message, error) {
	var dataLen uint16
	if err := binary.Read(r, binary.BigEndian, &dataLen); err != nil {
		return nil, fmt.Errorf("failed to read topic length: %w", err)
	}
	data := make([]byte, dataLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read topic: %w", err)
	}
	decryptedData, err := cypher.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}
	buffer := bytes.NewBuffer(decryptedData)
	msg, err := decode(buffer)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (m *Message) encode(w io.Writer) error {
	// Message format: [Type:1][TopicLen:2][Topic:n][PayloadLen:4][Payload:n][ClientIDLen:2][ClientId:n]
	if err := binary.Write(w, binary.BigEndian, m.Type); err != nil {
		return fmt.Errorf("failed to write type: %w", err)
	}

	topicBytes := []byte(m.Topic)
	if err := binary.Write(w, binary.BigEndian, uint16(len(topicBytes))); err != nil {
		return fmt.Errorf("failed to write topic length: %w", err)
	}
	if _, err := w.Write(topicBytes); err != nil {
		return fmt.Errorf("failed to write topic: %w", err)
	}

	if err := binary.Write(w, binary.BigEndian, uint32(len(m.Payload))); err != nil {
		return fmt.Errorf("failed to write payload length: %w", err)
	}
	if _, err := w.Write(m.Payload); err != nil {
		return fmt.Errorf("failed to write payload: %w", err)
	}

	clientIDBytes := []byte(m.ClientId)
	if err := binary.Write(w, binary.BigEndian, uint16(len(clientIDBytes))); err != nil {
		return fmt.Errorf("failed to write client ID length: %w", err)
	}
	if _, err := w.Write(clientIDBytes); err != nil {
		return fmt.Errorf("failed to write client ID: %w", err)
	}

	return nil
}

func decode(r io.Reader) (*Message, error) {
	msg := &Message{}

	if err := binary.Read(r, binary.BigEndian, &msg.Type); err != nil {
		return nil, fmt.Errorf("failed to read type: %w", err)
	}

	var topicLen uint16
	if err := binary.Read(r, binary.BigEndian, &topicLen); err != nil {
		return nil, fmt.Errorf("failed to read topic length: %w", err)
	}
	topicBytes := make([]byte, topicLen)
	if _, err := io.ReadFull(r, topicBytes); err != nil {
		return nil, fmt.Errorf("failed to read topic: %w", err)
	}
	msg.Topic = string(topicBytes)

	var payloadLen uint32
	if err := binary.Read(r, binary.BigEndian, &payloadLen); err != nil {
		return nil, fmt.Errorf("failed to read payload length: %w", err)
	}
	msg.Payload = make([]byte, payloadLen)
	if _, err := io.ReadFull(r, msg.Payload); err != nil {
		return nil, fmt.Errorf("failed to read payload: %w", err)
	}

	var clientIDLen uint16
	if err := binary.Read(r, binary.BigEndian, &clientIDLen); err != nil {
		return nil, fmt.Errorf("failed to read client ID length: %w", err)
	}
	clientIDBytes := make([]byte, clientIDLen)
	if _, err := io.ReadFull(r, clientIDBytes); err != nil {
		return nil, fmt.Errorf("failed to read client ID: %w", err)
	}
	msg.ClientId = string(clientIDBytes)

	return msg, nil
}
