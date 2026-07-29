package accountportfolio

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	pointCursorTokenPrefix = "plc1."
	pointCursorPurpose     = "henukit-account-portfolio-point-ledger-cursor-v1"
	pointCursorTTL         = 10 * time.Minute
	pointCursorKeyLength   = 32
)

var errInvalidPointCursor = errors.New("invalid point cursor")

// pointCursorCodec encrypts the pagination boundary so a browser cannot infer
// ledger timestamps or identifiers, and authenticates it before reuse.
type pointCursorCodec struct {
	aead cipher.AEAD
}

type sealedPointCursor struct {
	Version   int       `json:"v"`
	UserID    string    `json:"u"`
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
	IssuedAt  time.Time `json:"n"`
	ExpiresAt time.Time `json:"e"`
}

func newPointCursorCodec(key []byte) (*pointCursorCodec, error) {
	if !validPointCursorKey(key) {
		return nil, errors.New("point cursor key must contain exactly 32 bytes and not be all zero")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &pointCursorCodec{aead: aead}, nil
}

func validPointCursorKey(key []byte) bool {
	if len(key) != pointCursorKeyLength {
		return false
	}
	for _, value := range key {
		if value != 0 {
			return true
		}
	}
	return false
}

func (c *pointCursorCodec) encode(userID string, entry pointLedgerEntryView, issuedAt time.Time) (string, error) {
	if uuid.Validate(userID) != nil || uuid.Validate(entry.ID) != nil || entry.CreatedAt.IsZero() {
		return "", errInvalidPointCursor
	}
	issuedAt = issuedAt.UTC()
	payload, err := json.Marshal(sealedPointCursor{
		Version:   1,
		UserID:    userID,
		CreatedAt: entry.CreatedAt.UTC(),
		ID:        entry.ID,
		IssuedAt:  issuedAt,
		ExpiresAt: issuedAt.Add(pointCursorTTL),
	})
	if err != nil {
		return "", err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, payload, pointCursorAssociatedData(userID))
	return pointCursorTokenPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *pointCursorCodec) decode(encoded, expectedUserID string, now time.Time) (pointCursor, error) {
	if uuid.Validate(expectedUserID) != nil || !strings.HasPrefix(encoded, pointCursorTokenPrefix) {
		return pointCursor{}, errInvalidPointCursor
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, pointCursorTokenPrefix))
	if err != nil || len(ciphertext) < c.aead.NonceSize()+c.aead.Overhead() {
		return pointCursor{}, errInvalidPointCursor
	}
	nonce, ciphertext := ciphertext[:c.aead.NonceSize()], ciphertext[c.aead.NonceSize():]
	payload, err := c.aead.Open(nil, nonce, ciphertext, pointCursorAssociatedData(expectedUserID))
	if err != nil {
		return pointCursor{}, errInvalidPointCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var sealed sealedPointCursor
	if err := decoder.Decode(&sealed); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return pointCursor{}, errInvalidPointCursor
	}
	if !sealed.validFor(expectedUserID, now.UTC()) {
		return pointCursor{}, errInvalidPointCursor
	}
	return pointCursor{CreatedAt: sealed.CreatedAt.UTC(), ID: sealed.ID}, nil
}

func (value sealedPointCursor) validFor(expectedUserID string, now time.Time) bool {
	if value.Version != 1 || uuid.Validate(value.UserID) != nil || uuid.Validate(value.ID) != nil || value.CreatedAt.IsZero() || value.IssuedAt.IsZero() || value.ExpiresAt.IsZero() {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(value.UserID), []byte(expectedUserID)) != 1 || value.IssuedAt.After(now) || !value.ExpiresAt.Equal(value.IssuedAt.Add(pointCursorTTL)) || !value.ExpiresAt.After(now) {
		return false
	}
	return true
}

func pointCursorAssociatedData(userID string) []byte {
	return []byte(pointCursorPurpose + "\x00" + userID)
}
