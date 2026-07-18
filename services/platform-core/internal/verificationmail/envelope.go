package verificationmail

import (
	"encoding/json"
	"errors"
	"time"

	"henukit.dev/platform-core/internal/securebox"
)

type Payload struct {
	Code      string    `json:"code"`
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Codec struct {
	recipient *securebox.Codec
	payload   *securebox.Codec
}

func NewCodec(masterKey []byte) (*Codec, error) {
	recipient, err := securebox.New(masterKey, "verification-recipient")
	if err != nil {
		return nil, err
	}
	payload, err := securebox.New(masterKey, "verification-payload")
	if err != nil {
		return nil, err
	}
	return &Codec{recipient: recipient, payload: payload}, nil
}

func (c *Codec) Encode(recipient string, payload Payload) ([]byte, []byte, error) {
	if recipient == "" || payload.Code == "" || payload.Purpose == "" || payload.ExpiresAt.IsZero() {
		return nil, nil, errors.New("verification mail envelope is incomplete")
	}
	recipientCiphertext, err := c.recipient.Seal([]byte(recipient))
	if err != nil {
		return nil, nil, err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	payloadCiphertext, err := c.payload.Seal(payloadBytes)
	if err != nil {
		return nil, nil, err
	}
	return recipientCiphertext, payloadCiphertext, nil
}

func (c *Codec) Decode(recipientCiphertext, payloadCiphertext []byte) (string, Payload, error) {
	recipient, err := c.recipient.Open(recipientCiphertext)
	if err != nil {
		return "", Payload{}, err
	}
	payloadBytes, err := c.payload.Open(payloadCiphertext)
	if err != nil {
		return "", Payload{}, err
	}
	var payload Payload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil || payload.Code == "" || payload.Purpose == "" || payload.ExpiresAt.IsZero() {
		return "", Payload{}, errors.New("verification mail payload is invalid")
	}
	return string(recipient), payload, nil
}
