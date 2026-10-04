package saas

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server "github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
)

type ZohoTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type ZohoAccount struct {
	OrgId                   string `json:"org_id"`
	AccessToken             string `json:"-"`
	RefreshToken            string `json:"-"`
	TokenExpirationDateTime string `json:"token_expiration_date_time"`
}

type zohoAccountWithToken struct {
	OrgId                   string `json:"org_id"`
	AccessToken             string `json:"access_token"`
	RefreshToken            string `json:"refresh_token"`
	TokenExpirationDateTime string `json:"token_expiration_date_time"`
}

const (
	GetTickets          = "get_tickets"
	GetOrganizations    = "get_organizations"
	GetTicketThread     = "get_ticket_thread"
	GetTicketContent    = "get_ticket_content"
	GetTicketAttachment = "get_ticket_attachment"
	Login               = "login"
	RenewToken          = "renew_token"
	GetSsoUrl           = "get_sso_url"
	StopAutoRenew       = "stop_auto_renew"
)

const (
	DeskBaseUrl = "https://desk.zoho.in/api/v1"
)

type ZohoDeskTool struct {
	tools.Tool
	ZohoAccount ZohoAccount `json:"zoho_account"`
	AuthName    string      `json:"auth_name"`
}

type zohoDeskToolWithToken struct {
	tools.Tool
	ZohoAccount zohoAccountWithToken
	AuthName    string
}

func (zohoDeskTool *ZohoDeskTool) GetActionsList() []tools.ActionInfo {
	return []tools.ActionInfo{
		{Name: GetTickets},
		{Name: GetOrganizations},
		{Name: GetTicketThread},
		{Name: GetTicketContent},
		{Name: GetTicketAttachment},
		{Name: Login},
		{Name: RenewToken},
		{Name: GetSsoUrl},
		{Name: StopAutoRenew},
	}
}

func (zohoDeskTool *ZohoDeskTool) GetSpec() tools.Tooling {
	return zohoDeskTool
}

