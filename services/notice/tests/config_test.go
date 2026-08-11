package tests

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	notice "henukit.dev/notice"
)

func TestNoticeRejectsSharedConsoleAndPortalKeyIDs(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()

	for _, testCase := range []struct {
		name       string
		consoleKey string
		portalKey  string
		wantErr    bool
	}{
		{name: "shared key ID", consoleKey: "shared-key", portalKey: "shared-key", wantErr: true},
		{name: "distinct key IDs", consoleKey: "console-key", portalKey: "portal-key"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler, err := notice.New(notice.Config{
				Database:       &pgxpool.Pool{},
				Redis:          redisClient,
				ClientID:       "console-gateway-notice",
				Keys:           map[string]string{testCase.consoleKey: "console-notice-secret-at-least-32-bytes"},
				PortalClientID: "portal-gateway-notice-read",
				PortalKeys:     map[string]string{testCase.portalKey: "portal-notice-secret-at-least-32-bytes"},
			})
			if testCase.wantErr && err == nil {
				t.Fatalf("Notice.New accepted shared Console and Portal key ID: %#v", handler)
			}
			if !testCase.wantErr && (err != nil || handler == nil) {
				t.Fatalf("Notice.New rejected distinct key IDs: %#v, %v", handler, err)
			}
		})
	}
}
