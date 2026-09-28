package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	erusha "github.com/eru-os/eru/eru-crypto/sha"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	INSERT_OAUTH_AUTH_REQUEST = "insert into eruauth_oauth_auth_requests (request_hash, project_id, auth_name, client_id, redirect_uri, requested_scope, state, nonce, code_challenge, code_challenge_method, expires_at) values (???,???,???,???,???,???,???,???,???,???,???)"
	SELECT_OAUTH_AUTH_REQUEST = "select request_hash, project_id, auth_name, client_id, redirect_uri, requested_scope, granted_scope, state, nonce, code_challenge, code_challenge_method, identity_id, code_hash, consumed_at, expires_at from eruauth_oauth_auth_requests where request_hash = ??? and project_id = ??? and auth_name = ???"
	UPDATE_OAUTH_AUTH_LOGIN   = "update eruauth_oauth_auth_requests set identity_id = ??? where request_hash = ??? and project_id = ??? and auth_name = ???"
	UPDATE_OAUTH_AUTH_CONSENT = "update eruauth_oauth_auth_requests set granted_scope = ???, code_hash = ??? where request_hash = ??? and project_id = ??? and auth_name = ???"

	// authRequestLifetime bounds how long a half finished authorization may sit around waiting for
	// someone to log in.
	authRequestLifetime = 15 * time.Minute
)

// AuthorizationRequest is one authorization in flight. A single row carries it from /oauth2/auth
// through login and consent to the issued code, which keeps the states of one grant in one place
// rather than spread over several tables that have to agree with each other.
type AuthorizationRequest struct {
	RequestId           string
	ProjectId           string
	AuthName            string
	ClientId            string
	RedirectUri         string
	RequestedScope      string
	GrantedScope        string
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	IdentityId          string
	CodeHash            string
	Consumed            bool
	ExpiresAt           time.Time
}

func (authRequest AuthorizationRequest) LoggedIn() bool  { return authRequest.IdentityId != "" }
func (authRequest AuthorizationRequest) Consented() bool { return authRequest.CodeHash != "" }

// EruAuthorizationFlow drives the interactive grant against eru-auth's own store. The login and
// consent screens talk to AuthorizationFlowI, so a token backend's flow can stand in for it.
type EruAuthorizationFlow struct {
	AuthDb    AuthDbI
	ProjectId string
	AuthName  string
	Issuer    string
	Registry  ClientRegistryI
	Policy    OAuthClientPolicy
}

// CreateAuthorizationRequest records a validated authorization request and returns the opaque id
// the login and consent steps are addressed by. Only the hash is stored, so a leaked row cannot be
// used to resume somebody else's authorization.
func (flow EruAuthorizationFlow) CreateAuthorizationRequest(ctx context.Context, authRequest AuthorizationRequest) (string, error) {
	logs.WithContext(ctx).Debug("CreateAuthorizationRequest - Start")
	if err := flow.requireConnection(ctx); err != nil {
		return "", err
	}

	requestId, err := randomToken()
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("authorization request could not be created")
	}

	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, INSERT_OAUTH_AUTH_REQUEST)}
	query.Vals = append(query.Vals, hashToken(requestId), flow.ProjectId, flow.AuthName, authRequest.ClientId,
		authRequest.RedirectUri, authRequest.RequestedScope, authRequest.State, authRequest.Nonce,
		authRequest.CodeChallenge, authRequest.CodeChallengeMethod, time.Now().Add(authRequestLifetime))
	query.Rank = 1

	if _, err = utils.ExecuteDbSave(ctx, flow.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("authorization request could not be created")
	}
	return requestId, nil
}

