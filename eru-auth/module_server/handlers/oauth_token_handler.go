package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_store"
	erujwt "github.com/eru-os/eru/eru-crypto/jwt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
)

// storeTokenSigner signs with the project's rsa key pair, the same keys eru-auth already publishes
// at /{project}/.well-known/jwks.json - so a resource server validates our tokens against a key set
// it can already fetch, with no introspection call.
type storeTokenSigner struct {
	store module_store.ModuleStoreI
}

func (signer storeTokenSigner) SignToken(ctx context.Context, projectId string, kid string, claims map[string]interface{}) (string, error) {
	if kid == "" {
		return "", fmt.Errorf("oauth_server.signing_kid is not set")
	}
	keyPair, err := signer.store.GetSigningKid(ctx, projectId, fmt.Sprint("ERUAUTH_KID_", kid), signer.store)
	if err != nil {
		return "", err
	}
	header := map[string]interface{}{"alg": "RS256", "typ": "JWT", "kid": kid}
	return erujwt.CreateJWT(ctx, keyPair.PrivateKey, claims, header)
}

// TokenHandler is the token endpoint of eru-auth's own oauth server, reached only when the auth
// runs with backend ERU.
func TokenHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("TokenHandler - Start")
		service, err := tokenServiceFromRequest(sh, r)
		if err != nil {
			writeOAuthError(w, r, err)
			return
		}

		if err = r.ParseForm(); err != nil {
			writeOAuthError(w, r, auth.NewOAuthError(400, "invalid_request", err.Error()))
			return
		}

		clientId, clientSecret := clientCredentials(r)
		if err = authenticateTokenClient(r, service, clientId, clientSecret); err != nil {
			writeOAuthError(w, r, err)
			return
		}

		var tokens auth.TokenResponse
		switch r.PostFormValue("grant_type") {
		case "authorization_code":
			tokens, err = service.ExchangeCode(r.Context(), clientId, r.PostFormValue("code"),
				r.PostFormValue("redirect_uri"), r.PostFormValue("code_verifier"))
		case "refresh_token":
			tokens, err = service.RefreshToken(r.Context(), clientId, r.PostFormValue("refresh_token"))
		default:
			err = auth.NewOAuthError(400, "unsupported_grant_type", "only authorization_code and refresh_token are supported")
		}
		if err != nil {
			writeOAuthError(w, r, err)
			return
		}

		// A token response must never be cached - it is a bearer credential.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(tokens)
	}
}

// RevokeTokenHandler retires a refresh token and every token issued from the same grant. It answers
// 200 whether or not the token existed, so it cannot be used to probe for valid tokens.
func RevokeTokenHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("RevokeTokenHandler - Start")
		service, err := tokenServiceFromRequest(sh, r)
		if err != nil {
			writeOAuthError(w, r, err)
			return
		}
		if err = r.ParseForm(); err != nil {
			writeOAuthError(w, r, auth.NewOAuthError(400, "invalid_request", err.Error()))
			return
		}
		if err = service.RevokeGrant(r.Context(), r.PostFormValue("token")); err != nil {
			writeOAuthError(w, r, auth.NewOAuthError(500, "server_error", err.Error()))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
	}
}

func tokenServiceFromRequest(sh *module_store.StoreHolder, r *http.Request) (auth.TokenService, error) {
	authObj, flowI, err := oauthFlowFromRequest(sh, r)
	if err != nil {
		return auth.TokenService{}, auth.NewOAuthError(404, "invalid_request", err.Error())
	}
	flow, flowOk := flowI.(auth.EruAuthorizationFlow)
	if !flowOk {
		return auth.TokenService{}, auth.NewOAuthError(404, "invalid_request",
			"the token endpoint is served by the configured oauth backend, not by eru-auth")
	}
	return auth.TokenService{
		Flow:   flow,
		Signer: storeTokenSigner{store: sh.Store},
		Config: authObj.OAuthServer(r.Context()),
	}, nil
}

// clientCredentials reads the client from basic auth if present, falling back to the form. Basic
// auth wins so a client that sends both cannot claim to be two different clients.
func clientCredentials(r *http.Request) (string, string) {
	if clientId, clientSecret, ok := r.BasicAuth(); ok && clientId != "" {
		return clientId, clientSecret
	}
	return r.PostFormValue("client_id"), r.PostFormValue("client_secret")
}

// authenticateTokenClient verifies a confidential client's secret. A public client presents no
// secret and is authenticated by pkce at the grant instead.
func authenticateTokenClient(r *http.Request, service auth.TokenService, clientId string, clientSecret string) error {
	if clientId == "" {
		return auth.NewOAuthError(401, "invalid_client", "client_id is required")
	}
	client, err := service.Flow.Registry.GetClient(r.Context(), clientId)
	if err != nil {
		return auth.NewOAuthError(401, "invalid_client", "client authentication failed")
	}
	if strings.EqualFold(client.TokenEndpointAuthMethod, "none") {
		return nil
	}
	eruRegistry, registryOk := service.Flow.Registry.(auth.EruClientRegistry)
	if !registryOk {
		return auth.NewOAuthError(401, "invalid_client", "client authentication failed")
	}
	return eruRegistry.VerifyClientSecret(r.Context(), clientId, clientSecret)
}
