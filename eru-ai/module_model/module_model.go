package module_model

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agents "github.com/eru-os/eru/eru-ai/agents"
	"github.com/eru-os/eru/eru-ai/agents/agents_factory"
	models "github.com/eru-os/eru/eru-ai/models"
	tools "github.com/eru-os/eru/eru-ai/tools"
	tools_factory "github.com/eru-os/eru/eru-ai/tools/tools_factory"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	scheduler "github.com/eru-os/eru/eru-scheduler/scheduler"
	"github.com/eru-os/eru/eru-secret-manager/sm"
	"github.com/eru-os/eru/eru-store/store"
	utils "github.com/eru-os/eru/eru-utils"
	vectorstore "github.com/eru-os/eru/eru-vectorstore/vectorstore"
	"github.com/google/go-cmp/cmp"
)

type StoreCompare struct {
	store.StoreCompare
	DeleteModel   []string               `json:"delete_model"`
	NewModel      []string               `json:"new_model"`
	MismatchModel map[string]interface{} `json:"mismatch_model"`

	DeleteAgent   []string               `json:"delete_agent"`
	NewAgent      []string               `json:"new_agent"`
	MismatchAgent map[string]interface{} `json:"mismatch_agent"`

	DeleteTool   []string               `json:"delete_tool"`
	NewTool      []string               `json:"new_tool"`
	MismatchTool map[string]interface{} `json:"mismatch_tool"`
}

type ModuleProjectI interface {
	AddModel(ctx context.Context, tenantId string, modelObj models.ModelI) error
	RemoveModel(ctx context.Context, tenantId string, modelName string) error
	AddTool(ctx context.Context, tenantId string, toolObj tools.Tool) error
	RemoveTool(ctx context.Context, tenantId string, toolName string) error
	AddAgent(ctx context.Context, tenantId string, agentObj tools.Tool) error
	RemoveAgent(ctx context.Context, tenantId string, agentName string) error
	CompareProject(ctx context.Context, compareProject Project) (StoreCompare, error)
}
type ExtendedProject struct {
	Project
	Variables           store.Variables            `json:"variables"`
	SecretManager       sm.SmStoreI                `json:"secret_manager"`
	Scheduler           scheduler.SchedulerI       `json:"scheduler"`
	TenantSecretManager sm.SmStoreI                `json:"tenant_secret_manager"`
	TenantVariables     map[string]store.Variables `json:"tenant_variables"`
}

type Project struct {
	ProjectId       string                  `json:"project_id" eru:"required"`
	Tenants         map[string]TenantConfig `json:"tenants"`
	ProjectSettings ProjectSettings         `json:"project_settings"`
}

type TenantConfig struct {
	TenantId     string                              `json:"tenant_id" eru:"required"`
	Models       map[string]models.ModelI            `json:"models"`
	Tools        map[string]tools.Tooling            `json:"tools"`
	Agents       map[string]agents.AgentI            `json:"agents"`
	VectorStores map[string]vectorstore.VectorStoreI `json:"vector_stores"`
}

type ProjectSettings struct {
	ClaimsKey         string                       `json:"claims_key" eru:"required"`
	ToolCatalogAccess map[string]ToolCatalogAccess `json:"tool_catalog_access"`
}

type ToolCatalogAccess struct {
	Public         *bool    `json:"public,omitempty"`
	AllowedTenants []string `json:"allowed_tenants"`
}

type ToolCatalogAccessRequest struct {
	ToolType      string   `json:"tool_type" eru:"required"`
	Public        *bool    `json:"public,omitempty"`
	AddTenants    []string `json:"add_tenants"`
	RemoveTenants []string `json:"remove_tenants"`
}

// IsToolVisible tells whether a tenant may see a catalog entry. The persisted
// access overlay wins over defaultPublic (the tool's code level tag) when it
// sets public explicitly, and an allowed tenant always sees the entry.
func (ps ProjectSettings) IsToolVisible(toolType string, defaultPublic bool, tenantId string) bool {
	_, access, ok := ps.accessFor(toolType)
	if !ok {
		return defaultPublic
	}
	public := defaultPublic
	if access.Public != nil {
		public = *access.Public
	}
	if public {
		return true
	}
	if tenantId == "" {
		return false
	}
	for _, allowedTenant := range access.AllowedTenants {
		if allowedTenant == tenantId {
			return true
		}
	}
	return false
}

