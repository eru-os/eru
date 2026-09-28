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

const AskUserToolName = "ask_user"

var askUserToolActions = []tools.ToolAction{
	{
		ActionName:   AskUserToolName,
		Description:  "Ask the user clarifying questions",
		SystemPrompt: "Ask the user clarifying questions",
		OutputSchema: eru_models.JSONSchema{},
		Parameters:   eru_models.JSONSchema{},
	},
}

// AskUserToolSchema is the fixed parameter schema the model must fill when it
// needs clarification. It mirrors agents.ClarificationRequest.
func AskUserToolSchema() eru_models.JSONSchema {
	optionSchema := eru_models.JSONSchema{
		Type: "object",
		Properties: map[string]eru_models.JSONSchema{
			"value": {Type: "string", Description: "Stable machine value for the option"},
			"label": {Type: "string", Description: "Human-readable label shown to the user"},
		},
		Required: []string{"value", "label"},
	}
	questionSchema := eru_models.JSONSchema{
		Type: "object",
		Properties: map[string]eru_models.JSONSchema{
			"id":       {Type: "string", Description: "Unique id for the question (e.g. q1). Assigned automatically if omitted."},
			"question": {Type: "string", Description: "The clarifying question to ask the user"},
			"options": {
				Type:        "array",
				Description: "2-4 concrete, mutually exclusive choices. Omit only when free text is the sole sensible answer.",
				Items:       &optionSchema,
			},
			"multi_select":    {Type: "boolean", Description: "true if more than one option may be selected"},
			"free_text_label": {Type: "string", Description: "Label for the free-text input (e.g. 'Something else'). The user can always type an answer; this only names the box."},
			"required":        {Type: "boolean", Description: "true if the user must answer this question"},
		},
		Required: []string{"question"},
	}
	return eru_models.JSONSchema{
		Type: "object",
		Properties: map[string]eru_models.JSONSchema{
			"prompt": {Type: "string", Description: "Optional context line shown above the questions"},
			"questions": {
				Type:        "array",
				Description: "One or more clarification questions",
				Items:       &questionSchema,
			},
		},
		Required: []string{"questions"},
	}
}

type AskUserTool struct {
	tools.Tool
}

func (auTool *AskUserTool) GetActionsList() []tools.ActionInfo {
	infos := make([]tools.ActionInfo, len(askUserToolActions))
	for i, action := range askUserToolActions {
		infos[i] = tools.ActionInfo{Name: action.ActionName, Description: action.Description}
	}
	return infos
}

func (auTool *AskUserTool) GetActions() []tools.ToolAction {
	return askUserToolActions
}

func (auTool *AskUserTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("AskUserTool MakeFromJson - Start")
	err := json.Unmarshal(*rj, &auTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (auTool *AskUserTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("AskUserTool Execute - Start")

	toolResult, err = normalizeClarificationRequest(params)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, false, err
	}

	gm := server.GetGlobalGoroutineManager(ctx)
	gm.SafeGoWithRestartBehavior("tool-post-execute-hook", func(bgCtx context.Context) {
		bgCtx = tools.CopyClaims(ctx, bgCtx)
		efurl := ctx.Value(tools.EruFuncBaseUrlKey)
		if efurl == nil {
			logs.WithContext(ctx).Error("erufuncbaseurl not found in context")
			return
		}
		efurlString, ok := efurl.(string)
		if !ok {
			logs.WithContext(ctx).Error("erufuncbaseurl is not a string")
			return
		}
		bgCtx = context.WithValue(bgCtx, tools.EruFuncBaseUrlKey, efurlString)

		body := make(map[string]interface{})
		body["request"] = map[string]interface{}{"body": params}
		body["response"] = toolResult
		body["tenant_id"] = tenantId
		body["project_id"] = projectId
		if params["metadata"] != nil {
			body["metadata"] = params["metadata"]
		}

		hookResult, hookErr := auTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if hookErr != nil {
			logs.WithContext(bgCtx).Error(hookErr.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, false, nil
}

type askUserOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type askUserQuestion struct {
	Id            string          `json:"id"`
	Question      string          `json:"question"`
	Options       []askUserOption `json:"options,omitempty"`
	MultiSelect   bool            `json:"multi_select,omitempty"`
	AllowFreeText bool            `json:"allow_free_text"`
	FreeTextLabel string          `json:"free_text_label,omitempty"`
	Required      bool            `json:"required,omitempty"`
}

type askUserRequest struct {
	Prompt    string            `json:"prompt,omitempty"`
	Questions []askUserQuestion `json:"questions"`
}

// defaultFreeTextLabel names the free-text box when the asker did not, so the
// client always has something to render it with.
const defaultFreeTextLabel = "Something else - let me describe it"

// normalizeClarificationRequest validates and fills in defaults: at least one
// question, an id per question, and free text on every question.
func normalizeClarificationRequest(params map[string]interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var req askUserRequest
	if err := json.Unmarshal(b, &req); err != nil {
		return nil, fmt.Errorf("invalid ask_user input: %w", err)
	}
	if len(req.Questions) == 0 {
		return nil, errors.New("ask_user requires at least one question")
	}
	for i := range req.Questions {
		q := &req.Questions[i]
		if q.Question == "" {
			return nil, fmt.Errorf("ask_user question %d has empty text", i+1)
		}
		if q.Id == "" {
			q.Id = fmt.Sprintf("q%d", i+1)
		}
		// The user can always type their own answer. Whether the offered options
		// cover the situation is not something the asker can judge: options are
		// written from what the agent believes is possible, so the case it has not
		// thought of is exactly the one the user needs to be able to state - and
		// that is the case where the option list is most likely to be wrong.
		q.AllowFreeText = true
		if q.FreeTextLabel == "" {
			q.FreeTextLabel = defaultFreeTextLabel
		}
	}

	out := map[string]interface{}{}
	nb, _ := json.Marshal(req)
	if err := json.Unmarshal(nb, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (auTool *AskUserTool) GetSpec() tools.Tooling {
	return auTool
}

func (auTool *AskUserTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	newTool := &AskUserTool{}
	err := json.Unmarshal(toolObjJson, newTool)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return newTool, nil
}

func init() {
	tools.RegisterTool("ASK_USER", func() tools.Tooling { return new(AskUserTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       false,
		ToolType:     "AskUser",
		Category:     "Utility",
		Description:  "Human-in-the-loop clarification: agent asks the user multiple-choice questions with a free-text fallback",
		Actions:      []tools.ActionInfo{{Name: AskUserToolName}},
		OAuthEnabled: false,
		Icon:         "",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(AskUserTool{}), []string{}),
	})
}
