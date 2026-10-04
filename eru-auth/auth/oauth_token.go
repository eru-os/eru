package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	SELECT_OAUTH_REQUEST_BY_CODE = "select request_hash, project_id, auth_name, client_id, redirect_uri, requested_scope, granted_scope, state, nonce, code_challenge, code_challenge_method, identity_id, code_hash, consumed_at, expires_at from eruauth_oauth_auth_requests where code_hash = ??? and project_id = ??? and auth_name = ???"
	CONSUME_OAUTH_AUTH_CODE      = "update eruauth_oauth_auth_requests set consumed_at = CURRENT_TIMESTAMP where code_hash = ??? and project_id = ??? and auth_name = ??? and consumed_at is null"

	INSERT_OAUTH_REFRESH_TOKEN = "insert into eruauth_oauth_refresh_tokens (token_hash, grant_id, project_id, auth_name, client_id, identity_id, scope, expires_at) values (???,???,???,???,???,???,???,???)"
	SELECT_OAUTH_REFRESH_TOKEN = "select token_hash, grant_id, project_id, auth_name, client_id, identity_id, scope, rotated_at, revoked_at, expires_at from eruauth_oauth_refresh_tokens where token_hash = ??? and project_id = ??? and auth_name = ???"
	ROTATE_OAUTH_REFRESH_TOKEN = "update eruauth_oauth_refresh_tokens set rotated_at = CURRENT_TIMESTAMP where token_hash = ??? and project_id = ??? and auth_name = ??? and rotated_at is null"
	REVOKE_OAUTH_GRANT         = "update eruauth_oauth_refresh_tokens set revoked_at = CURRENT_TIMESTAMP where grant_id = ??? and project_id = ??? and auth_name = ??? and revoked_at is null"

	defaultAccessTokenLifespan  = 3600
	defaultIdTokenLifespan      = 3600
	defaultRefreshTokenLifespan = 30 * 24 * 3600
)

// TokenSignerI signs a set of claims with the project's key. It lives here as an interface because
// the keys are held by the store, which already depends on this package.
type TokenSignerI interface {
	SignToken(ctx context.Context, projectId string, kid string, claims map[string]interface{}) (string, error)
}

// TokenVerifierI validates a token this server signed, against the key named in its header. A
// signer that can also verify saves the userinfo path a round trip to our own published key set.
type TokenVerifierI interface {
	VerifyToken(ctx context.Context, projectId string, token string) (map[string]interface{}, error)
}

// TokenResponse is the RFC 6749 token endpoint response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IdToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type refreshTokenRecord struct {
	TokenHash  string
	GrantId    string
	ClientId   string
	IdentityId string
	Scope      string
	Rotated    bool
	Revoked    bool
	ExpiresAt  time.Time
}

// TokenService issues tokens for the ERU backend. It shares the authorization request store with
// EruAuthorizationFlow - a code and the request that produced it are the same row.
type TokenService struct {
	Flow   EruAuthorizationFlow
	Signer TokenSignerI
	Config OAuthServerConfig
}

func (config OAuthServerConfig) accessTokenLifespan() int {
	if config.AccessTokenLifespan > 0 {
		return config.AccessTokenLifespan.Seconds()
	}
	return defaultAccessTokenLifespan
}

func (config OAuthServerConfig) idTokenLifespan() int {
	if config.IdTokenLifespan > 0 {
		return config.IdTokenLifespan.Seconds()
	}
	return defaultIdTokenLifespan
}

// RefreshTokenLifespanSeconds is how long a refresh token, and the cookie carrying it, are good for.
func (config OAuthServerConfig) RefreshTokenLifespanSeconds() int {
	return config.refreshTokenLifespan()
}

func (config OAuthServerConfig) refreshTokenLifespan() int {
	if config.RefreshTokenLifespan > 0 {
		return config.RefreshTokenLifespan.Seconds()
	}
	return defaultRefreshTokenLifespan
}

