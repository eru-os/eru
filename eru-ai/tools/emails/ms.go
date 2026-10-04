package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	"github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	INSERT_FUNC_ASYNC = "insert into eruai_cb_msemail (project_id, tenant_id, request_body, request_params) values ($1, $2, $3, $4)"
)

type MsNotificationCollection struct {
	Value []MsNotification `json:"value"`
}

type MsNotification struct {
	ChangeType                     string         `json:"changeType"`
	ClientState                    string         `json:"clientState"`
	Resource                       string         `json:"resource"`
	ResourceData                   MsResourceData `json:"resourceData"`
	SubscriptionId                 string         `json:"subscriptionId"`
	Id                             string         `json:"id"`
	LifecycleEvent                 string         `json:"lifecycleEvent"`
	SubscriptionExpirationDateTime string         `json:"subscriptionExpirationDateTime"`
	TenantId                       string         `json:"tenantId"`
}

type MsTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IdToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExtExpiresIn int    `json:"ext_expires_in"`
}

type MsResourceData struct {
	Id        string `json:"id"`
	ODataId   string `json:"@odata.id"`
	ODataType string `json:"@odata.type"`
	ODataEtag string `json:"@odata.etag"`
}

type MsEmailTool struct {
	tools.Tool
	EmailAccount EmailAccount `json:"email_account"`
	AuthName     string       `json:"auth_name"`
}
type msEmailToolWithToken struct {
	tools.Tool
	EmailAccount emailAccountWithToken
	AuthName     string
}

const (
	BaseUrl = "https://graph.microsoft.com"
)

func (msEmailTool *MsEmailTool) GetActionsList() []tools.ActionInfo {
	return []tools.ActionInfo{
		{Name: ReadEmail},
		{Name: SendEmail},
		{Name: SubscribeEmail},
		{Name: ReadMessage},
		{Name: Callback},
		{Name: GetSsoUrl},
		{Name: Login},
		{Name: RenewToken},
		{Name: RenewSubscription},
		{Name: StopAutoRenew},
		{Name: StopSubscription},
		{Name: ReadConversation},
	}
}

func (msEmailTool *MsEmailTool) GetSpec() tools.Tooling {
	return msEmailTool
}

