package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const encryptedTokenPrefix = "enc:v1:"

type tokenCipher struct {
	aead cipher.AEAD
}

func newTokenCipher(secret string) (*tokenCipher, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("token encryption secret is required")
	}

	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create token cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create token aead: %w", err)
	}

	return &tokenCipher{aead: aead}, nil
}

func (c *tokenCipher) encrypt(plaintext string) (string, error) {
	if c == nil {
		return plaintext, nil
	}
	if strings.HasPrefix(plaintext, encryptedTokenPrefix) {
		return plaintext, nil
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate token nonce: %w", err)
	}

	ciphertext := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return encryptedTokenPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (c *tokenCipher) decrypt(value string) (string, error) {
	if c == nil || !strings.HasPrefix(value, encryptedTokenPrefix) {
		return value, nil
	}

	raw := strings.TrimPrefix(value, encryptedTokenPrefix)
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("decode encrypted token: %w", err)
	}

	nonceSize := c.aead.NonceSize()
	if len(payload) < nonceSize {
		return "", fmt.Errorf("encrypted token payload is too short")
	}
	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]

	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt token: %w", err)
	}
	return string(plaintext), nil
}
