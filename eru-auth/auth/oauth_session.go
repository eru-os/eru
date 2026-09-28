package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	INSERT_OAUTH_SESSION = "insert into eruauth_oauth_sessions (session_hash, project_id, auth_name, identity_id, expires_at) values (???,???,???,???,???)"
	SELECT_OAUTH_SESSION = "select session_hash, identity_id, revoked_at, expires_at from eruauth_oauth_sessions where session_hash = ??? and project_id = ??? and auth_name = ???"
	REVOKE_OAUTH_SESSION = "update eruauth_oauth_sessions set revoked_at = CURRENT_TIMESTAMP where session_hash = ??? and project_id = ??? and auth_name = ??? and revoked_at is null"

	INSERT_OAUTH_CONSENT = "insert into eruauth_oauth_consents (project_id, auth_name, client_id, identity_id, granted_scope, updated_date) values (???,???,???,???,???,CURRENT_TIMESTAMP) on conflict on constraint eruauth_oauth_consents_pk do update set granted_scope = EXCLUDED.granted_scope, updated_date = CURRENT_TIMESTAMP, revoked_at = null"
	SELECT_OAUTH_CONSENT = "select granted_scope, revoked_at from eruauth_oauth_consents where project_id = ??? and auth_name = ??? and client_id = ??? and identity_id = ???"

	defaultSessionLifespan = 12 * 3600
)

// RememberConsent records what a user has already agreed to give a client, so the next
// authorization for the same scope does not ask again.
func (flow EruAuthorizationFlow) RememberConsent(ctx context.Context, clientId string, identityId string, grantedScope []string) error {
	logs.WithContext(ctx).Debug("RememberConsent - Start")
	if err := flow.requireConnection(ctx); err != nil {
		return err
	}
	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, INSERT_OAUTH_CONSENT)}
	query.Vals = append(query.Vals, flow.ProjectId, flow.AuthName, clientId, identityId, strings.Join(grantedScope, " "))
	query.Rank = 1
	if _, err := utils.ExecuteDbSave(ctx, flow.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return errors.New("consent could not be remembered")
	}
	return nil
}

// ConsentCovers reports whether a remembered consent already covers everything being asked for. It
// is deliberately one directional: a previously granted set may be wider than the current request,
// but a request for anything new has to be shown to the user.
func (flow EruAuthorizationFlow) ConsentCovers(ctx context.Context, clientId string, identityId string, requestedScope []string) bool {
	if identityId == "" || clientId == "" || flow.requireConnection(ctx) != nil {
		return false
	}
	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, SELECT_OAUTH_CONSENT)}
	query.Vals = append(query.Vals, flow.ProjectId, flow.AuthName, clientId, identityId)
	query.Rank = 1

	output, err := utils.ExecuteDbFetch(ctx, flow.AuthDb.GetConn(), query)
	if err != nil || len(output) == 0 {
		return false
	}
	if timestampSet(output[0]["revoked_at"]) {
		return false
	}
	granted := strings.Fields(rowString(output[0]["granted_scope"]))
	for _, scope := range requestedScope {
		if !contains(granted, scope) {
			return false
		}
	}
	return len(requestedScope) > 0
}

// SessionLifespanSeconds is how long a browser session is honoured for.
func (config OAuthServerConfig) SessionLifespanSeconds() int {
	return config.sessionLifespan()
}

func (config OAuthServerConfig) sessionLifespan() int {
	if config.SessionLifespan > 0 {
		return config.SessionLifespan
	}
	return defaultSessionLifespan
}

// CreateSession records that this browser has authenticated, so a second authorization does not ask
// for credentials again. Only the digest is stored, so the cookie cannot be reconstructed from the
// table.
func (flow EruAuthorizationFlow) CreateSession(ctx context.Context, identityId string, lifespanSeconds int) (string, error) {
	logs.WithContext(ctx).Debug("CreateSession - Start")
	if err := flow.requireConnection(ctx); err != nil {
		return "", err
	}
	if identityId == "" {
		return "", errors.New("cannot create a session without an identity")
	}

	sessionId, err := randomToken()
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("session could not be created")
	}

	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, INSERT_OAUTH_SESSION)}
	query.Vals = append(query.Vals, hashToken(sessionId), flow.ProjectId, flow.AuthName, identityId,
		time.Now().Add(time.Duration(lifespanSeconds)*time.Second))
	query.Rank = 1

	if _, err = utils.ExecuteDbSave(ctx, flow.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", errors.New("session could not be created")
	}
	return sessionId, nil
}

// SessionIdentity returns who a session belongs to, or an empty string when there is no usable
// session. A missing, expired or revoked session is not an error - it just means the browser has to
// log in, which is the ordinary case.
func (flow EruAuthorizationFlow) SessionIdentity(ctx context.Context, sessionId string) string {
	if sessionId == "" || flow.requireConnection(ctx) != nil {
		return ""
	}

	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, SELECT_OAUTH_SESSION)}
	query.Vals = append(query.Vals, hashToken(sessionId), flow.ProjectId, flow.AuthName)
	query.Rank = 1

	output, err := utils.ExecuteDbFetch(ctx, flow.AuthDb.GetConn(), query)
	if err != nil || len(output) == 0 {
		return ""
	}
	if timestampSet(output[0]["revoked_at"]) {
		return ""
	}
	if expiresAt, ok := output[0]["expires_at"].(time.Time); ok && time.Now().After(expiresAt) {
		return ""
	}
	return rowString(output[0]["identity_id"])
}

// RevokeSession ends a browser session. It reports nothing about whether the session existed.
func (flow EruAuthorizationFlow) RevokeSession(ctx context.Context, sessionId string) error {
	logs.WithContext(ctx).Debug("RevokeSession - Start")
	if sessionId == "" {
		return nil
	}
	if err := flow.requireConnection(ctx); err != nil {
		return err
	}

	query := models.Queries{Query: flow.AuthDb.GetDbQuery(ctx, REVOKE_OAUTH_SESSION)}
	query.Vals = append(query.Vals, hashToken(sessionId), flow.ProjectId, flow.AuthName)
	query.Rank = 1

	if _, err := utils.ExecuteDbSave(ctx, flow.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return errors.New("session could not be revoked")
	}
	return nil
}
