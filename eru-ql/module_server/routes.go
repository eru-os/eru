package module_server

import (
	"net/http"

	module_handlers "github.com/eru-os/eru/eru-ql/module_server/handlers"
	"github.com/eru-os/eru/eru-ql/module_store"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func SetServiceName() {
	server_handlers.ServerName = "eru-ql"
	server_handlers.RepoName = "eruql.json"
}

func AddModuleRoutes(serverRouter *mux.Router, sh *module_store.StoreHolder) {

	//store functions specific to files
	serverRouter.Methods(http.MethodPost).Path("/graphql/{project}/{tenantId}/execute").HandlerFunc(module_handlers.GraphqlExecuteHandler(sh))
	serverRouter.Methods(http.MethodPost).Path("/graphql/{project}/execute").HandlerFunc(module_handlers.GraphqlExecuteHandler(sh))
	serverRouter.Path("/graphql/{project}/ws/execute").HandlerFunc(module_handlers.GraphqlWsExecuteHandler(sh))
	serverRouter.Methods(http.MethodPost).Path("/sql/{project}/{tenantId}/execute").HandlerFunc(module_handlers.SqlExecuteHandler(sh))
	serverRouter.Methods(http.MethodPost).Path("/sql/{project}/execute").HandlerFunc(module_handlers.SqlExecuteHandler(sh))

	storeRouter := serverRouter.PathPrefix("/store").Subrouter()
	storeRouter.Methods(http.MethodGet).Path("/load").HandlerFunc(module_handlers.StoreLoadHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/compare").HandlerFunc(module_handlers.StoreCompareHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/save").HandlerFunc(module_handlers.ProjectSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/remove").HandlerFunc(module_handlers.ProjectRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/project/list").HandlerFunc(module_handlers.ProjectListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/config").HandlerFunc(module_handlers.ProjectConfigHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/settings/save").HandlerFunc(module_handlers.ProjectSetingsSaveHandler(sh))
	//storeRouter.Methods(http.MethodGet).Path("/{project}/generateaeskey").HandlerFunc(module_handlers.ProjectGenerateAesKeyHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/myquery/save/{queryname}/{querytype}").HandlerFunc(module_handlers.ProjectMyQuerySaveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/myquery/save/{queryname}/{querytype}").HandlerFunc(module_handlers.ProjectMyQuerySaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenantId}/myquery/remove/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryRemoveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/myquery/remove/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/myquery/list/{querytype}").HandlerFunc(module_handlers.ProjectMyQueryListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/myquery/list/{querytype}").HandlerFunc(module_handlers.ProjectMyQueryListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/myquery/list").HandlerFunc(module_handlers.ProjectMyQueryListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/myquery/list").HandlerFunc(module_handlers.ProjectMyQueryListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/myquery/config/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryConfigHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/myquery/config/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryConfigHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/myquery/fetch/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryConfigHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/myquery/fetch/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryConfigHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/myquery/ast/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryASTHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/myquery/ast/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryASTHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/myquery/execute/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryExecuteHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/myquery/execute/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryExecuteHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/myquery/execute/{queryname}/{outputtype}").HandlerFunc(module_handlers.ProjectMyQueryExecuteHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/myquery/execute/{queryname}/{outputtype}").HandlerFunc(module_handlers.ProjectMyQueryExecuteHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/myquery/execute/{queryname}/{outputtype}/{encode}").HandlerFunc(module_handlers.ProjectMyQueryExecuteHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/myquery/execute/{queryname}/{outputtype}/{encode}").HandlerFunc(module_handlers.ProjectMyQueryExecuteHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/myquery/executegroup/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryExecuteGroupHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/myquery/executegroup/{queryname}").HandlerFunc(module_handlers.ProjectMyQueryExecuteGroupHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/myquery/executegroup/{queryname}/{outputtype}").HandlerFunc(module_handlers.ProjectMyQueryExecuteGroupHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/myquery/executegroup/{queryname}/{outputtype}").HandlerFunc(module_handlers.ProjectMyQueryExecuteGroupHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/myquery/executegroup/{queryname}/{outputtype}/{encode}").HandlerFunc(module_handlers.ProjectMyQueryExecuteGroupHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/myquery/executegroup/{queryname}/{outputtype}/{encode}").HandlerFunc(module_handlers.ProjectMyQueryExecuteGroupHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/defaultdriverconfig/{dbtype}").HandlerFunc(module_handlers.DefaultDriverConfigHandler())
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/defaultotherdbconfig/{dbtype}").HandlerFunc(module_handlers.DefaultOtherDBConfigHandler())
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/defaultdbsecurityrules/{dbtype}").HandlerFunc(module_handlers.DefaultDBSecurityRulesHandler())
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/defaultreadpolicy/{dbtype}").HandlerFunc(module_handlers.DefaultReadPolicyHandler())

	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/save/{dbalias}").HandlerFunc(module_handlers.ProjectDataSourceSaveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/save/{dbalias}").HandlerFunc(module_handlers.ProjectDataSourceSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenantId}/datasource/remove/{dbalias}").HandlerFunc(module_handlers.ProjectDataSourceRemoveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/datasource/remove/{dbalias}").HandlerFunc(module_handlers.ProjectDataSourceRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/datasource/list").HandlerFunc(module_handlers.ProjectDataSourceListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/list").HandlerFunc(module_handlers.ProjectDataSourceListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/datasource/config/{dbalias}").HandlerFunc(module_handlers.ProjectDataSourceConfigHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/config/{dbalias}").HandlerFunc(module_handlers.ProjectDataSourceConfigHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/schema/{dbalias}/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/datasource/schema/{dbalias}").HandlerFunc(module_handlers.ProjectDataSourceSchemaHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/schema/{dbalias}").HandlerFunc(module_handlers.ProjectDataSourceSchemaHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/datasource/tablecheck/{dbalias}/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceTableCheckHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/tablecheck/{dbalias}/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceTableCheckHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/addtable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaAddTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/addtable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaAddTableHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/removetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaRemoveTableHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/datasource/schema/{dbalias}/removetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaRemoveTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/addjoin").HandlerFunc(module_handlers.ProjectDataSourceSchemaAddJoinHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/addjoin").HandlerFunc(module_handlers.ProjectDataSourceSchemaAddJoinHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/removejoin").HandlerFunc(module_handlers.ProjectDataSourceSchemaRemoveJoinHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/datasource/schema/{dbalias}/removejoin").HandlerFunc(module_handlers.ProjectDataSourceSchemaRemoveJoinHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/savetable/{tablename}/{addInSchema}").HandlerFunc(module_handlers.ProjectDataSourceSchemaSaveTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/savetable/{tablename}/{addInSchema}").HandlerFunc(module_handlers.ProjectDataSourceSchemaSaveTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/savetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaSaveTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/savetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaSaveTableHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/droptable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaDropTableHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/datasource/schema/{dbalias}/droptable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaDropTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/securetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaSecureTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/securetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaSecureTableHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/securetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceGetSecureTableHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/schema/{dbalias}/securetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceGetSecureTableHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/securetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceRemoveSecureTableHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/datasource/schema/{dbalias}/securetable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceRemoveSecureTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/transformtable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaTransformTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/transformtable/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceSchemaTransformTableHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/maskcolumn/{tablename}/{colname}").HandlerFunc(module_handlers.ProjectDataSourceSchemaMasColumnHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/maskcolumn/{tablename}/{colname}").HandlerFunc(module_handlers.ProjectDataSourceSchemaMasColumnHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/derivedfield/save").HandlerFunc(module_handlers.ProjectDataSourceSchemaSaveDerivedFieldHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/derivedfield/save").HandlerFunc(module_handlers.ProjectDataSourceSchemaSaveDerivedFieldHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/derivedfield/validate").HandlerFunc(module_handlers.ProjectDataSourceSchemaValidateDerivedFieldHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/datasource/schema/{dbalias}/derivedfield/validate").HandlerFunc(module_handlers.ProjectDataSourceSchemaValidateDerivedFieldHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/derivedfield/{tablename}/{fieldname}").HandlerFunc(module_handlers.ProjectDataSourceSchemaRemoveDerivedFieldHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/datasource/schema/{dbalias}/derivedfield/{tablename}/{fieldname}").HandlerFunc(module_handlers.ProjectDataSourceSchemaRemoveDerivedFieldHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenantId}/datasource/schema/{dbalias}/derivedfield/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceGetDerivedFieldsHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/datasource/schema/{dbalias}/derivedfield/{tablename}").HandlerFunc(module_handlers.ProjectDataSourceGetDerivedFieldsHandler(sh))

	cacheRouter := serverRouter.PathPrefix("/cache").Subrouter()
	cacheRouter.Methods(http.MethodPost).Path("/{project}/{tenantId}/invalidate").HandlerFunc(module_handlers.CacheInvalidateHandler(sh))
	cacheRouter.Methods(http.MethodPost).Path("/{project}/invalidate").HandlerFunc(module_handlers.CacheInvalidateHandler(sh))
	cacheRouter.Methods(http.MethodGet).Path("/stats").HandlerFunc(module_handlers.CacheStatsHandler())
}
