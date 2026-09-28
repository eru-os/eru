package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	db "github.com/eru-os/eru/eru-db/db"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	scheduler "github.com/eru-os/eru/eru-scheduler/scheduler"
	utils "github.com/eru-os/eru/eru-utils"
	gojsonschema "github.com/xeipuuv/gojsonschema"
)

type ToolHooks struct {
	CLBK string            `json:"clbk,omitempty"` //callback
	POEX map[string]string `json:"poex,omitempty"` //post execute
	ARSU string            `json:"arsu,omitempty"` //auto renew subscription
	ARRT string            `json:"arrt,omitempty"` //auto renew refresh token
}

type Tool struct {
	ToolType     string `json:"tool_type" eru:"required"`
	ToolName     string `json:"tool_name" eru:"required"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"`
	// Deprecated: tool-level OutputSchema is no longer used (no live readers; ValidateOutput is uncalled).
	// Describe an action's result on ToolAction.OutputSchema instead.
	OutputSchema    eru_models.JSONSchema `json:"output_schema"`
	Parameters      eru_models.JSONSchema `json:"parameters"`
	ToolAction      ToolAction            `json:"-" eru:"optional"`
	Hooks           ToolHooks             `json:"hooks"`
	HookAsyncEvent  string                `json:"hook_async_event"`
	Scheduler       scheduler.SchedulerI  `json:"-"`
	ToolDb          db.DbI                `json:"-"`
	CallbackBaseUrl string                `json:"callback_base_url"`
	//Inputs       []ToolInput           `json:"inputs"`
}

type ToolCallback struct {
	ResponseContentType string `json:"response_content_type"`
}

type ActionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ToolAction struct {
	ActionName    string                       `json:"action_name" eru:"required"`
	Description   string                       `json:"description"`
	SystemPrompt  string                       `json:"system_prompt"`
	OutputSchema  eru_models.JSONSchema        `json:"output_schema"`
	Parameters    eru_models.JSONSchema        `json:"parameters"`
	GetParameters func() eru_models.JSONSchema `json:"-"`
}

type ToolInput struct {
	FieldName  string `json:"field_name" eru:"required"`
	FieldValue string `json:"field_value"`
}

type ToolInputFields struct {
	FieldId          string `json:"field_id" eru:"required"`
	FieldName        string `json:"field_name" eru:"required"`
	FieldLabel       string `json:"field_label"`
	FieldType        string `json:"field_type" eru:"required"`
	FieldDescription string `json:"field_description"`
	FieldRequired    bool   `json:"field_required"`
}
type contextKey string

const (
	EruFuncBaseUrlKey       contextKey = "Erufuncbaseurl"
	eruFuncBaseUrlKey                  = EruFuncBaseUrlKey
	RequestAuthorizationKey contextKey = "RequestAuthorization"
	RequestHeadersKey       contextKey = "RequestHeaders"
)

type Tooling interface {
	GetSpec() Tooling
	GetBytes(ctx context.Context) ([]byte, error)
	BytesToTool(ctx context.Context, toolObjJson []byte) (Tooling, error)
	GetActionsList() []ActionInfo
	GetActions() []ToolAction
	ValidateAction(ctx context.Context, actionName string, realTool Tooling) (err error)
	SetPrivateAttributes(ctx context.Context, realTool Tooling) (err error)
	GetInputFields() []ToolInputFields
	Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (map[string]interface{}, bool, error)
	Callback(ctx context.Context, projectId string, tenantId string, actionName string, body map[string]interface{}, params map[string][]string) (callbackResult interface{}, persistStore bool, err error)
	ValidateOutput(ctx context.Context, output json.RawMessage) error
	MakeFromJson(ctx context.Context, rj *json.RawMessage) error
	GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error)
	SetAttribute(ctx context.Context, attributeName string, attributeValue interface{}) (err error)
	GetToolCallback() ToolCallback
	GetToolCbUrl(projectId string, tenantId string) string
	ExecuteHook(ctx context.Context, hookType string, actionName string, projectId string, tenantId string, body map[string]interface{}, params map[string][]string) (callbackResult interface{}, err error)
	GetToolDb() db.DbI
	SetToolDb(db.DbI)
	SetScheduler(scheduler.SchedulerI)
	SaveTenantSecret(ctx context.Context, projectId string, tenantId string, secretName string, secretValue string) (err error)
	SetToolAction(actionName string)
	GetParameters() eru_models.JSONSchema
}

