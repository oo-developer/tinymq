# tinymq

A lightweight, high-performance message broker written in Go, inspired by MQTT but simplified for speed and ease of use.

## Features

- **Lightweight**: Minimal overhead, no QoS complexity (for now)
- **Fast**: Built on Go's efficient networking
- **Flexible Transport**: Support for both TCP/IP and Unix domain sockets
- **Built-in Encryption**: TLS support for TCP connections
- **Simple Protocol**: Binary protocol optimized for performance
- **Pub/Sub**: Topic-based message routing with wildcard support

## Architecture

```
tinymq/
├── cmd/
│   ├── broker/        # Main broker server
│   ├── publisher/     # Example publisher client
│   └── subscriber/    # Example subscriber client
├── internal/
│   ├── broker/        # Core pub/sub logic
│   ├── protocol/      # Message protocol implementation
│   └── transport/     # Network transport layer (TCP/Unix)
└── pkg/
    └── client/        # Client library
```

## Building

```bash
# Build the broker
go build -o bin/broker ./cmd/broker

# Build example clients
go build -o bin/publisher ./cmd/publisher
go build -o bin/subscriber ./cmd/subscriber
```

## Running the Broker

### TCP Mode (Default)

```bash
# Start broker on TCP port 1883
./bin/broker -network tcp -address localhost:1883

# With TLS encryption
./bin/broker -network tcp -address localhost:8883 \
  -tls-cert /path/to/cert.pem -tls-key /path/to/key.pem
```

### Unix Socket Mode

```bash
# Start broker on Unix domain socket
./bin/broker -network unix -address /tmp/broker.sock
```

## Using the Client Library

### Publishing Messages

```go
package main

import (
    "log"
    "github.com/oo-developer/tinymq/pkg/client"
)

func main() {
    cfg := &client.Config{
        ClientID: "my-publisher",
        Network:  "tcp",
        Address:  "localhost:1883",
    }

    c, err := client.NewClient(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer c.Disconnect()

    err = c.Publish("sensors/temperature", []byte("23.5"))
    if err != nil {
        log.Fatal(err)
    }
}
```

### Subscribing to Topics

```go
package main

import (
    "log"
    "github.com/oo-developer/tinymq/pkg/client"
)

func main() {
    cfg := &client.Config{
        ClientID: "my-subscriber",
        Network:  "tcp",
        Address:  "localhost:1883",
    }

    c, err := client.NewClient(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer c.Disconnect()

    // Set message handler
    c.SetMessageHandler(func(topic string, payload []byte) {
        log.Printf("Received on %s: %s", topic, string(payload))
    })

    // Subscribe to topic
    err = c.Subscribe("sensors/temperature")
    if err != nil {
        log.Fatal(err)
    }

    // Keep running
    select {}
}
```

## Example Usage

### Terminal 1: Start the Broker
```bash
./bin/broker
```

### Terminal 2: Subscribe to a Topic
```bash
./bin/subscriber -topic "sensors/#"
```

### Terminal 3: Publish Messages
```bash
# Publish once
./bin/publisher -topic "sensors/temperature" -message "22.5"

# Publish continuously every 2 seconds
./bin/publisher -topic "sensors/humidity" -message "65%" -interval 2
```

## Protocol

The broker uses a simple binary protocol:

### Message Format
```
[Type:1][TopicLen:2][Topic:n][PayloadLen:4][Payload:n][ClientIDLen:2][ClientID:n]
```

### Message Types
- `TypeConnect` (0): Client connection request
- `TypeConnAck` (1): Connection acknowledgment
- `TypePublish` (2): Publish message
- `TypeSubscribe` (3): Subscribe to topic
- `TypeSubAck` (4): Subscription acknowledgment
- `TypeUnsubscribe` (5): Unsubscribe from topic
- `TypeUnsubAck` (6): Unsubscription acknowledgment
- `TypePing` (7): Keep-alive ping
- `TypePong` (8): Keep-alive response
- `TypeDisconnect` (9): Disconnect notification

## Topic Wildcards

Simple wildcard support:
- `sensors/#` - matches `sensors/temperature`, `sensors/humidity`, etc.
- Exact match: `sensors/temperature` matches only that topic

## TLS/Encryption

### Generating Self-Signed Certificates

```bash
# Generate private key
openssl genrsa -out server.key 2048

# Generate certificate
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 365
```

### Using TLS

```bash
# Start broker with TLS
./bin/broker -tls-cert server.crt -tls-key server.key -address localhost:8883
```

```go
// Client with TLS
cfg := &client.Config{
    ClientID: "secure-client",
    Network:  "tcp",
    Address:  "localhost:8883",
    TLSConfig: &tls.Config{
        InsecureSkipVerify: true, // For self-signed certs
    },
}
```

## Performance Considerations

- **Connection pooling**: Reuse client connections
- **Batch publishing**: Group messages when possible
- **Unix sockets**: Use for local communication (lower latency)
- **Channel buffer size**: Adjust `MessageChan` size in broker (currently 100)

## Future Enhancements

- [ ] QoS levels (at least once, exactly once)
- [ ] Message persistence
- [ ] Authentication/Authorization
- [ ] WebSocket support
- [ ] Clustering/horizontal scaling
- [ ] Advanced wildcards (MQTT-style + and # at any level)
- [ ] Retained messages
- [ ] Last will and testament
- [ ] Session persistence
- [ ] Metrics and monitoring

## Contributing

This is a foundational implementation. Contributions welcome for:
- Performance optimizations
- Additional features
- Test coverage
- Documentation improvements

## License

MIT License
