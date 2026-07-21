package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const associatedData = "henukit-portal-session-v1"

// Value is the session payload stored in the encrypted cookie.
type Value struct {
	UserID        string    `json:"user_id"`
	ExchangeToken string    `json:"exchange_token"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// Codec encrypts and decrypts session cookie values using AES-256-GCM.
type Codec struct {
	aead cipher.AEAD
}

// NewCodec creates a Codec with a 32-byte key.
func NewCodec(key []byte) (*Codec, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("session key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	return &Codec{aead: aead}, nil
}

// Encode encrypts the session value into a base64 string.
func (c *Codec) Encode(v Value) (string, error) {
	plaintext, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("json.Marshal: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	ciphertext := c.aead.Seal(nonce, nonce, plaintext, []byte(associatedData))
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decode decrypts a base64-encoded session string into a Value.
func (c *Codec) Decode(encoded string) (Value, error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Value{}, fmt.Errorf("base64 decode: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return Value{}, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(associatedData))
	if err != nil {
		return Value{}, fmt.Errorf("aes-gcm open: %w", err)
	}
	var v Value
	if err := json.Unmarshal(plaintext, &v); err != nil {
		return Value{}, fmt.Errorf("json.Unmarshal: %w", err)
	}
	if v.UserID == "" {
		return Value{}, fmt.Errorf("empty user_id")
	}
	if len(v.ExchangeToken) < 32 {
		return Value{}, fmt.Errorf("exchange_token too short")
	}
	if v.ExpiresAt.IsZero() {
		return Value{}, fmt.Errorf("zero expires_at")
	}
	return v, nil
}