// AuthorizationRequestById reads a request that has not expired or been redeemed.
func (flow EruAuthorizationFlow) AuthorizationRequestById(ctx context.Context, requestId string) (AuthorizationRequest, error) {
	logs.WithContext(ctx).Debug("AuthorizationRequestById - Start")
	if err := flow.requireConnection(ctx); err != nil {
		return AuthorizationRequest{}, err
	}
	if requestId == "" {
		return AuthorizationRequest{}, errors.New("authorization request id is missing")
	}

	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, SELECT_OAUTH_AUTH_REQUEST)}
	query.Vals = append(query.Vals, hashToken(requestId), flow.ProjectId, flow.AuthName)
	query.Rank = 1

	output, err := utils.ExecuteDbFetch(ctx, flow.AuthDb.GetConn(), query)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return AuthorizationRequest{}, errors.New("authorization request could not be read")
	}
	if len(output) == 0 {
		return AuthorizationRequest{}, errors.New("authorization request not found")
	}

	authRequest := authRequestFromRow(output[0])
	authRequest.RequestId = requestId
	if authRequest.Consumed {
		return AuthorizationRequest{}, errors.New("authorization request has already been used")
	}
	if !authRequest.ExpiresAt.IsZero() && time.Now().After(authRequest.ExpiresAt) {
		return AuthorizationRequest{}, errors.New("authorization request has expired")
	}
	return authRequest, nil
}

func (flow EruAuthorizationFlow) LoginRequest(ctx context.Context, challenge string) (LoginRequestInfo, error) {
	logs.WithContext(ctx).Debug("LoginRequest - Start")
	authRequest, err := flow.AuthorizationRequestById(ctx, challenge)
	if err != nil {
		return LoginRequestInfo{}, err
	}
	loginRequest := LoginRequestInfo{
		Challenge:      challenge,
		Skip:           authRequest.LoggedIn(),
		Subject:        authRequest.IdentityId,
		ClientId:       authRequest.ClientId,
		RequestedScope: strings.Fields(authRequest.RequestedScope),
	}
	if client, clientErr := flow.Registry.GetClient(ctx, authRequest.ClientId); clientErr == nil {
		loginRequest.ClientName = client.ClientName
	}
	return loginRequest, nil
}

// AcceptLogin records who signed in and sends the browser back to the authorize endpoint, which
// decides whether consent is still needed.
func (flow EruAuthorizationFlow) AcceptLogin(ctx context.Context, challenge string, subject string, remember bool) (string, error) {
	logs.WithContext(ctx).Debug("AcceptLogin - Start")
	if subject == "" {
		err := errors.New("cannot accept a login request without a subject")
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	authRequest, err := flow.AuthorizationRequestById(ctx, challenge)
	if err != nil {
		return "", err
	}

	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, UPDATE_OAUTH_AUTH_LOGIN)}
	query.Vals = append(query.Vals, subject, hashToken(challenge), flow.ProjectId, flow.AuthName)
	query.Rank = 1
	if _, err = utils.ExecuteDbSave(ctx, flow.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("login could not be recorded")
	}
	_ = authRequest
	return flow.resumeUrl(challenge), nil
}

// RejectLogin sends the caller back to its redirect uri with an error rather than a code.
func (flow EruAuthorizationFlow) RejectLogin(ctx context.Context, challenge string, errorCode string, errorDescription string) (string, error) {
	logs.WithContext(ctx).Debug("RejectLogin - Start")
	authRequest, err := flow.AuthorizationRequestById(ctx, challenge)
	if err != nil {
		return "", err
	}
	return ErrorRedirectUrl(authRequest.RedirectUri, authRequest.State, errorCode, errorDescription), nil
}

func (flow EruAuthorizationFlow) ConsentRequest(ctx context.Context, challenge string) (ConsentRequestInfo, error) {
	logs.WithContext(ctx).Debug("ConsentRequest - Start")
	authRequest, err := flow.AuthorizationRequestById(ctx, challenge)
	if err != nil {
		return ConsentRequestInfo{}, err
	}
	requestedScope := strings.Fields(authRequest.RequestedScope)
	consentRequest := ConsentRequestInfo{
		Challenge: challenge,
		Subject:   authRequest.IdentityId,
		ClientId:  authRequest.ClientId,
		// Already agreed to by this user for this client, so there is nothing new to show them.
		Skip:           flow.ConsentCovers(ctx, authRequest.ClientId, authRequest.IdentityId, requestedScope),
		RequestedScope: requestedScope,
		// The resource this grant is for, so the access token can be bound to it.
		RequestedAudience: []string{},
	}
	if client, clientErr := flow.Registry.GetClient(ctx, authRequest.ClientId); clientErr == nil {
		consentRequest.ClientName = client.ClientName
	}
	return consentRequest, nil
}