// ExchangeCode redeems an authorization code. The code is single use: redeeming it a second time
// revokes every refresh token issued from the same grant, because a replay means the code was
// seen by someone who should not have had it.
func (service TokenService) ExchangeCode(ctx context.Context, clientId string, code string, redirectUri string, codeVerifier string) (TokenResponse, error) {
	logs.WithContext(ctx).Debug("ExchangeCode - Start")
	if code == "" {
		return TokenResponse{}, NewOAuthError(400, "invalid_request", "code is required")
	}

	authRequest, err := service.authRequestByCode(ctx, code)
	if err != nil {
		return TokenResponse{}, err
	}

	if authRequest.Consumed {
		logs.WithContext(ctx).Error(fmt.Sprint("authorization code replayed for client ", authRequest.ClientId))
		_ = service.revokeGrant(ctx, authRequest.RequestId)
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "authorization code has already been used")
	}
	if !authRequest.ExpiresAt.IsZero() && time.Now().After(authRequest.ExpiresAt) {
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "authorization code has expired")
	}
	if clientId != "" && authRequest.ClientId != clientId {
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "authorization code was issued to another client")
	}
	if redirectUri != "" && redirectUri != authRequest.RedirectUri {
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "redirect_uri does not match the authorization request")
	}
	if !VerifyCodeChallenge(authRequest.CodeChallenge, authRequest.CodeChallengeMethod, codeVerifier) {
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "code_verifier does not match the code_challenge")
	}

	// Marking consumed is conditional on it still being unconsumed, so two simultaneous redemptions
	// cannot both succeed.
	consumed, err := service.consumeCode(ctx, code)
	if err != nil {
		return TokenResponse{}, err
	}
	if !consumed {
		_ = service.revokeGrant(ctx, authRequest.RequestId)
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "authorization code has already been used")
	}

	return service.issueTokens(ctx, authRequest.RequestId, authRequest.ClientId, authRequest.IdentityId,
		authRequest.GrantedScope, authRequest.Nonce)
}

// RefreshToken rotates a refresh token. The presented token is retired as the new one is issued;
// presenting a retired or revoked token revokes the whole grant, since only a thief would use one.
func (service TokenService) RefreshToken(ctx context.Context, clientId string, refreshToken string) (TokenResponse, error) {
	logs.WithContext(ctx).Debug("RefreshToken - Start")
	if refreshToken == "" {
		return TokenResponse{}, NewOAuthError(400, "invalid_request", "refresh_token is required")
	}

	record, err := service.refreshTokenByValue(ctx, refreshToken)
	if err != nil {
		return TokenResponse{}, err
	}
	if record.Revoked {
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "refresh token has been revoked")
	}
	if record.Rotated {
		logs.WithContext(ctx).Error(fmt.Sprint("rotated refresh token reused for grant ", record.GrantId))
		_ = service.revokeGrant(ctx, record.GrantId)
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "refresh token has already been used")
	}
	if !record.ExpiresAt.IsZero() && time.Now().After(record.ExpiresAt) {
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "refresh token has expired")
	}
	if clientId != "" && record.ClientId != clientId {
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "refresh token was issued to another client")
	}

	rotated, err := service.rotateRefreshToken(ctx, refreshToken)
	if err != nil {
		return TokenResponse{}, err
	}
	if !rotated {
		_ = service.revokeGrant(ctx, record.GrantId)
		return TokenResponse{}, NewOAuthError(400, "invalid_grant", "refresh token has already been used")
	}

	return service.issueTokens(ctx, record.GrantId, record.ClientId, record.IdentityId, record.Scope, "")
}

// RevokeGrant retires every refresh token of a grant. Used by the revocation endpoint and whenever
// a replay is detected.
func (service TokenService) RevokeGrant(ctx context.Context, refreshToken string) error {
	record, err := service.refreshTokenByValue(ctx, refreshToken)
	if err != nil {
		// Revocation does not tell a caller whether the token existed.
		return nil
	}
	return service.revokeGrant(ctx, record.GrantId)
}

