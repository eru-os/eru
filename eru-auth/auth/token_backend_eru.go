package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// eruTokenBackend mints tokens with the project's own keys, for the first party login api - the
// path a browser uses when it posts credentials rather than going through the authorization code
// flow.
//
// It replaces driving an external authorization server's browser flow server side to mint a token
// for a user that has already been authenticated. The claims it produces are the ones that path
// produced, so nothing downstream has to change: the id token carries the whole Identity under
// "identity", which is what getTokenAttributes and the gateway's claims header read.
type eruTokenBackend struct {
	auth *Auth
}

// firstPartyClientId identifies the login api itself in the tokens it mints. There is no oauth
// client in this flow, so it falls back to the auth name rather than inventing one.
func (backend eruTokenBackend) firstPartyClientId() string {
	if backend.auth.OAuthServerConfig.FirstPartyClientId != "" {
		return backend.auth.OAuthServerConfig.FirstPartyClientId
	}
	return backend.auth.AuthName
}

func (backend eruTokenBackend) tokenService() (TokenService, error) {
	if backend.auth.TokenSigner == nil {
		return TokenService{}, errors.New("no token signer is configured for this auth")
	}
	if backend.auth.ProjectId == "" {
		return TokenService{}, errors.New("no project is set on this auth")
	}
	config := backend.auth.OAuthServerConfig
	if config.SigningKid == "" {
		return TokenService{}, errors.New("oauth_server.signing_kid is not set")
	}
	return TokenService{
		Flow: EruAuthorizationFlow{
			AuthDb:    backend.auth.AuthDb,
			ProjectId: backend.auth.ProjectId,
			AuthName:  backend.auth.AuthName,
			Issuer:    backend.auth.OAuthIssuer(context.Background()),
			Policy:    config.EffectiveClientPolicy(),
		},
		Signer: backend.auth.TokenSigner,
		Config: config,
	}, nil
}

// MakeTokens issues the pair the login api returns. The scope always includes offline_access so a
// refresh token comes back, which is what the existing api promises its callers.
func (backend eruTokenBackend) MakeTokens(ctx context.Context, identity Identity) (LoginSuccess, error) {
	logs.WithContext(ctx).Debug("MakeTokens - Start")
	service, err := backend.tokenService()
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return LoginSuccess{}, err
	}

	// A direct login has no authorization request behind it, so the grant gets an id of its own -
	// refresh rotation and reuse detection key off it exactly as they do for a code grant.
	grantId, err := randomToken()
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return LoginSuccess{}, errors.New("tokens could not be issued")
	}

	return backend.issue(ctx, service, grantId, identity, backend.firstPartyClientId())
}

// issue signs the pair and records the refresh token. It is shared by login and by refresh so the
// two cannot drift in what they put in a token.
func (backend eruTokenBackend) issue(ctx context.Context, service TokenService, grantId string, identity Identity, clientId string) (LoginSuccess, error) {
	config := backend.auth.OAuthServerConfig
	issuedAt := time.Now()
	scope := backend.loginScope()
	accessExpiry := issuedAt.Add(time.Duration(config.accessTokenLifespan()) * time.Second)

	jti, err := randomToken()
	if err != nil {
		return LoginSuccess{}, errors.New("tokens could not be issued")
	}

	accessClaims := map[string]interface{}{
		"iss":       service.Flow.Issuer,
		"sub":       identity.Id,
		"client_id": clientId,
		"scp":       strings.Fields(scope),
		"jti":       jti,
		"iat":       issuedAt.Unix(),
		"nbf":       issuedAt.Unix(),
		"exp":       accessExpiry.Unix(),
		// Carried for shape compatibility: the previous issuer always emitted it, empty unless the
		// grant put something there.
		"ext": map[string]interface{}{},
	}
	if len(config.AccessTokenAudience) > 0 {
		accessClaims["aud"] = config.AccessTokenAudience
	} else {
		accessClaims["aud"] = []string{}
	}

	accessToken, err := service.signToken(ctx, accessClaims)
	if err != nil {
		return LoginSuccess{}, err
	}

	idJti, err := randomToken()
	if err != nil {
		return LoginSuccess{}, errors.New("tokens could not be issued")
	}
	idClaims := map[string]interface{}{
		"iss":       service.Flow.Issuer,
		"sub":       identity.Id,
		"aud":       []string{clientId},
		"jti":       idJti,
		"iat":       issuedAt.Unix(),
		"rat":       issuedAt.Unix(),
		"auth_time": issuedAt.Unix(),
		"exp":       issuedAt.Add(time.Duration(config.idTokenLifespan()) * time.Second).Unix(),
		// The contract everything downstream depends on: the whole identity, read as
		// claims["identity"]["attributes"] by getTokenAttributes and by the gateway's claims header.
		"identity": identity,
	}
	if identity.AuthDetails.SessionId != "" {
		idClaims["sid"] = identity.AuthDetails.SessionId
	}

	idToken, err := service.signToken(ctx, idClaims)
	if err != nil {
		return LoginSuccess{}, err
	}

	refreshToken, err := service.newRefreshToken(ctx, grantId, clientId, identity.Id, scope)
	if err != nil {
		return LoginSuccess{}, err
	}

	return LoginSuccess{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		IdToken:      idToken,
		Expiry:       accessExpiry,
		ExpiresIn:    float64(config.accessTokenLifespan()),
		Id:           identity.Id,
	}, nil
}

