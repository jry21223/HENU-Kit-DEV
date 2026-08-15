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

// CareerDigest is the #397 controlled digest payload: only a summary of the
// persisted Search Result, never the raw profile. The worker renders it into a
// fixed template; the browser never supplies arbitrary recipients or subjects.
type CareerDigest struct {
	SearchID     string `json:"search_id"`
	CompletedAt  string `json:"completed_at"`
	SourceCount  int    `json:"source_count"`
	JobCount     int    `json:"job_count"`
	MatchedCount int    `json:"matched_count"`
	Summary      string `json:"summary"`
	CareerURL    string `json:"career_url"`
	TopJobs      []Job  `json:"top_jobs"`
}

// Job is the browser-safe subset of a Career search result for one top match.
type Job struct {
	Company      string   `json:"company"`
	Title        string   `json:"title"`
	Location     string   `json:"location"`
	URL          string   `json:"url"`
	MatchScore   int      `json:"match_score"`
	MatchReasons []string `json:"match_reasons"`
}

// DigestCodec seals a Career digest recipient+payload under dedicated labels,
// separate from the verification codec so the two mail kinds never decode each
// other's ciphertext.
type DigestCodec struct {
	recipient *securebox.Codec
	payload   *securebox.Codec
}

func NewDigestCodec(masterKey []byte) (*DigestCodec, error) {
	recipient, err := securebox.New(masterKey, "career-digest-recipient")
	if err != nil {
		return nil, err
	}
	payload, err := securebox.New(masterKey, "career-digest-payload")
	if err != nil {
		return nil, err
	}
	return &DigestCodec{recipient: recipient, payload: payload}, nil
}

func (c *DigestCodec) Encode(recipient string, digest CareerDigest) ([]byte, []byte, error) {
	if recipient == "" || digest.SearchID == "" {
		return nil, nil, errors.New("career digest envelope is incomplete")
	}
	recipientCiphertext, err := c.recipient.Seal([]byte(recipient))
	if err != nil {
		return nil, nil, err
	}
	payloadBytes, err := json.Marshal(digest)
	if err != nil {
		return nil, nil, err
	}
	payloadCiphertext, err := c.payload.Seal(payloadBytes)
	if err != nil {
		return nil, nil, err
	}
	return recipientCiphertext, payloadCiphertext, nil
}

func (c *DigestCodec) Decode(recipientCiphertext, payloadCiphertext []byte) (string, CareerDigest, error) {
	recipient, err := c.recipient.Open(recipientCiphertext)
	if err != nil {
		return "", CareerDigest{}, err
	}
	payloadBytes, err := c.payload.Open(payloadCiphertext)
	if err != nil {
		return "", CareerDigest{}, err
	}
	var digest CareerDigest
	if err := json.Unmarshal(payloadBytes, &digest); err != nil || digest.SearchID == "" {
		return "", CareerDigest{}, errors.New("career digest payload is invalid")
	}
	return string(recipient), digest, nil
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