func (service TokenService) issueTokens(ctx context.Context, grantId string, clientId string, identityId string, scope string, nonce string) (TokenResponse, error) {
	issuedAt := time.Now()
	scopes := strings.Fields(scope)

	accessClaims := map[string]interface{}{
		"iss":       service.Flow.Issuer,
		"sub":       identityId,
		"client_id": clientId,
		"scp":       scopes,
		"scope":     scope,
		"iat":       issuedAt.Unix(),
		"exp":       issuedAt.Add(time.Duration(service.Config.accessTokenLifespan()) * time.Second).Unix(),
	}
	// The audience is what lets a resource server tell a token minted for it from any other token
	// this issuer ever produced.
	if len(service.Config.AccessTokenAudience) > 0 {
		accessClaims["aud"] = service.Config.AccessTokenAudience
	}

	accessToken, err := service.signToken(ctx, accessClaims)
	if err != nil {
		return TokenResponse{}, err
	}

	response := TokenResponse{
		AccessToken: accessToken,
		TokenType:   "bearer",
		ExpiresIn:   service.Config.accessTokenLifespan(),
		Scope:       scope,
	}

	if contains(scopes, "openid") {
		idClaims := map[string]interface{}{
			"iss": service.Flow.Issuer,
			"sub": identityId,
			"aud": []string{clientId},
			"iat": issuedAt.Unix(),
			"exp": issuedAt.Add(time.Duration(service.Config.idTokenLifespan()) * time.Second).Unix(),
		}
		if nonce != "" {
			idClaims["nonce"] = nonce
		}
		if response.IdToken, err = service.signToken(ctx, idClaims); err != nil {
			return TokenResponse{}, err
		}
	}

	if contains(scopes, "offline_access") {
		refreshToken, refreshErr := service.newRefreshToken(ctx, grantId, clientId, identityId, scope)
		if refreshErr != nil {
			return TokenResponse{}, refreshErr
		}
		response.RefreshToken = refreshToken
	}
	return response, nil
}

func (service TokenService) signToken(ctx context.Context, claims map[string]interface{}) (string, error) {
	if service.Signer == nil {
		err := errors.New("no token signer is configured")
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	token, err := service.Signer.SignToken(ctx, service.Flow.ProjectId, service.Config.SigningKid, claims)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("token could not be issued")
	}
	return token, nil
}

func (service TokenService) newRefreshToken(ctx context.Context, grantId string, clientId string, identityId string, scope string) (string, error) {
	if err := service.Flow.requireConnection(ctx); err != nil {
		return "", err
	}
	refreshToken, err := randomToken()
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("refresh token could not be issued")
	}

	query := models.Queries{Query: service.Flow.AuthDb.GetDbQuery(ctx, INSERT_OAUTH_REFRESH_TOKEN)}
	query.Vals = append(query.Vals, hashToken(refreshToken), grantId, service.Flow.ProjectId, service.Flow.AuthName,
		clientId, identityId, scope, time.Now().Add(time.Duration(service.Config.refreshTokenLifespan())*time.Second))
	query.Rank = 1

	if _, err = utils.ExecuteDbSave(ctx, service.Flow.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("refresh token could not be issued")
	}
	return refreshToken, nil
}

func (service TokenService) authRequestByCode(ctx context.Context, code string) (AuthorizationRequest, error) {
	if err := service.Flow.requireConnection(ctx); err != nil {
		return AuthorizationRequest{}, err
	}
	query := models.Queries{Query: service.Flow.AuthDb.GetDbQuery(ctx, SELECT_OAUTH_REQUEST_BY_CODE)}
	query.Vals = append(query.Vals, hashToken(code), service.Flow.ProjectId, service.Flow.AuthName)
	query.Rank = 1

	output, err := utils.ExecuteDbFetch(ctx, service.Flow.AuthDb.GetConn(), query)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return AuthorizationRequest{}, errors.New("authorization code could not be read")
	}
	if len(output) == 0 {
		return AuthorizationRequest{}, NewOAuthError(400, "invalid_grant", "authorization code is not valid")
	}
	authRequest := authRequestFromRow(output[0])
	// RequestId doubles as the grant id, and is only known here as its digest.
	authRequest.RequestId = rowString(output[0]["request_hash"])
	return authRequest, nil
}

