package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type TokenBackendI interface {
	MakeTokens(ctx context.Context, identity Identity) (LoginSuccess, error)
	FetchTokens(ctx context.Context, refreshToken string, userId string) (interface{}, error)
	LoginApi(ctx context.Context, refreshToken string) (interface{}, error)
	GetUserInfo(ctx context.Context, accessToken string) (Identity, error)
	RevokeToken(ctx context.Context, refreshToken string) (int, error)
	SaveClients(ctx context.Context) error
	RemoveClients(ctx context.Context) error
	PublicUrl() string
	GetAttribute(attributeName string) (interface{}, bool)
	ClientRegistry() ClientRegistryI
	AuthorizationFlow() AuthorizationFlowI
}

var (
	tokenBackendName    string
	tokenBackendFactory func(config json.RawMessage) (TokenBackendI, error)
)

var ErrNoTokenBackend = errors.New("no token backend is configured for this auth - use the oauth server with backend ERU")

func RegisterTokenBackend(name string, newBackend func(config json.RawMessage) (TokenBackendI, error)) {
	if tokenBackendFactory != nil {
		panic(fmt.Sprintf("token backend %s registered after %s", name, tokenBackendName))
	}
	tokenBackendName = strings.ToUpper(name)
	tokenBackendFactory = newBackend
}

func (auth *Auth) tokenBackend(ctx context.Context) (TokenBackendI, error) {
	if tokenBackendFactory == nil || len(auth.TokenBackendConfig) == 0 {
		return nil, ErrNoTokenBackend
	}
	backend, err := tokenBackendFactory(auth.TokenBackendConfig)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return backend, nil
}

func (auth *Auth) oauthBackend() string {
	if auth.OAuthServerConfig.Backend != "" {
		return strings.ToUpper(auth.OAuthServerConfig.Backend)
	}
	if tokenBackendName != "" {
		return tokenBackendName
	}
	return OAuthBackendEru
}

func (auth *Auth) usesTokenBackend() bool {
	return tokenBackendName != "" && auth.oauthBackend() == tokenBackendName
}

func (auth *Auth) saveTokenBackendClients(ctx context.Context) error {
	backend, err := auth.tokenBackend(ctx)
	if errors.Is(err, ErrNoTokenBackend) {
		return nil
	}
	if err != nil {
		return err
	}
	return backend.SaveClients(ctx)
}

func (auth *Auth) tokenBackendUserInfo(ctx context.Context, accessToken string) (Identity, error) {
	backend, err := auth.tokenBackend(ctx)
	if err != nil {
		return Identity{}, err
	}
	return backend.GetUserInfo(ctx, accessToken)
}
