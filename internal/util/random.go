package util

import (
	"crypto/rand"
	"encoding/base64"
)

// RandomString generates URL-safe random string with requested byte size.
func RandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
