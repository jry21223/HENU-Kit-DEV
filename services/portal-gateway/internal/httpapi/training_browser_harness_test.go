package httpapi

import (
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"henukit.dev/portal-gateway/internal/config"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrainingBrowserGatewayHarness(t *testing.T) {
	directory := os.Getenv("TRAINING_STACK_DIRECTORY")
	if directory == "" {
		t.Skip("explicit browser stack acceptance harness")
	}
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	defer rdb.Close()
	coreURL := "http://127.0.0.1:" + os.Getenv("TRAINING_CORE_PORT")
	read := config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: "portal-catalog-secret-at-least-32-bytes", KeyID: "portal-catalog-key-1"}
	command := config.ServiceAuth{ClientID: "portal-gateway-practice-command", ClientSecret: "portal-practice-command-secret-at-least-32-bytes", KeyID: "portal-practice-command-key-1"}
	handler, err := New(config.Config{SessionKey: []byte("0123456789abcdef0123456789abcdef"), PracticeURL: coreURL, QuizCraftCoreURL: coreURL, PracticeAuth: read, QuizCraftCoreAuth: read, PracticeCommandAuth: command, PracticeCommandsEnabled: true, QuizCraftCatalogEnabled: true, QuizCraftV2ReadsEnabled: true}, rdb)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+os.Getenv("TRAINING_GATEWAY_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler.Router())
	server.Listener = listener
	server.Start()
	defer server.Close()
	if err = os.WriteFile(filepath.Join(directory, "gateway-ready"), []byte(server.URL), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("browser acceptance timed out")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(directory, "stop")); err == nil {
				return
			}
		}
	}
}
