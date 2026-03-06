package db

import "testing"

func TestTokenCipherEncryptDecrypt(t *testing.T) {
	t.Parallel()

	c, err := newTokenCipher("test-secret-1234567890")
	if err != nil {
		t.Fatalf("newTokenCipher returned error: %v", err)
	}

	ciphertext, err := c.encrypt("access-token-value")
	if err != nil {
		t.Fatalf("encrypt returned error: %v", err)
	}
	if ciphertext == "access-token-value" {
		t.Fatalf("encrypt did not transform plaintext")
	}

	plaintext, err := c.decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt returned error: %v", err)
	}
	if plaintext != "access-token-value" {
		t.Fatalf("decrypt mismatch: got %q", plaintext)
	}
}

func TestTokenCipherBackwardCompatibilityWithPlaintext(t *testing.T) {
	t.Parallel()

	c, err := newTokenCipher("test-secret-1234567890")
	if err != nil {
		t.Fatalf("newTokenCipher returned error: %v", err)
	}

	plaintext, err := c.decrypt("legacy-plain-token")
	if err != nil {
		t.Fatalf("decrypt plaintext returned error: %v", err)
	}
	if plaintext != "legacy-plain-token" {
		t.Fatalf("plaintext mismatch: got %q", plaintext)
	}
}
