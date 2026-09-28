package server

import (
	"net/http"

	handlers "github.com/eru-os/eru/eru-server/server/handlers"
	store "github.com/eru-os/eru/eru-store/store"
	"github.com/gorilla/mux"
)

func (s *Server) GetRouter() *mux.Router {
	router := mux.NewRouter()
	router.Methods(http.MethodGet).Path("/hello").HandlerFunc(handlers.HelloHandler)
	router.Methods(http.MethodGet).Path("/state").HandlerFunc(handlers.StateHandler(s.Store))
	router.Methods(http.MethodGet).Path("/config/status").HandlerFunc(handlers.ConfigStatusHandler(s.Store))
	router.Methods(http.MethodGet).Path("/env/{env}").HandlerFunc(handlers.EnvHandler(s.Store))

	router.Name("variables_list").Methods(http.MethodGet).Path("/store/{project}/variables/list").HandlerFunc(handlers.FetchVarsHandler(s.Store))
	router.Name("variables_savevar").Methods(http.MethodPost).Path("/store/{project}/variables/savevar").HandlerFunc(handlers.SaveVarHandler(s.Store))
	router.Name("variables_removevar").Methods(http.MethodDelete).Path("/store/{project}/variables/removevar/{key}").HandlerFunc(handlers.RemoveVarHandler(s.Store))
	router.Name("variables_saveenvvar").Methods(http.MethodPost).Path("/store/{project}/variables/saveenvvar").HandlerFunc(handlers.SaveEnvVarHandler(s.Store))
	router.Name("variables_removeenvvar").Methods(http.MethodDelete).Path("/store/{project}/variables/removeenvvar/{key}").HandlerFunc(handlers.RemoveEnvVarHandler(s.Store))
	router.Name("variables_savesecret").Methods(http.MethodPost).Path("/store/{project}/variables/savesecret").HandlerFunc(handlers.SaveSecretHandler(s.Store))
	router.Name("variables_removesecret").Methods(http.MethodDelete).Path("/store/{project}/variables/removesecret/{key}").HandlerFunc(handlers.RemoveSecretHandler(s.Store))
	router.Methods(http.MethodGet).Path("/store/gvariables/list").HandlerFunc(handlers.FetchVarsHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/gvariables/savevar").HandlerFunc(handlers.SaveVarHandler(s.Store))
	router.Methods(http.MethodDelete).Path("/store/gvariables/removevar/{key}").HandlerFunc(handlers.RemoveVarHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/gvariables/saveenvvar").HandlerFunc(handlers.SaveEnvVarHandler(s.Store))
	router.Methods(http.MethodDelete).Path("/store/gvariables/removeenvvar/{key}").HandlerFunc(handlers.RemoveEnvVarHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/gvariables/savesecret").HandlerFunc(handlers.SaveSecretHandler(s.Store))
	router.Methods(http.MethodDelete).Path("/store/gvariables/removesecret/{key}").HandlerFunc(handlers.RemoveSecretHandler(s.Store))

	router.Name("repo_list").Methods(http.MethodGet).Path("/store/{project}/repo/list").HandlerFunc(handlers.FetchRepoHandler(s.Store))
	router.Name("repo_save").Methods(http.MethodPost).Path("/store/{project}/repo/save/{repotype}").HandlerFunc(handlers.SaveRepoHandler(s.Store))
	router.Name("repo_commit").Methods(http.MethodPost).Path("/store/{project}/repo/commit").HandlerFunc(handlers.CommitRepoHandler(s.Store))
	router.Name("repo_save_token").Methods(http.MethodPost).Path("/store/{project}/repo/savetoken").HandlerFunc(handlers.SaveRepoTokenHandler(s.Store))
	router.Methods(http.MethodGet).Path("/store/grepo/list").HandlerFunc(handlers.FetchRepoHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/grepo/save/{repotype}").HandlerFunc(handlers.SaveRepoHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/grepo/commit").HandlerFunc(handlers.CommitRepoHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/grepo/savetoken").HandlerFunc(handlers.SaveRepoTokenHandler(s.Store))

	router.Name("sch").Methods(http.MethodPost).Path("/store/{project}/scheduler/save/{schedulertype}").HandlerFunc(handlers.SaveSchedulerHandler(s.Store))
	router.Name("sch_list").Methods(http.MethodGet).Path("/store/{project}/scheduler/list").HandlerFunc(handlers.FetchSchedulerHandler(s.Store))

	router.Name("sm").Methods(http.MethodPost).Path("/store/{project}/sm/save/{smtype}").HandlerFunc(handlers.SaveSmHandler(s.Store))
	router.Name("sm_list").Methods(http.MethodGet).Path("/store/{project}/sm/list").HandlerFunc(handlers.FetchSmHandler(s.Store))
	router.Name("sm_value").Methods(http.MethodGet).Path("/store/{project}/sm/load").HandlerFunc(handlers.LoadSmValueHandler(s.Store))

	router.Name("sm_set").Methods(http.MethodPost).Path("/store/{project}/sm/set").HandlerFunc(handlers.SetSmValueHandler(s.Store))
	router.Name("sm_unset").Methods(http.MethodPost).Path("/store/{project}/sm/unset").HandlerFunc(handlers.UnsetSmValueHandler(s.Store))
	router.Name("sm_get").Methods(http.MethodPost).Path("/store/{project}/sm/get").HandlerFunc(handlers.GetSmValueHandler(s.Store))
	router.Name("tsm_set").Methods(http.MethodPost).Path("/store/{project}/{tenant}/sm/set").HandlerFunc(handlers.SetSmValueHandler(s.Store))
	router.Name("tsm_unset").Methods(http.MethodPost).Path("/store/{project}/{tenant}/sm/unset").HandlerFunc(handlers.UnsetSmValueHandler(s.Store))
	router.Name("tsm_get").Methods(http.MethodPost).Path("/store/{project}/{tenant}/sm/get").HandlerFunc(handlers.GetSmValueHandler(s.Store))

	router.Name("kms_list").Methods(http.MethodGet).Path("/store/{project}/kms/list").HandlerFunc(handlers.FetchKmsHandler(s.Store))
	router.Name("kms_save").Methods(http.MethodPost).Path("/store/{project}/kms/save/{kmstype}").HandlerFunc(handlers.SaveKmsHandler(s.Store))
	router.Name("kms_remove").Methods(http.MethodDelete).Path("/store/{project}/kms/remove/{kmsname}").HandlerFunc(handlers.RemoveKmsHandler(s.Store))
	router.Name("kms_remove_cd").Methods(http.MethodDelete).Path("/store/{project}/kms/remove/{kmsname}/{clouddelete}").HandlerFunc(handlers.RemoveKmsHandler(s.Store))
	router.Name("kms_remove_dd").Methods(http.MethodDelete).Path("/store/{project}/kms/remove/{kmsname}/{clouddelete}/{deletedays}").HandlerFunc(handlers.RemoveKmsHandler(s.Store))

	router.Name("event_list").Methods(http.MethodGet).Path("/store/{project}/event/list").HandlerFunc(handlers.FetchEventsHandler(s.Store))
	router.Name("event_save").Methods(http.MethodPost).Path("/store/{project}/event/save/{eventtype}").HandlerFunc(handlers.SaveEventHandler(s.Store))
	router.Name("event_remove").Methods(http.MethodDelete).Path("/store/{project}/event/remove/{eventname}").HandlerFunc(handlers.RemoveEventHandler(s.Store))
	router.Name("event_remove_cd").Methods(http.MethodDelete).Path("/store/{project}/event/remove/{eventname}/{clouddelete}").HandlerFunc(handlers.RemoveEventHandler(s.Store))
	router.Name("event_pub").Methods(http.MethodPost).Path("/store/{project}/event/publish/{eventname}").HandlerFunc(handlers.PublishEventHandler(s.Store))
	router.Name("event_poll").Methods(http.MethodPost).Path("/store/{project}/event/poll/{eventname}").HandlerFunc(handlers.PollEventHandler(s.Store))
	router.Name("event_subscribe").Methods(http.MethodPost).Path("/store/{project}/event/subscribe/{eventname}").HandlerFunc(handlers.SubscribeEventHandler(s.Store))
	router.Name("event_unsubscribe").Methods(http.MethodDelete).Path("/store/{project}/event/unsubscribe/{eventname}/{subscriptionid}").HandlerFunc(handlers.UnsubscribeEventHandler(s.Store))
	router.Name("event_list_subscriptions").Methods(http.MethodGet).Path("/store/{project}/event/list_subscriptions/{eventname}").HandlerFunc(handlers.ListSubscriptionsEventHandler(s.Store))

	router.Name("sr_list").Methods(http.MethodGet).Path("/store/{project}/{resource}/request/list").HandlerFunc(handlers.RequestListHandler(s.Store))
	router.Name("sr_save").Methods(http.MethodPost).Path("/store/{project}/request/save").HandlerFunc(handlers.RequestSaveHandler(s.Store))
	router.Name("sr_remove").Methods(http.MethodDelete).Path("/store/{project}/request/remove/{requestid}").HandlerFunc(handlers.RequestRemoveHandler(s.Store))
	router.Name("tsr_list").Methods(http.MethodGet).Path("/store/{project}/{tenant}/{resource}/request/list").HandlerFunc(handlers.RequestListHandler(s.Store))
	router.Name("tsr_save").Methods(http.MethodPost).Path("/store/{project}/{tenant}/request/save").HandlerFunc(handlers.RequestSaveHandler(s.Store))
	router.Name("tsr_remove").Methods(http.MethodDelete).Path("/store/{project}/{tenant}/request/remove/{requestid}").HandlerFunc(handlers.RequestRemoveHandler(s.Store))

	router.Methods(http.MethodPost).Path("/store/gsm/save/{smtype}").HandlerFunc(handlers.SaveSmHandler(s.Store))
	router.Methods(http.MethodGet).Path("/store/gsm/list").HandlerFunc(handlers.FetchSmHandler(s.Store))
	router.Methods(http.MethodGet).Path("/store/gsm/load").HandlerFunc(handlers.LoadSmValueHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/gsm/sm_set").HandlerFunc(handlers.SetSmValueHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/gsm/sm_unset").HandlerFunc(handlers.UnsetSmValueHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/gsm/sm_get").HandlerFunc(handlers.GetSmValueHandler(s.Store))

	router.Methods(http.MethodGet).Path("/store/{project}/gkms/list").HandlerFunc(handlers.FetchKmsHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/{project}/gkms/save/{kmstype}").HandlerFunc(handlers.SaveKmsHandler(s.Store))
	router.Methods(http.MethodDelete).Path("/store/{project}/gkms/remove/{kmsname}").HandlerFunc(handlers.RemoveKmsHandler(s.Store))
	router.Methods(http.MethodDelete).Path("/store/{project}/gkms/remove/{kmsname}/{clouddelete}").HandlerFunc(handlers.RemoveKmsHandler(s.Store))
	router.Methods(http.MethodDelete).Path("/store/{project}/gkms/remove/{kmsname}/{clouddelete}/{deletedays}").HandlerFunc(handlers.RemoveKmsHandler(s.Store))

	router.Methods(http.MethodGet).Path("/store/{project}/gevent/list").HandlerFunc(handlers.FetchEventsHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/{project}/gevent/save/{eventtype}").HandlerFunc(handlers.SaveEventHandler(s.Store))
	router.Methods(http.MethodDelete).Path("/store/{project}/gevent/remove/{eventname}").HandlerFunc(handlers.RemoveEventHandler(s.Store))
	router.Methods(http.MethodDelete).Path("/store/{project}/gevent/remove/{eventname}/{clouddelete}").HandlerFunc(handlers.RemoveEventHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/{project}/gevent/publish/{eventname}").HandlerFunc(handlers.PublishEventHandler(s.Store))
	router.Methods(http.MethodPost).Path("/store/{project}/gevent/poll/{eventname}").HandlerFunc(handlers.PollEventHandler(s.Store))
	//router.Methods(http.MethodPost).Path("/store/project/save/{project}").HandlerFunc(handlers.ProjectSaveHandler(s.Store))
	//router.Methods(http.MethodDelete).Path("/store/project/remove/{project}").HandlerFunc(handlers.ProjectRemoveHandler(s.Store))
	//router.Methods(http.MethodGet).Path("/store/project/list").HandlerFunc(handlers.ProjectListHandler(s.Store))
	//router.Methods(http.MethodGet).Path("/store/project/config/{project}").HandlerFunc(handlers.ProjectConfigHandler(s.Store))

	//router.Methods(http.MethodPost).Path("/file/upload").HandlerFunc(handlers.FileUploadHandler(s.Store))
	for _, addRoutes := range routeHooks {
		addRoutes(router, s.Store)
	}
	return router
}

var routeHooks []func(router *mux.Router, s store.StoreI)

func RegisterRoutes(addRoutes func(router *mux.Router, s store.StoreI)) {
	routeHooks = append(routeHooks, addRoutes)
}
