package module_server

import (
	"fmt"
	"net/http"

	module_handlers "github.com/eru-os/eru/eru-ai/module_server/handlers"
	"github.com/eru-os/eru/eru-ai/module_store"
	"github.com/eru-os/eru/eru-server/server"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	"github.com/gorilla/mux"
)

func SetServiceName() {
	server_handlers.ServerName = "eru-ai"
}

func AddModuleRoutes(serverRouter *mux.Router, sh *module_store.StoreHolder) {

	// MCP server endpoints using eru-server infrastructure
	mcpServer := NewEruAIMCPServer(sh)

	// Streamable HTTP MCP endpoint (Claude Desktop compatible)
	mcpHttpHandler := server.CreateMCPHttpHandler(mcpServer)
	serverRouter.Methods(http.MethodPost).Path("/mcp").HandlerFunc(mcpHttpHandler)
	serverRouter.Methods(http.MethodGet).Path("/mcp").HandlerFunc(mcpHttpHandler)
	serverRouter.Methods(http.MethodDelete).Path("/mcp").HandlerFunc(mcpHttpHandler)

	// WebSocket MCP endpoint
	serverRouter.Methods(http.MethodGet).Path("/mcp/websocket").HandlerFunc(
		server.CreateMCPWebSocketHandler(mcpServer),
	)

	// MCP health endpoint
	serverRouter.Methods(http.MethodGet).Path("/mcp/health").HandlerFunc(
		server.CreateMCPHealthHandler(mcpServer),
	)

	// MCP stats endpoint
	serverRouter.Methods(http.MethodGet).Path("/mcp/stats").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectId := server.RequestProject(r)
		ctx, tenantId := server.RequestTenantContext(r)
		tools, err := mcpServer.ListTools(ctx, projectId, tenantId)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		info := mcpServer.GetServerInfo()
		w.Write([]byte(fmt.Sprintf(`{"tools_count":%d,"status":"ready","server_info":{"name":%q,"version":%q}}`, len(tools.Tools), info.Name, info.Version)))
	})

	// A2A protocol endpoints
	a2aServer := NewEruAIA2AServer(sh)
	serverRouter.Methods(http.MethodGet).Path("/.well-known/agent.json").HandlerFunc(a2aServer.CreateAgentCardHandler())
	serverRouter.Methods(http.MethodPost).Path("/a2a").HandlerFunc(a2aServer.CreateHttpHandler())

	storeRouter := serverRouter.PathPrefix("/store").Subrouter()
	storeRouter.Methods(http.MethodGet).Path("/load").HandlerFunc(module_handlers.StoreLoadHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/compare").HandlerFunc(module_handlers.StoreCompareHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/save").HandlerFunc(module_handlers.ProjectSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/remove").HandlerFunc(module_handlers.ProjectRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/project/list").HandlerFunc(module_handlers.ProjectListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/config").HandlerFunc(module_handlers.ProjectConfigHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/save/model").HandlerFunc(module_handlers.ModelSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenant}/remove/model/{modelname}").HandlerFunc(module_handlers.ModelRemoveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/save/agent").HandlerFunc(module_handlers.AgentSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenant}/remove/agent/{agentname}").HandlerFunc(module_handlers.AgentRemoveHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/agent/catalog").HandlerFunc(module_handlers.AgentCatalogHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/agent/rules/scorecard").HandlerFunc(module_handlers.RuleScorecardHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/validate/agent").HandlerFunc(module_handlers.AgentValidateHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/agent/builder/context").HandlerFunc(module_handlers.AgentBuilderContextHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/models").HandlerFunc(module_handlers.ModelListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/tools/overview").HandlerFunc(module_handlers.ToolOverviewHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/secret/names").HandlerFunc(module_handlers.SecretNamesHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/save/vectorstore").HandlerFunc(module_handlers.VectorStoreSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenant}/remove/vectorstore/{vectorstorename}").HandlerFunc(module_handlers.VectorStoreRemoveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/sync/vectorstore/{vectorstorename}").HandlerFunc(module_handlers.VectorStoreSyncHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/save/vector/{vectorstorename}").HandlerFunc(module_handlers.VectorSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenant}/remove/vector/{vectorstorename}").HandlerFunc(module_handlers.VectorRemoveHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/search/vector/{vectorstorename}").HandlerFunc(module_handlers.VectorSearchHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/list/vector/{vectorstorename}").HandlerFunc(module_handlers.VectorListHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/{tenant}/save/tool").HandlerFunc(module_handlers.ToolSaveHandler(sh))
	storeRouter.Methods(http.MethodDelete).Path("/{project}/{tenant}/remove/tool/{toolname}").HandlerFunc(module_handlers.ToolRemoveHandler(sh))

	storeRouter.Methods(http.MethodPost).Path("/{project}/settings/save").HandlerFunc(module_handlers.ProjectSettingsSaveHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/{tenant}/agent/list").HandlerFunc(module_handlers.AgentListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/agent/list").HandlerFunc(module_handlers.AgentListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/{tenant}/agents").HandlerFunc(module_handlers.AgentListHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/agents").HandlerFunc(module_handlers.AgentListHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/{tenant}/vectorstore/list").HandlerFunc(module_handlers.VectorStoreListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/vectorstore/list").HandlerFunc(module_handlers.VectorStoreListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/{tenant}/tool/list").HandlerFunc(module_handlers.ToolListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/tool/list").HandlerFunc(module_handlers.ToolListNamesHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/{tenant}/tools").HandlerFunc(module_handlers.ToolListHandler(sh))
	storeRouter.Methods(http.MethodGet).PathPrefix("/{project}/tools").HandlerFunc(module_handlers.ToolListHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/tool/catalog").HandlerFunc(module_handlers.ToolCatalogHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/{tenant}/tool/catalog").HandlerFunc(module_handlers.ToolCatalogHandler(sh))
	storeRouter.Methods(http.MethodGet).Path("/{project}/tool/catalog").HandlerFunc(module_handlers.ToolCatalogHandler(sh))
	storeRouter.Methods(http.MethodPost).Path("/{project}/settings/tool/access").HandlerFunc(module_handlers.ToolCatalogAccessSaveHandler(sh))
	// functions for ai events
	aiRouter := serverRouter.PathPrefix("/{project}").Subrouter()
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/tokencount").HandlerFunc(module_handlers.TokenCountHandler())
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/{model}/query").HandlerFunc(module_handlers.ModelQueryHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/{model}/embeddings").HandlerFunc(module_handlers.ModelEmbeddingsHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/{model}/{tool}/query").HandlerFunc(module_handlers.ModelQueryHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/{model}/{tool}/{action}/query").HandlerFunc(module_handlers.ModelQueryHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/login/{tenant}/execute/tool/{toolname}/{actionname}").HandlerFunc(module_handlers.ToolExecuteHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/whatsapp/{tenant}/execute/tool/{toolname}/{actionname}/{endpoint}").HandlerFunc(module_handlers.ToolWhatsAppEndpointExecuteHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/execute/tool/{toolname}/{actionname}").HandlerFunc(module_handlers.ToolExecuteHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/execute/tool/{toolname}").HandlerFunc(module_handlers.ToolExecuteHandler(sh))
	aiRouter.Methods(http.MethodGet).PathPrefix("/{tenant}/cburl/tool/{toolname}").HandlerFunc(module_handlers.ToolCbUrlHandler(sh))
	aiRouter.PathPrefix("/callback/{tenant}/tool/{toolname}").HandlerFunc(module_handlers.ToolCallbackHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/stream/{streamid}/event").HandlerFunc(module_handlers.AgentStreamEventHandler())
	aiRouter.Methods(http.MethodPost).Path("/{tenant}/client/{agentname}/{requestid}/reply").HandlerFunc(module_handlers.ClientReplyHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/execute/agent/{agentname}/{conversationid}/stream").HandlerFunc(module_handlers.AgentExecuteHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/execute/agent/{agentname}/stream").HandlerFunc(module_handlers.AgentExecuteHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/execute/agent/{agentname}/{conversationid}").HandlerFunc(module_handlers.AgentExecuteHandler(sh))
	aiRouter.Methods(http.MethodPost).PathPrefix("/{tenant}/execute/agent/{agentname}").HandlerFunc(module_handlers.AgentExecuteHandler(sh))
	aiRouter.Methods(http.MethodGet).PathPrefix("/{tenant}/list/conversations/{agentname}/{conversationid}").HandlerFunc(module_handlers.FetchConversationHandler(sh))
	aiRouter.Methods(http.MethodGet).PathPrefix("/{tenant}/list/conversations/{agentname}").HandlerFunc(module_handlers.ListConversationsHandler(sh))

}
