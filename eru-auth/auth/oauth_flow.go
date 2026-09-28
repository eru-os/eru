package auth

import (
	"context"
	"errors"
	"fmt"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// LoginRequestInfo is what the authorization server knows about a login step before anyone has
// been authenticated. Skip is set when the server already has a session it is willing to reuse,
// in which case no credentials are collected and Subject is already filled in.
type LoginRequestInfo struct {
	Challenge      string   `json:"challenge"`
	Skip           bool     `json:"skip"`
	Subject        string   `json:"subject"`
	ClientId       string   `json:"client_id"`
	ClientName     string   `json:"client_name"`
	RequestedScope []string `json:"requested_scope"`
	RequestUrl     string   `json:"request_url"`
}

// ConsentRequestInfo is what the consent screen renders: who is asking, and for what.
type ConsentRequestInfo struct {
	Challenge         string   `json:"challenge"`
	Skip              bool     `json:"skip"`
	Subject           string   `json:"subject"`
	ClientId          string   `json:"client_id"`
	ClientName        string   `json:"client_name"`
	ClientUri         string   `json:"client_uri"`
	LogoUri           string   `json:"logo_uri"`
	PolicyUri         string   `json:"policy_uri"`
	TosUri            string   `json:"tos_uri"`
	RequestedScope    []string `json:"requested_scope"`
	RequestedAudience []string `json:"requested_audience"`
}

// AuthorizationFlowI is the browser facing half of the authorization code grant: the steps that
// have to pause for a human. It is the second seam alongside ClientRegistryI - eru-auth's own
// implementation or a token backend's drives it without the handlers changing.
//
// Every Accept and Reject returns the url the browser must be sent to. That is the whole point of
// this interface: the existing AcceptLoginRequest and AcceptConsentRequest follow those redirects
// server side to mint tokens headlessly, which is right for the password api and wrong here.
type AuthorizationFlowI interface {
	LoginRequest(ctx context.Context, challenge string) (LoginRequestInfo, error)
	AcceptLogin(ctx context.Context, challenge string, subject string, remember bool) (redirectTo string, err error)
	RejectLogin(ctx context.Context, challenge string, errorCode string, errorDescription string) (redirectTo string, err error)
	ConsentRequest(ctx context.Context, challenge string) (ConsentRequestInfo, error)
	AcceptConsent(ctx context.Context, challenge string, grantScope []string, grantAudience []string, idTokenClaims map[string]interface{}, accessTokenClaims map[string]interface{}, remember bool) (redirectTo string, err error)
	RejectConsent(ctx context.Context, challenge string, errorCode string, errorDescription string) (redirectTo string, err error)
}

// AuthorizationFlow resolves the backend driving the interactive grant.
func (auth *Auth) AuthorizationFlow(ctx context.Context, projectId string) (AuthorizationFlowI, error) {
	if auth.usesTokenBackend() {
		tokenBackend, err := auth.tokenBackend(ctx)
		if err != nil {
			return nil, err
		}
		return tokenBackend.AuthorizationFlow(), nil
	}
	backend := auth.oauthBackend()
	switch backend {
	case OAuthBackendEru:
		registry, registryErr := auth.ClientRegistry(ctx, projectId)
		if registryErr != nil {
			return nil, registryErr
		}
		return EruAuthorizationFlow{
			AuthDb:    auth.AuthDb,
			ProjectId: projectId,
			AuthName:  auth.AuthName,
			Issuer:    auth.OAuthIssuer(ctx),
			Registry:  registry,
			Policy:    auth.OAuthServerConfig.ClientPolicy,
		}, nil
	default:
		err := errors.New(fmt.Sprint("unknown oauth server backend : ", backend))
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

// GrantableScope narrows what a consent step may grant to what the project allows, so a client
// cannot widen its own grant by asking for more than it registered for.
func (oAuthServerConfig OAuthServerConfig) GrantableScope(requestedScope []string) []string {
	allowed := oAuthServerConfig.ClientPolicy.Scopes()
	var grantScope []string
	for _, scope := range requestedScope {
		if contains(allowed, scope) {
			grantScope = append(grantScope, scope)
		}
	}
	return grantScope
}

func stringValue(value interface{}) string {
	if v, ok := value.(string); ok {
		return v
	}
	return ""
}

func boolValue(value interface{}) bool {
	if v, ok := value.(bool); ok {
		return v
	}
	return false
}

func stringValues(value interface{}) []string {
	values, _ := stringSlice(value)
	return values
}
