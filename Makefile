.PHONY: all build test clean run-broker run-sub run-pub certs

all: build

build:
	@echo "Building broker..."
	@go build -o bin/broker ./cmd/broker
	@echo "Building example clients..."
	@go build -o bin/publisher ./cmd/publisher
	@go build -o bin/subscriber ./cmd/subscriber
	@echo "Build complete!"

test:
	@echo "Running tests..."
	@go test -v ./...

clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f *.crt *.key *.sock
	@echo "Clean complete!"

run-broker:
	@./bin/broker -network tcp -address localhost:1883

run-broker-tls: certs
	@./bin/broker -network tcp -address localhost:8883 \
		-tls-cert server.crt -tls-key server.key

run-broker-unix:
	@./bin/broker -network unix -address /tmp/broker.sock

run-sub:
	@./bin/subscriber -topic "test/#"

run-pub:
	@./bin/publisher -topic "test/demo" -message "Hello from tinymq!" -interval 2

# Generate self-signed certificates for testing
certs:
	@echo "Generating self-signed certificates..."
	@openssl genrsa -out server.key 2048
	@openssl req -new -x509 -sha256 -key server.key -out server.crt -days 365 \
		-subj "/C=US/ST=State/L=City/O=Organization/CN=localhost"
	@echo "Certificates generated: server.crt, server.key"

fmt:
	@go fmt ./...

vet:
	@go vet ./...

lint: fmt vet
	@echo "Linting complete!"
