package auth

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	erusha "github.com/eru-os/eru/eru-crypto/sha"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/uuid"
)

const (
	SELECT_OAUTH_CLIENT = "select client_id, project_id, auth_name, client_name, client_secret_hash, token_endpoint_auth_method, redirect_uris, grant_types, response_types, scope from eruauth_oauth_clients where project_id = ??? and auth_name = ??? and client_id = ???"
	INSERT_OAUTH_CLIENT = "insert into eruauth_oauth_clients (client_id, project_id, auth_name, client_name, client_secret_hash, token_endpoint_auth_method, redirect_uris, grant_types, response_types, scope) values (???,???,???,???,???,???,???,???,???,???)"
	UPDATE_OAUTH_CLIENT = "update eruauth_oauth_clients set client_name = ???, token_endpoint_auth_method = ???, redirect_uris = ???, grant_types = ???, response_types = ???, scope = ???, updated_date = CURRENT_TIMESTAMP where project_id = ??? and auth_name = ??? and client_id = ???"
	DELETE_OAUTH_CLIENT = "delete from eruauth_oauth_clients where project_id = ??? and auth_name = ??? and client_id = ???"
)

// EruClientRegistry keeps oauth clients in eru-auth's own store, selected by
// OAuthServerConfig.Backend, and is the first half of owning issuance - the grant endpoints are the
// other half.
type EruClientRegistry struct {
	AuthDb    AuthDbI
	ProjectId string
	AuthName  string
}

// CreateClient mints the client id, and a secret only for a client that authenticates with one.
// The secret is returned to the caller once and stored only as a hash, so a leaked row cannot be
// used to impersonate the client.
func (registry EruClientRegistry) CreateClient(ctx context.Context, client OAuthClient) (OAuthClient, error) {
	logs.WithContext(ctx).Debug("CreateClient - Start")
	if registry.AuthDb == nil || registry.AuthDb.GetConn() == nil {
		return OAuthClient{}, registry.noConnection(ctx)
	}

	client.ClientId = uuid.New().String()
	secretHash := ""
	if client.TokenEndpointAuthMethod != "none" {
		client.ClientSecret = uuid.New().String()
		secretHash = hashClientSecret(client.ClientSecret)
	} else {
		client.ClientSecret = ""
	}

	redirectUris, grantTypes, responseTypes, err := marshalClientLists(ctx, client)
	if err != nil {
		return OAuthClient{}, err
	}

	query := models.Queries{Query: registry.AuthDb.GetDbQuery(ctx, INSERT_OAUTH_CLIENT)}
	query.Vals = append(query.Vals, client.ClientId, registry.ProjectId, registry.AuthName, client.ClientName,
		secretHash, client.TokenEndpointAuthMethod, redirectUris, grantTypes, responseTypes, client.Scope)
	query.Rank = 1

	if _, err = utils.ExecuteDbSave(ctx, registry.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return OAuthClient{}, errors.New("oauth client could not be saved")
	}
	return client, nil
}

func (registry EruClientRegistry) GetClient(ctx context.Context, clientId string) (OAuthClient, error) {
	logs.WithContext(ctx).Debug("GetClient - Start")
	if registry.AuthDb == nil || registry.AuthDb.GetConn() == nil {
		return OAuthClient{}, registry.noConnection(ctx)
	}

	query := models.Queries{Query: registry.AuthDb.GetDbQuery(ctx, SELECT_OAUTH_CLIENT)}
	query.Vals = append(query.Vals, registry.ProjectId, registry.AuthName, clientId)
	query.Rank = 1

	output, err := utils.ExecuteDbFetch(ctx, registry.AuthDb.GetConn(), query)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return OAuthClient{}, errors.New("oauth client could not be read")
	}
	if len(output) == 0 {
		err = fmt.Errorf("oauth client not found : %s", clientId)
		logs.WithContext(ctx).Info(err.Error())
		return OAuthClient{}, err
	}
	return clientFromRow(ctx, output[0])
}

func (registry EruClientRegistry) UpdateClient(ctx context.Context, clientId string, client OAuthClient) (OAuthClient, error) {
	logs.WithContext(ctx).Debug("UpdateClient - Start")
	if registry.AuthDb == nil || registry.AuthDb.GetConn() == nil {
		return OAuthClient{}, registry.noConnection(ctx)
	}
	// The row has to exist, and an update never touches the secret - rotating one is its own
	// operation rather than a side effect of editing metadata.
	if _, err := registry.GetClient(ctx, clientId); err != nil {
		return OAuthClient{}, err
	}

	redirectUris, grantTypes, responseTypes, err := marshalClientLists(ctx, client)
	if err != nil {
		return OAuthClient{}, err
	}

	query := models.Queries{Query: registry.AuthDb.GetDbQuery(ctx, UPDATE_OAUTH_CLIENT)}
	query.Vals = append(query.Vals, client.ClientName, client.TokenEndpointAuthMethod, redirectUris,
		grantTypes, responseTypes, client.Scope, registry.ProjectId, registry.AuthName, clientId)
	query.Rank = 1

	if _, err = utils.ExecuteDbSave(ctx, registry.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return OAuthClient{}, errors.New("oauth client could not be saved")
	}

	client.ClientId = clientId
	client.ClientSecret = ""
	return client, nil
}

