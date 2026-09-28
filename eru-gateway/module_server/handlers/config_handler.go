package handlers

import (
	"encoding/json"
	"net/http"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func ConfigStatusHandler(w http.ResponseWriter, r *http.Request) {
	logs.WithContext(r.Context()).Debug("ConfigStatusHandler - Start")
	serviceName := mux.Vars(r)["servicename"]

	report, err := configSyncNotifier.Status(r.Context(), serviceName)
	if err != nil {
		logs.WithContext(r.Context()).Error(err.Error())
		server_handlers.FormatResponse(w, http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	server_handlers.FormatResponse(w, http.StatusOK)
	_ = json.NewEncoder(w).Encode(report)
}

func ConfigForceSyncHandler(w http.ResponseWriter, r *http.Request) {
	logs.WithContext(r.Context()).Debug("ConfigForceSyncHandler - Start")
	serviceName := mux.Vars(r)["servicename"]

	report, err := configSyncNotifier.ForceSync(r.Context(), serviceName)
	if err != nil {
		logs.WithContext(r.Context()).Error(err.Error())
		server_handlers.FormatResponse(w, http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	server_handlers.FormatResponse(w, http.StatusOK)
	_ = json.NewEncoder(w).Encode(report)
}