func (backend eruTokenBackend) loginScope() string {
	if len(backend.auth.OAuthServerConfig.ScopesSupported) > 0 {
		return strings.Join(backend.auth.OAuthServerConfig.ScopesSupported, " ")
	}
	return strings.Join(defaultOAuthScopes, " ")
}

// FetchTokens rotates a refresh token and returns a fresh pair. The identity is read again rather
// than carried in the refresh row, so a user whose attributes changed gets the current ones.
func (backend eruTokenBackend) FetchTokens(ctx context.Context, refreshToken string, userId string) (interface{}, error) {
	logs.WithContext(ctx).Debug("FetchTokens - Start")
	return backend.refresh(ctx, refreshToken, userId)
}

func (backend eruTokenBackend) LoginApi(ctx context.Context, refreshToken string) (interface{}, error) {
	logs.WithContext(ctx).Debug("LoginApi - Start")
	return backend.refresh(ctx, refreshToken, "")
}

func (backend eruTokenBackend) refresh(ctx context.Context, refreshToken string, userId string) (interface{}, error) {
	service, err := backend.tokenService()
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	record, err := service.refreshTokenByValue(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	if record.Revoked {
		return nil, NewOAuthError(400, "invalid_grant", "refresh token has been revoked")
	}
	if record.Rotated {
		// Reusing a retired token means it was seen by someone who should not have had it, so the
		// whole grant goes - the same rule the code grant applies.
		logs.WithContext(ctx).Error(fmt.Sprint("rotated refresh token reused for grant ", record.GrantId))
		_ = service.revokeGrant(ctx, record.GrantId)
		return nil, NewOAuthError(400, "invalid_grant", "refresh token has already been used")
	}
	if !record.ExpiresAt.IsZero() && time.Now().After(record.ExpiresAt) {
		return nil, NewOAuthError(400, "invalid_grant", "refresh token has expired")
	}
	if userId != "" && record.IdentityId != userId {
		return nil, NewOAuthError(400, "invalid_grant", "refresh token was issued to another user")
	}

	// The identity is read before the token is retired. Rotating first meant that anything failing
	// afterwards burned the refresh token, so the caller's retry looked like a replay and revoked
	// the whole grant.
	identity, err := backend.auth.concrete().GetUser(ctx, record.IdentityId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, errors.New("tokens could not be refreshed")
	}

	rotated, err := service.rotateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	if !rotated {
		_ = service.revokeGrant(ctx, record.GrantId)
		return nil, NewOAuthError(400, "invalid_grant", "refresh token has already been used")
	}

	return backend.issue(ctx, service, record.GrantId, identity, record.ClientId)
}

// GetUserInfo answers for the subject of the presented access token, verified against the key it
// was signed with.
func (backend eruTokenBackend) GetUserInfo(ctx context.Context, accessToken string) (Identity, error) {
	logs.WithContext(ctx).Debug("GetUserInfo - Start")
	if backend.auth.TokenSigner == nil {
		return Identity{}, errors.New("no token signer is configured for this auth")
	}
	verifier, verifierOk := backend.auth.TokenSigner.(TokenVerifierI)
	if !verifierOk {
		return Identity{}, errors.New("the configured token signer cannot verify tokens")
	}
	claims, err := verifier.VerifyToken(ctx, backend.auth.ProjectId, accessToken)
	if err != nil {
		return Identity{}, err
	}
	subject, _ := claims["sub"].(string)
	if subject == "" {
		return Identity{}, errors.New("access token has no subject")
	}
	return backend.auth.concrete().GetUser(ctx, subject)
}

func (backend eruTokenBackend) RevokeToken(ctx context.Context, refreshToken string) (int, error) {
	logs.WithContext(ctx).Debug("RevokeToken - Start")
	service, err := backend.tokenService()
	if err != nil {
		return 400, err
	}
	if err = service.RevokeGrant(ctx, refreshToken); err != nil {
		return 400, err
	}
	return 200, nil
}

// SaveClients and RemoveClients exist because an external backend has to be told about clients.
// Ours keeps them in its own store, written through the client registry, so there is nothing to
// push anywhere.
func (backend eruTokenBackend) SaveClients(ctx context.Context) error   { return nil }
func (backend eruTokenBackend) RemoveClients(ctx context.Context) error { return nil }

// PublicUrl is the issuer this backend mints under. It reads the configured value directly rather
// than asking OAuthIssuer, which falls back to this method - going through it would recurse.
func (backend eruTokenBackend) PublicUrl() string {
	return strings.TrimSuffix(backend.auth.OAuthServerConfig.Issuer, "/")
}

func (backend eruTokenBackend) GetAttribute(attributeName string) (interface{}, bool) {
	switch attributeName {
	case "issuer", "public_url":
		return backend.PublicUrl(), true
	case "first_party_client_id":
		return backend.firstPartyClientId(), true
	}
	return nil, false
}

func (backend eruTokenBackend) ClientRegistry() ClientRegistryI {
	return EruClientRegistry{
		AuthDb:    backend.auth.AuthDb,
		ProjectId: backend.auth.ProjectId,
		AuthName:  backend.auth.AuthName,
	}
}

func (backend eruTokenBackend) AuthorizationFlow() AuthorizationFlowI {
	return EruAuthorizationFlow{
		AuthDb:    backend.auth.AuthDb,
		ProjectId: backend.auth.ProjectId,
		AuthName:  backend.auth.AuthName,
		Issuer:    backend.auth.OAuthIssuer(context.Background()),
		Registry:  backend.ClientRegistry(),
		Policy:    backend.auth.OAuthServerConfig.EffectiveClientPolicy(),
	}
}