// accessFor finds a tool type's access overlay. Catalog types used to be
// spelled differently from the factory's (MsEmail for MS_EMAIL, Eruql for
// ERUQL) and access saved under those spellings must keep applying, so a
// lookup that misses by exact key matches ignoring case and underscores. It
// returns the key the overlay is stored under.
func (ps ProjectSettings) accessFor(toolType string) (string, ToolCatalogAccess, bool) {
	if access, ok := ps.ToolCatalogAccess[toolType]; ok {
		return toolType, access, true
	}
	want := foldToolType(toolType)
	for key, access := range ps.ToolCatalogAccess {
		if foldToolType(key) == want {
			return key, access, true
		}
	}
	return "", ToolCatalogAccess{}, false
}

func foldToolType(toolType string) string {
	return strings.ToLower(strings.ReplaceAll(toolType, "_", ""))
}

func (ps *ProjectSettings) SetToolCatalogAccess(ctx context.Context, accessRequest ToolCatalogAccessRequest) error {
	logs.WithContext(ctx).Debug("SetToolCatalogAccess - Start")
	if accessRequest.ToolType == "" {
		err := errors.New("tool_type cannot be blank")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if ps.ToolCatalogAccess == nil {
		ps.ToolCatalogAccess = make(map[string]ToolCatalogAccess)
	}
	legacyKey, access, _ := ps.accessFor(accessRequest.ToolType)
	if legacyKey != "" && legacyKey != accessRequest.ToolType {
		// Rewritten under the canonical type, so the old spelling does not
		// linger as a second, stale entry.
		delete(ps.ToolCatalogAccess, legacyKey)
	}
	if accessRequest.Public != nil {
		public := *accessRequest.Public
		access.Public = &public
	}
	removeTenants := make(map[string]bool)
	for _, tenantId := range accessRequest.RemoveTenants {
		removeTenants[tenantId] = true
	}
	allowedTenants := make([]string, 0, len(access.AllowedTenants)+len(accessRequest.AddTenants))
	existingTenants := make(map[string]bool)
	for _, tenantId := range access.AllowedTenants {
		if removeTenants[tenantId] || existingTenants[tenantId] {
			continue
		}
		existingTenants[tenantId] = true
		allowedTenants = append(allowedTenants, tenantId)
	}
	for _, tenantId := range accessRequest.AddTenants {
		if tenantId == "" || removeTenants[tenantId] || existingTenants[tenantId] {
			continue
		}
		existingTenants[tenantId] = true
		allowedTenants = append(allowedTenants, tenantId)
	}
	access.AllowedTenants = allowedTenants
	ps.ToolCatalogAccess[accessRequest.ToolType] = access
	return nil
}

func (prj *Project) AddModel(ctx context.Context, tenantId string, modelObj models.ModelI) error {
	logs.WithContext(ctx).Debug("AddModel - Start")
	if prj.Tenants == nil {
		prj.Tenants = make(map[string]TenantConfig)
	}

	if _, ok := prj.Tenants[tenantId]; !ok {
		prj.Tenants[tenantId] = TenantConfig{
			TenantId:     tenantId,
			Models:       make(map[string]models.ModelI),
			Tools:        make(map[string]tools.Tooling),
			Agents:       make(map[string]agents.AgentI),
			VectorStores: make(map[string]vectorstore.VectorStoreI),
		}
	}
	modelNameI, _ := modelObj.GetAttribute(ctx, "model_name")
	modelName := modelNameI.(string)
	if modelName == "" {
		return errors.New("model_name cannot be blank")
	}
	prj.Tenants[tenantId].Models[modelName] = modelObj
	return nil
}

func (prj *Project) RemoveModel(ctx context.Context, tenantId string, modelName string) error {
	logs.WithContext(ctx).Debug("RemoveModel - Start")
	if _, ok := prj.Tenants[tenantId]; !ok {
		return errors.New("tenant not found")
	}
	if _, ok := prj.Tenants[tenantId].Models[modelName]; !ok {
		return errors.New("model not found")
	}
	delete(prj.Tenants[tenantId].Models, modelName)
	return nil
}
func (prj *Project) AddTool(ctx context.Context, tenantId string, toolObj tools.Tooling) error {
	logs.WithContext(ctx).Debug("AddTool - Start")
	if prj.Tenants == nil {
		prj.Tenants = make(map[string]TenantConfig)
	}

	if _, ok := prj.Tenants[tenantId]; !ok {
		prj.Tenants[tenantId] = TenantConfig{
			TenantId:     tenantId,
			Models:       make(map[string]models.ModelI),
			Tools:        make(map[string]tools.Tooling),
			Agents:       make(map[string]agents.AgentI),
			VectorStores: make(map[string]vectorstore.VectorStoreI),
		}
	}
	toolNameI, _ := toolObj.GetAttribute(ctx, "tool_name")
	toolName := toolNameI.(string)
	if toolName == "" {
		return errors.New("tool_name cannot be blank")
	}
	prj.Tenants[tenantId].Tools[toolName] = toolObj
	return nil
}

func (prj *Project) RemoveTool(ctx context.Context, tenantId string, toolName string) error {
	logs.WithContext(ctx).Debug("RemoveTool - Start")
	if _, ok := prj.Tenants[tenantId]; !ok {
		return errors.New("tenant not found")
	}
	if _, ok := prj.Tenants[tenantId].Tools[toolName]; !ok {
		return errors.New("tool not found")
	}
	delete(prj.Tenants[tenantId].Tools, toolName)
	return nil
}
func (prj *Project) AddAgent(ctx context.Context, tenantId string, agentObj agents.AgentI) error {
	logs.WithContext(ctx).Debug("AddAgent - Start")
	if prj.Tenants == nil {
		prj.Tenants = make(map[string]TenantConfig)
	}

	if _, ok := prj.Tenants[tenantId]; !ok {
		prj.Tenants[tenantId] = TenantConfig{
			TenantId:     tenantId,
			Models:       make(map[string]models.ModelI),
			Tools:        make(map[string]tools.Tooling),
			Agents:       make(map[string]agents.AgentI),
			VectorStores: make(map[string]vectorstore.VectorStoreI),
		}
	}
	agentNameI, _ := agentObj.GetAttribute(ctx, "agent_name")
	agentName := agentNameI.(string)
	if agentName == "" {
		return errors.New("agent_name cannot be blank")
	}
	oldAgentObj, ok := prj.Tenants[tenantId].Agents[agentName]
	if ok {
		cacheI := oldAgentObj.GetChatMemory()
		if cacheI != nil {
			err := cacheI.SyncPersistence(ctx, oldAgentObj.GetChatMemory())
			if err != nil {
				return err
			}
		}
	}
	err := agentObj.ValidateChatMemory(ctx, prj.ProjectId)
	if err != nil {
		return err
	}
	prj.Tenants[tenantId].Agents[agentName] = agentObj
	return nil
}

func (prj *Project) RemoveAgent(ctx context.Context, tenantId string, agentName string) error {
	logs.WithContext(ctx).Debug("RemoveAgent - Start")
	if _, ok := prj.Tenants[tenantId]; !ok {
		return errors.New("tenant not found")
	}
	if _, ok := prj.Tenants[tenantId].Agents[agentName]; !ok {
		return errors.New("agent not found")
	}
	delete(prj.Tenants[tenantId].Agents, agentName)
	return nil
}

func (prj *Project) AddVectorStore(ctx context.Context, tenantId string, vectorStoreObj vectorstore.VectorStoreI) (isNew bool, updatedVectorStoreObj vectorstore.VectorStoreI, err error) {
	logs.WithContext(ctx).Debug("AddVectorStore - Start")
	isNew = false
	if prj.Tenants == nil {
		prj.Tenants = make(map[string]TenantConfig)
	}

	if _, ok := prj.Tenants[tenantId]; !ok {
		prj.Tenants[tenantId] = TenantConfig{
			TenantId:     tenantId,
			Models:       make(map[string]models.ModelI),
			Tools:        make(map[string]tools.Tooling),
			Agents:       make(map[string]agents.AgentI),
			VectorStores: make(map[string]vectorstore.VectorStoreI),
		}
	}
	vectorStoreName := vectorStoreObj.GetAttribute(ctx, "vector_name")
	if vectorStoreName == "" {
		return isNew, nil, errors.New("vectorstore name cannot be blank")
	}

	bn := vectorStoreObj.GetAttribute(ctx, "bucket_name")
	if !strings.Contains(bn, tenantId) && bn != "" {
		vectorStoreObj.SetAttribute(ctx, "bucket_name", bn+"-"+tenantId)
	}

	if vs, ok := prj.Tenants[tenantId].VectorStores[vectorStoreName]; !ok {
		isNew = true
		prj.Tenants[tenantId].VectorStores[vectorStoreName] = vectorStoreObj
	} else {
		err = vs.UpdateVectorStore(ctx, vectorStoreObj)
		if err != nil {
			return false, nil, err
		}
		vectorStoreObj = vs
	}
	return isNew, vectorStoreObj, nil
}

func (prj *Project) RemoveVectorStore(ctx context.Context, tenantId string, vectorStoreName string) error {
	logs.WithContext(ctx).Debug("RemoveVectorStore - Start")
	if _, ok := prj.Tenants[tenantId]; !ok {
		return errors.New("tenant not found")
	}
	if _, ok := prj.Tenants[tenantId].VectorStores[vectorStoreName]; !ok {
		return errors.New("vectorstore not found")
	}

	delete(prj.Tenants[tenantId].VectorStores, vectorStoreName)
	return nil
}

func (ePrj *ExtendedProject) CompareProject(ctx context.Context, compareProject ExtendedProject) (StoreCompare, error) {
	logs.WithContext(ctx).Debug("CompareProject - Start")
	storeCompare := StoreCompare{}
	storeCompare.CompareVariables(ctx, ePrj.Variables, compareProject.Variables)
	storeCompare.CompareSecretManager(ctx, ePrj.SecretManager, compareProject.SecretManager)

	var diffR utils.DiffReporter
	if !cmp.Equal(ePrj.ProjectSettings, compareProject.ProjectSettings, cmp.Reporter(&diffR)) {
		if storeCompare.MismatchSettings == nil {
			storeCompare.MismatchSettings = make(map[string]interface{})
		}
		storeCompare.MismatchSettings["settings"] = diffR.Output()
	}
	for _, tenantConfig := range ePrj.Tenants {
		for _, mm := range tenantConfig.Models {
			mllmNameI, _ := mm.GetAttribute(ctx, "model_name")
			mllmName := mllmNameI.(string)
			var diffR utils.DiffReporter
			aFound := false
			for _, cm := range compareProject.Tenants[tenantConfig.TenantId].Models {
				cllmNameI, _ := cm.GetAttribute(ctx, "model_name")
				cllmName := cllmNameI.(string)
				if mllmName == cllmName {
					aFound = true
					if !cmp.Equal(mm, cm, cmp.Reporter(&diffR)) {
						if storeCompare.MismatchModel == nil {
							storeCompare.MismatchModel = make(map[string]interface{})
						}
						storeCompare.MismatchModel[mllmName] = diffR.Output()
					}
					break
				}
			}
			if !aFound {
				storeCompare.DeleteModel = append(storeCompare.DeleteModel, mllmName)
			}
			for _, cm := range compareProject.Tenants[tenantConfig.TenantId].Models {
				cllmNameI, _ := cm.GetAttribute(ctx, "model_name")
				cllmName := cllmNameI.(string)
				rFound := false
				for _, mm := range tenantConfig.Models {
					mllmNameI, _ := mm.GetAttribute(ctx, "model_name")
					mllmName := mllmNameI.(string)
					if mllmName == cllmName {
						rFound = true
						break
					}
				}
				if !rFound {
					storeCompare.NewModel = append(storeCompare.NewModel, cllmName)
				}
			}
		}

		//comparing tools
		for _, mt := range tenantConfig.Tools {
			mtoolNameI, _ := mt.GetAttribute(ctx, "tool_name")
			mtoolName := mtoolNameI.(string)
			var diffR utils.DiffReporter
			aFound := false
			for _, ct := range compareProject.Tenants[tenantConfig.TenantId].Tools {
				ctoolNameI, _ := ct.GetAttribute(ctx, "tool_name")
				ctoolName := ctoolNameI.(string)

				if mtoolName == ctoolName {
					aFound = true
					if !cmp.Equal(mt, ct, cmp.Reporter(&diffR)) {
						if storeCompare.MismatchTool == nil {
							storeCompare.MismatchTool = make(map[string]interface{})
						}
						storeCompare.MismatchTool[mtoolName] = diffR.Output()
					}
					break
				}
			}
			if !aFound {
				storeCompare.DeleteTool = append(storeCompare.DeleteTool, mtoolName)
			}
			for _, ct := range compareProject.Tenants[tenantConfig.TenantId].Tools {
				ctoolNameI, _ := ct.GetAttribute(ctx, "tool_name")
				ctoolName := ctoolNameI.(string)
				rFound := false
				for _, mt := range tenantConfig.Tools {
					mtoolNameI, _ := mt.GetAttribute(ctx, "tool_name")
					mtoolName := mtoolNameI.(string)
					if mtoolName == ctoolName {
						rFound = true
						break
					}
				}
				if !rFound {
					storeCompare.NewTool = append(storeCompare.NewTool, ctoolName)
				}
			}
		}

		//comparing agents
		for _, ma := range tenantConfig.Agents {
			magentNameI, _ := ma.GetAttribute(ctx, "agent_name")
			magentName := magentNameI.(string)
			var diffR utils.DiffReporter
			aFound := false
			for _, ca := range compareProject.Tenants[tenantConfig.TenantId].Agents {
				cagentNameI, _ := ca.GetAttribute(ctx, "agent_name")
				cagentName := cagentNameI.(string)
				if magentName == cagentName {
					aFound = true
					if !cmp.Equal(ma, ca, cmp.Reporter(&diffR)) {
						if storeCompare.MismatchAgent == nil {
							storeCompare.MismatchAgent = make(map[string]interface{})
						}
						storeCompare.MismatchAgent[magentName] = diffR.Output()
					}
					break
				}
			}
			if !aFound {
				storeCompare.DeleteAgent = append(storeCompare.DeleteAgent, magentName)
			}
			for _, ca := range compareProject.Tenants[tenantConfig.TenantId].Agents {
				cagentNameI, _ := ca.GetAttribute(ctx, "agent_name")
				cagentName := cagentNameI.(string)
				rFound := false
				for _, ma := range tenantConfig.Agents {
					magentNameI, _ := ma.GetAttribute(ctx, "agent_name")
					magentName := magentNameI.(string)
					if magentName == cagentName {
						rFound = true
						break
					}
				}
				if !rFound {
					storeCompare.NewAgent = append(storeCompare.NewAgent, cagentName)
				}
			}
		}
	}
	return storeCompare, nil
}

func (ePrj *ExtendedProject) UnmarshalJSON(b []byte) error {
	logs.Logger.Info("UnMarshal ExtendedProject - Start")
	ctx := context.Background()
	var ePrjMap map[string]*json.RawMessage
	err := json.Unmarshal(b, &ePrjMap)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	projectId := ""
	if _, ok := ePrjMap["project_id"]; ok {
		if ePrjMap["project_id"] != nil {
			err = json.Unmarshal(*ePrjMap["project_id"], &projectId)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.ProjectId = projectId
		}
	}

	var ps ProjectSettings
	if _, ok := ePrjMap["project_settings"]; ok {
		if ePrjMap["project_settings"] != nil {
			err = json.Unmarshal(*ePrjMap["project_settings"], &ps)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.ProjectSettings = ps
		}
	}

	var vars store.Variables
	if _, ok := ePrjMap["variables"]; ok {
		if ePrjMap["variables"] != nil {
			err = json.Unmarshal(*ePrjMap["variables"], &vars)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			ePrj.Variables = vars
		}
	}

	var smObj map[string]*json.RawMessage
	var smJson *json.RawMessage
	if _, ok := ePrjMap["secret_manager"]; ok {
		if ePrjMap["secret_manager"] != nil {
			err = json.Unmarshal(*ePrjMap["secret_manager"], &smObj)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			err = json.Unmarshal(*ePrjMap["secret_manager"], &smJson)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}

			var smType string
			if _, stOk := smObj["sm_store_type"]; stOk {
				err = json.Unmarshal(*smObj["sm_store_type"], &smType)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}
				smI := sm.GetSm(smType)
				err = smI.MakeFromJson(ctx, smJson)
				if err == nil {
					ePrj.SecretManager = smI
				} else {
					return err
				}
			} else {
				logs.WithContext(ctx).Info("ignoring secret manager as sm_store_type attribute not found")
			}
		} else {
			logs.WithContext(ctx).Info("secret manager attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("secret manager attribute not found in store")
	}

	var tenants map[string]*json.RawMessage
	if _, ok := ePrjMap["tenants"]; ok {
		if ePrjMap["tenants"] != nil {
			err = json.Unmarshal(*ePrjMap["tenants"], &tenants)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return err
			}
			for tenantId, tenantConfigJson := range tenants {
				var tenantConfigObj map[string]*json.RawMessage
				err = json.Unmarshal(*tenantConfigJson, &tenantConfigObj)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return err
				}

				var agent map[string]*json.RawMessage
				if _, ok := tenantConfigObj["agents"]; ok {
					if tenantConfigObj["agents"] != nil {
						err = json.Unmarshal(*tenantConfigObj["agents"], &agent)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
						for _, agentJson := range agent {
							var agentObj map[string]*json.RawMessage
							err = json.Unmarshal(*agentJson, &agentObj)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							var agentType string
							err = json.Unmarshal(*agentObj["agent_type"], &agentType)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							var agentName string
							err = json.Unmarshal(*agentObj["agent_name"], &agentName)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							agentI := agents_factory.GetAgent(agentType)
							err = agentI.MakeFromJson(ctx, agentJson)
							if err == nil {
								if ePrj.Tenants == nil {
									ePrj.Tenants = make(map[string]TenantConfig)
								}
								tenantConfig := ePrj.Tenants[tenantId]
								tenantConfig.Agents[agentName] = agentI
								ePrj.Tenants[tenantId] = tenantConfig
							} else {
								return err
							}
						}
					}
				} else {
					logs.WithContext(ctx).Info("agents attribute is nil")
				}

				var tool map[string]*json.RawMessage
				if _, ok := tenantConfigObj["tools"]; ok {
					if tenantConfigObj["tools"] != nil {
						err = json.Unmarshal(*tenantConfigObj["tools"], &tool)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
						for _, toolJson := range tool {
							var toolObj map[string]*json.RawMessage
							err = json.Unmarshal(*toolJson, &toolObj)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							var toolType string
							err = json.Unmarshal(*toolObj["tool_type"], &toolType)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							var toolName string
							err = json.Unmarshal(*toolObj["tool_name"], &toolName)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							toolI := tools_factory.GetTool(toolType)
							err = toolI.MakeFromJson(ctx, toolJson)
							if err == nil {
								if ePrj.Tenants == nil {
									ePrj.Tenants = make(map[string]TenantConfig)
								}
								tenantConfig := ePrj.Tenants[tenantId]
								tenantConfig.Tools[toolName] = toolI
								ePrj.Tenants[tenantId] = tenantConfig
							} else {
								return err
							}
						}
					}
				} else {
					logs.WithContext(ctx).Info("tools attribute is nil")
				}

				var model map[string]*json.RawMessage
				if _, ok := tenantConfigObj["models"]; ok {
					if tenantConfigObj["models"] != nil {
						err = json.Unmarshal(*tenantConfigObj["models"], &model)
						if err != nil {
							logs.WithContext(ctx).Error(err.Error())
							return err
						}
						for _, modelJson := range model {
							var modelObj map[string]*json.RawMessage
							err = json.Unmarshal(*modelJson, &modelObj)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							var provider string
							err = json.Unmarshal(*modelObj["provider"], &provider)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							var modelName string
							err = json.Unmarshal(*modelObj["model_name"], &modelName)
							if err != nil {
								logs.WithContext(ctx).Error(err.Error())
								return err
							}
							modelI := models.GetModel(provider)
							err = modelI.MakeFromJson(ctx, modelJson)
							if err == nil {
								if ePrj.Tenants == nil {
									ePrj.Tenants = make(map[string]TenantConfig)
								}
								tenantConfig := ePrj.Tenants[tenantId]
								tenantConfig.Models[modelName] = modelI
								ePrj.Tenants[tenantId] = tenantConfig
							} else {
								return err
							}
						}
					}
				} else {
					logs.WithContext(ctx).Info("model attribute is nil")
				}
			}
		} else {
			logs.WithContext(ctx).Info("tenant attribute is nil")
		}
	} else {
		logs.WithContext(ctx).Info("model attribute not found in store")
	}

	return nil
}
