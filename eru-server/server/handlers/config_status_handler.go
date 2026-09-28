package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-store/store"
)

type ConfigStatus struct {
	Service        string    `json:"service"`
	InstanceId     string    `json:"instance_id"`
	ConfigUpdateAt time.Time `json:"config_update_at"`
}

func ConfigStatusHandler(s store.StoreI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ConfigStatusHandler - Start")
		FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(ConfigStatus{
			Service:        ServerName,
			InstanceId:     InstanceId,
			ConfigUpdateAt: s.GetUpdateTime(),
		})
	}
}
