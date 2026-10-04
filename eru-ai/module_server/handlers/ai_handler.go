package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	model "github.com/eru-os/eru/eru-ai/models"
	"github.com/eru-os/eru/eru-ai/module_store"
	"github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

func ModelEmbeddingsHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ModelEmbeddingsHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		modelId := vars["model"]
		embeddingFromReq := json.NewDecoder(r.Body)
		embeddingFromReq.DisallowUnknownFields()

		var embeddingInputRequest model.EmbeddingInputRequest
		if err := embeddingFromReq.Decode(&embeddingInputRequest); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		err := utils.ValidateStruct(r.Context(), embeddingInputRequest, "")
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
			return
		}
		modelObj, err := sh.Store.GetModel(r.Context(), projectId, tenantId, modelId, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		embeddings, embeddingsErr := modelObj.GenerateEmbeddings(r.Context(), embeddingInputRequest.Inputs, embeddingInputRequest.ChunkConfig, embeddingInputRequest.Dimension)
		if embeddingsErr != nil {
			logs.WithContext(r.Context()).Error(embeddingsErr.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": embeddingsErr.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(embeddings)
	}
}

func TokenCountHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("TokenCountHandler - Start")
		q := r.URL.Query()
		providerParam := q.Get("provider")
		modelParam := q.Get("model")
		direction := q.Get("direction")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		result, err := model.EstimateTokens(r.Context(), body, providerParam, modelParam, direction)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func ModelQueryHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ModelSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		modelId := vars["model"]
		toolName := vars["tool"]
		actionName := vars["action"]
		modelFromReq := json.NewDecoder(r.Body)
		modelFromReq.DisallowUnknownFields()

		var chatMessage model.ChatRequest
		if err := modelFromReq.Decode(&chatMessage); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		modelObj, err := sh.Store.GetModel(r.Context(), projectId, tenantId, modelId, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if toolName == "" {
			res, err := modelObj.QueryModel(r.Context(), chatMessage)
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		} else {
			tool, tErr := sh.Store.GetTool(r.Context(), projectId, tenantId, toolName, actionName, sh.Store)
			if tErr != nil {
				logs.WithContext(r.Context()).Error(tErr.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": tErr.Error()})
				return
			}
			toolMap := make(map[string]tools.Tooling)
			toolMap[toolName] = tool
			res, err := modelObj.QueryModelWithTool(r.Context(), chatMessage, toolMap, "", "")
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(res)
		}
	}
}

func AgentListNamesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("AgentListNamesHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		tenantID := vars["tenant"]
		agents, err := sh.Store.GetAgentNames(r.Context(), projectID, tenantID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			logs.WithContext(r.Context()).Info(fmt.Sprintf("Agents: %v", agents))
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"agents": agents})
		}
	}
}

func VectorStoreListNamesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("VectorStoreListNamesHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		tenantID := vars["tenant"]
		vectorStores, err := sh.Store.GetVectorStoreNames(r.Context(), projectID, tenantID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			logs.WithContext(r.Context()).Info(fmt.Sprintf("VectorStores: %v", vectorStores))
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"vectorstores": vectorStores})
		}
	}
}

func ToolListNamesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ToolListNamesHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		tenantID := vars["tenant"]
		tools, err := sh.Store.GetToolNames(r.Context(), projectID, tenantID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			logs.WithContext(r.Context()).Info(fmt.Sprintf("Tools: %v", tools))
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"tools": tools})
		}
	}
}

func ToolListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ToolListHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		tenantID := vars["tenant"]
		names, err := sh.Store.GetToolNames(r.Context(), projectID, tenantID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		prj, err := sh.Store.GetProjectConfig(r.Context(), projectID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		// The stored config, not GetTool's clone: the clone has $SECRET_ placeholders
		// already replaced with secret values, and this list goes to the browser.
		lookup := []string{projectID}
		if tenantID != "" {
			lookup = utils.TenantLookupOrder(r.Context(), tenantID, projectID)
		}
		toolList := make([]interface{}, 0, len(names))
		for _, name := range names {
			var stored tools.Tooling
			for _, tid := range lookup {
				if tenant, ok := prj.Tenants[tid]; ok {
					if t, ok := tenant.Tools[name]; ok {
						stored = t
						break
					}
				}
			}
			if stored == nil {
				continue
			}
			raw, mErr := json.Marshal(stored.GetSpec())
			if mErr != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprintf("tool %s: %v", name, mErr))
				continue
			}
			var spec map[string]interface{}
			if uErr := json.Unmarshal(raw, &spec); uErr != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprintf("tool %s: %v", name, uErr))
				continue
			}
			toolList = append(toolList, spec)
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"tools": toolList})
	}
}

func AgentListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("AgentListHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		tenantID := vars["tenant"]
		includeSystem, _ := strconv.ParseBool(r.URL.Query().Get("system"))
		// raw=true returns each agent exactly as stored: variables unresolved,
		// tools and model not looked up. It is the listing an editor needs. The
		// resolved listing below drops any agent whose model or tool is missing -
		// the very agents someone needs to open and fix - and hands back values
		// with variables substituted, which an editor would then save as literals.
		if raw, _ := strconv.ParseBool(r.URL.Query().Get("raw")); raw {
			stored, err := StoredAgents(r.Context(), sh, projectID, tenantID)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			agentList := make([]interface{}, 0, len(stored))
			for _, agent := range stored {
				if includeSystem || !agent.GetIsSystem() {
					agentList = append(agentList, agent.GetSpec())
				}
			}
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"agents": agentList})
			return
		}
		names, err := sh.Store.GetAgentNames(r.Context(), projectID, tenantID)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		agentList := make([]interface{}, 0, len(names))
		for _, name := range names {
			agent, err := sh.Store.GetAgent(r.Context(), projectID, tenantID, "", name, sh.Store)
			if err != nil {
				logs.WithContext(r.Context()).Error(fmt.Sprintf("GetAgent %s: %v", name, err))
				continue
			}
			if !includeSystem && agent.GetIsSystem() {
				continue
			}
			agentList = append(agentList, agent.GetSpec())
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"agents": agentList})
	}
}