func (tool *Tool) SetToolAction(actionName string) {
	tool.ToolAction = ToolAction{}
}

func (tool *Tool) GetParameters() eru_models.JSONSchema {
	if tool.ToolAction.GetParameters == nil {
		return tool.Parameters
	}
	return tool.ToolAction.GetParameters()
}

func (tool *Tool) GetBytes(ctx context.Context) ([]byte, error) {
	toolJson, err := json.Marshal(tool)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}

func (tool *Tool) BytesToTool(ctx context.Context, toolObjJson []byte) (Tooling, error) {
	iCloneI := reflect.New(reflect.TypeOf(tool))
	toolObjCloneErr := json.Unmarshal(toolObjJson, iCloneI.Interface())
	if toolObjCloneErr != nil {
		err := logs.Err(ctx, toolObjCloneErr, "error while cloning toolObj(unmarshal)")
		return nil, err
	}
	return iCloneI.Elem().Interface().(Tooling), nil
}

func (tool *Tool) GetToolDb() db.DbI {
	return tool.ToolDb
}

func (tool *Tool) SetToolDb(db db.DbI) {
	tool.ToolDb = db
}

func (tool *Tool) SetScheduler(s scheduler.SchedulerI) {
	tool.Scheduler = s
}

func (tool *Tool) GetToolCallback() ToolCallback {
	return ToolCallback{
		ResponseContentType: "application/json",
	}
}
func (tool *Tool) Callback(ctx context.Context, projectId string, tenantId string, actionName string, body map[string]interface{}, params map[string][]string) (callbackResult interface{}, persistStore bool, err error) {
	err = logs.Err(ctx, fmt.Errorf("callback not implemented"), "")
	return nil, false, err
}

func (tool *Tool) GetToolCbUrl(projectId string, tenantId string) string {
	return ""
}

func (tool *Tool) SetPrivateAttributes(ctx context.Context, realTool Tooling) (err error) {
	return nil
}

func (tool *Tool) GetActionsList() []ActionInfo {
	return []ActionInfo{}
}

func (tool *Tool) GetActions() []ToolAction {
	return []ToolAction{}
}

func (tool *Tool) GetInputFields() []ToolInputFields {
	fields := []ToolInputFields{}
	return fields
}

func (tool *Tool) ValidateOutput(ctx context.Context, output json.RawMessage) error {
	schema := gojsonschema.NewGoLoader(tool.OutputSchema)
	document := gojsonschema.NewBytesLoader(output)

	result, err := gojsonschema.Validate(schema, document)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("schema validation error: %v", err))
		return err
	}

	if !result.Valid() {
		var errors []string
		for _, err := range result.Errors() {
			errors = append(errors, err.String())
		}
		err = fmt.Errorf("schema validation error: %v", errors)
		logs.WithContext(ctx).Error(err.Error())
		return err
	}

	return nil
}

func (tool *Tool) GetSpec() Tooling {
	return tool
}

