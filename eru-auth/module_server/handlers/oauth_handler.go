package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-server/server"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

// authFromRequest resolves the auth an unrouted endpoint belongs to. The well known documents and
// the registration endpoint are read from the root of the issuer by clients that know nothing of
// eru's paths, so the project and auth come from the headers the gateway listener rule adds.
func authFromRequest(sh *module_store.StoreHolder, r *http.Request) (auth.AuthI, error) {
	projectId := server.RequestProject(r)
	authName := server.RequestAuthName(r)
	if authName == "" {
		err := fmt.Errorf("%s is not set - add it to the add_headers of the listener rule", server.AuthNameHeaderKey)
		logs.WithContext(r.Context()).Error(err.Error())
		return nil, err
	}
	return sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
}

// writeOAuthError answers in the RFC 6749 error shape so a spec compliant client can read it. A
// plain error falls back to server_error, since anything that is not an OAuthError has not been
// classified and should not leak its message as a protocol code.
func writeOAuthError(w http.ResponseWriter, r *http.Request, err error) {
	oAuthError, oAuthErrorOk := err.(auth.OAuthError)
	if !oAuthErrorOk {
		oAuthError = auth.NewOAuthError(http.StatusBadRequest, "invalid_request", err.Error())
	}
	statusCode := oAuthError.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusBadRequest
	}
	logs.WithContext(r.Context()).Error(oAuthError.Error())
	server_handlers.FormatResponse(w, statusCode)
	_ = json.NewEncoder(w).Encode(oAuthError)
}

// AuthorizationServerMetadataHandler serves the RFC 8414 document, and the same document as the
// openid-configuration. It is composed here so the registration endpoint and the
// oauth-authorization-server path are always published, whichever backend holds the grants.
func AuthorizationServerMetadataHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("AuthorizationServerMetadataHandler - Start")
		authObj, err := authFromRequest(sh, r)
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		metadata, err := authObj.AuthorizationServerMetadata(r.Context())
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(metadata)
	}
}

// RegisterOAuthClientHandler is the RFC 7591 dynamic client registration endpoint. Every client that
// arrives here is vetted against the project's OAuthClientPolicy first.
func RegisterOAuthClientHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("RegisterOAuthClientHandler - Start")
		authObj, err := authFromRequest(sh, r)
		if err != nil {
			writeOAuthError(w, r, auth.NewOAuthError(http.StatusNotFound, "invalid_request", err.Error()))
			return
		}

		oAuthServer := authObj.OAuthServer(r.Context())
		if !oAuthServer.Enabled {
			writeOAuthError(w, r, auth.NewOAuthError(http.StatusNotFound, "invalid_request", "oauth server is not enabled"))
			return
		}
		if !oAuthServer.ClientPolicy.AllowDynamicRegistration {
			writeOAuthError(w, r, auth.NewOAuthError(http.StatusForbidden, "access_denied", "dynamic client registration is not enabled"))
			return
		}

		clientFromReq := json.NewDecoder(r.Body)
		var client auth.OAuthClient
		if err = clientFromReq.Decode(&client); err != nil {
			writeOAuthError(w, r, auth.NewOAuthError(http.StatusBadRequest, "invalid_client_metadata", err.Error()))
			return
		}
		// A caller does not get to pick its own credentials.
		client.ClientId = ""
		client.ClientSecret = ""

		client, err = oAuthServer.ClientPolicy.ApplyPolicy(r.Context(), client)
		if err != nil {
			writeOAuthError(w, r, err)
			return
		}

		registry, err := authObj.ClientRegistry(r.Context(), server.RequestProject(r))
		if err != nil {
			writeOAuthError(w, r, auth.NewOAuthError(http.StatusInternalServerError, "server_error", err.Error()))
			return
		}

		client, err = registry.CreateClient(r.Context(), client)
		if err != nil {
			writeOAuthError(w, r, auth.NewOAuthError(http.StatusBadRequest, "invalid_client_metadata", err.Error()))
			return
		}

		server_handlers.FormatResponse(w, http.StatusCreated)
		_ = json.NewEncoder(w).Encode(client)
	}
}

// OAuthClientSaveHandler registers or updates a client from the store api, for the first party
// clients that are configured rather than self registered.
func OAuthClientSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("OAuthClientSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]

		authObj, err := sh.Store.GetAuth(r.Context(), projectId, authName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		clientFromReq := json.NewDecoder(r.Body)
		clientFromReq.DisallowUnknownFields()
		var client auth.OAuthClient
		if err = clientFromReq.Decode(&client); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		client, err = authObj.OAuthServer(r.Context()).ClientPolicy.ApplyPolicy(r.Context(), client)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		registry, err := authObj.ClientRegistry(r.Context(), projectId)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if client.ClientId == "" {
			client, err = registry.CreateClient(r.Context(), client)
		} else {
			client, err = registry.UpdateClient(r.Context(), client.ClientId, client)
		}
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(client)
	}
}

func OAuthClientGetHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("OAuthClientGetHandler - Start")
		vars := mux.Vars(r)

		authObj, err := sh.Store.GetAuth(r.Context(), vars["project"], vars["authname"], sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		registry, err := authObj.ClientRegistry(r.Context(), vars["project"])
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		client, err := registry.GetClient(r.Context(), vars["clientid"])
		if err != nil {
			server_handlers.FormatResponse(w, 404)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(client)
	}
}

func OAuthClientRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("OAuthClientRemoveHandler - Start")
		vars := mux.Vars(r)
		clientId := vars["clientid"]

		authObj, err := sh.Store.GetAuth(r.Context(), vars["project"], vars["authname"], sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		registry, err := authObj.ClientRegistry(r.Context(), vars["project"])
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if err = registry.DeleteClient(r.Context(), clientId); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("oauth client ", clientId, " removed successfully")})
	}
}