func (msEmailTool *MsEmailTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &msEmailTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (msEmailTool *MsEmailTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("MsEmailTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case ReadEmail:
		toolResult, toolRequest, persistStore, err = msEmailTool.ReadEmail(ctx, params)
	case SendEmail:
		toolResult, toolRequest, persistStore, err = msEmailTool.SendEmail(ctx, params)
	case SubscribeEmail:
		toolResult, toolRequest, persistStore, err = msEmailTool.SubscribeEmail(ctx, projectId, tenantId, params, "", false)
	case ReadMessage:
		toolResult, toolRequest, persistStore, err = msEmailTool.ReadMessage(ctx, params)
	case GetSsoUrl:
		toolResult, toolRequest, persistStore, err = msEmailTool.GetSsoUrl(ctx, projectId, tenantId, params)
	case Login:
		toolResult, toolRequest, persistStore, err = msEmailTool.Login(ctx, projectId, tenantId, params, "")
	case RenewToken:
		toolResult, toolRequest, persistStore, err = msEmailTool.RenewToken(ctx, projectId, tenantId, params)
	case RenewSubscription:
		toolResult, toolRequest, persistStore, err = msEmailTool.RenewSubscription(ctx, projectId, tenantId, params)
	case StopAutoRenew:
		toolResult, toolRequest, persistStore, err = msEmailTool.StopAutoRenew(ctx, projectId, tenantId, params)
	case StopSubscription:
		toolResult, toolRequest, persistStore, err = msEmailTool.StopSubscription(ctx, projectId, tenantId, params)
	case ReadConversation:
		toolResult, toolRequest, persistStore, err = msEmailTool.ReadConversation(ctx, params)
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

		hookResult, err := msEmailTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (msEmailTool *MsEmailTool) ReadEmail(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ReadEmail Execute - Start")
	url := fmt.Sprint(BaseUrl, "/v1.0/me/messages")
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", msEmailTool.EmailAccount.AccessToken))
	headers.Set("Content-Type", "application/json")
	strParams := make(map[string]string)
	for k, v := range params {
		strParams[k] = fmt.Sprint(v)
	}
	logs.WithContext(ctx).Info(fmt.Sprint("strParams: ", strParams))
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, strParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	_ = url
	toolResult = make(map[string]interface{})
	toolResult["emails"] = res
	return toolResult, map[string]interface{}{"query": strParams}, false, nil
}

func (msEmailTool *MsEmailTool) ReadMessage(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ReadMessage Execute - Start")
	messageId := params["message_id"].(string)
	url := fmt.Sprint(BaseUrl, "/v1.0/me/messages/", messageId, "?$expand=attachments")
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", msEmailTool.EmailAccount.AccessToken))
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	toolResultOk := false
	toolResult, toolResultOk = res.(map[string]interface{})
	if !toolResultOk {
		err = errors.New("toolResult is not a map")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	return toolResult, map[string]interface{}{"query": map[string]string{"message_id": messageId}}, false, nil
}

func (msEmailTool *MsEmailTool) SubscribeEmail(ctx context.Context, projectId string, tenantId string, params map[string]interface{}, subscriptionId string, unsubscribe bool) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SubscribeEmail Execute - Start")
	url := fmt.Sprint(BaseUrl, "/v1.0/subscriptions", subscriptionId)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	// Calculate expiration time (current time + 24 hours)
	expirationTime := time.Now().UTC().Add(50 * time.Hour).Format(time.RFC3339)

	subPost := make(map[string]interface{})
	httpMethod := http.MethodPost
	if unsubscribe {
		httpMethod = http.MethodDelete
		subPost = nil
		subscriptionId = ""
	} else if subscriptionId == "" {
		subPost = map[string]interface{}{
			"changeType":         "created",
			"notificationUrl":    msEmailTool.GetToolCbUrl(projectId, tenantId),
			"resource":           "me/mailFolders('Inbox')/messages",
			"expirationDateTime": expirationTime,
			"clientState":        tenantId,
		}
	} else {
		httpMethod = http.MethodPatch
		subPost = map[string]interface{}{
			"expirationDateTime": expirationTime,
		}
	}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", msEmailTool.EmailAccount.AccessToken))
	res, _, _, _, err := utils.CallHttp(ctx, httpMethod, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, subPost)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(res))

	if unsubscribe {
		jobName := fmt.Sprint(msEmailTool.ToolName, "_", msEmailTool.Tool.Hooks.ARSU, "_", tenantId)
		err = msEmailTool.Scheduler.Unschedule(ctx, "", jobName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			//continue even if unschedule fails
		}
		toolResult = make(map[string]interface{})
		msEmailTool.EmailAccount.SubscriptionId = ""
		msEmailTool.EmailAccount.SubscriptionExpirationDateTime = ""
		persistStore = true
		toolResult["unsubscription_status"] = "success"
		return toolResult, map[string]interface{}{"body": subPost}, persistStore, nil
	}

	subResultOk := false
	subResult, subResultOk := res.(map[string]interface{})
	if !subResultOk {
		err = errors.New("subResult is not a map")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	subcriptionId, subcriptionIdOk := subResult["id"]
	if !subcriptionIdOk {
		err = errors.New("subcription id not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	msEmailTool.EmailAccount.SubscriptionId = subcriptionId.(string)
	msEmailTool.EmailAccount.SubscriptionExpirationDateTime = expirationTime
	persistStore = true

	if msEmailTool.Tool.Hooks.ARSU != "" {
		hookBody := map[string]interface{}{
			"Vars": map[string]interface{}{
				"Body": map[string]interface{}{
					"tool_name": msEmailTool.ToolName,
					"tenant_id": tenantId,
				},
				"OrgBody": map[string]interface{}{
					"tool_name": msEmailTool.ToolName,
					"tenant_id": tenantId,
				},
			},
			"ReqVars": map[string]interface{}{},
			"ResVars": map[string]interface{}{},
		}
		hookBodyBytes, err := json.Marshal(hookBody)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, nil, persistStore, err
		}
		jobName := fmt.Sprint(msEmailTool.ToolName, "_", msEmailTool.Tool.Hooks.ARSU, "_", tenantId)
		err = msEmailTool.Scheduler.Unschedule(ctx, "", jobName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			//continue even if unschedule fails
		}
		cronStr := utils.GetCronStr(ctx, time.Now().UTC().Add(48*time.Hour))
		schedulerCommand := fmt.Sprint("CALL schedule_procedure('", msEmailTool.Tool.Hooks.ARSU, "','", string(hookBodyBytes), "','", msEmailTool.Scheduler.GetSchedulerName(), "')")
		jobId, err := msEmailTool.Scheduler.Schedule(ctx, jobName, schedulerCommand, cronStr)
		if err != nil {
			return nil, nil, persistStore, err
		}
		logs.WithContext(ctx).Info(fmt.Sprint("jobId: ", jobId))
	}
	toolResult = make(map[string]interface{})
	toolResult["subscription_status"] = "success"
	toolResult["subscription_id"] = msEmailTool.EmailAccount.SubscriptionId
	_ = url
	return toolResult, map[string]interface{}{"body": subPost}, persistStore, nil
}

func (msEmailTool *MsEmailTool) SendEmail(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SendEmail Execute - Start")
	url := "/v1.0/me/sendMail"
	_ = url
	to := emailNormalizeRecipients(params["to"])
	cc := emailNormalizeRecipients(params["cc"])
	bcc := emailNormalizeRecipients(params["bcc"])
	if len(to) == 0 && len(cc) == 0 && len(bcc) == 0 {
		err = errors.New("at least one of to, cc or bcc is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	return nil, map[string]interface{}{"body": params}, false, nil
}

func (msEmailTool *MsEmailTool) GetToolCallback() tools.ToolCallback {
	return tools.ToolCallback{
		ResponseContentType: "plain/text",
	}
}

func (msEmailTool *MsEmailTool) Callback(ctx context.Context, projectId string, tenantId string, actionName string, body map[string]interface{}, params map[string][]string) (callbackResult interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Callback Execute - Start")

	validationString := ""
	if vToken, vTokenOk := params["validationToken"]; vTokenOk {
		validationString = vToken[0]
	}

	// Process the message in a separate goroutine with panic recovery using global GoroutineManager
	gm := server.GetGlobalGoroutineManager(ctx)
	gm.SafeGoWithRestartBehavior("ms-email-callback", func(bgCtx context.Context) {
		// Copy any important values from the original context if needed

		requestId := ctx.Value("request_id")
		if requestId != nil {
			bgCtx = context.WithValue(bgCtx, "request_id", requestId)
		}

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
		logs.WithContext(bgCtx).Info(fmt.Sprint("body: ", body))
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint("bodyBytes: ", string(bodyBytes)))

		paramBytes, err := json.Marshal(params)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}

		var insertQueries []*models.Queries
		insertQueryFuncAsync := models.Queries{}
		insertQueryFuncAsync.Query = msEmailTool.ToolDb.GetDbQuery(bgCtx, INSERT_FUNC_ASYNC)
		insertQueryFuncAsync.Vals = append(insertQueryFuncAsync.Vals, projectId, tenantId, string(bodyBytes), string(paramBytes))
		insertQueryFuncAsync.Rank = 1
		insertQueries = append(insertQueries, &insertQueryFuncAsync)
		_, insertOutputErr := utils.ExecuteDbSave(bgCtx, msEmailTool.ToolDb.GetConn(), insertQueries)
		if insertOutputErr != nil {
			logs.WithContext(bgCtx).Error(insertOutputErr.Error())
			return
		}

		var notificationCollection MsNotificationCollection
		err = json.Unmarshal(bodyBytes, &notificationCollection)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}

		for _, notification := range notificationCollection.Value {
			processMsg := true

			if notification.ClientState != tenantId {
				processMsg = false
				logs.WithContext(bgCtx).Info(fmt.Sprint("notification client state does not match tenant id : ", tenantId, " : ", notification.ClientState))
				break
			}
			readMsg := make(map[string]interface{})
			if notification.ResourceData.ODataType != "microsoft.graph.message" && notification.ChangeType == "created" {
				messageId := notification.ResourceData.Id
				readMsg, _, _, err = msEmailTool.ReadMessage(bgCtx, map[string]interface{}{"message_id": messageId})
				if err != nil {
					logs.WithContext(bgCtx).Error(err.Error())
					return
				}
			}

			if processMsg {
				body := map[string]interface{}{
					"mail":      readMsg,
					"tenant_id": tenantId,
				}
				hookResult, err := msEmailTool.ExecuteHook(bgCtx, "clbk", "", projectId, tenantId, body, params)
				if err != nil {
					logs.WithContext(bgCtx).Error(err.Error())
					return
				}
				logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
			}
		}
	}, server.ContinueOnMaxRetries)

	return validationString, false, nil
}

func (msEmailTool *MsEmailTool) GetToolCbUrl(projectId string, tenantId string) string {
	return fmt.Sprint(msEmailTool.CallbackBaseUrl, "/", projectId, "/callback/", tenantId, "/tool/", msEmailTool.ToolName)
}

func (msEmailTool *MsEmailTool) GetSsoUrl(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetSsoUrl Execute - Start")
	if msEmailTool.AuthName == "" {
		err = errors.New("auth name is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", msEmailTool.AuthName, "/getssourl")
	logs.WithContext(ctx).Info(fmt.Sprint("url: ", url))
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	qParams := make(map[string]string)
	if params["state"] != nil {
		qParams["state"] = params["state"].(string)
	}
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, qParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	toolResultOk := false
	toolResult, toolResultOk = res.(map[string]interface{})
	if !toolResultOk {
		err = errors.New("toolResult is not a map")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	logs.WithContext(ctx).Info(fmt.Sprint("toolResult: ", toolResult))
	return toolResult, map[string]interface{}{"query": qParams}, false, nil
}
func (msEmailTool *MsEmailTool) RenewToken(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	params["refresh_token"] = msEmailTool.EmailAccount.RefreshToken
	return msEmailTool.Login(ctx, projectId, tenantId, params, "/renew")
}
func (msEmailTool *MsEmailTool) RenewSubscription(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("RenewSubscription Execute - Start")
	subscriptionId := msEmailTool.EmailAccount.SubscriptionId
	if subscriptionId == "" {
		err = errors.New("subscription id is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	return msEmailTool.SubscribeEmail(ctx, projectId, tenantId, params, fmt.Sprint("/", subscriptionId), false)
}
func (msEmailTool *MsEmailTool) Login(ctx context.Context, projectId string, tenantId string, params map[string]interface{}, renewStr string) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Login Execute - Start")
	if msEmailTool.AuthName == "" {
		err = errors.New("auth name is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", msEmailTool.AuthName, "/idptoken", renewStr)
	logs.WithContext(ctx).Info(fmt.Sprint("url: ", url))
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, params)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var msTokens MsTokens
	resBytes, _ := json.Marshal(res)
	err = json.Unmarshal(resBytes, &msTokens)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = json.Unmarshal(resBytes, &msTokens)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = msEmailTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(msEmailTool.ToolName, "_access_token"), msTokens.AccessToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = msEmailTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(msEmailTool.ToolName, "_refresh_token"), msTokens.RefreshToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = msEmailTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(msEmailTool.ToolName, "_id_token"), msTokens.IdToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	msEmailTool.EmailAccount.TokenExpirationDateTime = time.Now().UTC().Add(time.Duration(msTokens.ExpiresIn) * time.Second).Format(time.RFC3339)
	persistStore = true

	if msEmailTool.Tool.Hooks.ARRT != "" {
		hookBody := map[string]interface{}{
			"Vars": map[string]interface{}{
				"Body": map[string]interface{}{
					"tool_name": msEmailTool.ToolName,
					"tenant_id": tenantId,
				},
				"OrgBody": map[string]interface{}{
					"tool_name": msEmailTool.ToolName,
					"tenant_id": tenantId,
				},
			},
			"ReqVars": map[string]interface{}{},
			"ResVars": map[string]interface{}{},
		}
		hookBodyBytes, err := json.Marshal(hookBody)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, nil, persistStore, err
		}
		jobName := fmt.Sprint(msEmailTool.ToolName, "_", msEmailTool.Tool.Hooks.ARRT, "_", tenantId)
		err = msEmailTool.Scheduler.Unschedule(ctx, "", jobName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			//continue even if unschedule fails
		}

		schedulerCommand := fmt.Sprint("CALL schedule_procedure('", msEmailTool.Tool.Hooks.ARRT, "','", string(hookBodyBytes), "','", msEmailTool.Scheduler.GetSchedulerName(), "')")

		cronStr := utils.GetCronStr(ctx, time.Now().UTC().Add(1*time.Hour))
		jobId, err := msEmailTool.Scheduler.Schedule(ctx, jobName, schedulerCommand, cronStr)
		if err != nil {
			return nil, nil, persistStore, err
		}
		logs.WithContext(ctx).Info(fmt.Sprint("jobId: ", jobId))
	}
	toolResult = make(map[string]interface{})
	toolResult["login_status"] = "success"
	return toolResult, map[string]interface{}{"body": params}, persistStore, nil
}

func (msEmailTool *MsEmailTool) SetPrivateAttributes(ctx context.Context, realTool tools.Tooling) (err error) {
	msEmailTool.EmailAccount.AccessToken = fmt.Sprint("$SECRET_", msEmailTool.ToolName, "_access_token")
	msEmailTool.EmailAccount.RefreshToken = fmt.Sprint("$SECRET_", msEmailTool.ToolName, "_refresh_token")
	return nil
}

func (msEmailTool *MsEmailTool) GetBytes(ctx context.Context) ([]byte, error) {

	msEmailToolWithToken := msEmailToolWithToken{
		Tool: msEmailTool.Tool,
		EmailAccount: emailAccountWithToken{
			DisplayName:                    msEmailTool.EmailAccount.DisplayName,
			AccessToken:                    msEmailTool.EmailAccount.AccessToken,
			RefreshToken:                   msEmailTool.EmailAccount.RefreshToken,
			SubscriptionId:                 msEmailTool.EmailAccount.SubscriptionId,
			SubscriptionExpirationDateTime: msEmailTool.EmailAccount.SubscriptionExpirationDateTime,
			TokenExpirationDateTime:        msEmailTool.EmailAccount.TokenExpirationDateTime,
		},
		AuthName: msEmailTool.AuthName,
	}

	toolJson, err := json.Marshal(msEmailToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}
func (msEmailTool *MsEmailTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	msEmailToolWithToken := msEmailToolWithToken{}
	err := json.Unmarshal(toolObjJson, &msEmailToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	msEmailTool = &MsEmailTool{
		Tool: msEmailToolWithToken.Tool,
		EmailAccount: EmailAccount{
			DisplayName:                    msEmailToolWithToken.EmailAccount.DisplayName,
			AccessToken:                    msEmailToolWithToken.EmailAccount.AccessToken,
			RefreshToken:                   msEmailToolWithToken.EmailAccount.RefreshToken,
			SubscriptionId:                 msEmailToolWithToken.EmailAccount.SubscriptionId,
			SubscriptionExpirationDateTime: msEmailToolWithToken.EmailAccount.SubscriptionExpirationDateTime,
			TokenExpirationDateTime:        msEmailToolWithToken.EmailAccount.TokenExpirationDateTime,
		},
		AuthName: msEmailToolWithToken.AuthName,
	}
	return msEmailTool, nil
}
func (msEmailTool *MsEmailTool) StopAutoRenew(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	if msEmailTool.Scheduler == nil {
		err = errors.New("scheduler not defined")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	msEmailTool.Scheduler.Unschedule(ctx, "", fmt.Sprint(msEmailTool.ToolName, "_", msEmailTool.Tool.Hooks.ARRT, "_", tenantId))
	toolResult = make(map[string]interface{})
	toolResult["stop_auto_renew_status"] = "success"
	msEmailTool.EmailAccount.TokenExpirationDateTime = ""
	persistStore = true
	return toolResult, map[string]interface{}{"body": params}, persistStore, nil
}
func (msEmailTool *MsEmailTool) StopSubscription(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("StopSubscription Execute - Start")
	subscriptionId := msEmailTool.EmailAccount.SubscriptionId
	if subscriptionId == "" {
		err = errors.New("subscription id is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	return msEmailTool.SubscribeEmail(ctx, projectId, tenantId, params, fmt.Sprint("/", subscriptionId), true)
}
func (msEmailTool *MsEmailTool) ReadConversation(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ReadConversation Execute - Start")

	type rcParams struct {
		ConversationId   string `json:"conversation_id" required:"true"`
		NumberofMessages int    `json:"number_of_messages"`
		HasAttachments   bool   `json:"has_attachments"`
	}
	// Convert params map to struct using json marshal/unmarshal
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var rcParamsObj rcParams
	err = json.Unmarshal(paramsBytes, &rcParamsObj)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = utils.ValidateStruct(ctx, rcParamsObj, "")
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	attachedStr := ""
	if rcParamsObj.HasAttachments {
		attachedStr = "and hasAttachments eq true"
	}
	if rcParamsObj.NumberofMessages == 0 {
		rcParamsObj.NumberofMessages = 5
	}

	// Create params map for ReadEmail with conversation filter
	readEmailParams := map[string]interface{}{
		"$top":     rcParamsObj.NumberofMessages,
		"$skip":    0,
		"$select":  "sender,subject,id,receivedDateTime,conversationId,hasAttachments",
		"$filter":  fmt.Sprintf("receivedDateTime ge 2025-04-01T00:00:00Z and conversationId eq '%s' %s", rcParamsObj.ConversationId, attachedStr),
		"$orderby": "receivedDateTime desc",
	}

	// Call ReadEmail to get messages in the conversation
	emailResult, _, _, err := msEmailTool.ReadEmail(ctx, readEmailParams)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	// Extract emails array from result
	emails, ok := emailResult["emails"].(map[string]interface{})
	if !ok {
		err = errors.New("emails result is not a map")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	value, ok := emails["value"].([]interface{})
	if !ok {
		err = errors.New("emails value is not an array")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	// Array to accumulate message responses
	var messageResponses []map[string]interface{}

	// Loop through each email and extract ID, then call ReadMessage
	for _, email := range value {
		emailMap, ok := email.(map[string]interface{})
		if !ok {
			logs.WithContext(ctx).Warn("email is not a map, skipping")
			continue
		}

		messageId, ok := emailMap["id"].(string)
		if !ok {
			logs.WithContext(ctx).Warn("message id is not a string, skipping")
			continue
		}

		// Create params for ReadMessage
		readMessageParams := map[string]interface{}{
			"message_id": messageId,
		}

		// Call ReadMessage for this specific message
		messageResult, _, _, err := msEmailTool.ReadMessage(ctx, readMessageParams)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("Error reading message %s: %s", messageId, err.Error()))
			continue // Continue with other messages even if one fails
		}

		// Add the message result to our accumulated responses
		messageResponses = append(messageResponses, messageResult)
	}

	// Return the accumulated message responses
	toolResult = make(map[string]interface{})
	toolResult["conversation_messages"] = messageResponses
	toolResult["total_messages"] = len(messageResponses)

	return toolResult, map[string]interface{}{"body": params}, false, nil
}

func init() {
	tools.RegisterTool("MS_EMAIL", func() tools.Tooling { return new(MsEmailTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       true,
		ToolType:     "MS_EMAIL",
		Category:     "Communication",
		Description:  "Microsoft email integration for reading, sending, and subscribing to emails via Microsoft Graph API",
		Actions:      []tools.ActionInfo{{Name: ReadEmail}, {Name: SendEmail}, {Name: SubscribeEmail}, {Name: ReadMessage}, {Name: GetSsoUrl}, {Name: Login}, {Name: RenewToken}, {Name: RenewSubscription}},
		OAuthEnabled: true,
		Icon:         "PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxZW0iIGhlaWdodD0iMWVtIiB2aWV3Qm94PSIwIDAgMjU2IDI1NiI+PHBhdGggZmlsbD0iI2YxNTExYiIgZD0iTTEyMS42NjYgMTIxLjY2NkgwVjBoMTIxLjY2NnoiLz48cGF0aCBmaWxsPSIjODBjYzI4IiBkPSJNMjU2IDEyMS42NjZIMTM0LjMzNVYwSDI1NnoiLz48cGF0aCBmaWxsPSIjMDBhZGVmIiBkPSJNMTIxLjY2MyAyNTYuMDAySDBWMTM0LjMzNmgxMjEuNjYzeiIvPjxwYXRoIGZpbGw9IiNmYmJjMDkiIGQ9Ik0yNTYgMjU2LjAwMkgxMzQuMzM1VjEzNC4zMzZIMjU2eiIvPjwvc3ZnPg==",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(MsEmailTool{}), []string{}),
	})
}