func (zohoDeskTool *ZohoDeskTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &zohoDeskTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (zohoDeskTool *ZohoDeskTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ZohoDeskTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case GetTickets:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.GetTickets(ctx, params)
	case GetOrganizations:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.GetOrganizations(ctx, params)
	case GetTicketThread:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.GetTicketThread(ctx, params)
	case GetTicketContent:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.GetTicketContent(ctx, params)
	case GetTicketAttachment:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.GetTicketAttachment(ctx, params)
	case Login:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.Login(ctx, projectId, tenantId, params, "")
	case RenewToken:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.RenewToken(ctx, projectId, tenantId, params)
	case GetSsoUrl:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.GetSsoUrl(ctx, projectId, tenantId, params)
	case StopAutoRenew:
		toolResult, toolRequest, persistStore, err = zohoDeskTool.StopAutoRenew(ctx, projectId, tenantId, params)
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

		hookResult, err := zohoDeskTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (zohoDeskTool *ZohoDeskTool) GetTickets(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetTickets Execute - Start")

	url := fmt.Sprint(DeskBaseUrl, "/tickets")
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Zoho-oauthtoken ", zohoDeskTool.ZohoAccount.AccessToken))
	headers.Set("orgId", zohoDeskTool.ZohoAccount.OrgId)
	headers.Set("Content-Type", "application/json")

	queryParams := make(map[string]string)
	for k, v := range params {
		queryParams[k] = fmt.Sprint(v)
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if resMap, ok := res.(map[string]interface{}); ok {
		toolResult = resMap
	} else {
		toolResult["data"] = res
	}

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (zohoDeskTool *ZohoDeskTool) GetOrganizations(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetOrganizations Execute - Start")

	url := fmt.Sprint(DeskBaseUrl, "/organizations")
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Zoho-oauthtoken ", zohoDeskTool.ZohoAccount.AccessToken))
	headers.Set("orgId", zohoDeskTool.ZohoAccount.OrgId)
	headers.Set("Content-Type", "application/json")

	queryParams := make(map[string]string)
	for k, v := range params {
		queryParams[k] = fmt.Sprint(v)
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if resMap, ok := res.(map[string]interface{}); ok {
		toolResult = resMap
	} else {
		toolResult["data"] = res
	}

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (zohoDeskTool *ZohoDeskTool) GetTicketThread(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetTicketThread Execute - Start")

	ticketId, ok := params["ticket_id"].(string)
	if !ok {
		err = errors.New("ticket_id is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	url := fmt.Sprint(DeskBaseUrl, "/tickets/", ticketId, "/threads")
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Zoho-oauthtoken ", zohoDeskTool.ZohoAccount.AccessToken))
	headers.Set("orgId", zohoDeskTool.ZohoAccount.OrgId)
	headers.Set("Content-Type", "application/json")

	queryParams := make(map[string]string)
	for k, v := range params {
		if k != "ticket_id" {
			queryParams[k] = fmt.Sprint(v)
		}
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var threads []interface{}
	if resMap, ok := res.(map[string]interface{}); ok {
		if data, exists := resMap["data"]; exists {
			if threadsArray, ok := data.([]interface{}); ok {
				threads = threadsArray
			} else {
				threads = []interface{}{data}
			}
		} else {
			toolResult = resMap
			return toolResult, map[string]interface{}{"query": queryParams}, false, nil
		}
	} else if threadsArray, ok := res.([]interface{}); ok {
		threads = threadsArray
	} else {
		toolResult = make(map[string]interface{})
		toolResult["data"] = res
		return toolResult, map[string]interface{}{"query": queryParams}, false, nil
	}

	enrichedThreads := make([]interface{}, 0, len(threads))
	for _, thread := range threads {
		threadMap, ok := thread.(map[string]interface{})
		if !ok {
			enrichedThreads = append(enrichedThreads, thread)
			continue
		}

		threadId, exists := threadMap["id"].(string)
		if !exists || threadId == "" {
			enrichedThreads = append(enrichedThreads, thread)
			continue
		}

		contentParams := make(map[string]interface{})
		contentParams["ticket_id"] = ticketId
		contentParams["thread_id"] = threadId
		for k, v := range params {
			if k != "ticket_id" {
				contentParams[k] = v
			}
		}

		contentResult, _, _, err := zohoDeskTool.GetTicketContent(ctx, contentParams)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("Error getting content for thread ", threadId, ": ", err.Error()))
			enrichedThreads = append(enrichedThreads, thread)
			continue
		}

		if contentData, ok := contentResult["data"].(map[string]interface{}); ok {
			if attachments, exists := contentData["attachments"]; exists {
				threadMap["attachments"] = attachments
			}
			if content, exists := contentData["content"]; exists {
				threadMap["content"] = content
			}
		} else {
			if attachments, exists := contentResult["attachments"]; exists {
				threadMap["attachments"] = attachments
			}
			if content, exists := contentResult["content"]; exists {
				threadMap["content"] = content
			}
		}

		enrichedThreads = append(enrichedThreads, threadMap)
	}

	toolResult = make(map[string]interface{})
	if resMap, ok := res.(map[string]interface{}); ok {
		toolResult = resMap
		toolResult["data"] = enrichedThreads
	} else {
		toolResult["data"] = enrichedThreads
	}

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (zohoDeskTool *ZohoDeskTool) GetTicketContent(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetTicketContent Execute - Start")

	ticketId, ok := params["ticket_id"].(string)
	if !ok {
		err = errors.New("ticket_id is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	threadId, ok := params["thread_id"].(string)
	if !ok {
		err = errors.New("thread_id is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	url := fmt.Sprint(DeskBaseUrl, "/tickets/", ticketId, "/threads/", threadId)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Zoho-oauthtoken ", zohoDeskTool.ZohoAccount.AccessToken))
	headers.Set("orgId", zohoDeskTool.ZohoAccount.OrgId)
	headers.Set("Content-Type", "application/json")

	queryParams := make(map[string]string)
	for k, v := range params {
		if k != "ticket_id" && k != "thread_id" {
			queryParams[k] = fmt.Sprint(v)
		}
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if resMap, ok := res.(map[string]interface{}); ok {
		toolResult = resMap
	} else {
		toolResult["data"] = res
	}

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (zohoDeskTool *ZohoDeskTool) GetTicketAttachment(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetTicketAttachment Execute - Start")

	ticketId, ok := params["ticket_id"].(string)
	if !ok {
		err = errors.New("ticket_id is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	threadId, ok := params["thread_id"].(string)
	if !ok {
		err = errors.New("thread_id is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	attachmentId, ok := params["attachment_id"].(string)
	if !ok {
		err = errors.New("attachment_id is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	url := fmt.Sprint(DeskBaseUrl, "/tickets/", ticketId, "/threads/", threadId, "/attachments/", attachmentId, "/content")
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprint("Zoho-oauthtoken ", zohoDeskTool.ZohoAccount.AccessToken))
	headers.Set("orgId", zohoDeskTool.ZohoAccount.OrgId)
	headers.Set("Content-Type", "application/json")

	queryParams := make(map[string]string)
	for k, v := range params {
		if k != "ticket_id" && k != "thread_id" && k != "attachment_id" {
			queryParams[k] = fmt.Sprint(v)
		}
	}

	res, respHeaders, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var fileContent []byte
	if resMap, ok := res.(map[string]interface{}); ok {
		if body, exists := resMap["body"].(string); exists {
			fileContent = []byte(body)
		} else {
			err = errors.New("response body not found")
			logs.Err(ctx, err, "")
			return nil, nil, false, err
		}
	} else {
		err = errors.New("unexpected response format")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}

	mimeType := respHeaders.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	base64Content := base64.StdEncoding.EncodeToString(fileContent)

	toolResult = make(map[string]interface{})
	toolResult["file"] = base64Content
	toolResult["mime_type"] = mimeType

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (zohoDeskTool *ZohoDeskTool) GetSsoUrl(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetSsoUrl Execute - Start")

	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", zohoDeskTool.AuthName, "/getssourl")
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

func (zohoDeskTool *ZohoDeskTool) RenewToken(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	params["refresh_token"] = zohoDeskTool.ZohoAccount.RefreshToken
	return zohoDeskTool.Login(ctx, projectId, tenantId, params, "/renew")
}

func (zohoDeskTool *ZohoDeskTool) Login(ctx context.Context, projectId string, tenantId string, params map[string]interface{}, renewStr string) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Login Execute - Start")
	if zohoDeskTool.AuthName == "" {
		err = errors.New("auth name is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", zohoDeskTool.AuthName, "/idptoken", renewStr)
	logs.WithContext(ctx).Info(fmt.Sprint("url: ", url))
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, params)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var zohoTokens ZohoTokens
	resBytes, _ := json.Marshal(res)
	err = json.Unmarshal(resBytes, &zohoTokens)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = json.Unmarshal(resBytes, &zohoTokens)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if zohoTokens.RefreshToken == "" {
		zohoTokens.RefreshToken = params["refresh_token"].(string)
	}
	err = zohoDeskTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(zohoDeskTool.ToolName, "_access_token"), zohoTokens.AccessToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = zohoDeskTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(zohoDeskTool.ToolName, "_refresh_token"), zohoTokens.RefreshToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	zohoDeskTool.ZohoAccount.TokenExpirationDateTime = time.Now().UTC().Add(time.Duration(zohoTokens.ExpiresIn) * time.Second).Format(time.RFC3339)
	persistStore = true

	if zohoDeskTool.Tool.Hooks.ARRT != "" {
		hookBody := map[string]interface{}{
			"Vars": map[string]interface{}{
				"Body": map[string]interface{}{
					"tool_name": zohoDeskTool.ToolName,
					"tenant_id": tenantId,
				},
				"OrgBody": map[string]interface{}{
					"tool_name": zohoDeskTool.ToolName,
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
		jobName := fmt.Sprint(zohoDeskTool.ToolName, "_", zohoDeskTool.Tool.Hooks.ARRT, "_", tenantId)
		err = zohoDeskTool.Scheduler.Unschedule(ctx, "", jobName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}

		schedulerCommand := fmt.Sprint("CALL schedule_procedure('", zohoDeskTool.Tool.Hooks.ARRT, "','", string(hookBodyBytes), "','", zohoDeskTool.Scheduler.GetSchedulerName(), "')")

		cronStr := utils.GetCronStr(ctx, time.Now().UTC().Add(1*time.Hour))
		jobId, err := zohoDeskTool.Scheduler.Schedule(ctx, jobName, schedulerCommand, cronStr)
		if err != nil {
			return nil, nil, persistStore, err
		}
		logs.WithContext(ctx).Info(fmt.Sprint("jobId: ", jobId))
	}
	toolResult = make(map[string]interface{})
	toolResult["login_status"] = "success"
	toolResult["access_token"] = zohoTokens.AccessToken
	toolResult["refresh_token"] = zohoTokens.RefreshToken
	toolResult["expires_in"] = zohoTokens.ExpiresIn
	toolResult["token_type"] = zohoTokens.TokenType
	return toolResult, map[string]interface{}{"body": params}, persistStore, nil
}

func (zohoDeskTool *ZohoDeskTool) SetPrivateAttributes(ctx context.Context, realTool tools.Tooling) (err error) {
	zohoDeskTool.ZohoAccount.AccessToken = fmt.Sprint("$SECRET_", zohoDeskTool.ToolName, "_access_token")
	zohoDeskTool.ZohoAccount.RefreshToken = fmt.Sprint("$SECRET_", zohoDeskTool.ToolName, "_refresh_token")
	return nil
}

func (zohoDeskTool *ZohoDeskTool) GetBytes(ctx context.Context) ([]byte, error) {

	zohoDeskToolWithToken := zohoDeskToolWithToken{
		Tool: zohoDeskTool.Tool,
		ZohoAccount: zohoAccountWithToken{
			OrgId:                   zohoDeskTool.ZohoAccount.OrgId,
			AccessToken:             zohoDeskTool.ZohoAccount.AccessToken,
			RefreshToken:            zohoDeskTool.ZohoAccount.RefreshToken,
			TokenExpirationDateTime: zohoDeskTool.ZohoAccount.TokenExpirationDateTime,
		},
		AuthName: zohoDeskTool.AuthName,
	}

	toolJson, err := json.Marshal(zohoDeskToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}

func (zohoDeskTool *ZohoDeskTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	zohoDeskToolWithToken := zohoDeskToolWithToken{}
	err := json.Unmarshal(toolObjJson, &zohoDeskToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	zohoDeskTool = &ZohoDeskTool{
		Tool: zohoDeskToolWithToken.Tool,
		ZohoAccount: ZohoAccount{
			OrgId:                   zohoDeskToolWithToken.ZohoAccount.OrgId,
			AccessToken:             zohoDeskToolWithToken.ZohoAccount.AccessToken,
			RefreshToken:            zohoDeskToolWithToken.ZohoAccount.RefreshToken,
			TokenExpirationDateTime: zohoDeskToolWithToken.ZohoAccount.TokenExpirationDateTime,
		},
		AuthName: zohoDeskToolWithToken.AuthName,
	}
	return zohoDeskTool, nil
}

func (zohoDeskTool *ZohoDeskTool) StopAutoRenew(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	if zohoDeskTool.Scheduler == nil {
		err = errors.New("scheduler not defined")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	zohoDeskTool.Scheduler.Unschedule(ctx, "", fmt.Sprint(zohoDeskTool.ToolName, "_", zohoDeskTool.Tool.Hooks.ARRT, "_", tenantId))
	toolResult = make(map[string]interface{})
	toolResult["stop_auto_renew_status"] = "success"
	zohoDeskTool.ZohoAccount.TokenExpirationDateTime = ""
	persistStore = true
	return toolResult, nil, persistStore, nil
}

func init() {
	tools.RegisterTool("ZOHODESK", func() tools.Tooling { return new(ZohoDeskTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       true,
		ToolType:     "ZOHODESK",
		Category:     "SaaS",
		Description:  "Zoho Desk integration for ticket management, organizations, and support operations",
		Actions:      []tools.ActionInfo{{Name: GetTickets}, {Name: GetOrganizations}, {Name: GetTicketThread}, {Name: GetTicketContent}, {Name: GetTicketAttachment}, {Name: Login}, {Name: RenewToken}, {Name: GetSsoUrl}, {Name: StopAutoRenew}},
		OAuthEnabled: true,
		Icon:         "PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIyLjMzZW0iIGhlaWdodD0iMWVtIiB2aWV3Qm94PSIwIDAgNTEyIDIyMCI+PHBhdGggZmlsbD0iIzA4OTk0OSIgZD0iTTIyOC42NDggMTc0LjkxNWMtMy45MDUgMC03Ljg2MS0uODEyLTExLjY2NS0yLjQ4NWwtODEuMTQ2LTM2LjE2MWMtMTQuNTA1LTYuNDQxLTIxLjA0Ny0yMy41MzItMTQuNjA2LTM4LjAzN2wzNi4xNi04MS4xNDZDMTYzLjgzMiAyLjU4IDE4MC45MjQtMy45NjIgMTk1LjQzIDIuNDc5bDgxLjE0NiAzNi4xNjFjMTQuNTA1IDYuNDQxIDIxLjA0NyAyMy41MzIgMTQuNjA2IDM4LjAzN2wtMzYuMTYgODEuMTQ2Yy00LjgxOSAxMC43NTItMTUuMzY4IDE3LjA5Mi0yNi4zNzMgMTcuMDkybS00LjkyLTE3LjdjNi4xMzcgMi43MzggMTMuMzM5LS4wNSAxNi4wNzctNi4xMzdsMzYuMTYxLTgxLjE0NmMyLjczOS02LjEzNy0uMDUtMTMuMzM4LTYuMTM3LTE2LjA3N2wtODEuMTk2LTM2LjE2Yy02LjEzNy0yLjc0LTEzLjMzOS4wNS0xNi4wNzcgNi4xMzZsLTM2LjE2MSA4MS4xNDZjLTIuNzM5IDYuMTM3LjA1IDEzLjMzOCA2LjEzNiAxNi4wNzd6Ii8+PHBhdGggZmlsbD0iI2Y5YjIxZCIgZD0iTTQ4My4xOTMgMTc0Ljk2NmgtODguODU1Yy0xNS44NzQgMC0yOC44MDctMTIuOTMzLTI4LjgwNy0yOC44MDdWNTcuMzA0YzAtMTUuODc1IDEyLjkzMy0yOC44MDcgMjguODA3LTI4LjgwN2g4OC44NTVjMTUuODc0IDAgMjguODA3IDEyLjkzMiAyOC44MDcgMjguODA3djg4Ljg1NWMwIDE1Ljg3NC0xMi45MzMgMjguODA3LTI4LjgwNyAyOC44MDdNMzk0LjMzOCA0NS4xMzJjLTYuNjk0IDAtMTIuMTcyIDUuNDc3LTEyLjE3MiAxMi4xNzJ2ODguODU1YzAgNi42OTQgNS40NzggMTIuMTcyIDEyLjE3MiAxMi4xNzJoODguODU1YzYuNjk1IDAgMTIuMTcyLTUuNDc4IDEyLjE3Mi0xMi4xNzJWNTcuMzA0YzAtNi42OTUtNS40NzctMTIuMTcyLTEyLjE3Mi0xMi4xNzJ6Ii8+PHBhdGggZmlsbD0iI2U0MjUyNyIgZD0ibTE1MC40NDMgNzMuNTg0bC0xMS45NjkgMjYuNzc4Yy0uMTUyLjMwNC0uMzA0LjU1OC0uNDU2LjgxMWw0LjY2NiAyOC44MDdjMS4wNjUgNi42NDQtMy40NSAxMi44ODItMTAuMDQyIDEzLjk0N2wtODcuNzQgMTQuMmExMi4zIDEyLjMgMCAwIDEtOS4wNzgtMi4xM2MtMi42MzctMS44NzYtNC4zNjEtNC43MTYtNC44NjktNy45MTFsLTE0LjItODcuNzRhMTIuMyAxMi4zIDAgMCAxIDIuMTMtOS4wNzhjMS44NzYtMi42MzcgNC43MTctNC4zNjEgNy45MTItNC44NjhsODcuNzM5LTE0LjIwMWMuNjYtLjEwMSAxLjMxOS0uMTUyIDEuOTI3LS4xNTJjNS44MzMgMCAxMS4wNTYgNC4yNiAxMi4wMiAxMC4yNDVsNC43MTcgMjkuMDFsMTIuMzc0LTI3Ljc0M2wtLjY1OS0zLjkwNUMxNDIuMzggMjMuOTgzIDEyNy41NyAxMy4yODIgMTExLjkgMTUuODE4bC04Ny43NCAxNC4yQTI4LjIgMjguMiAwIDAgMCA1LjQ0NSA0MS41MzFDLjkzMSA0Ny43NjktLjg0NCA1NS4zNzYuMzc0IDYyLjk4NGwxNC4yIDg3Ljc0YzEuMjE3IDcuNjA3IDUuMzI1IDE0LjI1IDExLjU2MyAxOC43NjRjNC45MiAzLjYwMSAxMC43NTIgNS40MjcgMTYuNzg4IDUuNDI3YzEuNTIxIDAgMy4wOTMtLjEwMiA0LjY2NS0uMzU1bDg3Ljc0LTE0LjJjMTUuNjcxLTIuNTM3IDI2LjM3Mi0xNy4zNDYgMjMuODM2LTMzLjAxN3oiLz48cGF0aCBmaWxsPSIjMjI2ZGI0IiBkPSJtMjU1LjY4IDExNS40NzVsMTIuODgxLTI4Ljg1N2wtMy42NTEtMjYuODNjLS40NTctMy4xOTQuNDA2LTYuMzkgMi4zODQtOC45NzZzNC44MTgtNC4yNiA4LjA2My00LjY2Nmw4OC4wNDQtMTEuOTdjLjU1OC0uMDUgMS4xMTYtLjEgMS42NzQtLjFjMi42MzcgMCA1LjE3My44NjIgNy4zNTQgMi40ODVjLjQwNS4zMDQuNzYuNjU5IDEuMTE1Ljk2M2EyOSAyOSAwIDAgMSAxNC43NTktOC4zMTdhMjguMSAyOC4xIDAgMCAwLTUuODMzLTUuOTM0Yy02LjEzNi00LjY2Ni0xMy42OTMtNi42NDQtMjEuMy01LjYzbC04OC4xNDUgMTEuOTdjLTcuNjA4IDEuMDE0LTE0LjQwNCA0Ljk3LTE5LjAyIDExLjEwNmMtNC42NjUgNi4xMzctNi42NDMgMTMuNjk0LTUuNjI5IDIxLjMwMXptMTQ5LjgxNiAxNC44NmwtMTEuNTY0LTg1LjIwM2MtNi40OTEuMjAzLTExLjcxNSA1LjU3OC0xMS43MTUgMTIuMTJ2MjUuMDA0bDYuODQ3IDUwLjMxYy40NTYgMy4xOTYtLjQwNiA2LjM5LTIuMzg0IDguOTc3cy00LjgxOCA0LjI2LTguMDY0IDQuNjY2bC04OC4wNDQgMTEuOTdjLTMuMTk1LjQ1Ni02LjM5LS40MDYtOC45NzYtMi4zODRzLTQuMjYtNC44MTgtNC42NjYtOC4wNjRsLTQuMDU4LTI5Ljg3MmwtMTIuODgyIDI4Ljg1N2wuNDU3IDMuMjQ2YzEuMDE0IDcuNjA4IDQuOTcgMTQuNDA0IDExLjEwNyAxOS4wMTljNS4wNzEgMy44NTQgMTEuMTA3IDUuODgzIDE3LjM5NSA1Ljg4M2MxLjMyIDAgMi42MzgtLjEwMSAzLjk1Ni0uMjU0bDg3Ljk0Mi0xMS44NjdjNy42MDgtMS4wMTQgMTQuNDA0LTQuOTcgMTkuMDItMTEuMTA3YzQuNjY1LTYuMTM3IDYuNjQzLTEzLjY5MyA1LjYyOS0yMS4zIi8+PHBhdGggZD0ibTE1Ny4wODcgMjE3LjExbDEyLjU3OC0xOC41NjFoLTEwLjM0NmExLjAxNyAxLjAxNyAwIDAgMS0xLjAxNS0xLjAxNXYtMi40ODVjMC0uNTU4LjQ1Ny0xLjAxNCAxLjAxNS0xLjAxNGgxNi45OWMuNTU3IDAgMS4wMTQuNDU2IDEuMDE0IDEuMDE0di45NjRjMCAuMjAzLS4wNS40MDYtLjE1Mi41NThsLTEyLjMyNCAxOC41NjJoMTEuMDU2Yy41NTggMCAxLjAxNC40NTYgMS4wMTQgMS4wMTR2Mi40ODVjMCAuNTU4LS40NTYgMS4wMTUtMS4wMTQgMS4wMTVoLTE3Ljk1NGExLjAxNyAxLjAxNyAwIDAgMS0xLjAxNC0xLjAxNXYtLjkxM2MtLjA1LS4yNTMuMDUtLjQ1Ni4xNTItLjYwOG01Mi45NDgtMTAuNDQ3YzAtNy42MDcgNS41NzktMTMuMDg1IDEzLjE4Ni0xMy4wODVjNy44NjEgMCAxMy4xODYgNS4zNzYgMTMuMTg2IDEzLjEzNmMwIDcuODYxLTUuNDI2IDEzLjI4OC0xMy4yODcgMTMuMjg4Yy03LjkxMiAwLTEzLjA4NS01LjQyNy0xMy4wODUtMTMuMzM5bTIwLjMzNy4xMDJjMC00LjYxNi0yLjIzMS04LjU3MS03LjI1Mi04LjU3MWMtNS4wNzIgMC03IDQuMTA4LTcgOC43NzRjMCA0LjQxMiAyLjM4NSA4LjQ3IDcuMjUzIDguNDdjNS4wMjEtLjA1MiA3LTQuMzYyIDctOC42NzNtNDIuMDk1LTEyLjc4aDMuNzUzYy41NTggMCAxLjAxNC40NTYgMS4wMTQgMS4wMTN2OS40MzRoMTAuNjV2LTkuNDM0YzAtLjU1Ny40NTctMS4wMTQgMS4wMTUtMS4wMTRoMy43NTNjLjU1OCAwIDEuMDE0LjQ1NyAxLjAxNCAxLjAxNHYyMy41ODRjMCAuNTU3LS40NTYgMS4wMTQtMS4wMTQgMS4wMTRoLTMuNzAyYTEuMDE3IDEuMDE3IDAgMCAxLTEuMDE1LTEuMDE0di05LjUzNWgtMTAuNjV2OS41MzVjMCAuNTU3LS40NTcgMS4wMTQtMS4wMTUgMS4wMTRoLTMuNzUzYTEuMDE3IDEuMDE3IDAgMCAxLTEuMDE0LTEuMDE0di0yMy41ODRjLS4wNS0uNTU3LjQwNi0xLjAxNC45NjQtMS4wMTRtNTYuMjQ0IDEyLjY3OGMwLTcuNjA3IDUuNTc5LTEzLjA4NSAxMy4xODYtMTMuMDg1YzcuODYxIDAgMTMuMTg3IDUuMzc2IDEzLjE4NyAxMy4xMzZjMCA3Ljg2MS01LjQyNyAxMy4yODgtMTMuMjg4IDEzLjI4OGMtNy45MTIgMC0xMy4wODUtNS40MjctMTMuMDg1LTEzLjMzOW0yMC4yODcuMTAyYzAtNC42MTYtMi4yMzItOC41NzEtNy4yNTMtOC41NzFjLTUuMDcxIDAtNi45OTkgNC4xMDgtNi45OTkgOC43NzRjMCA0LjQxMiAyLjM4NCA4LjQ3IDcuMjUzIDguNDdjNS4wMi0uMDUyIDYuOTk5LTQuMzYyIDYuOTk5LTguNjczIi8+PC9zdmc+",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(ZohoDeskTool{}), []string{}),
	})
}
