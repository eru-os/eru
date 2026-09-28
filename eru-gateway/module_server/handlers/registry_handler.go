package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/eru-os/eru/eru-gateway/registry"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

// RegistryHandler holds a reference to the service registry.
type RegistryHandler struct {
	Registry *registry.Registry
}

// RegisterHandler handles the HTTP request for registering a new service instance.
func (h *RegistryHandler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logs.WithContext(ctx).Debug("RegisterHandler - Start")
	var instance eru_models.ServiceInstance
	if err := json.NewDecoder(r.Body).Decode(&instance); err != nil {
		server_handlers.FormatResponse(w, http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}
	if err := utils.ValidateStruct(ctx, &instance, ""); err != nil {
		server_handlers.FormatResponse(w, http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("validation failed: %v", err)})
		return
	}

	if err := h.Registry.Register(ctx, instance); err != nil {
		server_handlers.FormatResponse(w, http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	server_handlers.FormatResponse(w, http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"msg": fmt.Sprintf("service with id %s registered successfully", instance.Id)})
}

// DeregisterHandler handles the HTTP request for deregistering a service instance.
func (h *RegistryHandler) DeregisterHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logs.WithContext(ctx).Debug("DeregisterHandler - Start")
	vars := mux.Vars(r)
	serviceId := vars["serviceid"]
	if serviceId == "" {
		server_handlers.FormatResponse(w, http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "serviceid path parameter is required"})
		return
	}

	if err := h.Registry.Deregister(ctx, serviceId); err != nil {
		server_handlers.FormatResponse(w, http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	server_handlers.FormatResponse(w, http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("service with id %s deregistered successfully", serviceId)})
}

// HeartbeatHandler handles the HTTP request for service heartbeats.
func (h *RegistryHandler) HeartbeatHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	serviceID := vars["serviceid"]
	if serviceID == "" {
		server_handlers.FormatResponse(w, http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "serviceid path parameter is required"})
		return
	}

	if err := h.Registry.Heartbeat(ctx, serviceID); err != nil {
		server_handlers.FormatResponse(w, http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	server_handlers.FormatResponse(w, http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ListServicesHandler handles the HTTP request to list registered services.
func (h *RegistryHandler) ListServicesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logs.WithContext(ctx).Debug("ListServicesHandler - Start")
	vars := mux.Vars(r)
	serviceName := vars["servicename"]

	var instances []eru_models.ServiceInstance
	var err error

	if serviceName == "" {
		instances, err = h.Registry.ListAllServices(ctx)
	} else {
		instances, err = h.Registry.ListServices(ctx, serviceName)
	}

	if err != nil {
		server_handlers.FormatResponse(w, http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	server_handlers.FormatResponse(w, http.StatusOK)
	_ = json.NewEncoder(w).Encode(instances)
}
