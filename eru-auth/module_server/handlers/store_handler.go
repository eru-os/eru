package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/eru-os/eru/eru-auth/auth"
	"github.com/eru-os/eru/eru-auth/module_model"
	"github.com/eru-os/eru/eru-auth/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

const StoreTableName = "eruauth_config"
const StoreTenantTableName = "eruauth_config_tenant"

func StoreLoadHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("StoreLoadHandler - Start")

		// Load new store from DB
		newStore, err := module_store.LoadStore(r.Context(), StoreTableName, StoreTenantTableName)
		if err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprintf("Failed to load store: %v", err))
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		// Update the global StoreHolder with the new store
		sh.Store = newStore

		logs.WithContext(r.Context()).Info("Store loaded and replaced successfully")
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(newStore)
	}
}
func StoreCompareHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("StoreCompareHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]

		comparePrjFromReq := json.NewDecoder(r.Body)
		comparePrjFromReq.DisallowUnknownFields()
		var compareProject module_model.ExtendedProject

		if err := comparePrjFromReq.Decode(&compareProject); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		storeCompare := module_model.StoreCompare{}
		myPrj, err := sh.Store.GetExtendedProjectConfig(r.Context(), projectID, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		storeCompare, err = myPrj.CompareProject(r.Context(), compareProject)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(storeCompare)
	}
}

func ProjectSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ProjectSaveHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		err := sh.Store.SaveProject(r.Context(), projectID, sh.Store, true)

		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project ", projectID, " created successfully")})
		}
	}
}

func ProjectRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ProjectRemoveHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		err := sh.Store.RemoveProject(r.Context(), projectID, sh.Store)

		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project ", projectID, " removed successfully")})
		}
	}
}

func ProjectListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ProjectListHandler - Start")
		projectIds := sh.Store.GetProjectList(r.Context())
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"projects": projectIds})
	}
}

func ProjectConfigHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ProjectConfigHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		project, err := sh.Store.GetExtendedProjectConfig(r.Context(), projectID, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"project": project})
		}
	}
}

func KidSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("KidSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		kidFromReq := json.NewDecoder(r.Body)
		kidFromReq.DisallowUnknownFields()

		type kidStruct struct {
			Kid string `json:"kid"`
		}

		var kidObj kidStruct
		if err := kidFromReq.Decode(&kidObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		err := utils.ValidateStruct(r.Context(), kidObj, "")
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
			return
		}

		_, err = sh.Store.SaveKid(r.Context(), fmt.Sprint("ERUAUTH_KID_", kidObj.Kid), projectId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("kid ", kidObj.Kid, " saved successfully")})
		return
	}
}
func KidRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("KidRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		kid := vars["kid"]
		err := sh.Store.RemoveKid(r.Context(), fmt.Sprint("ERUAUTH_KID_", kid), projectId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("kid ", kid, " removed successfully")})
		}
		return
	}
}

func ApiTokenSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ApiTokenSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		tokenFromReq := json.NewDecoder(r.Body)
		tokenFromReq.DisallowUnknownFields()

		type tokenStruct struct {
			Kid          string                 `json:"kid" eru:"required"`
			IdentityId   string                 `json:"user_id" eru:"required"`
			TokenClaims  map[string]interface{} `json:"token_claims" eru:"required"`
			TokenName    string                 `json:"token_name" eru:"required"`
			TokenHeaders map[string]interface{} `json:"token_headers"`
		}

		var tokenObj tokenStruct
		if err := tokenFromReq.Decode(&tokenObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		err := utils.ValidateStruct(r.Context(), tokenObj, "")
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
			return
		}
		jwt := ""
		jwt, err = sh.Store.SaveApiToken(r.Context(), tokenObj.IdentityId, fmt.Sprint("ERUAUTH_KID_", tokenObj.Kid), projectId, tokenObj.TokenHeaders, tokenObj.TokenClaims, tokenObj.TokenName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"token": jwt})
		return
	}
}
func ApiTokenRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ApiTokenRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tokenId := vars["token_id"]
		err := sh.Store.RevokeApiToken(r.Context(), tokenId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("token ", tokenId, " revoked successfully")})
		}
		return
	}
}

func JWKHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("JWKHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		kid := vars["kid"]

		keys, err := sh.Store.FetchJWKKeys(r.Context(), projectId, kid, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"keys": keys})
		}
	}
}

// JWKSetHandler publishes the project's whole key set at the unparameterised jwks_uri that the
// discovery document advertises.
// KidStatusHandler retires or reactivates a signing key. Retiring leaves the key published so
// tokens it already signed keep verifying, which is what makes an overlapping rotation possible.
func KidStatusHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("KidStatusHandler - Start")
		vars := mux.Vars(r)
		status := strings.ToUpper(vars["status"])
		if status != module_model.KidStatusActive && status != module_model.KidStatusRetired {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "status must be ACTIVE or RETIRED"})
			return
		}

		err := sh.Store.SetKidStatus(r.Context(), vars["project"], fmt.Sprint("ERUAUTH_KID_", vars["kid"]), status, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("kid ", vars["kid"], " is now ", status)})
	}
}

func JWKSetHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("JWKSetHandler - Start")
		vars := mux.Vars(r)
		keys, err := sh.Store.FetchJWKKeySet(r.Context(), vars["project"], sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 404)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"keys": keys})
	}
}

func OAuthProtectedResourceHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("OAuthProtectedResourceHandler - Start")
		authObj, err := authFromRequest(sh, r)
		if err != nil {
			server_handlers.FormatResponse(w, 404)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		issuer := authObj.OAuthIssuer(r.Context())
		if issuer == "" {
			err = fmt.Errorf("no oauth issuer resolved - set oauth_server.issuer")
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 404)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(module_model.OAuthProtectedResource{
			Resource:               module_model.McpResourceUrl(r),
			AuthorizationServers:   []string{issuer},
			ScopesSupported:        authObj.OAuthServer(r.Context()).Scopes(),
			BearerMethodsSupported: []string{"header"},
		})
	}
}

func ApiTokenListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ApiTokenListHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		identityId := vars["identity_id"]
		tokens, err := sh.Store.GetApiTokens(r.Context(), identityId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"tokens": tokens})
		}
		return
	}
}

func AuthSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("AuthSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authType := ""
		authFromReq := json.NewDecoder(r.Body)
		authFromReq.DisallowUnknownFields()

		var authObjTmp map[string]interface{}
		if err := authFromReq.Decode(&authObjTmp); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			if at, ok := authObjTmp["auth_type"]; !ok {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : auth_type")})
				return
			} else {
				authType = at.(string)
			}
		}
		authObj := auth.GetAuth(authType)
		authJson, err := json.Marshal(authObjTmp)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if err = json.Unmarshal(authJson, &authObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), authObj, "")
			//TODO to uncomment this code and validate the incoming json
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}

		err = sh.Store.SaveAuth(r.Context(), authObj, projectId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			authName, anErr := authObj.GetAttribute(r.Context(), "auth_name")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": anErr.Error()})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("auth config ", authName, " saved successfully")})
		}
		return
	}
}

func AuthRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("AuthRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		authName := vars["authname"]
		err := sh.Store.RemoveAuth(r.Context(), authName, projectId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("auth config ", authName, " removed successfully")})
		}
		return
	}
}

func ProjectSetingsSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ProjectSetingsSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		prjConfigFromReq := json.NewDecoder(r.Body)
		prjConfigFromReq.DisallowUnknownFields()

		var projectSettings module_model.ProjectSettings

		if err := prjConfigFromReq.Decode(&projectSettings); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			logs.WithContext(r.Context()).Info(fmt.Sprint(projectSettings))
			err := utils.ValidateStruct(r.Context(), projectSettings, "")
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}

		err := sh.Store.SaveProjectSettings(r.Context(), projectId, projectSettings, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project settings for ", projectId, " saved successfully")})
		}
	}
}

func ProjectFunctionListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ProjectFunctionListHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]

		reqHeader := http.Header{}
		res, _, _, _, err := utils.CallHttp(r.Context(), http.MethodGet, fmt.Sprint(module_store.Erufuncbaseurl, "/store/", projectID, "/func/list"), reqHeader, nil, nil, nil, nil)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		}

		return
	}
}
