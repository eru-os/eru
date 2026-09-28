package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

const StoreTableName = "eruql_config"
const StoreTenantTableName = "eruql_tenant_config"

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

		//projectJson := json.NewDecoder(r.Body)
		//projectJson.DisallowUnknownFields()

		//if err := projectJson.Decode(&compareProject); err == nil {
		//
		//} else {
		//	logs.WithContext(r.Context()).Error("err from unmarshal")
		//	server_handlers.FormatResponse(w, 400)
		//	_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		//	return
		//}
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
			logs.WithContext(r.Context()).Error(err.Error())
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
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"project": project})
		}
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

//func ProjectGenerateAesKeyHandler(sh *module_store.StoreHolder) http.HandlerFunc {
//	return func(w http.ResponseWriter, r *http.Request) {
//		logs.WithContext(r.Context()).Debug("ProjectGenerateAesKeyHandler - Start")
//		bytes := make([]byte, 32) //generate a random 32 byte key for AES-256
//		_, err := rand.Read(bytes)
//		if err != nil {
//			logs.WithContext(r.Context()).Error(err.Error())
//			server_handlers.FormatResponse(w, 400)
//			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
//		} else {
//			server_handlers.FormatResponse(w, 200)
//			_ = json.NewEncoder(w).Encode(map[string]interface{}{"key": hex.EncodeToString(bytes)})
//		}
//		return
//	}
//}