// AcceptConsent records the granted scope, mints the authorization code and returns the redirect
// back to the client. The code is stored only as a hash and the state is echoed back unchanged.
func (flow EruAuthorizationFlow) AcceptConsent(ctx context.Context, challenge string, grantScope []string, grantAudience []string, idTokenClaims map[string]interface{}, accessTokenClaims map[string]interface{}, remember bool) (string, error) {
	logs.WithContext(ctx).Debug("AcceptConsent - Start")
	authRequest, err := flow.AuthorizationRequestById(ctx, challenge)
	if err != nil {
		return "", err
	}
	if !authRequest.LoggedIn() {
		err = errors.New("consent cannot be accepted before login")
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}

	code, err := randomToken()
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("authorization code could not be issued")
	}

	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, UPDATE_OAUTH_AUTH_CONSENT)}
	query.Vals = append(query.Vals, strings.Join(grantScope, " "), hashToken(code), hashToken(challenge), flow.ProjectId, flow.AuthName)
	query.Rank = 1
	if _, err = utils.ExecuteDbSave(ctx, flow.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("consent could not be recorded")
	}

	// Remembering is best effort - failing to record it only means the user is asked again.
	if remember {
		if rememberErr := flow.RememberConsent(ctx, authRequest.ClientId, authRequest.IdentityId, grantScope); rememberErr != nil {
			logs.WithContext(ctx).Info(rememberErr.Error())
		}
	}

	return CodeRedirectUrl(authRequest.RedirectUri, authRequest.State, code), nil
}

func (flow EruAuthorizationFlow) RejectConsent(ctx context.Context, challenge string, errorCode string, errorDescription string) (string, error) {
	logs.WithContext(ctx).Debug("RejectConsent - Start")
	authRequest, err := flow.AuthorizationRequestById(ctx, challenge)
	if err != nil {
		return "", err
	}
	return ErrorRedirectUrl(authRequest.RedirectUri, authRequest.State, errorCode, errorDescription), nil
}

func (flow EruAuthorizationFlow) resumeUrl(requestId string) string {
	return fmt.Sprint(strings.TrimSuffix(flow.Issuer, "/"), OAuthAuthorizePath, "?request_id=", url.QueryEscape(requestId))
}

