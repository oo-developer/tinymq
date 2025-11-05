package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/pem"
	"fmt"
	"io"
	"os"

	"github.com/cloudflare/circl/kem/kyber/kyber768"
)

// Kyber768 provides security equivalent to AES-192 (NIST Level 3)
// Other options: kyber512 (AES-128), kyber1024 (AES-256)

type KyberPublicKey struct {
	key *kyber768.PublicKey
}

type KyberPrivateKey struct {
	key *kyber768.PrivateKey
}

// GenerateKyberKeyPair generates a new quantum-secure Kyber key pair
func GenerateKyberKeyPair() (*KyberPublicKey, *KyberPrivateKey, error) {
	pub, priv, err := kyber768.GenerateKeyPair(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate Kyber key pair: %w", err)
	}
	return &KyberPublicKey{key: pub}, &KyberPrivateKey{key: priv}, nil
}

// LoadKyberPrivateKeyFile loads a Kyber private key from a file
func LoadKyberPrivateKeyFile(filepath string) (*KyberPrivateKey, error) {
	keyData, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	// Try PEM format first
	block, _ := pem.Decode(keyData)
	if block != nil && block.Type == "KYBER PRIVATE KEY" {
		keyData = block.Bytes
	}

	scheme := kyber768.Scheme()
	priv, _ := scheme.UnmarshalBinaryPrivateKey(keyData)
	if priv == nil {
		return nil, fmt.Errorf("failed to parse Kyber private key")
	}

	kyberPriv, ok := priv.(*kyber768.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid Kyber private key type")
	}

	return &KyberPrivateKey{key: kyberPriv}, nil
}

// LoadKyberPublicKeyFile loads a Kyber public key from a file
func LoadKyberPublicKeyFile(filepath string) (*KyberPublicKey, error) {
	keyData, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	return LoadKyberPublicKey(keyData)
}

// LoadKyberPublicKey parses a Kyber public key from bytes
func LoadKyberPublicKey(keyData []byte) (*KyberPublicKey, error) {
	// Try PEM format first
	block, _ := pem.Decode(keyData)
	if block != nil && block.Type == "KYBER PUBLIC KEY" {
		keyData = block.Bytes
	}

	scheme := kyber768.Scheme()
	pub, _ := scheme.UnmarshalBinaryPublicKey(keyData)
	if pub == nil {
		return nil, fmt.Errorf("failed to parse Kyber public key")
	}

	kyberPub, ok := pub.(*kyber768.PublicKey)
	if !ok {
		return nil, fmt.Errorf("invalid Kyber public key type")
	}

	return &KyberPublicKey{key: kyberPub}, nil
}

// EncodeKyberPrivateKeyPEM encodes a Kyber private key to PEM format
func EncodeKyberPrivateKeyPEM(key *KyberPrivateKey) ([]byte, error) {
	privBytes, err := key.key.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	privBlock := &pem.Block{
		Type:  "KYBER PRIVATE KEY",
		Bytes: privBytes,
	}
	return pem.EncodeToMemory(privBlock), nil
}

// EncodeKyberPublicKeyPEM encodes a Kyber public key to PEM format
func EncodeKyberPublicKeyPEM(key *KyberPublicKey) ([]byte, error) {
	pubBytes, err := key.key.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	pubBlock := &pem.Block{
		Type:  "KYBER PUBLIC KEY",
		Bytes: pubBytes,
	}
	return pem.EncodeToMemory(pubBlock), nil
}

// EncryptKyber encrypts data using Kyber KEM + AES-GCM
// Returns: encapsulated key || nonce || ciphertext || tag
func EncryptKyber(publicKey *KyberPublicKey, plaintext []byte) ([]byte, error) {
	scheme := kyber768.Scheme()

	// Step 1: Encapsulate - generate shared secret and ciphertext
	ciphertext, sharedSecret, err := scheme.Encapsulate(publicKey.key)
	if err != nil {
		return nil, fmt.Errorf("encapsulation failed: %w", err)
	}

	// Step 2: Derive AES key from shared secret
	aesKey := sha256.Sum256(sharedSecret)

	// Step 3: Encrypt plaintext with AES-GCM
	block, err := aes.NewCipher(aesKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	encrypted := gcm.Seal(nil, nonce, plaintext, nil)

	// Step 4: Combine all parts
	// Format: [kyber_ciphertext][nonce][aes_encrypted_data]
	result := make([]byte, 0, len(ciphertext)+len(nonce)+len(encrypted))
	result = append(result, ciphertext...)
	result = append(result, nonce...)
	result = append(result, encrypted...)

	return result, nil
}

// DecryptKyber decrypts data encrypted with EncryptKyber
func DecryptKyber(privateKey *KyberPrivateKey, ciphertext []byte) ([]byte, error) {
	scheme := kyber768.Scheme()

	// Expected sizes
	kemCiphertextSize := scheme.CiphertextSize()
	nonceSize := 12 // GCM standard nonce size

	if len(ciphertext) < kemCiphertextSize+nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Step 1: Extract parts
	kemCiphertext := ciphertext[:kemCiphertextSize]
	nonce := ciphertext[kemCiphertextSize : kemCiphertextSize+nonceSize]
	encryptedData := ciphertext[kemCiphertextSize+nonceSize:]

	// Step 2: Decapsulate - recover shared secret
	sharedSecret, err := scheme.Decapsulate(privateKey.key, kemCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decapsulation failed: %w", err)
	}

	// Step 3: Derive AES key from shared secret
	aesKey := sha256.Sum256(sharedSecret)

	// Step 4: Decrypt with AES-GCM
	block, err := aes.NewCipher(aesKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// SaveKyberKeyPair saves a key pair to files
func SaveKyberKeyPair(publicPath, privatePath string, pub *KyberPublicKey, priv *KyberPrivateKey) error {
	pubPEM, err := EncodeKyberPublicKeyPEM(pub)
	if err != nil {
		return err
	}

	privPEM, err := EncodeKyberPrivateKeyPEM(priv)
	if err != nil {
		return err
	}

	if err := os.WriteFile(publicPath, pubPEM, 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	if err := os.WriteFile(privatePath, privPEM, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	return nil
}
