package securebox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

type Codec struct {
	aead cipher.AEAD
}

func New(master []byte, purpose string) (*Codec, error) {
	if len(master) != 32 || purpose == "" {
		return nil, errors.New("a 32-byte master key and purpose are required")
	}
	mac := hmac.New(sha256.New, master)
	_, _ = mac.Write([]byte("henukit-securebox:" + purpose))
	key := mac.Sum(nil)
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

func (c *Codec) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *Codec) Open(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < c.aead.NonceSize() {
		return nil, errors.New("ciphertext is invalid")
	}
	return c.aead.Open(nil, ciphertext[:c.aead.NonceSize()], ciphertext[c.aead.NonceSize():], nil)
}
