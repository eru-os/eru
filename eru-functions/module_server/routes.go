package module_server

import (
	"net/http"

	module_handlers "github.com/eru-os/eru/eru-functions/module_server/handlers"
	"github.com/eru-os/eru/eru-functions/module_store"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func SetServiceName() {
	server_handlers.ServerName = "eru-functions"
}
func AddModuleRoutes(serverRouter *mux.Router, sh *module_store.StoreHolder) {

	//store functions specific to files
	storeRouter := serverRouter.PathPrefix("/store").Subrouter()
	storeRouter.Methods(http.MethodGet).Path("/load").HandlerFunc(module_handlers.StoreLoadHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/compare").HandlerFunc(module_handlers.StoreCompareHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/route/save").HandlerFunc(module_handlers.RouteSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/route/remove/{routename}").HandlerFunc(module_handlers.RouteRemoveHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/func/fetch/{funcname}").HandlerFunc(module_handlers.FuncFetchHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/func/fetch/{funcname}").HandlerFunc(module_handlers.FuncFetchHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/func/save").HandlerFunc(module_handlers.FuncSaveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/func/save").HandlerFunc(module_handlers.FuncSaveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/func/validate").HandlerFunc(module_handlers.FuncValidateHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenant}/func/remove/{funcname}").HandlerFunc(module_handlers.FuncRemoveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/func/remove/{funcname}").HandlerFunc(module_handlers.FuncRemoveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/func/run/{funcstepname}/{endfuncstepname}").HandlerFunc(module_handlers.SFuncRunHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/func/run/{funcstepname}/{endfuncstepname}").HandlerFunc(module_handlers.SFuncRunHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/func/run/{funcstepname}").HandlerFunc(module_handlers.SFuncRunHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/func/run/{funcstepname}").HandlerFunc(module_handlers.SFuncRunHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/func/run").HandlerFunc(module_handlers.FuncRunHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/func/run").HandlerFunc(module_handlers.FuncRunHandler(sh))

	//storeRouter.Methods(http.MethodGet).Path("/{project}/func/{funcname}/request/list").HandlerFunc(module_handlers.FuncRequestListHandler(sh))
	//storeRouter.Methods(http.MethodPost).Path("/{project}/func/request/save").HandlerFunc(module_handlers.FuncRequestSaveHandler(sh))
	//storeRouter.Methods(http.MethodDelete).Path("/{project}/func/request/remove/{requestid}").HandlerFunc(module_handlers.FuncRequestRemoveHandler(sh))
	//storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/func/{funcname}/request/list").HandlerFunc(module_handlers.FuncRequestListHandler(sh))
	//storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/func/request/save").HandlerFunc(module_handlers.FuncRequestSaveHandler(sh))
	//storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenant}/func/request/remove/{requestid}").HandlerFunc(module_handlers.FuncRequestRemoveHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/wf/save").HandlerFunc(module_handlers.WfSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/wf/remove/{wfname}").HandlerFunc(module_handlers.WfRemoveHandler(sh))
	//storeRouter.Methods(http.MethodPost).Path("/{project}/wf/run").HandlerFunc(module_handlers.WfRunHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/save").HandlerFunc(module_handlers.ProjectSaveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/settings/save").HandlerFunc(module_handlers.ProjectSetingsSaveHandler(sh))
	//storeRouter.Methods(http.MethodPost).Path("/{project}/authorizer/save").HandlerFunc(module_handlers.ProjectAuthorizerSaveHandler(sh))
	//storeRouter.Methods(http.MethodDelete).Path("/{project}/authorizer/remove/{authorizername}").HandlerFunc(module_handlers.ProjectAuthorizerRemoveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/remove").HandlerFunc(module_handlers.ProjectRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/project/list").HandlerFunc(module_handlers.ProjectListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/config").HandlerFunc(module_handlers.ProjectConfigHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/template/execute").HandlerFunc(module_handlers.ExecuteTemplateHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/myquery/list").HandlerFunc(module_handlers.ProjectMyQueryListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/func/list").HandlerFunc(module_handlers.ProjectFunctionListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/func/list").HandlerFunc(module_handlers.ProjectFunctionListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/route/list").HandlerFunc(module_handlers.ProjectRouteListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/agent/list").HandlerFunc(module_handlers.ProjectAgentListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/agent/list").HandlerFunc(module_handlers.ProjectAgentListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/tool/list").HandlerFunc(module_handlers.ProjectToolListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/tool/list").HandlerFunc(module_handlers.ProjectToolListNamesHandler(sh))
	// Adding routing handler to track all incoming requests
	serverRouter.PathPrefix("/{project}/route/{routename}").HandlerFunc(module_handlers.RouteHandler(sh))
	//serverRouter.PathPrefix("/public/{project}/route/{routename}").HandlerFunc(module_handlers.RouteHandler(sh))

	serverRouter.PathPrefix("/{project}/func/event/{eventname}/{eventid}").HandlerFunc(module_handlers.AsyncFuncHandler(sh))
	serverRouter.PathPrefix("/{project}/func/event/{eventname}").HandlerFunc(module_handlers.AsyncFuncHandler(sh))
	serverRouter.PathPrefix("/{project}/sfunc/{funcname}/{funcstepname}/{endfuncstepname}").HandlerFunc(module_handlers.SFuncHandler(sh))
	serverRouter.PathPrefix("/{project}/sfunc/{funcname}/{funcstepname}").HandlerFunc(module_handlers.SFuncHandler(sh))
	serverRouter.PathPrefix("/{project}/sfunc/{funcname}").HandlerFunc(module_handlers.SFuncHandler(sh))
	serverRouter.PathPrefix("/{project}/{tenant}/schedule/func/{funcname}").HandlerFunc(module_handlers.FuncScheduleHandler(sh))
	serverRouter.PathPrefix("/{project}/schedule/func/{funcname}").HandlerFunc(module_handlers.FuncScheduleHandler(sh))
	serverRouter.PathPrefix("/{project}/{tenant}/unschedule/func/{jobid}").HandlerFunc(module_handlers.FuncUnScheduleHandler(sh))
	serverRouter.PathPrefix("/{project}/unschedule/func/{jobid}").HandlerFunc(module_handlers.FuncUnScheduleHandler(sh))
	serverRouter.PathPrefix("/{project}/{tenant}/func/{funcname}/{eventname}").HandlerFunc(module_handlers.FuncHandler(sh))
	serverRouter.PathPrefix("/{project}/{tenant}/func/{funcname}").HandlerFunc(module_handlers.FuncHandler(sh))
	serverRouter.PathPrefix("/{project}/func/{funcname}/{eventname}").HandlerFunc(module_handlers.FuncHandler(sh))
	serverRouter.PathPrefix("/{project}/func/{funcname}").HandlerFunc(module_handlers.FuncHandler(sh))
	serverRouter.PathPrefix("/x/{project}/func/{funcname}").HandlerFunc(module_handlers.FuncHandler(sh))
	serverRouter.PathPrefix("/{project}/script/{domain}").HandlerFunc(module_handlers.ScriptHandler(sh))

	//serverRouter.PathPrefix("/public/{project}/func/{funcname}").HandlerFunc(module_handlers.FuncHandler(sh))

	serverRouter.PathPrefix("/{project}/wf/{wfname}").HandlerFunc(module_handlers.WfHandler(sh))

	serverRouter.PathPrefix("/{project}/notify").HandlerFunc(module_handlers.NotifyHandler(sh))
	serverRouter.PathPrefix("/asynctest").HandlerFunc(module_handlers.RouteAsyncTestHandler(sh))
	serverRouter.PathPrefix("/").HandlerFunc(module_handlers.RouteForwardHandler(sh))

}