func (registry EruClientRegistry) DeleteClient(ctx context.Context, clientId string) error {
	logs.WithContext(ctx).Debug("DeleteClient - Start")
	if registry.AuthDb == nil || registry.AuthDb.GetConn() == nil {
		return registry.noConnection(ctx)
	}

	query := models.Queries{Query: registry.AuthDb.GetDbQuery(ctx, DELETE_OAUTH_CLIENT)}
	query.Vals = append(query.Vals, registry.ProjectId, registry.AuthName, clientId)
	query.Rank = 1

	if _, err := utils.ExecuteDbSave(ctx, registry.AuthDb.GetConn(), []*models.Queries{&query}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return errors.New("oauth client could not be removed")
	}
	return nil
}

// VerifyClientSecret checks a presented secret against the stored hash in constant time. A client
// registered with token_endpoint_auth_method none holds no secret and is authenticated by pkce
// alone, so presenting one is a mismatch rather than a shortcut.
func (registry EruClientRegistry) VerifyClientSecret(ctx context.Context, clientId string, clientSecret string) error {
	logs.WithContext(ctx).Debug("VerifyClientSecret - Start")
	if registry.AuthDb == nil || registry.AuthDb.GetConn() == nil {
		return registry.noConnection(ctx)
	}

	query := models.Queries{Query: registry.AuthDb.GetDbQuery(ctx, SELECT_OAUTH_CLIENT)}
	query.Vals = append(query.Vals, registry.ProjectId, registry.AuthName, clientId)
	query.Rank = 1

	output, err := utils.ExecuteDbFetch(ctx, registry.AuthDb.GetConn(), query)
	if err != nil || len(output) == 0 {
		return NewOAuthError(401, "invalid_client", "client authentication failed")
	}
	storedHash := rowString(output[0]["client_secret_hash"])
	if storedHash == "" || subtle.ConstantTimeCompare([]byte(storedHash), []byte(hashClientSecret(clientSecret))) != 1 {
		return NewOAuthError(401, "invalid_client", "client authentication failed")
	}
	return nil
}

func (registry EruClientRegistry) noConnection(ctx context.Context) error {
	err := errors.New("oauth client store has no database connection")
	logs.WithContext(ctx).Error(err.Error())
	return err
}

func hashClientSecret(clientSecret string) string {
	return hex.EncodeToString(erusha.NewSHA512([]byte(clientSecret)))
}

func marshalClientLists(ctx context.Context, client OAuthClient) (string, string, string, error) {
	redirectUris, err := json.Marshal(client.RedirectURIs)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", "", "", errors.New("oauth client redirect_uris could not be stored")
	}
	grantTypes, err := json.Marshal(client.GrantTypes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", "", "", errors.New("oauth client grant_types could not be stored")
	}
	responseTypes, err := json.Marshal(client.ResponseTypes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", "", "", errors.New("oauth client response_types could not be stored")
	}
	return string(redirectUris), string(grantTypes), string(responseTypes), nil
}

// clientFromRow never carries the stored hash back out - nothing above this layer has a use for it,
// and a value that is never returned cannot be logged or serialised by accident.
func clientFromRow(ctx context.Context, row map[string]interface{}) (OAuthClient, error) {
	client := OAuthClient{
		ClientId:                rowString(row["client_id"]),
		ClientName:              rowString(row["client_name"]),
		TokenEndpointAuthMethod: rowString(row["token_endpoint_auth_method"]),
		Scope:                   rowString(row["scope"]),
	}
	client.RedirectURIs = rowStringList(ctx, row["redirect_uris"])
	client.GrantTypes = rowStringList(ctx, row["grant_types"])
	client.ResponseTypes = rowStringList(ctx, row["response_types"])
	return client, nil
}

func rowString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case *string:
		if v != nil {
			return *v
		}
	case []byte:
		return string(v)
	}
	return ""
}

// rowStringList reads a json array column, which the driver may hand back as a string, as bytes or
// already decoded depending on the column type.
func rowStringList(ctx context.Context, value interface{}) []string {
	var values []string
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {
			if itemStr, ok := item.(string); ok {
				values = append(values, itemStr)
			}
		}
		return values
	case []string:
		return v
	}
	raw := rowString(value)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("oauth client list column could not be read : ", err.Error()))
		return nil
	}
	return values
}
