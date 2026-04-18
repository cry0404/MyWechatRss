package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type Codec struct {
	gcm cipher.AEAD
}

func New(appSecret string) (*Codec, error) {
	if appSecret == "" {
		return nil, errors.New("app secret empty")
	}
	sum := sha256.Sum256([]byte(appSecret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Codec{gcm: gcm}, nil
}

func (c *Codec) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := c.gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.RawURLEncoding.EncodeToString(ct), nil
}

func (c *Codec) Decrypt(cipherText string) (string, error) {
	if cipherText == "" {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cipherText)
	if err != nil {
		return "", fmt.Errorf("base64: %w", err)
	}
	n := c.gcm.NonceSize()
	if len(raw) < n {
		return "", errors.New("cipher text too short")
	}
	nonce, ct := raw[:n], raw[n:]
	out, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("gcm open: %w", err)
	}
	return string(out), nil
}
