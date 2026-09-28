package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	agents "github.com/eru-os/eru/eru-ai/agents"
	agents_factory "github.com/eru-os/eru/eru-ai/agents/agents_factory"
	models "github.com/eru-os/eru/eru-ai/models"
	"github.com/eru-os/eru/eru-ai/module_model"
	"github.com/eru-os/eru/eru-ai/module_store"
	"github.com/eru-os/eru/eru-ai/tools"
	tools_factory "github.com/eru-os/eru/eru-ai/tools/tools_factory"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	utils "github.com/eru-os/eru/eru-utils"
	vectorstore "github.com/eru-os/eru/eru-vectorstore/vectorstore"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const StoreTableName = "eruai_config"
const StoreTenantTableName = "eruai_tenant_config"

func StoreLoadHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("StoreLoadHandler - Start")

		// Load new store from DB
		newStore, err := module_store.LoadStore(r.Context(), StoreTableName, StoreTenantTableName)
		if err != nil {
			logs.WithContext(r.Context()).Error(fmt.Sprintf("Failed to load store: %v", err))
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		// Update the global StoreHolder with the new store
		sh.Store = newStore

		logs.WithContext(r.Context()).Info("Store loaded and replaced successfully")
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(newStore)
	}
}

func StoreCompareHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("StoreCompareHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]

		comparePrjFromReq := json.NewDecoder(r.Body)
		comparePrjFromReq.DisallowUnknownFields()
		var compareProject module_model.ExtendedProject

		if err := comparePrjFromReq.Decode(&compareProject); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		myPrj, err := sh.Store.GetExtendedProjectConfig(r.Context(), projectID, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		storeCompare, err := myPrj.CompareProject(r.Context(), compareProject)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(storeCompare)
	}
}

func ProjectSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ProjectSaveHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		err := sh.Store.SaveProject(r.Context(), projectID, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {

			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project ", projectID, " created successfully")})
		}
	}
}

func ProjectRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ProjectRemoveHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		err := sh.Store.RemoveProject(r.Context(), projectID, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project ", projectID, " removed successfully")})
		}
	}
}

func ProjectListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ProjectListHandler - Start")
		projectIds := sh.Store.GetProjectList(r.Context())
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"projects": projectIds})
	}
}

func ProjectConfigHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ProjectConfigHandler - Start")
		vars := mux.Vars(r)
		projectID := vars["project"]
		project, err := sh.Store.GetExtendedProjectConfig(r.Context(), projectID, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"project": project})
		}
	}
}

func ModelSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ModelSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		provider := ""
		modelFromReq := json.NewDecoder(r.Body)
		modelFromReq.DisallowUnknownFields()

		var modelObjTmp map[string]interface{}
		if err := modelFromReq.Decode(&modelObjTmp); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			if providerI, ok := modelObjTmp["provider"]; !ok {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": "missing field in object : llm_name"})
				return
			} else {
				provider = providerI.(string)
			}
		}
		logs.WithContext(r.Context()).Info(fmt.Sprint(provider))
		modelObj := models.GetModel(provider)
		logs.WithContext(r.Context()).Info(fmt.Sprint(modelObj))
		modelJson, err := json.Marshal(modelObjTmp)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if err = json.Unmarshal(modelJson, &modelObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), modelObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}

		err = sh.Store.SaveModel(r.Context(), modelObj, projectId, tenantId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			modelName, anErr := modelObj.GetAttribute(r.Context(), "model_name")
			if anErr != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": anErr.Error()})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("model ", modelName, " saved successfully")})
		}
	}
}

func ModelRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ModelRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		modelName := vars["modelname"]
		err := sh.Store.RemoveModel(r.Context(), modelName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("model ", modelName, " removed successfully")})
		}
	}
}

func AgentSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("AgentSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		agentType := ""
		agentFromReq := json.NewDecoder(r.Body)
		agentFromReq.DisallowUnknownFields()

		var agentObjTmp map[string]interface{}
		if err := agentFromReq.Decode(&agentObjTmp); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			if agentTypeI, ok := agentObjTmp["agent_type"]; !ok {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": "missing field in object : agent_type"})
				return
			} else {
				agentType = agentTypeI.(string)
			}
		}
		agentObj := agents_factory.GetAgent(agentType)
		agentJson, err := json.Marshal(agentObjTmp)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		// MakeFromJson, not a bare Unmarshal: the load path builds agents through
		// it, and a specialised type wires its built-in prompt, schema and
		// validators there. Unmarshalled directly, a freshly saved agent ran
		// without them until the store was next reloaded.
		agentRaw := json.RawMessage(agentJson)
		if err = agentObj.MakeFromJson(r.Context(), &agentRaw); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), agentObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}

		err = sh.Store.SaveAgent(r.Context(), agentObj, projectId, tenantId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			agentName, anErr := agentObj.GetAttribute(r.Context(), "agent_name")
			if anErr != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": anErr.Error()})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("agent ", agentName, " saved successfully")})
		}
	}
}

func AgentRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("AgentRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		agentName := vars["agentname"]
		err := sh.Store.RemoveAgent(r.Context(), agentName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			sh.Store.SaveStore(r.Context(), projectId, "", sh.Store)
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("agent ", agentName, " removed successfully")})
		}
	}
}
func VectorSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("VectorSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		vectorStoreName := vars["vectorstorename"]

		vectorFromReq := json.NewDecoder(r.Body)
		vectorFromReq.DisallowUnknownFields()

		var vectorRecords vectorstore.VectorRecords
		if err := vectorFromReq.Decode(&vectorRecords); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), vectorRecords, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		err := sh.Store.SaveVectors(r.Context(), vectorRecords, vectorStoreName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("vectors saved successfully for vectorstore ", vectorStoreName)})
	}
}
func VectorRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("VectorRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		vectorStoreName := vars["vectorstorename"]

		vectorFromReq := json.NewDecoder(r.Body)
		vectorFromReq.DisallowUnknownFields()

		var vectorRecordsDelete vectorstore.VectorRecordsDelete
		if err := vectorFromReq.Decode(&vectorRecordsDelete); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), vectorRecordsDelete, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		err := sh.Store.RemoveVectors(r.Context(), vectorRecordsDelete, vectorStoreName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("vectors deleted successfully for vectorstore ", vectorStoreName)})
	}
}
func VectorListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("VectorListHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		vectorStoreName := vars["vectorstorename"]

		vectorFromReq := json.NewDecoder(r.Body)
		vectorFromReq.DisallowUnknownFields()

		var vectorRecordsList vectorstore.VectorRecordsList
		if err := vectorFromReq.Decode(&vectorRecordsList); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), vectorRecordsList, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		records, err := sh.Store.ListVectors(r.Context(), vectorRecordsList, vectorStoreName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(records)
	}
}
func VectorSearchHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("VectorSearchHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		vectorStoreName := vars["vectorstorename"]

		vectorFromReq := json.NewDecoder(r.Body)
		vectorFromReq.DisallowUnknownFields()

		var vectorRecordsSearch vectorstore.VectorRecordsSearch
		if err := vectorFromReq.Decode(&vectorRecordsSearch); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), vectorRecordsSearch, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}
		records, err := sh.Store.SearchVectors(r.Context(), vectorRecordsSearch, vectorStoreName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(records)
	}
}
func VectorStoreSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("VectorStoreSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		storeType := ""
		vectorStoreFromReq := json.NewDecoder(r.Body)
		vectorStoreFromReq.DisallowUnknownFields()

		var vectorStoreObjTmp map[string]interface{}
		if err := vectorStoreFromReq.Decode(&vectorStoreObjTmp); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			if storeTypeI, ok := vectorStoreObjTmp["vector_type"]; !ok {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": "missing field in object : store_type"})
				return
			} else {
				storeType = storeTypeI.(string)
			}
		}
		vectorStoreObj := vectorstore.GetVectorStore(storeType)
		vectorStoreJson, err := json.Marshal(vectorStoreObjTmp)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if err = json.Unmarshal(vectorStoreJson, &vectorStoreObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), vectorStoreObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}

		err = sh.Store.SaveVectorStore(r.Context(), vectorStoreObj, projectId, tenantId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			vectorStoreName := vectorStoreObj.GetAttribute(r.Context(), "vector_name")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("vectorstore ", vectorStoreName, " saved successfully")})
		}
	}
}
func VectorStoreSyncHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("VectorStoreSyncHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		vectorStoreName := vars["vectorstorename"]
		err := sh.Store.SyncVectorStore(r.Context(), vectorStoreName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("vectorstore ", vectorStoreName, " synced successfully")})
		}
	}
}
func VectorStoreRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("VectorStoreRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		vectorStoreName := vars["vectorstorename"]
		err := sh.Store.RemoveVectorStore(r.Context(), vectorStoreName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("vectorstore ", vectorStoreName, " removed successfully")})
		}
	}
}

func ToolSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ToolSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		toolType := ""
		toolFromReq := json.NewDecoder(r.Body)
		toolFromReq.DisallowUnknownFields()

		var toolObjTmp map[string]interface{}
		if err := toolFromReq.Decode(&toolObjTmp); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			if toolTypeI, ok := toolObjTmp["tool_type"]; !ok {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": "missing field in object : tool_type"})
				return
			} else {
				toolType = toolTypeI.(string)
			}
		}
		logs.WithContext(r.Context()).Info(fmt.Sprint(toolType))
		toolObj := tools_factory.GetTool(toolType)
		logs.WithContext(r.Context()).Info(fmt.Sprint(toolObj))
		toolJson, err := json.Marshal(toolObjTmp)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		if err = json.Unmarshal(toolJson, &toolObj); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			err = utils.ValidateStruct(r.Context(), toolObj, "")
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}

		err = sh.Store.SaveTool(r.Context(), toolObj, projectId, tenantId, sh.Store, true)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			toolName, _ := toolObj.GetAttribute(r.Context(), "tool_name")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("tool ", toolName, " saved successfully")})
		}
	}
}

func ToolRemoveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ToolRemoveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		toolName := vars["toolname"]
		err := sh.Store.RemoveTool(r.Context(), toolName, projectId, tenantId, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("tool ", toolName, " removed successfully")})
		}
	}
}
func FetchConversationHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("FetchConversationHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		agentName := vars["agentname"]
		conversationId := vars["conversationid"]
		agent, err := sh.Store.GetAgent(r.Context(), projectId, tenantId, conversationId, agentName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		}

		r = r.WithContext(module_store.WithProjectContext(r.Context(), projectId, sh.Store))

		conversation, err := agent.LoadConversationHistory(r.Context(), conversationId, projectId, tenantId)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(conversation)
		}

	}
}
func ListConversationsHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("ListConversationsHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		agentName := vars["agentname"]
		agent, err := sh.Store.GetAgent(r.Context(), projectId, tenantId, "", agentName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		}

		r = r.WithContext(module_store.WithProjectContext(r.Context(), projectId, sh.Store))

		// The client asks for a window: how many, and how many to skip.
		limit, skip := conversationWindow(r)

		// One more than asked for, so "is there another page" is answered by the
		// query rather than guessed from a full page.
		conversations, err := agent.LoadConversationList(r.Context(), projectId, tenantId, limit+1, skip)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			hasMore := false
			if limit > 0 && len(conversations) > limit {
				hasMore = true
				conversations = conversations[:limit]
			}
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"conversations": conversations,
				"limit":         limit,
				"skip":          skip,
				"has_more":      hasMore,
			})
		}

	}
}

func AgentExecuteHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.WithContext(r.Context()).Debug("AgentExecuteHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		agentName := vars["agentname"]
		conversationId := vars["conversationid"]

		isStream := strings.HasSuffix(r.URL.Path, "/stream")
		isRaw := strings.EqualFold(r.URL.Query().Get("raw"), "true")

		chain := agents.ParseAgentChain(r.Header.Get(agents.HeaderAgentChain))
		if len(chain) >= agents.MaxAgentChainDepth {
			err := fmt.Errorf("agent delegation depth limit of %d reached (chain: %s) - refusing to call %s", agents.MaxAgentChainDepth, agents.FormatAgentChain(chain), agentName)
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		agent, err := sh.Store.GetAgent(r.Context(), projectId, tenantId, conversationId, agentName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}

		agentParamsFromReq := json.NewDecoder(r.Body)
		agentParamsFromReq.DisallowUnknownFields()

		var agentMessage agents.AgentMessage
		if err := agentParamsFromReq.Decode(&agentMessage); err != nil {
			utils.PrintRequestBody(r.Context(), r, "")
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		if conversationId == "" {
			conversationId = uuid.New().String()
		}
		if agentMessage.MessageId == "" {
			agentMessage.MessageId = uuid.New().String()
		}
		r = r.WithContext(module_store.WithProjectContext(r.Context(), projectId, sh.Store))
		r = r.WithContext(agents.WithAgentChain(r.Context(), chain))
		if isRaw {
			logs.WithContext(r.Context()).Info(fmt.Sprint("AgentExecuteHandler - raw output requested for agent ", agentName))
			r = r.WithContext(agents.WithRawOutput(r.Context(), true))
		}

		// A sub-agent: it holds no SSE connection of its own, so its live events are
		// relayed to the pod that does. The same headers keep travelling downward, so
		// this agent's own children report to that same original stream.
		inheritedTarget, hasInheritedTarget := parseStreamTarget(r)
		if hasInheritedTarget && !isStream {
			r = r.WithContext(agents.WithStreamTarget(r.Context(), inheritedTarget))
			forwarder := newStreamForwarder(inheritedTarget, projectId, tenantId, agentName, chain)
			defer forwarder.Close()
			r = r.WithContext(agents.WithStreamCallback(r.Context(), forwarder.Callback()))
		}

		if isStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			flusher, canFlush := w.(http.Flusher)
			writeSSE := func(event agents.StreamEvent) {
				data, err := json.Marshal(event)
				if err != nil {
					logs.WithContext(r.Context()).Error(fmt.Sprint("stream event ", event.Event, " could not be encoded, sending an error in its place : ", err.Error()))
					data, err = json.Marshal(agents.StreamEvent{
						Event:    agents.StreamEventError,
						Data:     fmt.Sprint("the ", event.Event, " payload could not be encoded for the stream : ", err.Error()),
						Agent:    event.Agent,
						Chain:    event.Chain,
						RunId:    event.RunId,
						ThreadId: event.ThreadId,
						Seq:      event.Seq,
					})
					if err != nil {
						logs.WithContext(r.Context()).Error(fmt.Sprint("stream error event could not be encoded either : ", err.Error()))
						return
					}
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", string(data)); err != nil {
					logs.WithContext(r.Context()).Error(fmt.Sprint("stream event ", event.Event, " could not be written to the client : ", err.Error()))
					return
				}
				if canFlush {
					flusher.Flush()
				}
			}

			// This request owns the client's connection. Sub-agents on this and other
			// pods post their events to activeStreams under streamId; the writer
			// goroutine below is the only thing that ever touches w while the agent runs.
			streamId := uuid.New().String()
			sink := activeStreams.open(streamId)
			writerDone := make(chan struct{})
			go func() {
				defer close(writerDone)
				sink.drain(writeSSE)
			}()

			// One run identity for this execution, stamped on every event that
			// leaves it - including the ones relayed up from sub-agents on other
			// pods - so a client can tell two concurrent runs apart and pick a
			// reconnected stream back up.
			run := agents.RunIdentity{RunId: uuid.New().String(), ThreadId: conversationId}

			var seq int64
			ctx := agents.WithStreamCallback(r.Context(), func(event agents.StreamEvent) {
				event = event.Attribute(agentName, chain)
				event = event.Identify(run)
				event.Seq = atomic.AddInt64(&seq, 1)
				sink.send(event)
			})
			ctx = agents.WithRunIdentity(ctx, run)
			writeSSE(agents.StreamEvent{
				Event:    agents.StreamEventRunStarted,
				Agent:    agentName,
				RunId:    run.RunId,
				ThreadId: run.ThreadId,
				Seq:      atomic.AddInt64(&seq, 1),
			})
			if callbackUrl := streamCallbackUrl(); callbackUrl != "" {
				ctx = agents.WithStreamTarget(ctx, agents.StreamTarget{StreamId: streamId, CallbackUrl: callbackUrl})
			} else if hasInheritedTarget {
				ctx = agents.WithStreamTarget(ctx, inheritedTarget)
			}

			agentResult, execErr := agent.Execute(ctx, agentMessage, conversationId, projectId, tenantId)
			if execErr == nil {
				// See delivery.go: the artifact a caller receives must be the one
				// the loop validated. Reported, never fatal - the answer already
				// exists, and refusing to deliver it would turn a reporting
				// problem into an outage.
				_ = agents.VerifyDelivery(ctx, agent.GetProvider(), agentResult)
			}

			// Stop accepting relayed events, let the writer flush what is queued, and
			// only then write the terminal event directly - by now this goroutine is the
			// sole owner of w, so the final payload can never be dropped or interleaved.
			activeStreams.close(streamId)
			<-writerDone

			if execErr != nil {
				writeSSE(agents.StreamEvent{Event: agents.StreamEventError, Data: execErr.Error(), Agent: agentName, RunId: run.RunId, ThreadId: run.ThreadId})
				return
			}
			agentResult.ConversationId = conversationId
			writeSSE(agents.StreamEvent{Event: agents.StreamEventDone, Data: agentResult, Agent: agentName, RunId: run.RunId, ThreadId: run.ThreadId})
		} else {
			agentResult, err := agent.Execute(r.Context(), agentMessage, conversationId, projectId, tenantId)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			} else {
				agentResult.ConversationId = conversationId
				logs.WithContext(r.Context()).Info(fmt.Sprintf("AgentResult: %v", agentResult))
				server_handlers.FormatResponse(w, 200)
				_ = json.NewEncoder(w).Encode(agentResult)
			}
		}
	}
}

func ToolCallbackHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ToolCallbackHandler - Start")
		ctx := context.WithValue(r.Context(), tools.EruFuncBaseUrlKey, module_store.Erufuncbaseurl)
		ctx = context.WithValue(ctx, tools.RequestAuthorizationKey, r.Header.Get("Authorization"))
		ctx = context.WithValue(ctx, tools.RequestHeadersKey, r.Header)
		ctx = context.WithValue(ctx, "erufilesbaseurl", module_store.Erufilesbaseurl)
		ctx = context.WithValue(ctx, "eruauthbaseurl", module_store.Eruauthbaseurl)
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		toolName := vars["toolname"]
		actionName := "callback"

		tool, err := sh.Store.GetTool(ctx, projectId, tenantId, toolName, actionName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			toolBodyFromReq := json.NewDecoder(r.Body)
			toolBodyFromReq.DisallowUnknownFields()
			toolBody := make(map[string]interface{})
			contentType := strings.Split(r.Header.Get("Content-type"), ";")[0]
			logs.WithContext(r.Context()).Info(fmt.Sprintf("contentType: %v", contentType))
			switch contentType {
			case "application/json":
				if err := toolBodyFromReq.Decode(&toolBody); err != nil {
					logs.WithContext(r.Context()).Error(err.Error())
					toolBody = make(map[string]interface{})
				}
			case "application/x-www-form-urlencoded":
				if err := r.ParseForm(); err != nil {
					logs.WithContext(r.Context()).Error(err.Error())
					toolBody = make(map[string]interface{})
				} else {
					for key, values := range r.PostForm {
						if len(values) > 0 {
							toolBody[key] = values[0]
						}
					}
				}
			}

			params := r.URL.Query()
			toolParams := make(map[string][]string)
			for key, values := range params {
				toolParams[key] = values
			}

			toolResult, _, err := tool.Callback(ctx, projectId, tenantId, actionName, toolBody, toolParams)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			} else {
				responseContentType := tool.GetToolCallback().ResponseContentType
				w.Header().Set("Content-Type", responseContentType)
				w.WriteHeader(http.StatusOK)
				if responseContentType == "application/json" {
					_ = json.NewEncoder(w).Encode(toolResult)
				} else {
					w.Write([]byte(toolResult.(string)))
				}
			}
		}
	}
}

func ToolCbUrlHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ToolCbUrlHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		toolName := vars["toolname"]
		actionName := "callback"
		tool, err := sh.Store.GetTool(r.Context(), projectId, tenantId, toolName, actionName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"url": tool.GetToolCbUrl(projectId, tenantId)})
	}
}

func ToolExecuteHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ToolExecuteHandler - Start")
		ctx := module_store.WithProjectContext(r.Context(), mux.Vars(r)["project"], sh.Store)

		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		toolName := vars["toolname"]
		actionName := vars["actionname"]
		logs.WithContext(r.Context()).Info(fmt.Sprintf("Tool Name: %v", toolName))
		logs.WithContext(r.Context()).Info(fmt.Sprintf("Action Name: %v", actionName))
		tool, err := sh.Store.GetTool(r.Context(), projectId, tenantId, toolName, actionName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			toolParamsFromReq := json.NewDecoder(r.Body)
			toolParamsFromReq.DisallowUnknownFields()

			type tParams struct {
				Params map[string]interface{} `json:"params"`
			}

			var toolParams tParams
			if err := toolParamsFromReq.Decode(&toolParams); err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			if toolParams.Params == nil {
				toolParams.Params = make(map[string]interface{})
			}
			queryParams := r.URL.Query()
			for key, values := range queryParams {
				if len(values) > 0 {
					toolParams.Params[key] = values[0]
				}
			}

			toolResult, persistStore, err := tool.Execute(ctx, projectId, tenantId, actionName, toolParams.Params)
			if err != nil {
				server_handlers.FormatResponse(w, 400)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			} else {
				if persistStore && !sh.Store.IsTenantTool(r.Context(), projectId, tenantId, toolName) {
					logs.WithContext(r.Context()).Info(fmt.Sprintf("skipping store persistence of tool %v - not owned by tenant %v", toolName, tenantId))
					persistStore = false
				}
				if persistStore {
					err = sh.Store.SaveTool(r.Context(), tool, projectId, tenantId, sh.Store, true)
					if err != nil {
						logs.WithContext(r.Context()).Error(err.Error())
						server_handlers.FormatResponse(w, 400)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
					}
				}
				server_handlers.FormatResponse(w, 200)
				_ = json.NewEncoder(w).Encode(toolResult)
			}
		}
	}
}

func ToolWhatsAppEndpointExecuteHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		logs.WithContext(r.Context()).Debug("ToolExecuteHandler - Start")
		ctx := module_store.WithProjectContext(r.Context(), mux.Vars(r)["project"], sh.Store)
		vars := mux.Vars(r)
		projectId := vars["project"]
		tenantId := vars["tenant"]
		toolName := vars["toolname"]
		actionName := vars["actionname"]
		endpoint := vars["endpoint"]
		tool, err := sh.Store.GetTool(r.Context(), projectId, tenantId, toolName, actionName, sh.Store)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			toolParamsFromReq := json.NewDecoder(r.Body)
			toolParamsFromReq.DisallowUnknownFields()

			type tParams struct {
				Params map[string]interface{} `json:"params"`
			}

			var toolParams tParams
			var toolParamsMap map[string]interface{}
			if err := toolParamsFromReq.Decode(&toolParamsMap); err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
				return
			}
			if toolParams.Params == nil {
				toolParams.Params = make(map[string]interface{})
			}
			toolParams.Params = toolParamsMap
			queryParams := r.URL.Query()
			for key, values := range queryParams {
				if len(values) > 0 {
					toolParams.Params[key] = values[0]
				}
			}
			toolParams.Params["endpoint"] = endpoint

			toolResult, persistStore, err := tool.Execute(ctx, projectId, tenantId, actionName, toolParams.Params)
			if err != nil {
				server_handlers.FormatResponse(w, 421)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			} else {
				if persistStore && !sh.Store.IsTenantTool(r.Context(), projectId, tenantId, toolName) {
					logs.WithContext(r.Context()).Info(fmt.Sprintf("skipping store persistence of tool %v - not owned by tenant %v", toolName, tenantId))
					persistStore = false
				}
				if persistStore {
					err = sh.Store.SaveTool(r.Context(), tool, projectId, tenantId, sh.Store, true)
					if err != nil {
						logs.WithContext(r.Context()).Error(err.Error())
						server_handlers.FormatResponse(w, 400)
						_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
					}
				}
				encryptedResponse, ok := toolResult["encrypted_response"]
				if ok {
					encryptedResponseStr, ok := encryptedResponse.(string)
					if ok {
						w.Header().Set("Cache-Control", "no-store")
						w.Header().Set("Content-Type", "plain/text")
						w.WriteHeader(200)
						w.Write([]byte(encryptedResponseStr))
						return
					}
				}
				server_handlers.FormatResponse(w, 421)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "decryption failed"})
			}
		}
	}
}

func ProjectSettingsSaveHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sh.Lock()
		defer sh.Unlock()

		logs.WithContext(r.Context()).Debug("ProjectSetingsSaveHandler - Start")
		vars := mux.Vars(r)
		projectId := vars["project"]

		prjConfigFromReq := json.NewDecoder(r.Body)
		prjConfigFromReq.DisallowUnknownFields()

		var projectSettings module_model.ProjectSettings

		if err := prjConfigFromReq.Decode(&projectSettings); err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		} else {
			logs.WithContext(r.Context()).Info(fmt.Sprint(projectSettings))
			err := utils.ValidateStruct(r.Context(), projectSettings, "")
			if err != nil {
				logs.WithContext(r.Context()).Error(err.Error())
				server_handlers.FormatResponse(w, 400)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprint("missing field in object : ", err.Error())})
				return
			}
		}

		err := sh.Store.SaveProjectSettings(r.Context(), projectId, projectSettings, sh.Store)
		if err != nil {
			logs.WithContext(r.Context()).Error(err.Error())
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		} else {
			server_handlers.FormatResponse(w, 200)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"msg": fmt.Sprint("project settings for ", projectId, " saved successfully")})
		}
	}
}

// defaultConversationPageSize is how many conversations a page holds when the
// client does not say. Small enough that the first screen arrives quickly, big
// enough to fill it.
const defaultConversationPageSize = 25

// conversationWindow reads the window the client asked for: how many rows, and
// how many to skip past. Nothing else - a page number would only be these two
// values with arithmetic in between, and the client already knows how many rows
// it is holding.
func conversationWindow(r *http.Request) (limit int, skip int) {
	limit = defaultConversationPageSize
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if raw := r.URL.Query().Get("skip"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			skip = parsed
		}
	}
	return
}