func (flow EruAuthorizationFlow) requireConnection(ctx context.Context) error {
	if flow.AuthDb == nil || flow.AuthDb.GetConn() == nil {
		err := errors.New("authorization request store has no database connection")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

// ValidateAuthorizationRequest applies the checks that have to pass before a browser is sent
// anywhere. Anything wrong with the client or the redirect uri is returned as an error for the
// server to render, never as a redirect - redirecting on an unverified redirect_uri is how an
// authorization endpoint becomes an open redirect.
func ValidateAuthorizationRequest(ctx context.Context, params url.Values, client OAuthClient, policy OAuthClientPolicy) (AuthorizationRequest, error) {
	authRequest := AuthorizationRequest{
		ClientId:            params.Get("client_id"),
		RedirectUri:         params.Get("redirect_uri"),
		State:               params.Get("state"),
		Nonce:               params.Get("nonce"),
		CodeChallenge:       params.Get("code_challenge"),
		CodeChallengeMethod: params.Get("code_challenge_method"),
	}

	if authRequest.RedirectUri == "" {
		if len(client.RedirectURIs) != 1 {
			return authRequest, NewOAuthError(400, "invalid_request", "redirect_uri is required")
		}
		authRequest.RedirectUri = client.RedirectURIs[0]
	}
	if !contains(client.RedirectURIs, authRequest.RedirectUri) {
		return authRequest, NewOAuthError(400, "invalid_request", "redirect_uri is not registered for this client")
	}

	// From here a redirect is safe, so the rest are protocol errors the client should be told about.
	if params.Get("response_type") != "code" {
		return authRequest, NewOAuthError(400, "unsupported_response_type", "only the code response type is supported")
	}
	if len(client.GrantTypes) > 0 && !contains(client.GrantTypes, "authorization_code") {
		return authRequest, NewOAuthError(400, "unauthorized_client", "client may not use the authorization code grant")
	}

	requestedScope := strings.Fields(params.Get("scope"))
	if len(requestedScope) == 0 {
		requestedScope = strings.Fields(client.Scope)
	}
	registeredScope := strings.Fields(client.Scope)
	for _, scope := range requestedScope {
		if len(registeredScope) > 0 && !contains(registeredScope, scope) {
			return authRequest, NewOAuthError(400, "invalid_scope", fmt.Sprint("scope not allowed for this client : ", scope))
		}
		if !contains(policy.Scopes(), scope) {
			return authRequest, NewOAuthError(400, "invalid_scope", fmt.Sprint("scope not allowed : ", scope))
		}
	}
	authRequest.RequestedScope = strings.Join(requestedScope, " ")

	// A client with no secret has nothing but pkce standing between an intercepted code and a
	// token, so it is required rather than merely supported. Only S256 is accepted - plain leaves
	// the verifier in the request that an attacker already has.
	if client.TokenEndpointAuthMethod == "none" || policy.PublicClientsOnly {
		if authRequest.CodeChallenge == "" {
			return authRequest, NewOAuthError(400, "invalid_request", "code_challenge is required for a public client")
		}
		if authRequest.CodeChallengeMethod != "S256" {
			return authRequest, NewOAuthError(400, "invalid_request", "code_challenge_method must be S256")
		}
	}
	if authRequest.CodeChallenge != "" && authRequest.CodeChallengeMethod == "" {
		authRequest.CodeChallengeMethod = "S256"
	}
	return authRequest, nil
}

// VerifyCodeChallenge checks a presented verifier against the stored S256 challenge.
func VerifyCodeChallenge(codeChallenge string, codeChallengeMethod string, codeVerifier string) bool {
	if codeChallenge == "" {
		return true
	}
	if codeVerifier == "" || strings.ToUpper(codeChallengeMethod) != "S256" {
		return false
	}
	sum := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:]) == strings.TrimRight(codeChallenge, "=")
}

func CodeRedirectUrl(redirectUri string, state string, code string) string {
	return appendRedirectParams(redirectUri, map[string]string{"code": code, "state": state})
}

func ErrorRedirectUrl(redirectUri string, state string, errorCode string, errorDescription string) string {
	return appendRedirectParams(redirectUri, map[string]string{
		"error": errorCode, "error_description": errorDescription, "state": state})
}

func appendRedirectParams(redirectUri string, params map[string]string) string {
	separator := "?"
	if strings.Contains(redirectUri, "?") {
		separator = "&"
	}
	query := url.Values{}
	for key, value := range params {
		if value != "" {
			query.Set(key, value)
		}
	}
	return fmt.Sprint(redirectUri, separator, query.Encode())
}

func randomToken() (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(tokenBytes), nil
}

func hashToken(token string) string {
	return hex.EncodeToString(erusha.NewSHA512([]byte(token)))
}

func authRequestFromRow(row map[string]interface{}) AuthorizationRequest {
	authRequest := AuthorizationRequest{
		ProjectId:           rowString(row["project_id"]),
		AuthName:            rowString(row["auth_name"]),
		ClientId:            rowString(row["client_id"]),
		RedirectUri:         rowString(row["redirect_uri"]),
		RequestedScope:      rowString(row["requested_scope"]),
		GrantedScope:        rowString(row["granted_scope"]),
		State:               rowString(row["state"]),
		Nonce:               rowString(row["nonce"]),
		CodeChallenge:       rowString(row["code_challenge"]),
		CodeChallengeMethod: rowString(row["code_challenge_method"]),
		IdentityId:          rowString(row["identity_id"]),
		CodeHash:            rowString(row["code_hash"]),
	}
	authRequest.Consumed = row["consumed_at"] != nil && rowString(row["consumed_at"]) != ""
	if consumedAt, ok := row["consumed_at"].(time.Time); ok && !consumedAt.IsZero() {
		authRequest.Consumed = true
	}
	if expiresAt, ok := row["expires_at"].(time.Time); ok {
		authRequest.ExpiresAt = expiresAt
	}
	return authRequest
}
