package module_server

import (
	"net/http"

	module_handlers "github.com/eru-os/eru/eru-gateway/module_server/handlers"
	"github.com/eru-os/eru/eru-gateway/module_store"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func SetServiceName() {
	server_handlers.ServerName = "eru-gateway"
}
func AddModuleRoutes(serverRouter *mux.Router, sh *module_store.StoreHolder, rh *module_handlers.RegistryHandler) {

	//overwriting the handler of eru-server as gateway does not need variables and has to route to different services
	serverRouter.Get("variables_list").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("variables_savevar").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("variables_removevar").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("variables_saveenvvar").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("variables_removeenvvar").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("variables_savesecret").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("variables_removesecret").HandlerFunc(module_handlers.RouteHandler(sh, rh))

	serverRouter.Get("repo_list").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("repo_save").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("repo_save_token").HandlerFunc(module_handlers.RouteHandler(sh, rh))

	serverRouter.Get("sm").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("sm_list").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("sm_value").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("sm_set").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("sm_unset").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("sm_get").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("tsm_set").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("tsm_unset").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("tsm_get").HandlerFunc(module_handlers.RouteHandler(sh, rh))

	serverRouter.Get("kms_list").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("kms_save").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("kms_remove").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("kms_remove_cd").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("kms_remove_dd").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	//store functions specific to files
	serverRouter.Get("event_list").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("event_save").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("event_remove").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("event_remove_cd").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("event_pub").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("event_poll").HandlerFunc(module_handlers.RouteHandler(sh, rh))

	serverRouter.Get("sr_list").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("sr_save").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("sr_remove").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("tsr_list").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("tsr_save").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("tsr_remove").HandlerFunc(module_handlers.RouteHandler(sh, rh))

	serverRouter.Get("sch").HandlerFunc(module_handlers.RouteHandler(sh, rh))
	serverRouter.Get("sch_list").HandlerFunc(module_handlers.RouteHandler(sh, rh))

	// The gateway's own config routes. ConfigApiKeyMiddleware is mounted here and on the registry
	// subrouter only - never on the catch-all below, which forwards to other services.
	storeRouter := serverRouter.PathPrefix("/store").Subrouter()
	storeRouter.Use(module_handlers.ConfigApiKeyMiddleware)
	storeRouter.Methods(http.MethodGet).Path("/load").HandlerFunc(module_handlers.StoreLoadHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/compare").HandlerFunc(module_handlers.StoreCompareHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/config").HandlerFunc(module_handlers.GetConfigHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/listenerrule/save").HandlerFunc(module_handlers.SaveListenerRuleHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/listenerrule/remove/{listenerrulename}").HandlerFunc(module_handlers.RemoveListenerRuleHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/listenerrule/list").HandlerFunc(module_handlers.GetListenerRulesHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/authorizer/save").HandlerFunc(module_handlers.SaveAuthorizerHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/authorizer/remove/{authorizername}").HandlerFunc(module_handlers.RemoveAuthorizerHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/authorizer/list").HandlerFunc(module_handlers.GetAuthorizerHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/settings/save").HandlerFunc(module_handlers.ProjectSetingsSaveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/settings").HandlerFunc(module_handlers.GetProjectSetingsHandler(sh))

	// Add registry routes
	registryRouter := serverRouter.PathPrefix("/registry").Subrouter()
	registryRouter.Use(module_handlers.ConfigApiKeyMiddleware)
	registryRouter.Methods(http.MethodPost).Path("/register").HandlerFunc(rh.RegisterHandler)
	registryRouter.Methods(http.MethodDelete).Path("/deregister/{serviceid}").HandlerFunc(rh.DeregisterHandler)
	registryRouter.Methods(http.MethodPost).Path("/heartbeat/{serviceid}").HandlerFunc(rh.HeartbeatHandler)
	registryRouter.Methods(http.MethodGet).Path("/services").HandlerFunc(rh.ListServicesHandler)
	registryRouter.Methods(http.MethodGet).Path("/services/{servicename}").HandlerFunc(rh.ListServicesHandler)
	registryRouter.Methods(http.MethodGet).Path("/config/status").HandlerFunc(module_handlers.ConfigStatusHandler)
	registryRouter.Methods(http.MethodGet).Path("/config/status/{servicename}").HandlerFunc(module_handlers.ConfigStatusHandler)
	registryRouter.Methods(http.MethodPost).Path("/config/sync").HandlerFunc(module_handlers.ConfigForceSyncHandler)
	registryRouter.Methods(http.MethodPost).Path("/config/sync/{servicename}").HandlerFunc(module_handlers.ConfigForceSyncHandler)

	serverRouter.PathPrefix("/").HandlerFunc(module_handlers.RouteHandler(sh, rh))
}
