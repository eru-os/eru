package vectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	server "github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
	vectorstore "github.com/eru-os/eru/eru-vectorstore/vectorstore"
)

type VectorstoreAccount struct {
	tools.Tool
	VectorStore     vectorstore.VectorStoreI `json:"-"`
	VectorStoreName string                   `json:"vectorstore_name"`
}

const (
	SaveVectors   = "save_vectors"
	SearchVectors = "search_vectors"
)

var vectorstoreAccountActions = []tools.ToolAction{
	{
		ActionName:   SaveVectors,
		Description:  "Train the model",
		SystemPrompt: "Train the model",
		OutputSchema: eru_models.JSONSchema{},
		Parameters:   eru_models.JSONSchema{},
		GetParameters: func() eru_models.JSONSchema {
			return utils.StructToJSONSchema(reflect.TypeOf(vectorstore.VectorRecords{}), []string{})
		},
	},
	{
		ActionName:   SearchVectors,
		Description:  "Search the model",
		SystemPrompt: "Search the model",
		OutputSchema: eru_models.JSONSchema{},
		Parameters:   eru_models.JSONSchema{},
		GetParameters: func() eru_models.JSONSchema {
			return utils.StructToJSONSchema(reflect.TypeOf(vectorstore.VectorRecordsSearch{}), []string{})
		},
	},
}

func (vectorstoreAccount *VectorstoreAccount) GetActionsList() []tools.ActionInfo {
	infos := make([]tools.ActionInfo, len(vectorstoreAccountActions))
	for i, action := range vectorstoreAccountActions {
		infos[i] = tools.ActionInfo{Name: action.ActionName, Description: action.Description}
	}
	return infos
}

func (vectorstoreAccount *VectorstoreAccount) GetActions() []tools.ToolAction {
	return vectorstoreAccountActions
}

func (vectorstoreAccount *VectorstoreAccount) GetSpec() tools.Tooling {
	return vectorstoreAccount
}

