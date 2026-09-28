package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/eru-os/eru/eru-gateway/module_model"
	"github.com/eru-os/eru/eru-gateway/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

const StoreTableName = "erugateway_config"
const StoreTenantTableName = "erugateway_config_tenant"

func FetchVarsHandler(sh *module_store.StoreHolder) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		service := vars["service"]
		if service == "gateway" {

		}
	}
}
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

		projectJson := json.NewDecoder(r.Body)
		projectJson.DisallowUnknownFields()
		var moduleStore module_store.ExendedModuleStore
		storeCompare := module_model.StoreCompare{}
		if err := projectJson.Decode(&moduleStore); err == nil {
			storeCompare, err = sh.Store.CompareModuleStore(r.Context(), moduleStore, sh.Store)

		} else {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(storeCompare)

	}
}
func SaveListenerRuleHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("SaveListenerRuleHandler - Start")
		lrFromReq := json.NewDecoder(r.Body)
		lrFromReq.DisallowUnknownFields()

		lrObj := module_model.ListenerRule{}
		if err := lrFromReq.Decode(&lrObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), lrObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		err := sh.Store.SaveListenerRule(r.Context(), &lrObj, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Listener Rule ", lrObj.RuleName, " created successfully")})
		}
	}
}

func RemoveListenerRuleHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("RemoveListenerRuleHandler - Start")
		vars := mux.Vars(r)
		listenerRuleName := vars["listenerrulename"]
		err := sh.Store.RemoveListenerRule(r.Context(), listenerRuleName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Listener Rule  ", listenerRuleName, " removed successfully")})
		}
	}
}

func GetListenerRulesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("GetListenerRulesHandler - Start")
		listenerRules := sh.Store.GetListenerRules(r.Context())
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"listener_rules": listenerRules})
	}
}

func GetConfigHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("GetConfigHandler - Start")
		ms := sh.Store.GetExtendedGatewayConfig(r.Context(), sh.Store)
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(ms)
	}
}

func SaveAuthorizerHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("SaveAuthorizerHandler - Start")
		authFromReq := json.NewDecoder(r.Body)
		authFromReq.DisallowUnknownFields()

		authObj := module_model.Authorizer{}
		if err := authFromReq.Decode(&authObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err := utils.ValidateStruct(r.Context(), authObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		err := sh.Store.SaveAuthorizer(r.Context(), authObj, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Authorizer ", authObj.AuthorizerName, " created successfully")})
		}
	}
}

func RemoveAuthorizerHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("RemoveAuthorizerHandler - Start")
		vars := mux.Vars(r)
		authorizerName := vars["authorizername"]
		err := sh.Store.RemoveAuthorizer(r.Context(), authorizerName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("Authorizer  ", authorizerName, " removed successfully")})
		}
	}
}

func GetAuthorizerHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("GetAuthorizerHandler - Start")
		authorizers := sh.Store.GetAuthorizers(r.Context())
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"authorizers": authorizers})
	}
}

func GetProjectSetingsHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("GetProjectSetingsHandler - Start")
		project_settings := sh.Store.GetProjectSettings(r.Context())
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"project_settings": project_settings})
	}
}

func ProjectSetingsSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()
		logs.WithContext(r.Context()).Debug("ProjectSetingsSaveHandler - Start")

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

		err := sh.Store.SaveProjectSettings(r.Context(), projectSettings, sh.Store, true)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("gateway settings saved successfully")})
		}
	}
}
