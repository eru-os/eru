package module_store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	agents "github.com/eru-os/eru/eru-ai/agents"
	models "github.com/eru-os/eru/eru-ai/models"
	module_model "github.com/eru-os/eru/eru-ai/module_model"
	tools "github.com/eru-os/eru/eru-ai/tools"
	db "github.com/eru-os/eru/eru-db/db"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	scheduler "github.com/eru-os/eru/eru-scheduler/scheduler"
	"github.com/eru-os/eru/eru-store/store"
	eru_utils "github.com/eru-os/eru/eru-utils"
	vectorstore "github.com/eru-os/eru/eru-vectorstore/vectorstore"
)

var Erufuncbaseurl = "http://localhost:8083"
var Eruauthbaseurl = "http://localhost:8085"
var Eruqlbaseurl = "http://localhost:8087"
var Eruaibaseurl = "http://localhost:8088"
var Erufilesbaseurl = "http://localhost:8082"
var Eruaiport = "8088"

func tenantLookupOrder(ctx context.Context, projectId string, tenantId string) []string {
	return eru_utils.TenantLookupOrder(ctx, tenantId, projectId)
}

func tenantWriteOrder(projectId string, tenantId string) []string {
	if tenantId == "" || tenantId == projectId {
		return []string{projectId}
	}
	return []string{tenantId, projectId}
}

func tenantVisibilitySet(ctx context.Context, projectId string, tenantId string) map[string]bool {
	visible := make(map[string]bool)
	for _, tid := range tenantLookupOrder(ctx, projectId, tenantId) {
		visible[tid] = true
	}
	return visible
}