func (service TokenService) consumeCode(ctx context.Context, code string) (bool, error) {
	query := models.Queries{Query: service.Flow.AuthDb.GetDbQuery(ctx, CONSUME_OAUTH_AUTH_CODE)}
	query.Vals = append(query.Vals, hashToken(code), service.Flow.ProjectId, service.Flow.AuthName)
	query.Rank = 1
	output, err := utils.ExecuteDbSave(ctx, service.Flow.AuthDb.GetConn(), []*models.Queries{&query})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return false, errors.New("authorization code could not be redeemed")
	}
	return len(output) > 0, nil
}

func (service TokenService) refreshTokenByValue(ctx context.Context, refreshToken string) (refreshTokenRecord, error) {
	if err := service.Flow.requireConnection(ctx); err != nil {
		return refreshTokenRecord{}, err
	}
	query := models.Queries{Query: service.Flow.AuthDb.GetDbQuery(ctx, SELECT_OAUTH_REFRESH_TOKEN)}
	query.Vals = append(query.Vals, hashToken(refreshToken), service.Flow.ProjectId, service.Flow.AuthName)
	query.Rank = 1

	output, err := utils.ExecuteDbFetch(ctx, service.Flow.AuthDb.GetConn(), query)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return refreshTokenRecord{}, errors.New("refresh token could not be read")
	}
	if len(output) == 0 {
		return refreshTokenRecord{}, NewOAuthError(400, "invalid_grant", "refresh token is not valid")
	}
	return refreshRecordFromRow(output[0]), nil
}

func (service TokenService) rotateRefreshToken(ctx context.Context, refreshToken string) (bool, error) {
	query := models.Queries{Query: service.Flow.AuthDb.GetDbQuery(ctx, ROTATE_OAUTH_REFRESH_TOKEN)}
	query.Vals = append(query.Vals, hashToken(refreshToken), service.Flow.ProjectId, service.Flow.AuthName)
	query.Rank = 1
	output, err := utils.ExecuteDbSave(ctx, service.Flow.AuthDb.GetConn(), []*models.Queries{&query})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return false, errors.New("refresh token could not be rotated")
	}
	return len(output) > 0, nil
}

func (service TokenService) revokeGrant(ctx context.Context, grantId string) error {
	query := models.Queries{Query: service.Flow.AuthDb.GetDbQuery(ctx, REVOKE_OAUTH_GRANT)}
	query.Vals = append(query.Vals, grantId, service.Flow.ProjectId, service.Flow.AuthName)
	query.Rank = 1
	if _, err := utils.ExecuteDbSave(ctx, service.Flow.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return errors.New("grant could not be revoked")
	}
	return nil
}

func refreshRecordFromRow(row map[string]interface{}) refreshTokenRecord {
	record := refreshTokenRecord{
		TokenHash:  rowString(row["token_hash"]),
		GrantId:    rowString(row["grant_id"]),
		ClientId:   rowString(row["client_id"]),
		IdentityId: rowString(row["identity_id"]),
		Scope:      rowString(row["scope"]),
	}
	record.Rotated = timestampSet(row["rotated_at"])
	record.Revoked = timestampSet(row["revoked_at"])
	if expiresAt, ok := row["expires_at"].(time.Time); ok {
		record.ExpiresAt = expiresAt
	}
	return record
}

func timestampSet(value interface{}) bool {
	if value == nil {
		return false
	}
	if t, ok := value.(time.Time); ok {
		return !t.IsZero()
	}
	return rowString(value) != ""
}
