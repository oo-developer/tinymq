package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
)

type rsaCrypto struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	enabled    bool
}

func NewRsaCypher(privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey) Cypher {
	return &rsaCrypto{
		privateKey: privateKey,
		publicKey:  publicKey,
		enabled:    true,
	}
}

func (r *rsaCrypto) Enable(enabled bool) {
	r.enabled = enabled
}

func (r *rsaCrypto) Encrypt(bytes []byte) ([]byte, error) {
	if r.enabled {
		return Encrypt(r.publicKey, bytes)
	}
	return bytes, nil
}

func (r *rsaCrypto) Decrypt(bytes []byte) ([]byte, error) {
	if r.enabled {
		return Decrypt(r.privateKey, bytes)
	}
	return bytes, nil
}

func RsaLoadPrivateKeyFile(filepath string) (*rsa.PrivateKey, error) {
	keyData, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	// If OpenSSH parsing failed, try it with ParseRawPrivateKey
	key, err := ssh.ParseRawPrivateKey(keyData)
	if err == nil {
		// Type assert to *rsa.PrivateKey
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}
		return rsaKey, nil
	}
	return nil, fmt.Errorf("failed to parse private key: %w", err)
}

func LoadPublicKeyFile(filepath string) (*rsa.PublicKey, error) {
	keyData, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	return LoadPublicKey(keyData)
}

func LoadPublicKey(keyData []byte) (*rsa.PublicKey, error) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey(keyData)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(ssh.CryptoPublicKey).CryptoPublicKey().(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return rsaPub, nil
}

func Encrypt(publicKey *rsa.PublicKey, plaintext []byte) ([]byte, error) {
	hash := sha256.New()
	ciphertext, err := rsa.EncryptOAEP(hash, rand.Reader, publicKey, plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}
	return ciphertext, nil
}

// Decrypt decrypts data using RSA-OAEP with the private key
func Decrypt(privateKey *rsa.PrivateKey, ciphertext []byte) ([]byte, error) {
	hash := sha256.New()
	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, privateKey, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	return plaintext, nil
}
