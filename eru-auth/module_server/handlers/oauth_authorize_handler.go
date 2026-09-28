package handlers

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-server/server"
)

// AuthorizeHandler is the authorization endpoint of eru-auth's own oauth server. It is only
// reached when the auth runs with backend ERU; with a token backend the client goes to that
// backend's /oauth2/auth instead and this endpoint is not advertised.
//
// One request arrives here up to three times: once to start, once after login, once after consent.
// Each time it works out what is still missing and sends the browser to the step that supplies it.
func AuthorizeHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("AuthorizeHandler - Start")
		authObj, flowI, err := oauthFlowFromRequest(sh, r)
		if err != nil {
			writeOAuthPageError(w, r, auth.OAuthServerConfig{}, err)
			return
		}
		oAuthServer := authObj.OAuthServer(r.Context())

		flow, flowOk := flowI.(auth.EruAuthorizationFlow)
		if !flowOk {
			writeOAuthPageError(w, r, oAuthServer,
				fmt.Errorf("the authorization endpoint is served by the configured oauth backend, not by eru-auth"))
			return
		}

		// Resuming a request that has already been through login or consent.
		if requestId := r.URL.Query().Get("request_id"); requestId != "" {
			resumeAuthorization(w, r, oAuthServer, flow, requestId)
			return
		}

		registry, err := authObj.ClientRegistry(r.Context(), server.RequestProject(r))
		if err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}
		client, err := registry.GetClient(r.Context(), r.URL.Query().Get("client_id"))
		if err != nil {
			// An unknown client has no redirect uri worth trusting, so this is rendered rather than
			// redirected.
			writeOAuthPageError(w, r, oAuthServer, auth.NewOAuthError(400, "invalid_client", "unknown client"))
			return
		}

		authRequest, err := auth.ValidateAuthorizationRequest(r.Context(), r.URL.Query(), client, oAuthServer.ClientPolicy)
		if err != nil {
			// Once the redirect uri is known to be registered, protocol errors belong back at the
			// client. Before that they are rendered, so an unverified uri is never redirected to.
			if authRequest.RedirectUri != "" && redirectUriIsRegistered(authRequest.RedirectUri, client) {
				redirectWithOAuthError(w, r, authRequest.RedirectUri, authRequest.State, err)
				return
			}
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}

		requestId, err := flow.CreateAuthorizationRequest(r.Context(), authRequest)
		if err != nil {
			writeOAuthPageError(w, r, oAuthServer, err)
			return
		}

		// A browser that has already authenticated skips straight past the login screen. The session
		// only says who is signed in - consent is still asked for separately.
		if identityId := flow.SessionIdentity(r.Context(), oauthSessionId(r)); identityId != "" {
			if _, acceptErr := flow.AcceptLogin(r.Context(), requestId, identityId, true); acceptErr != nil {
				writeOAuthPageError(w, r, oAuthServer, acceptErr)
				return
			}
			http.Redirect(w, r, consentStepUrl(r, requestId), http.StatusFound)
			return
		}

		http.Redirect(w, r, loginStepUrl(r, requestId), http.StatusFound)
	}
}

// resumeAuthorization decides what the in flight request still needs.
func resumeAuthorization(w http.ResponseWriter, r *http.Request, oAuthServer auth.OAuthServerConfig, flow auth.EruAuthorizationFlow, requestId string) {
	authRequest, err := flow.AuthorizationRequestById(r.Context(), requestId)
	if err != nil {
		writeOAuthPageError(w, r, oAuthServer, err)
		return
	}
	switch {
	case !authRequest.LoggedIn():
		http.Redirect(w, r, loginStepUrl(r, requestId), http.StatusFound)
	case !authRequest.Consented():
		http.Redirect(w, r, consentStepUrl(r, requestId), http.StatusFound)
	default:
		// Already complete - the code was issued on the way through consent, and an authorization
		// request is not replayable, so there is nothing left to hand back.
		writeOAuthPageError(w, r, oAuthServer, fmt.Errorf("this authorization has already been completed"))
	}
}

func loginStepUrl(r *http.Request, requestId string) string {
	return fmt.Sprint(auth.OAuthLoginPath, "?login_challenge=", url.QueryEscape(requestId))
}

func consentStepUrl(r *http.Request, requestId string) string {
	return fmt.Sprint(auth.OAuthConsentPath, "?consent_challenge=", url.QueryEscape(requestId))
}

func redirectUriIsRegistered(redirectUri string, client auth.OAuthClient) bool {
	for _, registered := range client.RedirectURIs {
		if registered == redirectUri {
			return true
		}
	}
	return false
}

func redirectWithOAuthError(w http.ResponseWriter, r *http.Request, redirectUri string, state string, err error) {
	oAuthError, oAuthErrorOk := err.(auth.OAuthError)
	if !oAuthErrorOk {
		oAuthError = auth.NewOAuthError(400, "server_error", err.Error())
	}
	logs.WithContext(r.Context()).Info(fmt.Sprint("authorization rejected : ", oAuthError.Error()))
	http.Redirect(w, r, auth.ErrorRedirectUrl(redirectUri, state, oAuthError.ErrorCode, oAuthError.ErrorDescription), http.StatusFound)
}
