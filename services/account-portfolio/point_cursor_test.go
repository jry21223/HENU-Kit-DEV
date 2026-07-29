package accountportfolio

import (
	"testing"
	"time"
)

func TestPointCursorCodecRejectsFutureIssuedToken(t *testing.T) {
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	codec, err := newPointCursorCodec([]byte("account-portfolio-cursor-key-123"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := codec.encode("11111111-1111-4111-8111-111111111111", pointLedgerEntryView{
		ID:        "22222222-2222-4222-8222-222222222222",
		CreatedAt: now,
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codec.decode(token, "11111111-1111-4111-8111-111111111111", now); err == nil {
		t.Fatal("point cursor codec accepted a token issued in the future")
	}
}
