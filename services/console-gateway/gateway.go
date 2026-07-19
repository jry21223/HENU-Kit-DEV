package consolegateway

import (
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/httpapi"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

type Config struct {
	PlatformCoreURL, PlatformAccountOrigin     string
	ClientID, ClientSecret, KeyID, RedirectURI string
	SessionKey                                 []byte
	Redis                                      *redis.Client
	HTTPClient                                 *http.Client
	Logger                                     *slog.Logger
}

func New(config Config) (http.Handler, error) {
	codec, err := session.New(config.SessionKey)
	if err != nil {
		return nil, err
	}
	client, err := platformcore.New(config.PlatformCoreURL, config.ClientID, config.ClientSecret, config.KeyID, config.HTTPClient)
	if err != nil {
		return nil, err
	}
	return httpapi.New(config.PlatformAccountOrigin, config.ClientID, config.RedirectURI, client, config.Redis, codec, config.Logger)
}
