package consolegateway

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/httpapi"
	"henukit.dev/console-gateway/internal/overview"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

type Config struct {
	PlatformCoreURL, PlatformAccountOrigin     string
	ClientID, ClientSecret, KeyID, RedirectURI string
	SessionKey                                 []byte
	Redis                                      *redis.Client
	HTTPClient                                 *http.Client
	OverviewEndpoints                          map[string]string
	OverviewCredentials                        map[string]overview.Credentials
	Logger                                     *slog.Logger
}

func New(config Config) (http.Handler, error) {
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	codec, err := session.New(config.SessionKey)
	if err != nil {
		return nil, err
	}
	client, err := platformcore.New(config.PlatformCoreURL, config.ClientID, config.ClientSecret, config.KeyID, config.HTTPClient)
	if err != nil {
		return nil, err
	}
	for id, credential := range config.OverviewCredentials {
		if credential.ClientSecret == config.ClientSecret {
			return nil, fmt.Errorf("%s summary secret must be separate from Platform Core OAuth credentials", id)
		}
	}
	aggregator, err := overview.New(config.OverviewEndpoints, config.HTTPClient, config.Redis, config.OverviewCredentials, overview.Options{})
	if err != nil {
		return nil, err
	}
	return httpapi.New(config.PlatformAccountOrigin, config.ClientID, config.RedirectURI, client, aggregator, config.Redis, codec, config.Logger)
}
