package utiltiy

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
)

var structuredOutputToolActions = []tools.ToolAction{
	{
		ActionName:   "structured_output",
		Description:  "Output the result",
		SystemPrompt: "Output the result",
		OutputSchema: eru_models.JSONSchema{},
		Parameters:   eru_models.JSONSchema{},
	},
}

type StructuredOutputTool struct {
	tools.Tool
}

func (soTool *StructuredOutputTool) GetActionsList() []tools.ActionInfo {
	infos := make([]tools.ActionInfo, len(structuredOutputToolActions))
	for i, action := range structuredOutputToolActions {
		infos[i] = tools.ActionInfo{Name: action.ActionName, Description: action.Description}
	}
	return infos
}

func (soTool *StructuredOutputTool) GetActions() []tools.ToolAction {
	return structuredOutputToolActions
}

func (soTool *StructuredOutputTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &soTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (soTool *StructuredOutputTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("StructuredOutputTool Execute - Start")
	var toolRequest interface{}
	toolRequest = map[string]interface{}{"body": params}
	if paramsObj, ok := params["params"]; ok {
		if paramsMap, ok := paramsObj.(map[string]interface{}); ok {
			toolResult = paramsMap
		}
	}
	if toolResult == nil {
		toolResult = params
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

		hookResult, err := soTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, false, nil
}

func (soTool *StructuredOutputTool) GetSpec() tools.Tooling {
	return soTool
}
func (soTool *StructuredOutputTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	newTool := &StructuredOutputTool{}
	err := json.Unmarshal(toolObjJson, newTool)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return newTool, nil
}

func init() {
	tools.RegisterTool("STRUCTURED_OUTPUT", func() tools.Tooling { return new(StructuredOutputTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       false,
		ToolType:     "STRUCTURED_OUTPUT",
		Category:     "Utility",
		Description:  "AI structured output generation with schema-enforced JSON responses",
		Actions:      []tools.ActionInfo{{Name: "structured_output"}},
		OAuthEnabled: false,
		Icon:         "",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(StructuredOutputTool{}), []string{}),
	})
}
