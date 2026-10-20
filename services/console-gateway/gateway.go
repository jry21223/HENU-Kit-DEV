package consolegateway

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/accountportfolio"
	"henukit.dev/console-gateway/internal/food"
	"henukit.dev/console-gateway/internal/httpapi"
	"henukit.dev/console-gateway/internal/library"
	"henukit.dev/console-gateway/internal/notice"
	"henukit.dev/console-gateway/internal/overview"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/quizcraft"
	"henukit.dev/console-gateway/internal/session"
)

type Config struct {
	QuizcraftManagementEnabled                 bool
	QuizcraftAPIURL                            string
	QuizcraftCredentials                       overview.Credentials
	PlatformCoreURL, PlatformAccountOrigin     string
	ClientID, ClientSecret, KeyID, RedirectURI string
	SessionKey                                 []byte
	Redis                                      *redis.Client
	HTTPClient                                 *http.Client
	OverviewEndpoints                          map[string]string
	OverviewCredentials                        map[string]overview.Credentials
	NoticeAPIURL                               string
	NoticeCredentials                          overview.Credentials
	LibraryAPIURL                              string
	LibraryCredentials                         overview.Credentials
	FoodAPIURL                                 string
	FoodCredentials                            overview.Credentials
	AccountPortfolioAPIURL                     string
	AccountPortfolioCredentials                overview.Credentials
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
	var noticeClient *notice.Client
	if config.NoticeAPIURL != "" {
		noticeClient, err = notice.New(config.NoticeAPIURL, config.NoticeCredentials.ClientID, config.NoticeCredentials.ClientSecret, config.NoticeCredentials.KeyID, config.HTTPClient)
		if err != nil {
			return nil, err
		}
	}
	var libraryClient *library.Client
	if config.LibraryAPIURL != "" {
		libraryClient, err = library.New(config.LibraryAPIURL, config.LibraryCredentials.ClientID, config.LibraryCredentials.ClientSecret, config.LibraryCredentials.KeyID, config.HTTPClient)
		if err != nil {
			return nil, err
		}
	}
	var foodClient *food.Client
	if config.FoodAPIURL != "" {
		foodClient, err = food.New(config.FoodAPIURL, config.FoodCredentials.ClientID, config.FoodCredentials.ClientSecret, config.FoodCredentials.KeyID, config.HTTPClient)
		if err != nil {
			return nil, err
		}
	}
	var accountPortfolioClient *accountportfolio.Client
	if config.AccountPortfolioAPIURL != "" {
		if config.AccountPortfolioCredentials.ClientSecret == config.ClientSecret {
			return nil, fmt.Errorf("account portfolio secret must be separate from Platform Core OAuth credentials")
		}
		accountPortfolioClient, err = accountportfolio.New(config.AccountPortfolioAPIURL, config.AccountPortfolioCredentials.ClientID, config.AccountPortfolioCredentials.ClientSecret, config.AccountPortfolioCredentials.KeyID, config.HTTPClient)
		if err != nil {
			return nil, err
		}
	}
	var quizcraftClient *quizcraft.Client
	if config.QuizcraftManagementEnabled {
		secret := config.QuizcraftCredentials.ClientSecret
		if secret == config.ClientSecret || secret == string(config.SessionKey) || secret == base64.StdEncoding.EncodeToString(config.SessionKey) || secret == config.AccountPortfolioCredentials.ClientSecret || secret == config.NoticeCredentials.ClientSecret || secret == config.LibraryCredentials.ClientSecret || secret == config.FoodCredentials.ClientSecret {
			return nil, fmt.Errorf("QuizCraft management secret must be separate from browser Session and owner/OAuth credentials")
		}
		for _, credential := range config.OverviewCredentials {
			if credential.ClientSecret == secret {
				return nil, fmt.Errorf("QuizCraft management secret must be separate from summary credentials")
			}
		}
		quizcraftClient, err = quizcraft.New(config.QuizcraftAPIURL, config.QuizcraftCredentials.ClientID, secret, config.QuizcraftCredentials.KeyID, config.HTTPClient)
		if err != nil {
			return nil, err
		}
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
	return httpapi.New(config.PlatformAccountOrigin, config.ClientID, config.RedirectURI, client, noticeClient, aggregator, config.Redis, codec, config.Logger, libraryClient, foodClient, accountPortfolioClient, quizcraftClient)
}
