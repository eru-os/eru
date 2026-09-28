package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/eru-os/eru/eru-ai/module_model"
	"github.com/eru-os/eru/eru-ai/module_store"
	"github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

func ToolCatalogHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]

		var projectSettings module_model.ProjectSettings
		if projectId != "" {
			var err error
			projectSettings, err = sh.Store.GetProjectSettings(r.Context(), projectId)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
		}

		catalog := tools.GetToolCatalog()
		visibleCatalog := make([]tools.ToolCatalogEntry, 0, len(catalog))
		for _, entry := range catalog {
			if projectSettings.IsToolVisible(entry.ToolType, entry.Public, tenantId) {
				visibleCatalog = append(visibleCatalog, entry)
			}
		}

		catalogBytes, err := json.Marshal(visibleCatalog)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(catalogBytes)
	}
}

func ToolCatalogAccessSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ToolCatalogAccessSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		accessFromReq := json.NewDecoder(r.Body)
		accessFromReq.DisallowUnknownFields()

		var accessRequest module_model.ToolCatalogAccessRequest
		if err := accessFromReq.Decode(&accessRequest); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if err := utils.ValidateStruct(r.Context(), accessRequest, ""); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
			return
		}

		err := sh.Store.SaveToolCatalogAccess(r.Context(), projectId, accessRequest, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("tool catalog access for ", accessRequest.ToolType, " saved successfully")})
	}
}
