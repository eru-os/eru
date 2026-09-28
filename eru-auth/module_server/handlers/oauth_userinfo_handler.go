package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_store"
	erujwt "github.com/eru-os/eru/eru-crypto/jwt"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-server/server"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
)

// OAuthJWKSetHandler publishes the project's key set at the root path the discovery document
// advertises as jwks_uri. The project comes from the listener rule headers, the same way the other
// well known documents resolve it - a client fetching jwks_uri knows nothing of eru's paths.
func OAuthJWKSetHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("OAuthJWKSetHandler - Start")
		projectId := server.RequestProject(r)
		keys, err := sh.Store.FetchJWKKeySet(r.Context(), projectId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"keys": keys})
	}
}

// UserInfoHandler is the OIDC userinfo endpoint. It answers for the subject of the presented access
// token, never for a subject named in the request, so a valid token for one user can never be used
// to read another.
func OAuthUserInfoHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("OAuthUserInfoHandler - Start")
		authObj, err := authFromRequest(sh, r)
		if err != nil {
			writeBearerError(w, r, http.StatusUnauthorized, "invalid_token", err.Error())
			return
		}

		accessToken := bearerToken(r)
		if accessToken == "" {
			writeBearerError(w, r, http.StatusUnauthorized, "invalid_token", "an access token is required")
			return
		}

		claims, err := verifyOwnToken(sh, r, server.RequestProject(r), accessToken)
		if err != nil {
			writeBearerError(w, r, http.StatusUnauthorized, "invalid_token", "the access token is not valid")
			return
		}

		subject, _ := claims["sub"].(string)
		if subject == "" {
			writeBearerError(w, r, http.StatusUnauthorized, "invalid_token", "the access token has no subject")
			return
		}

		userInfo := map[string]interface{}{"sub": subject}
		// The identity is best effort: a token that verifies already establishes who the caller is,
		// so a lookup failure returns the subject rather than rejecting a valid token.
		if identity, identityErr := authObj.GetUser(r.Context(), subject); identityErr == nil {
			for key, value := range identity.Attributes {
				if key != "sub" {
					userInfo[key] = value
				}
			}
		} else {
			logs.WithContext(r.Context()).Info(fmt.Sprint("userinfo identity lookup failed : ", identityErr.Error()))
		}

		w.Header().Set("Cache-Control", "no-store")
		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(userInfo)
	}
}

// verifyOwnToken validates a token this server issued, against the key named in its header. The key
// is read locally rather than over http - fetching our own published key set would be a round trip
// to ourselves.
func verifyOwnToken(sh *module_store.StoreHolder, r *http.Request, projectId string, token string) (map[string]interface{}, error) {
	kid := erujwt.TokenKid(r.Context(), token)
	if kid == "" {
		return nil, fmt.Errorf("token has no kid")
	}
	// A retired key must still verify - that is what makes rotation possible - so this reads the key
	// itself rather than going through the signing check.
	keyPair, err := sh.Store.GetKid(r.Context(), fmt.Sprint("ERUAUTH_KID_", kid), projectId, sh.Store)
	if err != nil {
		return nil, err
	}
	return erujwt.VerifyTokenWithPublicKey(r.Context(), token, keyPair.PublicKey)
}

func bearerToken(r *http.Request) string {
	authorization := r.Header.Get("Authorization")
	const bearerPrefix = "bearer "
	if len(authorization) <= len(bearerPrefix) || !strings.EqualFold(authorization[:len(bearerPrefix)], bearerPrefix) {
		return ""
	}
	return strings.TrimSpace(authorization[len(bearerPrefix):])
}

// writeBearerError answers in the RFC 6750 shape, so a client is told why its token was refused.
func writeBearerError(w http.ResponseWriter, r *http.Request, statusCode int, errorCode string, description string) {
	logs.WithContext(r.Context()).Info(fmt.Sprint(errorCode, " : ", description))
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="%s", error_description="%s"`, errorCode, description))
	server_handlers.FormatResponse(w, statusCode)
	_ = json.NewEncoder(w).Encode(auth.OAuthError{ErrorCode: errorCode, ErrorDescription: description})
}
