package api

type Cypher interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
	Enable(enable bool)
}