func (vectorstoreAccount *VectorstoreAccount) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &vectorstoreAccount)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (vectorstoreAccount *VectorstoreAccount) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("vectorstoreAccount Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case SaveVectors:
		toolResult, toolRequest, persistStore, err = vectorstoreAccount.SaveVectors(ctx, params)
	case SearchVectors:
		toolResult, toolRequest, persistStore, err = vectorstoreAccount.SearchVectors(ctx, projectId, params)
	default:
		return nil, false, fmt.Errorf("action %s not found", actionName)
	}

	gm := server.GetGlobalGoroutineManager(ctx)
	gm.SafeGoWithRestartBehavior("tool-post-execute-hook", func(bgCtx context.Context) {
		bgCtx = tools.CopyClaims(ctx, bgCtx)
		efurl := ctx.Value(tools.EruFuncBaseUrlKey)
		if efurl == nil {
			err = errors.New("erufuncbaseurl not found in context")
			logs.WithContext(ctx).Error(err.Error())
			return
		}
		efurlString, ok := efurl.(string)
		if !ok {
			err = errors.New("erufuncbaseurl is not a string")
			logs.WithContext(ctx).Error(err.Error())
			return
		} else {
			bgCtx = context.WithValue(bgCtx, tools.EruFuncBaseUrlKey, efurlString)
		}

		body := make(map[string]interface{})
		if toolRequest != nil {
			body["request"] = toolRequest
		}
		if toolResult != nil {
			body["response"] = toolResult
		}
		body["tenant_id"] = tenantId
		body["project_id"] = projectId

		if params["metadata"] != nil {
			body["metadata"] = params["metadata"]
		}

		hookResult, err := vectorstoreAccount.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (vectorstoreAccount *VectorstoreAccount) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	newTool := &VectorstoreAccount{}
	err := json.Unmarshal(toolObjJson, newTool)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return newTool, nil
}

func (vectorstoreAccount *VectorstoreAccount) SaveVectors(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("vectorstoreAccount Train - Start")

	if vectorstoreAccount.VectorStore == nil {
		return nil, nil, false, fmt.Errorf("vectorstore not found")
	}

	vectorRecordsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, nil, false, fmt.Errorf("error marshalling vectorrecords: %w", err)
	}
	vectorRecords := vectorstore.VectorRecords{}
	err = json.Unmarshal(vectorRecordsBytes, &vectorRecords)
	if err != nil {
		return nil, nil, false, fmt.Errorf("error unmarshalling vectorrecords: %w", err)
	}
	err = vectorstoreAccount.VectorStore.SaveVectors(ctx, vectorRecords)
	if err != nil {
		return nil, nil, false, fmt.Errorf("error saving vectors: %w", err)
	}
	toolResult = map[string]interface{}{
		"message": "vectors saved successfully",
	}
	return toolResult, map[string]interface{}{"body": params}, false, nil
}
func (vectorstoreAccount *VectorstoreAccount) SearchVectors(ctx context.Context, projectId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("vectorstoreAccount Search - Start")
	if vectorstoreAccount.VectorStore == nil {
		return nil, nil, false, fmt.Errorf("vectorstore not found")
	}
	vectorSearchBytes, err := json.Marshal(params)
	if err != nil {
		return nil, nil, false, fmt.Errorf("error marshalling vectorrecords: %w", err)
	}
	vectorSearch := vectorstore.VectorRecordsSearch{}
	err = json.Unmarshal(vectorSearchBytes, &vectorSearch)
	if err != nil {
		return nil, nil, false, fmt.Errorf("error unmarshalling vectorrecords: %w", err)
	}

	combined := vectorstore.VectorResults{Records: []vectorstore.VectorResult{}}
	anySuccess := false
	var lastErr error

	tenantNamespace := vectorSearch.Namespace
	tenantResults, tErr := vectorstoreAccount.VectorStore.SearchVectors(ctx, vectorSearch)
	if tErr != nil {
		lastErr = tErr
		logs.WithContext(ctx).Error(fmt.Sprintf("error searching vectors in namespace %s (ignored, trying project fallback): %v", tenantNamespace, tErr))
	} else {
		anySuccess = true
		combined.Records = append(combined.Records, tenantResults.Records...)
		combined.Usage = tenantResults.Usage
	}

	if projectId != "" && projectId != tenantNamespace {
		projectSearch := vectorSearch
		projectSearch.Namespace = projectId
		projectResults, pErr := vectorstoreAccount.VectorStore.SearchVectors(ctx, projectSearch)
		if pErr != nil {
			lastErr = pErr
			logs.WithContext(ctx).Error(fmt.Sprintf("error searching vectors in project namespace %s (ignored): %v", projectId, pErr))
		} else {
			anySuccess = true
			combined.Records = append(combined.Records, projectResults.Records...)
			if combined.Usage == nil {
				combined.Usage = projectResults.Usage
			}
		}
	}

	if !anySuccess {
		return nil, nil, false, fmt.Errorf("error searching vectors: %w", lastErr)
	}

	toolResult = map[string]interface{}{
		"vector_search": combined,
	}

	return toolResult, map[string]interface{}{"body": params}, false, nil
}
func (vectorstoreAccount *VectorstoreAccount) GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "tool_name":
		return vectorstoreAccount.ToolName, nil
	case "tool_type":
		return vectorstoreAccount.ToolType, nil
	case "system_prompt":
		return vectorstoreAccount.SystemPrompt, nil
	case "output_schema":
		return vectorstoreAccount.OutputSchema, nil
	case "parameters":
		return vectorstoreAccount.Parameters, nil
	case "description":
		return vectorstoreAccount.Description, nil
	case "vectorstore_name":
		return vectorstoreAccount.VectorStoreName, nil
	default:
		err := errors.New("attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}
func (vectorstoreAccount *VectorstoreAccount) SetAttribute(ctx context.Context, attributeName string, attributeValue interface{}) (err error) {
	switch attributeName {
	case "tool_name":
		vectorstoreAccount.ToolName = attributeValue.(string)
	case "tool_type":
		vectorstoreAccount.ToolType = attributeValue.(string)
	case "system_prompt":
		vectorstoreAccount.SystemPrompt = attributeValue.(string)
	case "output_schema":
		vectorstoreAccount.OutputSchema = attributeValue.(eru_models.JSONSchema)
	case "parameters":
		vectorstoreAccount.Parameters = attributeValue.(eru_models.JSONSchema)
	case "description":
		vectorstoreAccount.Description = attributeValue.(string)
	case "vectorstore":
		vectorstoreAccount.VectorStore = attributeValue.(vectorstore.VectorStoreI)
	default:
		err = errors.New("attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
func (vectorstoreAccount *VectorstoreAccount) GetBytes(ctx context.Context) ([]byte, error) {

	toolJson, err := json.Marshal(vectorstoreAccount)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}

func (vectorstoreAccount *VectorstoreAccount) SetToolAction(actionName string) {
	for _, action := range vectorstoreAccountActions {
		if action.ActionName == actionName {
			vectorstoreAccount.ToolAction = action
			return
		}
	}
	vectorstoreAccount.ToolAction = tools.ToolAction{}
}

/* func (vectorstoreAccount *VectorstoreAccount) GetParameters() eru_models.JSONSchema {
	return vectorstoreAccount.ToolAction.GetParameters()
} */

func init() {
	tools.RegisterTool("VECTORSTORE", func() tools.Tooling { return new(VectorstoreAccount) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:      false,
		ToolType:    "Vectorstore",
		Category:    "AI",
		Description: "Vector store operations for semantic search and AI embedding storage",
		Actions: func() []tools.ActionInfo {
			infos := make([]tools.ActionInfo, len(vectorstoreAccountActions))
			for i, a := range vectorstoreAccountActions {
				infos[i] = tools.ActionInfo{Name: a.ActionName, Description: a.Description}
			}
			return infos
		}(),
		OAuthEnabled: false,
		Icon:         "",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(VectorstoreAccount{}), []string{}),
	})
}
