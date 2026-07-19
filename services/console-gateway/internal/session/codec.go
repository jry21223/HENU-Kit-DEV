package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"
)

var ErrInvalid = errors.New("invalid console session")

type Value struct {
	UserID        string    `json:"user_id"`
	ExchangeToken string    `json:"exchange_token"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type Codec struct{ aead cipher.AEAD }

func New(key []byte) (*Codec, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Codec{aead: aead}, nil
}

func (c *Codec) Encode(value Value) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, payload, []byte("henukit-console-session-v1"))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *Codec) Decode(encoded string) (Value, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) < c.aead.NonceSize() {
		return Value{}, ErrInvalid
	}
	nonce, ciphertext := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	payload, err := c.aead.Open(nil, nonce, ciphertext, []byte("henukit-console-session-v1"))
	if err != nil {
		return Value{}, ErrInvalid
	}
	var value Value
	if err := json.Unmarshal(payload, &value); err != nil || value.UserID == "" || len(value.ExchangeToken) < 32 || value.ExpiresAt.IsZero() {
		return Value{}, ErrInvalid
	}
	return value, nil
}
