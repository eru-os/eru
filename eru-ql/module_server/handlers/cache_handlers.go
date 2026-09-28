package handlers

import (
	"encoding/json"
	"net/http"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
	"github.com/eru-os/eru/eru-ql/module_store"
	"github.com/eru-os/eru/eru-ql/qlcache"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

type cacheInvalidateRequest struct {
	DataSources []string `json:"datasources"`
	Tables      []string `json:"tables"`
}

type cacheInvalidateResponse struct {
	Deleted  int            `json:"deleted"`
	Skipped  []string       `json:"skipped,omitempty"`
	PerDS    map[string]int `json:"per_datasource,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// CacheInvalidateHandler invalidates cache entries tagged with the given
// tables across the specified datasources (or all datasources in the project
// when none are specified). Datasources without a configured QueryCache are
// silently skipped so a project-wide call succeeds partially.
func CacheInvalidateHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logs.WithContext(ctx).Debug("CacheInvalidateHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		var req cacheInvalidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server_handlers.FormatResponse(w, http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		datasources, err := sh.Store.GetDataSources(ctx, projectId, vars["tenantId"])
		if err != nil {
			server_handlers.FormatResponse(w, http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		targets := datasources
		if len(req.DataSources) > 0 {
			targets = make(map[string]*module_model.DataSource, len(req.DataSources))
			for _, alias := range req.DataSources {
				if ds, ok := datasources[alias]; ok {
					targets[alias] = ds
				}
			}
		}

		resp := cacheInvalidateResponse{PerDS: map[string]int{}}
		for alias, ds := range targets {
			if ds == nil || ds.GetQueryCache() == nil {
				resp.Skipped = append(resp.Skipped, alias)
				continue
			}
			n, err := qlcache.InvalidateBlocking(ctx, ds, req.Tables)
			if err != nil {
				resp.Warnings = append(resp.Warnings, alias+": "+err.Error())
				continue
			}
			resp.PerDS[alias] = n
			resp.Deleted += n
		}

		server_handlers.FormatResponse(w, http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// CacheStatsHandler returns the process-wide cache counters.
func CacheStatsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		server_handlers.FormatResponse(w, http.StatusOK)
		stats := qlcache.GetStats()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"stats":         stats,
			"dropped":       qlcache.DropCount(),
			"breaker_open":  qlcache.BreakerOpen(),
			"breaker_trips": qlcache.BreakerTrips(),
		})
	}
}