type StoreHolder struct {
	sync.RWMutex
	Store ModuleStoreI
}
type ModuleStoreI interface {
	store.StoreI
	SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error
	RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error
	GetProjectConfig(ctx context.Context, projectId string) (*module_model.Project, error)
	GetExtendedProjectConfig(ctx context.Context, projectId string, realStore ModuleStoreI) (module_model.ExtendedProject, error)
	GetProjectList(ctx context.Context) []map[string]interface{}
	SaveModel(ctx context.Context, modelObj models.ModelI, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error
	RemoveModel(ctx context.Context, modelName string, projectId string, tenantId string, realStore ModuleStoreI) error
	GetModel(ctx context.Context, projectId string, tenantId string, modelName string, s ModuleStoreI) (models.ModelI, error)
	SaveAgent(ctx context.Context, agentObj agents.AgentI, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error
	RemoveAgent(ctx context.Context, agentName string, projectId string, tenantId string, realStore ModuleStoreI) error
	GetAgent(ctx context.Context, projectId string, tenantId string, conversationId string, agentName string, s ModuleStoreI) (agents.AgentI, error)
	SaveVectorStore(ctx context.Context, vectorStoreObj vectorstore.VectorStoreI, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error
	RemoveVectorStore(ctx context.Context, vectorStoreName string, projectId string, tenantId string, realStore ModuleStoreI) error
	GetVectorStore(ctx context.Context, projectId string, tenantId string, vectorStoreName string, realStore ModuleStoreI) (vectorstore.VectorStoreI, error)
	GetVectorStoreCloneObject(ctx context.Context, projectId string, tenantId string, vectorStoreObj vectorstore.VectorStoreI, s ModuleStoreI) (vectorStoreObjClone vectorstore.VectorStoreI, err error)
	SyncVectorStore(ctx context.Context, vectorStoreName string, projectId string, tenantId string, realStore ModuleStoreI) error
	GetVectorStoreNames(ctx context.Context, projectID string, tenantID string) (vectorStoreNames []string, err error)
	SaveVectors(ctx context.Context, vectorRecords vectorstore.VectorRecords, vectorName string, projectId string, tenantId string, realStore ModuleStoreI) error
	RemoveVectors(ctx context.Context, vectorRecords vectorstore.VectorRecordsDelete, vectorName string, projectId string, tenantId string, realStore ModuleStoreI) error
	SearchVectors(ctx context.Context, vectorRecords vectorstore.VectorRecordsSearch, vectorName string, projectId string, tenantId string, realStore ModuleStoreI) (vectorstore.VectorResults, error)
	ListVectors(ctx context.Context, vectorRecords vectorstore.VectorRecordsList, vectorName string, projectId string, tenantId string, realStore ModuleStoreI) (vectorstore.VectorResults, error)
	SaveProjectSettings(ctx context.Context, projectId string, projectSettings module_model.ProjectSettings, realStore ModuleStoreI) error
	SaveToolCatalogAccess(ctx context.Context, projectId string, accessRequest module_model.ToolCatalogAccessRequest, realStore ModuleStoreI) error
	GetProjectSettings(ctx context.Context, projectId string) (module_model.ProjectSettings, error)
	RemoveTenants()
	SaveTool(ctx context.Context, tooling tools.Tooling, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error
	RemoveTool(ctx context.Context, toolName string, projectId string, tenantId string, realStore ModuleStoreI) error
	GetTool(ctx context.Context, projectId string, tenantId string, toolName string, actionName string, s ModuleStoreI) (tools.Tooling, error)
	IsTenantTool(ctx context.Context, projectId string, tenantId string, toolName string) bool
	GetAgentNames(ctx context.Context, projectID string, tenantID string) (agentNames []string, err error)
	GetToolNames(ctx context.Context, projectID string, tenantID string) (toolNames []string, err error)
	DiscoverAgents(ctx context.Context, projectId string, tenantId string, selfName string, allowedNames []string, s ModuleStoreI) []agents.DiscoveredAgent
	DiscoverTools(ctx context.Context, projectId string, tenantId string, allowed map[string][]string, s ModuleStoreI) []agents.DiscoveredTool
}

type ModuleStore struct {
	Projects map[string]*module_model.Project `json:"projects"` //ProjectId is the key
}

type ModuleFileStore struct {
	store.FileStore
	ModuleStore
}
type ModuleDbStore struct {
	store.DbStore
	ModuleStore
}

func (ms *ModuleStore) SaveProject(ctx context.Context, projectId string, realStore ModuleStoreI, persist bool) error {
	//TODO to handle edit project once new project attributes are finalized
	logs.WithContext(ctx).Debug("SaveProject - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}
	if _, ok := ms.Projects[projectId]; !ok {
		project := new(module_model.Project)
		project.ProjectId = projectId
		if ms.Projects == nil {
			ms.Projects = make(map[string]*module_model.Project)
		}
		if project.Tenants == nil {
			project.Tenants = make(map[string]module_model.TenantConfig)
		}

		ms.Projects[projectId] = project
		if persist {
			logs.WithContext(ctx).Info("SaveStore called from SaveProject")
			return realStore.SaveStore(ctx, projectId, "", realStore)
		} else {
			return nil
		}
	} else {
		err := errors.New("Project " + projectId + " already exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}

func (ms *ModuleStore) RemoveProject(ctx context.Context, projectId string, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("RemoveProject - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	if _, ok := ms.Projects[projectId]; ok {
		delete(ms.Projects, projectId)
		logs.WithContext(ctx).Info("SaveStore called from RemoveProject")
		return realStore.SaveStore(ctx, projectId, "", realStore)
	} else {
		err := errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetExtendedProjectConfig(ctx context.Context, projectId string, realStore ModuleStoreI) (ePrj module_model.ExtendedProject, err error) {
	logs.WithContext(ctx).Debug("GetExtendedProjectConfig - Start")
	ePrj = module_model.ExtendedProject{}
	if prj, ok := ms.Projects[projectId]; ok {
		ePrj.TenantVariables, err = realStore.FetchTenantVars(ctx, projectId)
		if err != nil {
			logs.WithContext(ctx).Warn(err.Error())
		}
		ePrj.Variables, err = realStore.FetchVars(ctx, projectId)
		if err != nil {
			logs.WithContext(ctx).Warn(err.Error())
		}
		ePrj.SecretManager, err = realStore.FetchSm(ctx, projectId)
		if err != nil {
			logs.WithContext(ctx).Warn(err.Error())
		}
		ePrj.Scheduler, err = realStore.FetchScheduler(ctx, projectId)
		if err != nil {
			logs.WithContext(ctx).Warn(err.Error())
		}
		ePrj.ProjectId = prj.ProjectId
		ePrj.ProjectSettings = prj.ProjectSettings
		ePrj.Tenants = prj.Tenants
		return ePrj, nil
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		return module_model.ExtendedProject{}, err
	}
}

func (ms *ModuleStore) GetProjectConfig(ctx context.Context, projectId string) (*module_model.Project, error) {
	logs.WithContext(ctx).Debug("GetProjectConfig - Start")
	if _, ok := ms.Projects[projectId]; ok {
		return ms.Projects[projectId], nil
	} else {
		err := errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	}
}

func (ms *ModuleStore) GetProjectList(ctx context.Context) []map[string]interface{} {
	logs.WithContext(ctx).Debug("GetProjectList - Start")
	projects := make([]map[string]interface{}, len(ms.Projects))
	i := 0
	for k := range ms.Projects {
		project := make(map[string]interface{})
		project["project_name"] = k
		projects[i] = project
		i++
	}
	return projects
}
func (ms *ModuleStore) SaveModel(ctx context.Context, modelObj models.ModelI, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveModel - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}

	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return err
	}

	//save original modelObj with variables
	err = prj.AddModel(ctx, tenantId, modelObj)
	if err != nil {
		return err
	}

	if persist {
		return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
	}
	return nil
}

func (ms *ModuleStore) RemoveModel(ctx context.Context, modelName string, projectId string, tenantId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveModel - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		if _, ok := prj.Tenants[tenantId].Models[modelName]; !ok {
			err = errors.New("Model " + modelName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return err
		}
		err = prj.RemoveModel(ctx, tenantId, modelName)
		if err != nil {
			return err
		}
		return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetModelClone(ctx context.Context, projectId string, tenantId string, modelName string, s ModuleStoreI) (modelObjClone models.ModelI, err error) {
	logs.WithContext(ctx).Debug("GetModelClone - Start")
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	}
	var modelObj models.ModelI
	if _, ok := prj.Tenants[tenantId]; !ok {
		err = errors.New("tenant " + tenantId + " not found")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	lookupOrder := tenantLookupOrder(ctx, projectId, tenantId)
	found := false
	for _, tid := range lookupOrder {
		if mo, ok := prj.Tenants[tid].Models[modelName]; ok {
			modelObj = mo
			found = true
			break
		}
	}
	if !found {
		searched := make([]string, 0, len(lookupOrder))
		for _, tid := range lookupOrder {
			modelNames := make([]string, 0, len(prj.Tenants[tid].Models))
			for mn := range prj.Tenants[tid].Models {
				modelNames = append(modelNames, mn)
			}
			searched = append(searched, fmt.Sprint(tid, "=", modelNames))
		}
		logs.WithContext(ctx).Error(fmt.Sprint("model ", modelName, " not found - searched project=", projectId, " tenant=", tenantId, " models=", searched))
		err = errors.New("model " + modelName + " not found")
		return
	}
	modelObjClone, err = ms.GetModelCloneObject(ctx, projectId, tenantId, modelObj, s)
	return
}

func (ms *ModuleStore) GetModelCloneObject(ctx context.Context, projectId string, tenantId string, modelObj models.ModelI, s ModuleStoreI) (modelObjClone models.ModelI, err error) {
	logs.WithContext(ctx).Debug("GetModelCloneObject - Start")

	modelObjJson, modelObjJsonErr := json.Marshal(modelObj)
	if modelObjJsonErr != nil {
		err = errors.New("error while cloning modelObj (marshal)")
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(modelObjJsonErr.Error())
		return
	}
	modelObjJson = s.ReplaceTenantVariables(ctx, projectId, tenantId, "", modelObjJson)
	modelObjJson = s.ReplaceVariables(ctx, projectId, modelObjJson, nil)

	iCloneI := reflect.New(reflect.TypeOf(modelObj))
	modelObjCloneErr := json.Unmarshal(modelObjJson, iCloneI.Interface())
	if modelObjCloneErr != nil {
		err = errors.New("error while cloning modelObj(unmarshal)")
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(modelObjCloneErr.Error())
		return
	}
	return iCloneI.Elem().Interface().(models.ModelI), nil
}
func (ms *ModuleStore) GetModel(ctx context.Context, projectId string, tenantId string, modelName string, s ModuleStoreI) (models.ModelI, error) {
	logs.WithContext(ctx).Debug("GetModel - Start")
	return ms.GetModelClone(ctx, projectId, tenantId, modelName, s)

}

func (ms *ModuleStore) SaveTool(ctx context.Context, tooling tools.Tooling, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveTool - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}

	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return err
	}

	//save original modelObj with variables
	err = prj.AddTool(ctx, tenantId, tooling)
	if err != nil {
		return err
	}

	if persist {
		return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
	}
	return nil
}

func (ms *ModuleStore) RemoveTool(ctx context.Context, toolName string, projectId string, tenantId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveTool - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		if _, ok := prj.Tenants[tenantId].Tools[toolName]; !ok {
			err = errors.New("Tool " + toolName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return err
		}
		err = prj.RemoveTool(ctx, tenantId, toolName)
		if err != nil {
			return err
		}
		return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetToolClone(ctx context.Context, projectId string, tenantId string, toolName string, actionName string, s ModuleStoreI) (toolObjClone tools.Tooling, err error) {
	logs.WithContext(ctx).Debug("GetToolClone - Start")
	logs.WithContext(ctx).Info(actionName)
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	}
	var toolObj tools.Tooling
	var ok bool
	_, tenantExists := prj.Tenants[tenantId]
	for _, tid := range tenantLookupOrder(ctx, projectId, tenantId) {
		if tenant, tenantOk := prj.Tenants[tid]; tenantOk {
			if toolObj, ok = tenant.Tools[toolName]; ok {
				break
			}
		}
	}
	if !ok {
		if !tenantExists {
			err = errors.New("tenant " + tenantId + " not found")
		} else {
			err = errors.New("tool " + toolName + " not found")
		}
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	if actionName != "" {
		err = toolObj.ValidateAction(ctx, actionName, toolObj)
		if err != nil {
			return
		}
	}

	err = toolObj.SetPrivateAttributes(ctx, toolObj)
	if err != nil {
		return
	}

	toolObjClone, err = ms.GetToolCloneObject(ctx, projectId, tenantId, toolObj, s)
	if err != nil {
		return
	}
	toolObjClone.SetToolDb(db.GetDb(s.GetDbType()))
	toolObjClone.GetToolDb().SetConn(s.GetConn())
	toolObjClone.SetToolAction(actionName)

	var scheduler scheduler.SchedulerI
	scheduler, err = s.FetchScheduler(ctx, projectId)
	if err == nil {
		toolObjClone.SetScheduler(scheduler)
	} else {
		err = nil //ignore error and allow rest of object to be cloned
	}

	vectorStoreName, _ := toolObjClone.GetAttribute(ctx, "vectorstore_name")
	if vectorStoreName != nil {
		vectorStoreName := vectorStoreName.(string)
		vectorStore, vectorStoreErr := s.GetVectorStore(ctx, projectId, tenantId, vectorStoreName, s)
		if vectorStoreErr != nil {
			err = vectorStoreErr
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		vectorStoreClone, vectorStoreCloneErr := s.GetVectorStoreCloneObject(ctx, projectId, tenantId, vectorStore, s)
		if vectorStoreCloneErr != nil {
			err = vectorStoreCloneErr
			logs.WithContext(ctx).Error(err.Error())
			return
		} else {
			toolObjClone.SetAttribute(ctx, "vectorstore", vectorStoreClone)
		}

		embed, embedErr := vectorStoreClone.GetEmbed(ctx)
		if embedErr != nil {
			err = embedErr
			logs.WithContext(ctx).Error(err.Error())
			return
		}

		dimension := vectorStoreClone.GetAttribute(ctx, "dimension")
		dimensionInt := 0
		if dimension != "" {
			dimensionInt, err = strconv.Atoi(dimension)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return
			}
		}
		embed.Dimension = dimensionInt
		embed.Metric = vectorStoreClone.GetAttribute(ctx, "metric")
		if embed.ModelName != "" {
			model, err := s.GetModel(ctx, projectId, tenantId, embed.ModelName, s)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}
			embed.Model = model
			err = vectorStoreClone.SetEmbed(ctx, embed)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}
		}
	}

	return
}

func (ms *ModuleStore) GetToolCloneObject(ctx context.Context, projectId string, tenantId string, toolObj tools.Tooling, s ModuleStoreI) (toolObjClone tools.Tooling, err error) {
	logs.WithContext(ctx).Debug("GetToolCloneObject - Start")

	toolObjJson, toolObjJsonErr := toolObj.GetBytes(ctx)
	if toolObjJsonErr != nil {
		return
	}
	toolObjJson = s.ReplaceTenantVariables(ctx, projectId, tenantId, "", toolObjJson)
	toolObjJson = s.ReplaceVariables(ctx, projectId, toolObjJson, nil)

	return toolObj.BytesToTool(ctx, toolObjJson)

	/* iCloneI := reflect.New(reflect.TypeOf(toolObj))
	toolObjCloneErr := json.Unmarshal(toolObjJson, iCloneI.Interface())
	if toolObjCloneErr != nil {
		err = errors.New("error while cloning toolObj(unmarshal)")
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(toolObjCloneErr.Error())
		return
	}
	return iCloneI.Elem().Interface().(tools.Tooling), nil */
}
func (ms *ModuleStore) GetVectorStoreCloneObject(ctx context.Context, projectId string, tenantId string, vectorStoreObj vectorstore.VectorStoreI, s ModuleStoreI) (vectorStoreObjClone vectorstore.VectorStoreI, err error) {
	logs.WithContext(ctx).Debug("GetVectorStoreCloneObject - Start")

	vectorStoreObjJson, vectorStoreObjJsonErr := vectorStoreObj.GetBytes(ctx)
	if vectorStoreObjJsonErr != nil {
		return
	}
	vectorStoreObjJson = s.ReplaceTenantVariables(ctx, projectId, tenantId, "", vectorStoreObjJson)
	vectorStoreObjJson = s.ReplaceVariables(ctx, projectId, vectorStoreObjJson, nil)

	return vectorStoreObj.BytesToVectorStore(ctx, vectorStoreObjJson)

}
func (ms *ModuleStore) GetTool(ctx context.Context, projectId string, tenantId string, toolName string, actionName string, s ModuleStoreI) (toolObjClone tools.Tooling, err error) {
	logs.WithContext(ctx).Debug("GetTool - Start")
	return ms.GetToolClone(ctx, projectId, tenantId, toolName, actionName, s)

}

func (ms *ModuleStore) IsTenantTool(ctx context.Context, projectId string, tenantId string, toolName string) bool {
	logs.WithContext(ctx).Debug("IsTenantTool - Start")
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return false
	}
	tenant, tenantExists := prj.Tenants[tenantId]
	if !tenantExists {
		return false
	}
	_, toolExists := tenant.Tools[toolName]
	return toolExists
}

func (ms *ModuleStore) SaveAgent(ctx context.Context, agentObj agents.AgentI, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveAgent - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}

	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return err
	}

	//save original modelObj with variables
	err = prj.AddAgent(ctx, tenantId, agentObj)
	if err != nil {
		return err
	}

	if persist {
		return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
	}
	return nil
}

func (ms *ModuleStore) RemoveAgent(ctx context.Context, agentName string, projectId string, tenantId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveAgent - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		if _, ok := prj.Tenants[tenantId].Agents[agentName]; !ok {
			err = errors.New("Agent " + agentName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return err
		}
		err = prj.RemoveAgent(ctx, tenantId, agentName)
		if err != nil {
			return err
		}
		return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}

func (ms *ModuleStore) GetAgentClone(ctx context.Context, projectId string, tenantId string, conversationId string, agentName string, s ModuleStoreI) (agentObjClone agents.AgentI, err error) {
	logs.WithContext(ctx).Debug("GetAgentClone - Start")
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	}
	var agentObj agents.AgentI
	if _, ok := prj.Tenants[tenantId]; !ok {
		err = errors.New("tenant " + tenantId + " not found")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	agentFound := false
	for _, tid := range tenantLookupOrder(ctx, projectId, tenantId) {
		if ao, ok := prj.Tenants[tid].Agents[agentName]; ok {
			agentObj = ao
			agentFound = true
			break
		}
	}
	if !agentFound {
		err = errors.New("agent " + agentName + " not found")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	agentObjClone, err = ms.GetAgentCloneObject(ctx, projectId, tenantId, conversationId, agentObj, s)
	if err != nil {
		return
	}
	if provider := agentObj.GetProvider(); provider != nil {
		if cloneProvider, ok := agentObjClone.(agents.SystemPromptProvider); ok && any(provider) == any(agentObj) {
			agentObjClone.SetProvider(cloneProvider)
		} else {
			agentObjClone.SetProvider(provider)
		}
	}
	// The clone and the stored agent now resolve to the same shared store, so
	// there is nothing to copy across - and copying a store's persistence
	// settings onto itself is at best wasted work.
	cacheI := agentObj.GetChatMemory()
	if cacheI != nil && agentObjClone.GetChatMemory() != cacheI {
		err := agentObjClone.GetChatMemory().SyncPersistence(ctx, cacheI)
		if err != nil {
			return nil, err
		}
	}
	err = agentObjClone.ValidateChatMemory(ctx, projectId)
	if err != nil {
		return nil, err
	}
	return
}

func (ms *ModuleStore) GetAgentCloneObject(ctx context.Context, projectId string, tenantId string, conversationId string, agentObj agents.AgentI, s ModuleStoreI) (agentObjClone agents.AgentI, err error) {
	logs.WithContext(ctx).Debug("GetAgentCloneObject - Start")

	agentObjJson, agentObjJsonErr := json.Marshal(agentObj)
	if agentObjJsonErr != nil {
		err = errors.New("error while cloning agentObj (marshal)")
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(agentObjJsonErr.Error())
		return
	}
	agentObjJson = s.ReplaceTenantVariables(ctx, projectId, tenantId, conversationId, agentObjJson)
	agentObjJson = s.ReplaceVariables(ctx, projectId, agentObjJson, nil)

	iCloneI := reflect.New(reflect.TypeOf(agentObj))
	agentObjCloneErr := json.Unmarshal(agentObjJson, iCloneI.Interface())
	if agentObjCloneErr != nil {
		err = errors.New("error while cloning agentObj(unmarshal)")
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Error(agentObjCloneErr.Error())
		return
	}
	return iCloneI.Elem().Interface().(agents.AgentI), nil
}

// populateAgentTools recursively populates all tools including dependent tools
func (ms *ModuleStore) populateAgentTools(ctx context.Context, projectId string, tenantId string, agentTools []agents.AgentTools, s ModuleStoreI) error {
	for i, agentTool := range agentTools {
		// Get the main tool
		tool, err := ms.GetTool(ctx, projectId, tenantId, agentTool.ToolName, agentTool.ActionName, s)
		if err != nil {
			return err
		}
		// Wrapped where the tool is HYDRATED, not where it is called.
		//
		// There are at least four places a configured tool gets executed - the
		// reasoning agent's toolExecutor, generate_structured, Agent.ExecuteTools
		// and the orchestrator's delegates - and wrapping them one at a time is
		// how the record acquired a hole: save_field showed twice in the metrics
		// tally and not once in the record, so an assertion about its arguments
		// reported it had never been called. Every consumer reads AgentTools[i].Tool,
		// so wrapping here is the only point that cannot be bypassed by adding a
		// fifth call site.
		agentTools[i].Tool = agents.Recording(tool)

		// Recursively populate dependent tools if they exist
		if len(agentTool.DependentTools) > 0 {
			err = ms.populateAgentTools(ctx, projectId, tenantId, agentTool.DependentTools, s)
			if err != nil {
				return err
			}
			// Update the dependent tools in the current agent tool
			agentTools[i].DependentTools = agentTool.DependentTools
		}
	}
	return nil
}

// populateInternalTools gives an agent type the tools it needs for itself,
// resolved from the tenant's configured tools rather than from the agent's own
// tool list.
//
// Some lookups are part of what an agent IS, not something an owner chooses: the
// Eru Studio agent cannot name a form field without the entity metadata, and
// cannot answer "make it look like the invoice page" without reading that page.
// Requiring someone to attach those by hand means the agent silently does the
// job worse when they forget. A missing tool is logged and the agent degrades -
// it is never fatal, because the tenant may genuinely not have that tool.
func (ms *ModuleStore) populateInternalTools(ctx context.Context, projectId string, tenantId string, agent agents.AgentI, s ModuleStoreI) {
	provider, ok := agent.(agents.InternalToolProvider)
	if !ok {
		return
	}
	requests := provider.InternalToolRequests()
	if len(requests) == 0 {
		return
	}

	toolNames, err := ms.GetToolNames(ctx, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Info(fmt.Sprint("populateInternalTools - no tools listed for the tenant: ", err.Error()))
		return
	}
	sort.Strings(toolNames)

	wanted := make(map[string]agents.InternalToolRequest, len(requests))
	for _, request := range requests {
		if request.Action != "" {
			wanted[request.Action] = request
		}
	}

	resolved := make(map[string]tools.Tooling, len(wanted))
	for _, toolName := range toolNames {
		if len(resolved) == len(wanted) {
			break
		}
		toolObj, terr := ms.GetToolClone(ctx, projectId, tenantId, toolName, "", s)
		if terr != nil {
			continue
		}
		for _, action := range toolObj.GetActionsList() {
			request, needed := wanted[action.Name]
			if !needed {
				continue
			}
			if _, already := resolved[action.Name]; already {
				continue
			}
			// A fresh clone per action: the tool carries its action on itself.
			actionTool, aerr := ms.GetToolClone(ctx, projectId, tenantId, toolName, action.Name, s)
			if aerr != nil {
				logs.WithContext(ctx).Info(fmt.Sprint("populateInternalTools - ", toolName, " offers ", action.Name, " but could not be cloned: ", aerr.Error()))
				continue
			}
			// Internal tools reach the same recording boundary: they are executed
			// by provider code that never passes through the agent tool loops.
			resolved[action.Name] = agents.Recording(actionTool)
			logs.WithContext(ctx).Info(fmt.Sprint("populateInternalTools - resolved ", action.Name, " from tool ", toolName, " (", request.Why, ")"))
		}
	}

	for action, request := range wanted {
		if _, found := resolved[action]; !found {
			logs.WithContext(ctx).Info(fmt.Sprint("populateInternalTools - no tenant tool offers ", action,
				" so the agent will work without it (", request.Why, ")"))
		}
	}
	provider.SetInternalTools(resolved)
}

func (ms *ModuleStore) GetAgent(ctx context.Context, projectId string, tenantId string, conversationId string, agentName string, s ModuleStoreI) (agents.AgentI, error) {
	logs.WithContext(ctx).Debug("GetAgent - Start")
	agent, err := ms.GetAgentClone(ctx, projectId, tenantId, conversationId, agentName, s)
	if err != nil {
		return nil, err
	}
	agentToolsI, err := agent.GetAttribute(ctx, "agent_tools")
	if err != nil {
		return nil, err
	}
	agentTools, ok := agentToolsI.([]agents.AgentTools)
	if !ok {
		return nil, errors.New("agent_tools attribute is not an array")
	}

	// Use the recursive function to populate all tools including dependent tools
	err = ms.populateAgentTools(ctx, projectId, tenantId, agentTools, s)
	if err != nil {
		return nil, err
	}

	ms.populateInternalTools(ctx, projectId, tenantId, agent, s)

	modelNameI, err := agent.GetAttribute(ctx, "model")
	if err != nil {
		return nil, err
	}
	modelName, ok := modelNameI.(string)
	if !ok {
		return nil, errors.New("model attribute is not a string")
	}
	model, err := ms.GetModel(ctx, projectId, tenantId, modelName, s)
	if err != nil {
		return nil, err
	}
	agent.SetModel(model)

	if discoveryAgent, ok := agent.(agents.AgentDiscovery); ok {
		discoveryAgent.SetDiscoveredAgents(ms.DiscoverAgents(ctx, projectId, tenantId, agentName, discoveryAgent.AllowedAgentNames(), s))
	}

	if toolDiscoveryAgent, ok := agent.(agents.ToolDiscovery); ok {
		toolDiscoveryAgent.SetDiscoveredTools(ms.DiscoverTools(ctx, projectId, tenantId, toolDiscoveryAgent.AllowedToolActions(), s))
	}
	ms.attachMemory(ctx, projectId, tenantId, agent, s)

	if modelDiscoveryAgent, ok := agent.(agents.ModelDiscovery); ok {
		modelDiscoveryAgent.SetDiscoveredModels(ms.DiscoverModels(ctx, projectId, tenantId))
	}

	agent.InitializeConversationManager(ctx)
	summaryModelNameI, err := agent.GetAttribute(ctx, "summary_model")
	if err != nil {
		err = nil //ignore error and continue with main model
		return agent, nil
	}
	summaryModelName, ok := summaryModelNameI.(string)
	if !ok {
		logs.WithContext(ctx).Error("summary model attribute is not a string")
		err = nil //ignore error and continue with main model
		return agent, nil
	}
	if summaryModelName != "" {
		summaryModel, err := ms.GetModel(ctx, projectId, tenantId, summaryModelName, s)
		if err != nil {
			err = nil //ignore error and continue with main model
			return agent, nil
		}
		agent.SetSummaryModel(summaryModel)
	}
	return agent, nil
}

func (ms *ModuleStore) DiscoverAgents(ctx context.Context, projectId string, tenantId string, selfName string, allowedNames []string, s ModuleStoreI) []agents.DiscoveredAgent {
	logs.WithContext(ctx).Debug("DiscoverAgents - Start")
	allowSet := make(map[string]bool)
	for _, n := range allowedNames {
		allowSet[n] = true
	}
	agentNames, err := ms.GetAgentNames(ctx, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil
	}
	logs.WithContext(ctx).Info(fmt.Sprint("DiscoverAgents - project=", projectId, " tenant=", tenantId, " allowed=", allowedNames, " found_in_tenant=", agentNames))
	var discovered []agents.DiscoveredAgent
	for _, agentName := range agentNames {
		if agentName == selfName {
			continue
		}
		if len(allowSet) > 0 && !allowSet[agentName] {
			continue
		}
		agentObj, err := ms.GetAgentClone(ctx, projectId, tenantId, "", agentName, s)
		if err != nil {
			continue
		}
		description := ""
		if desc, derr := agentObj.GetAttribute(ctx, "description"); derr == nil {
			if descStr, ok := desc.(string); ok {
				description = descStr
			}
		}
		if description == "" {
			description = fmt.Sprint("AI Agent: ", agentName)
		}
		agentType := ""
		if at, aerr := agentObj.GetAttribute(ctx, "agent_type"); aerr == nil {
			if atStr, ok := at.(string); ok {
				agentType = atStr
			}
		}
		guardrail := ""
		if gp, gerr := agentObj.GetAttribute(ctx, "guardrail_prompt"); gerr == nil {
			if gpStr, ok := gp.(string); ok {
				guardrail = strings.TrimSpace(gpStr)
			}
		}
		supportsClarification := false
		if capable, ok := agentObj.(agents.ClarificationCapable); ok {
			supportsClarification = capable.ClarificationEnabled()
		}
		// What the agent looks up for itself, so the planner does not plan a step
		// to fetch the same thing by other means.
		var internalCapabilities []string
		if provider, ok := agentObj.(agents.InternalToolProvider); ok {
			for _, request := range provider.InternalToolRequests() {
				if strings.TrimSpace(request.Why) != "" {
					internalCapabilities = append(internalCapabilities, request.Why)
				}
			}
		}
		planningNote := ""
		if advisor, ok := agentObj.(agents.PlanningAdvisor); ok {
			planningNote = strings.TrimSpace(advisor.PlanningNote())
		}
		discovered = append(discovered, agents.DiscoveredAgent{
			AgentName:             agentName,
			AgentType:             agentType,
			Description:           description,
			TenantId:              tenantId,
			InputSchema:           AgentInputSchema(ctx, agentObj),
			OutputSchema:          AgentOutputSchema(ctx, agentObj),
			Tools:                 AgentToolNames(ctx, agentObj),
			Guardrail:             guardrail,
			SupportsClarification: supportsClarification,
			IsOrchestrator:        agentType == "ORCHESTRATOR",
			InternalCapabilities:  internalCapabilities,
			PlanningNote:          planningNote,
		})
	}
	logs.WithContext(ctx).Info(fmt.Sprint("DiscoverAgents - resolved ", len(discovered), " agent(s) for orchestrator ", selfName))
	return discovered
}

// AgentOutputSchema resolves the schema the agent actually responds with.
// Provider-backed agent types (ERU_STUDIO, ERU_FUNC, ...) build theirs at runtime
// and leave the stored output_schema attribute blank, so the provider is asked
// first - except where the agent declares a separate response schema because what
// it plans with is not what it answers with.
func AgentOutputSchema(ctx context.Context, agentObj agents.AgentI) eru_models.JSONSchema {
	if responder, ok := agentObj.(agents.AgentResponseSchemaProvider); ok {
		return responder.GetResponseSchema(ctx)
	}
	if provider := agentObj.GetProvider(); provider != nil {
		if js := provider.GetOutputSchema(ctx); js.Type != "" {
			return js
		}
	}
	if os, oerr := agentObj.GetAttribute(ctx, "output_schema"); oerr == nil {
		if js, ok := os.(eru_models.JSONSchema); ok {
			return js
		}
	}
	return eru_models.JSONSchema{}
}

// AgentInputSchema resolves the request contract the agent accepts, including the
// params keys its type actually reads.
func AgentInputSchema(ctx context.Context, agentObj agents.AgentI) eru_models.JSONSchema {
	if provider, ok := agentObj.(agents.AgentInputSchemaProvider); ok {
		return provider.GetInputSchema(ctx)
	}
	return agents.AgentInputSchema(nil, nil)
}

// AgentToolNames lists the tool actions an agent can call itself, so a caller can
// tell what the agent is capable of beyond its one-line description.
func AgentToolNames(ctx context.Context, agentObj agents.AgentI) []string {
	atI, err := agentObj.GetAttribute(ctx, "agent_tools")
	if err != nil {
		return nil
	}
	agentTools, ok := atI.([]agents.AgentTools)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	var collect func(list []agents.AgentTools)
	collect = func(list []agents.AgentTools) {
		for _, at := range list {
			name := at.ToolName
			if at.ActionName != "" {
				name = fmt.Sprint(at.ToolName, ".", at.ActionName)
			}
			if name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
			collect(at.DependentTools)
		}
	}
	collect(agentTools)
	sort.Strings(names)
	return names
}

// DiscoverModels lists the models a tenant can use, nearest tenant first.
func (ms *ModuleStore) DiscoverModels(ctx context.Context, projectId string, tenantId string) []agents.DiscoveredModel {
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []agents.DiscoveredModel
	for _, tid := range tenantLookupOrder(ctx, projectId, tenantId) {
		tenant, ok := prj.Tenants[tid]
		if !ok {
			continue
		}
		for name, model := range tenant.Models {
			if seen[name] {
				continue
			}
			seen[name] = true
			entry := agents.DiscoveredModel{ModelName: name}
			if v, e := model.GetAttribute(ctx, "provider"); e == nil {
				entry.Provider = fmt.Sprint(v)
			}
			if v, e := model.GetAttribute(ctx, "llm_name"); e == nil {
				entry.LLMName = fmt.Sprint(v)
			}
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModelName < out[j].ModelName })
	return out
}

func (ms *ModuleStore) DiscoverTools(ctx context.Context, projectId string, tenantId string, allowed map[string][]string, s ModuleStoreI) []agents.DiscoveredTool {
	logs.WithContext(ctx).Debug("DiscoverTools - Start")
	toolNames := make([]string, 0, len(allowed))
	if len(allowed) > 0 {
		for toolName := range allowed {
			toolNames = append(toolNames, toolName)
		}
	} else {
		names, err := ms.GetToolNames(ctx, projectId, tenantId)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil
		}
		toolNames = names
	}
	sort.Strings(toolNames)
	var discovered []agents.DiscoveredTool
	for _, toolName := range toolNames {
		toolObj, err := ms.GetToolClone(ctx, projectId, tenantId, toolName, "", s)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("DiscoverTools - tool ", toolName, " not found: ", err.Error()))
			continue
		}
		actionSet := make(map[string]bool)
		for _, a := range allowed[toolName] {
			actionSet[a] = true
		}
		toolDescription := ""
		if desc, derr := toolObj.GetAttribute(ctx, "description"); derr == nil {
			if descStr, ok := desc.(string); ok {
				toolDescription = descStr
			}
		}
		toolType := ""
		if tt, terr := toolObj.GetAttribute(ctx, "tool_type"); terr == nil {
			toolType, _ = tt.(string)
		}
		actions := toolObj.GetActions()
		// Some tool types (MS_EMAIL among them) list their actions only through
		// GetActionsList. Reading GetActions alone showed such a tool to planners
		// and to the agent builder as having no actions at all, so a thread
		// reader sat in the workspace and nobody could see it.
		if len(actions) == 0 {
			if listed := toolObj.GetActionsList(); len(listed) > 0 {
				for _, info := range listed {
					if len(actionSet) > 0 && !actionSet[info.Name] {
						continue
					}
					entry := agents.DiscoveredTool{
						ToolName:    toolName,
						ToolType:    toolType,
						ActionName:  info.Name,
						Description: strings.TrimSpace(strings.Trim(toolDescription+" - "+info.Description, " -")),
						TenantId:    tenantId,
					}
					if bound, bErr := ms.GetToolClone(ctx, projectId, tenantId, toolName, info.Name, s); bErr == nil {
						entry.InputSchema = bound.GetParameters()
					}
					discovered = append(discovered, entry)
				}
				continue
			}
			discovered = append(discovered, agents.DiscoveredTool{
				ToolName:    toolName,
				ToolType:    toolType,
				Description: toolDescription,
				InputSchema: toolObj.GetParameters(),
				TenantId:    tenantId,
			})
			continue
		}
		for _, action := range actions {
			if len(actionSet) > 0 && !actionSet[action.ActionName] {
				continue
			}
			inputSchema := action.Parameters
			if action.GetParameters != nil {
				inputSchema = action.GetParameters()
			}
			description := toolDescription
			if action.Description != "" {
				if description != "" {
					description = description + " - " + action.Description
				} else {
					description = action.Description
				}
			}
			discovered = append(discovered, agents.DiscoveredTool{
				ToolName:     toolName,
				ToolType:     toolType,
				ActionName:   action.ActionName,
				Description:  description,
				InputSchema:  inputSchema,
				OutputSchema: action.OutputSchema,
				TenantId:     tenantId,
			})
		}
	}
	logs.WithContext(ctx).Info(fmt.Sprint("DiscoverTools - resolved ", len(discovered), " tool action(s)"))
	return discovered
}

func (ms *ModuleStore) SaveProjectSettings(ctx context.Context, projectId string, projectSettings module_model.ProjectSettings, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("SaveProjectConfig - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	ms.Projects[projectId].ProjectSettings = projectSettings
	logs.WithContext(ctx).Info("SaveStore called from SaveProjectSettings")
	return realStore.SaveStore(ctx, projectId, "", realStore)
}
func (ms *ModuleStore) SaveToolCatalogAccess(ctx context.Context, projectId string, accessRequest module_model.ToolCatalogAccessRequest, realStore ModuleStoreI) error {
	logs.WithContext(ctx).Debug("SaveToolCatalogAccess - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()
	err := ms.checkProjectExists(ctx, projectId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	projectSettings := ms.Projects[projectId].ProjectSettings
	err = projectSettings.SetToolCatalogAccess(ctx, accessRequest)
	if err != nil {
		return err
	}
	ms.Projects[projectId].ProjectSettings = projectSettings
	return realStore.SaveStore(ctx, projectId, "", realStore)
}

func (ms *ModuleStore) GetProjectSettings(ctx context.Context, projectId string) (projectSettings module_model.ProjectSettings, err error) {
	logs.WithContext(ctx).Debug("GetProjectSettings - Start")
	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return
	}
	return prj.ProjectSettings, nil
}

func (ms *ModuleStore) GetStoreWithoutTenants(ctx context.Context, realStore store.StoreI) (b []byte, err error) {
	logs.WithContext(ctx).Debug("GetStoreByteArrayWithoutTenants - Start")
	logs.WithContext(ctx).Info("calling custom get store byte array without tenants from eruai")

	realStoreJson, err := json.Marshal(realStore)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	newMs := new(ModuleDbStore)
	err = UnMarshalStore(ctx, realStoreJson, newMs)
	//err = json.Unmarshal(realStoreJson, newMs)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	newMs.RemoveTenants()
	b, err = json.Marshal(newMs)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (ms *ModuleStore) RemoveTenants() {
	for key, project := range ms.Projects {
		project.Tenants = nil
		ms.Projects[key] = project
	}
}

func (ms *ModuleStore) GetAgentNames(ctx context.Context, projectId string, tenantId string) (agentNames []string, err error) {
	logs.WithContext(ctx).Debug("GetAgentNames - Start")
	if prj, ok := ms.Projects[projectId]; ok {
		visible := tenantVisibilitySet(ctx, projectId, tenantId)
		seen := make(map[string]bool)
		for _, tenant := range prj.Tenants {
			if tenantId == "" || visible[tenant.TenantId] {
				for agentName := range tenant.Agents {
					if seen[agentName] {
						continue
					}
					seen[agentName] = true
					agentNames = append(agentNames, agentName)
				}
			}
		}
		return agentNames, nil
	} else {
		err = errors.New("Project " + projectId + " does not exist")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

func (ms *ModuleStore) GetToolNames(ctx context.Context, projectId string, tenantId string) (toolNames []string, err error) {
	logs.WithContext(ctx).Debug("GetToolNames - Start")

	if prj, ok := ms.Projects[projectId]; ok {
		visible := tenantVisibilitySet(ctx, projectId, tenantId)
		seen := make(map[string]bool)
		for _, tenant := range prj.Tenants {
			if tenantId == "" || visible[tenant.TenantId] {
				for toolName := range tenant.Tools {
					if seen[toolName] {
						continue
					}
					seen[toolName] = true
					toolNames = append(toolNames, toolName)
				}
			}
		}
		return toolNames, nil
	} else {
		err = errors.New("Project " + projectId + " does not exist")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

func (ms *ModuleStore) SaveVectorStore(ctx context.Context, vectorStoreObj vectorstore.VectorStoreI, projectId string, tenantId string, realStore ModuleStoreI, persist bool) error {
	logs.WithContext(ctx).Debug("SaveVectorStore - Start")
	if persist {
		realStore.GetMutex().Lock()
		defer realStore.GetMutex().Unlock()
	}

	prj, err := ms.GetProjectConfig(ctx, projectId)
	if err != nil {
		return err
	}

	isNew, updatedVectorStoreObj, err := prj.AddVectorStore(ctx, tenantId, vectorStoreObj)
	if err != nil {
		return err
	}
	if persist {
		vectorStoreObjClone, err := ms.GetVectorStoreCloneObject(ctx, projectId, tenantId, updatedVectorStoreObj, realStore)
		if err != nil {
			return err
		}

		if isNew {
			err = updatedVectorStoreObj.CreateIndex(ctx, vectorStoreObjClone)
			if err != nil {
				return err
			}
		} else {
			err = updatedVectorStoreObj.EditIndex(ctx, vectorStoreObjClone)
			if err != nil {
				return err
			}
		}
		return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
	}

	return nil
}
func (ms *ModuleStore) SyncVectorStore(ctx context.Context, vectorStoreName string, projectId string, tenantId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("SyncVectorStore - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		if vs, ok := prj.Tenants[tenantId].VectorStores[vectorStoreName]; !ok {
			err = errors.New("VectorStore " + vectorStoreName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return err
		} else {
			vectorStoreClone, err := ms.GetVectorStoreCloneObject(ctx, projectId, tenantId, vs, realStore)
			if err != nil {
				return err
			}
			err = vs.SyncIndexDefinition(ctx, vectorStoreClone)
			if err != nil {
				return err
			}
			logs.WithContext(ctx).Info(fmt.Sprint(vs))
		}
		return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}
func (ms *ModuleStore) GetVectorStore(ctx context.Context, projectId string, tenantId string, vectorStoreName string, realStore ModuleStoreI) (vectorStore vectorstore.VectorStoreI, err error) {
	logs.WithContext(ctx).Debug("GetVectorStore - Start")

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		for _, tid := range tenantLookupOrder(ctx, projectId, tenantId) {
			if vs, ok := prj.Tenants[tid].VectorStores[vectorStoreName]; ok {
				return vs, nil
			}
		}
		err = errors.New("VectorStore " + vectorStoreName + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return nil, err
	}
}
func (ms *ModuleStore) RemoveVectorStore(ctx context.Context, vectorStoreName string, projectId string, tenantId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveVectorStore - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		if vs, ok := prj.Tenants[tenantId].VectorStores[vectorStoreName]; !ok {
			err = errors.New("VectorStore " + vectorStoreName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return err
		} else {
			err = prj.RemoveVectorStore(ctx, tenantId, vectorStoreName)
			if err != nil {
				return err
			} else {
				vectorStoreClone, err := ms.GetVectorStoreCloneObject(ctx, projectId, tenantId, vs, realStore)
				if err != nil {
					return err
				}
				_ = vectorStoreClone.DeleteIndex(ctx, vs.GetAttribute(ctx, "index_name"))
				// ignore error from DeleteIndex and still persists the vectorstore
			}
			return realStore.SaveTenantStore(ctx, projectId, tenantId, "", prj.Tenants[tenantId])
		}
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
}
func (ms *ModuleStore) SaveVectors(ctx context.Context, vectorRecords vectorstore.VectorRecords, vectorName string, projectId string, tenantId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("SaveVectors - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		var vs vectorstore.VectorStoreI
		var ok bool
		for _, tid := range tenantWriteOrder(projectId, tenantId) {
			if vs, ok = prj.Tenants[tid].VectorStores[vectorName]; ok {
				break
			}
		}
		if !ok {
			err = errors.New("VectorStore " + vectorName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return err
		}
		{
			vectorStoreClone, err := ms.GetVectorStoreCloneObject(ctx, projectId, tenantId, vs, realStore)
			if err != nil {
				return err
			}

			embed, err := vectorStoreClone.GetEmbed(ctx)
			if err != nil {
				return err
			}

			dimension := vectorStoreClone.GetAttribute(ctx, "dimension")
			dimensionInt := 0
			if dimension != "" {
				dimensionInt, err = strconv.Atoi(dimension)
				if err != nil {
					return err
				}
			}
			embed.Dimension = dimensionInt
			embed.Metric = vectorStoreClone.GetAttribute(ctx, "metric")
			if embed.ModelName != "" {
				model, err := ms.GetModel(ctx, projectId, tenantId, embed.ModelName, realStore)
				if err != nil {
					return err
				}
				embed.Model = model
				err = vectorStoreClone.SetEmbed(ctx, embed)
				if err != nil {
					return err
				}
			}

			err = vectorStoreClone.SaveVectors(ctx, vectorRecords)
			if err != nil {
				return err
			}
		}
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
	return nil
}
func (ms *ModuleStore) RemoveVectors(ctx context.Context, vectorRecordsDelete vectorstore.VectorRecordsDelete, vectorName string, projectId string, tenantId string, realStore ModuleStoreI) (err error) {
	logs.WithContext(ctx).Debug("RemoveVectors - Start")
	realStore.GetMutex().Lock()
	defer realStore.GetMutex().Unlock()

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return err
		}
		var vs vectorstore.VectorStoreI
		var ok bool
		for _, tid := range tenantWriteOrder(projectId, tenantId) {
			if vs, ok = prj.Tenants[tid].VectorStores[vectorName]; ok {
				break
			}
		}
		if !ok {
			err = errors.New("VectorStore " + vectorName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return err
		}
		{
			vectorStoreClone, err := ms.GetVectorStoreCloneObject(ctx, projectId, tenantId, vs, realStore)
			if err != nil {
				return err
			}
			err = vectorStoreClone.DeleteVectors(ctx, vectorRecordsDelete)
			if err != nil {
				return err
			}
		}
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return err
	}
	return nil

}
func (ms *ModuleStore) ListVectors(ctx context.Context, vectorRecordsList vectorstore.VectorRecordsList, vectorName string, projectId string, tenantId string, realStore ModuleStoreI) (vectorResults vectorstore.VectorResults, err error) {
	logs.WithContext(ctx).Debug("ListVectors - Start")

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return vectorstore.VectorResults{}, err
		}
		var vs vectorstore.VectorStoreI
		var ok bool
		for _, tid := range tenantLookupOrder(ctx, projectId, tenantId) {
			if vs, ok = prj.Tenants[tid].VectorStores[vectorName]; ok {
				break
			}
		}
		if !ok {
			err = errors.New("VectorStore " + vectorName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return vectorstore.VectorResults{}, err
		}
		{
			vectorStoreClone, err := ms.GetVectorStoreCloneObject(ctx, projectId, tenantId, vs, realStore)
			if err != nil {
				return vectorstore.VectorResults{}, err
			}
			vectorResults, err = vectorStoreClone.ListVectors(ctx, vectorRecordsList)
			if err != nil {
				return vectorstore.VectorResults{}, err
			}
		}
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return vectorstore.VectorResults{}, err
	}
	return vectorResults, nil

}

func (ms *ModuleStore) SearchVectors(ctx context.Context, vectorRecords vectorstore.VectorRecordsSearch, vectorName string, projectId string, tenantId string, realStore ModuleStoreI) (vectorResults vectorstore.VectorResults, err error) {
	logs.WithContext(ctx).Debug("SearchVectors - Start")

	if prj, ok := ms.Projects[projectId]; ok {
		if _, ok := prj.Tenants[tenantId]; !ok {
			err = errors.New("tenant " + tenantId + " does not exists")
			logs.WithContext(ctx).Error(err.Error())
			return vectorstore.VectorResults{}, err
		}
		var vs vectorstore.VectorStoreI
		var ok bool
		for _, tid := range tenantLookupOrder(ctx, projectId, tenantId) {
			if vs, ok = prj.Tenants[tid].VectorStores[vectorName]; ok {
				break
			}
		}
		if !ok {
			err = errors.New("VectorStore " + vectorName + " does not exists")
			logs.WithContext(ctx).Info(err.Error())
			return vectorstore.VectorResults{}, err
		}
		{
			vectorStoreClone, err := ms.GetVectorStoreCloneObject(ctx, projectId, tenantId, vs, realStore)
			if err != nil {
				return vectorstore.VectorResults{}, err
			}

			embed, err := vectorStoreClone.GetEmbed(ctx)
			if err != nil {
				return vectorstore.VectorResults{}, err
			}

			dimension := vectorStoreClone.GetAttribute(ctx, "dimension")
			dimensionInt := 0
			if dimension != "" {
				dimensionInt, err = strconv.Atoi(dimension)
				if err != nil {
					return vectorstore.VectorResults{}, err
				}
			}
			embed.Dimension = dimensionInt
			embed.Metric = vectorStoreClone.GetAttribute(ctx, "metric")
			if embed.ModelName != "" {
				model, err := ms.GetModel(ctx, projectId, tenantId, embed.ModelName, realStore)
				if err != nil {
					return vectorstore.VectorResults{}, err
				}
				embed.Model = model
				err = vectorStoreClone.SetEmbed(ctx, embed)
				if err != nil {
					return vectorstore.VectorResults{}, err
				}
			}

			vectorResults, err = vectorStoreClone.SearchVectors(ctx, vectorRecords)
			if err != nil {
				return vectorstore.VectorResults{}, err
			}
		}
	} else {
		err = errors.New("Project " + projectId + " does not exists")
		logs.WithContext(ctx).Info(err.Error())
		return vectorstore.VectorResults{}, err
	}
	return vectorResults, nil
}
func (ms *ModuleStore) GetVectorStoreNames(ctx context.Context, projectId string, tenantId string) (vectorStoreNames []string, err error) {
	logs.WithContext(ctx).Debug("GetVectorStoreNames - Start")

	if prj, ok := ms.Projects[projectId]; ok {
		// The same fallback every other lookup uses, including the project's own
		// tenant: without projectId here, a store shared from the project was
		// usable by a tenant's agents but missing from the tenant's list.
		seen := make(map[string]bool)
		order := tenantLookupOrder(ctx, projectId, tenantId)
		if tenantId == "" {
			order = order[:0]
			for tid := range prj.Tenants {
				order = append(order, tid)
			}
		}
		for _, tid := range order {
			tenant, found := prj.Tenants[tid]
			if !found {
				continue
			}
			for vectorStoreName := range tenant.VectorStores {
				if seen[vectorStoreName] {
					continue
				}
				seen[vectorStoreName] = true
				vectorStoreNames = append(vectorStoreNames, vectorStoreName)
			}
		}
		return vectorStoreNames, nil
	} else {
		err = errors.New("Project " + projectId + " does not exist")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}
func LoadStore(ctx context.Context, StoreTableName string, StoreTenantTableName string) (ModuleStoreI, error) {
	logs.WithContext(ctx).Info("Loading store")

	storeType := strings.ToUpper(os.Getenv("STORE_TYPE"))
	if storeType == "" {
		storeType = "STANDALONE"
		logs.WithContext(ctx).Info("STORE_TYPE environment variable not found - loading default standlone store")
	}
	var myStore ModuleStoreI
	var err error
	switch storeType {
	case "POSTGRES":
		myStore = new(ModuleDbStore)
		myStore.SetDbType(storeType)
		myStore.SetStoreTableName(StoreTableName)
		myStore.SetStoreTenantTableName(StoreTenantTableName)
		myStore.CreateConn()
	case "STANDALONE":
		// myStore, err = store.LoadStoreFromFile()
		myStore = new(ModuleFileStore)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New(fmt.Sprint("Invalid STORE_TYPE ", storeType))
	}
	storeBytes, err := myStore.GetStoreByteArray("")
	if err == nil {
		UnMarshalStore(ctx, storeBytes, myStore)
	} else {
		logs.WithContext(ctx).Error(err.Error())
	}
	//s.Store = myStore
	return myStore, err
}