func (tool *Tool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (map[string]interface{}, bool, error) {
	err := errors.New("Execute Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return nil, false, err
}

func (tool *Tool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &tool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (tool *Tool) GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "tool_name":
		return tool.ToolName, nil
	case "tool_type":
		return tool.ToolType, nil
	case "system_prompt":
		return tool.SystemPrompt, nil
	case "output_schema":
		return tool.OutputSchema, nil
	case "parameters":
		return tool.Parameters, nil
	case "description":
		return tool.Description, nil
	default:
		err := errors.New("attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}
func (tool *Tool) SetAttribute(ctx context.Context, attributeName string, attributeValue interface{}) (err error) {
	switch attributeName {
	case "tool_name":
		tool.ToolName = attributeValue.(string)
	case "tool_type":
		tool.ToolType = attributeValue.(string)
	case "system_prompt":
		tool.SystemPrompt = attributeValue.(string)
	case "output_schema":
		tool.OutputSchema = attributeValue.(eru_models.JSONSchema)
	case "parameters":
		tool.Parameters = attributeValue.(eru_models.JSONSchema)
	case "description":
		tool.Description = attributeValue.(string)
	default:
		err = errors.New("attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (tool *Tool) ValidateAction(ctx context.Context, actionName string, realTool Tooling) (err error) {
	logs.WithContext(ctx).Info("ValidateAction - Start")
	actions := realTool.GetActionsList()
	logs.WithContext(ctx).Info(fmt.Sprintf("Actions: %v", actions))
	logs.WithContext(ctx).Info(fmt.Sprintf("Action Name: %v", actionName))

	if len(actions) == 0 && actionName == "" {
		//if no actions are defined, and no action name is provided, return nil
		return
	}
	found := false
	for _, a := range actions {
		if a.Name == actionName {
			found = true
			break
		}
	}
	if !found {
		err = errors.New("action " + actionName + " not found")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	//TODO - add param validation for Action
	return
}

func (tool *Tool) ExecuteHook(ctx context.Context, hookType string, actionName string, projectId string, tenantId string, body map[string]interface{}, params map[string][]string) (callbackResult interface{}, err error) {
	logs.WithContext(ctx).Info("ExecuteCallbackHook - Start")
	hookFunction := ""
	switch hookType {
	case "clbk":
		hookFunction = tool.Hooks.CLBK
	case "poex":
		if tool.Hooks.POEX != nil {
			hookFunction = tool.Hooks.POEX[actionName]
		}
	case "arsu":
		hookFunction = tool.Hooks.ARSU
	case "arrt":
		hookFunction = tool.Hooks.ARRT
	default:
		err = errors.New("callback type not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	if hookFunction != "" {
		paramMap := make(map[string]string)
		for k, v := range params {
			paramMap[k] = strings.Join(v, ",")
		}
		asyncEvent := ""
		if tool.HookAsyncEvent != "" {
			asyncEvent = fmt.Sprint("/", tool.HookAsyncEvent)
		}
		efurl := ctx.Value(EruFuncBaseUrlKey)
		if efurl == nil {
			err = errors.New("erufuncbaseurl not found in context")
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
		efurlString, ok := efurl.(string)
		if !ok {
			err = errors.New("erufuncbaseurl is not a string")
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
		url := fmt.Sprint(efurlString, "/", projectId, "/func/", hookFunction, asyncEvent)
		logs.WithContext(ctx).Info(fmt.Sprintf("url: %v", url))
		headers := http.Header{}
		headers.Add("Content-Type", "application/json")
		if claimsKey, claims, hasClaims := ClaimsHeader(ctx); hasClaims {
			headers.Add(claimsKey, claims)
		}
		res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, nil, nil, paramMap, body)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		}
		logs.WithContext(ctx).Info(fmt.Sprintf("res: %v", res))
		return res, nil
	}
	return nil, nil
}

func (tool *Tool) ExecuteFunction(ctx context.Context, projectId string, tenantId string, functionName string, body map[string]interface{}, params map[string][]string) (functionResult interface{}, err error) {
	logs.WithContext(ctx).Info("ExecuteFunction - Start")
	paramMap := make(map[string]string)
	for k, v := range params {
		paramMap[k] = strings.Join(v, ",")
	}
	efurl := ctx.Value(eruFuncBaseUrlKey)
	if efurl == nil {
		err = errors.New("erufuncbaseurl not found in context")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	efurlString, ok := efurl.(string)
	if !ok {
		err = errors.New("erufuncbaseurl is not a string")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	url := fmt.Sprint(efurlString, "/", projectId, "/func/", functionName)
	logs.WithContext(ctx).Info(fmt.Sprintf("url: %v", url))
	headers := http.Header{}
	headers.Add("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, nil, nil, paramMap, body)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("res: %v", res))
	return res, nil
}

func (tool *Tool) SaveTenantSecret(ctx context.Context, projectId string, tenantId string, secretName string, secretValue string) (err error) {
	logs.WithContext(ctx).Debug("saveTenantSecret Execute - Start")
	eruaiport := ctx.Value("eruaiport").(string)
	url := fmt.Sprint("http://localhost:", eruaiport, "/store/", projectId, "/", tenantId, "/sm/set")
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	secretPost := make(map[string]interface{})
	secretInnerPost := make(map[string]interface{})
	secretInnerPost[secretName] = secretValue
	secretPost["secret_value"] = secretInnerPost
	_, _, _, _, err = utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, secretPost)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
