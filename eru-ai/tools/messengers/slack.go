package messengers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	server "github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
	"github.com/gabriel-vasile/mimetype"
)

const (
	INSERT_FUNC_ASYNC_SLACK = "insert into eruai_cb_slack (project_id, tenant_id, request_body, request_params) values ($1, $2, $3, $4)"
	SLACK_BASE_URL          = "https://slack.com/api"
	JoinChannel             = "join_channel"
)

type SlackTool struct {
	tools.Tool
	SlackAccount SlackAccount `json:"slack_account"`
	AuthName     string       `json:"auth_name"`
}

type slackToolWithToken struct {
	tools.Tool
	SlackAccount slackAccountWithToken
	AuthName     string
}

func (slackTool *SlackTool) GetActionsList() []tools.ActionInfo {
	return []tools.ActionInfo{
		{Name: SendMessage},
		{Name: ReadMessages},
		{Name: Login},
		{Name: GetSsoUrl},
		{Name: SubscribeWebhooks},
		{Name: ListChannels},
		{Name: ListUsers},
		{Name: CreateChannel},
		{Name: InviteToChannel},
		{Name: JoinChannel},
		{Name: UploadMedia},
		{Name: DownloadMedia},
		{Name: Callback},
	}
}

func (slackTool *SlackTool) GetSpec() tools.Tooling {
	return slackTool
}

func (slackTool *SlackTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &slackTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (slackTool *SlackTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SlackTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case SendMessage:
		toolResult, toolRequest, persistStore, err = slackTool.SendMessage(ctx, params)
	case ReadMessages:
		toolResult, toolRequest, persistStore, err = slackTool.ReadMessages(ctx, params)
	case SubscribeWebhooks:
		toolResult, toolRequest, persistStore, err = slackTool.SubscribeWebhooks(ctx, projectId, tenantId, params)
	case ListChannels:
		toolResult, toolRequest, persistStore, err = slackTool.ListChannels(ctx, params)
	case ListUsers:
		toolResult, toolRequest, persistStore, err = slackTool.ListUsers(ctx, params)
	case CreateChannel:
		toolResult, toolRequest, persistStore, err = slackTool.CreateChannel(ctx, params)
	case InviteToChannel:
		toolResult, toolRequest, persistStore, err = slackTool.InviteToChannel(ctx, params)
	case JoinChannel:
		toolResult, toolRequest, persistStore, err = slackTool.JoinChannel(ctx, params)
	case UploadMedia:
		toolResult, toolRequest, persistStore, err = slackTool.UploadMedia(ctx, params)
	case DownloadMedia:
		toolResult, toolRequest, persistStore, err = slackTool.DownloadMedia(ctx, params)
	case Login:
		toolResult, toolRequest, persistStore, err = slackTool.Login(ctx, projectId, tenantId, params, "")
	case GetSsoUrl:
		toolResult, toolRequest, persistStore, err = slackTool.GetSsoUrl(ctx, projectId, tenantId, params)
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

		hookResult, err := slackTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}
func (slackTool *SlackTool) getAccessToken(ctx context.Context, params map[string]interface{}) (token string) {
	token = ""
	if tokenType, tokenTypeOk := params["token_type"]; tokenTypeOk {
		if tokenTypeStr, tokenTypeStrOk := tokenType.(string); tokenTypeStrOk {
			switch tokenTypeStr {
			case "bot":
				logs.WithContext(ctx).Info("Using bot token")
				token = slackTool.SlackAccount.BotAccessToken
			case "user":
				logs.WithContext(ctx).Info("Using user token")
				token = slackTool.SlackAccount.AuthedUserAccessToken
			}
		}
	} else {
		//if token_type is not provided, use bot token by default
		logs.WithContext(ctx).Info("Using bot token by default")
		token = slackTool.SlackAccount.BotAccessToken
	}
	return token
}
func (slackTool *SlackTool) GetSsoUrl(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetSsoUrl Execute - Start")
	if slackTool.AuthName == "" {
		err = errors.New("auth name is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", slackTool.AuthName, "/getssourl")
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
func (slackTool *SlackTool) Login(ctx context.Context, projectId string, tenantId string, params map[string]interface{}, renewStr string) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Login Execute - Start")
	if slackTool.AuthName == "" {
		err = errors.New("auth name is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", slackTool.AuthName, "/idptoken", renewStr)
	logs.WithContext(ctx).Info(fmt.Sprint("url: ", url))
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, params)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var slackTokens SlackTokens
	resBytes, _ := json.Marshal(res)
	err = json.Unmarshal(resBytes, &slackTokens)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if !slackTokens.Ok {
		err = logs.Err(ctx, errors.New(slackTokens.Error), "")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	err = slackTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprintf("%s_authed_user_access_token", slackTool.ToolName), slackTokens.AuthedUser.AccessToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = slackTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprintf("%s_authed_user_id", slackTool.ToolName), slackTokens.AuthedUser.Id)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = slackTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprintf("%s_bot_access_token", slackTool.ToolName), slackTokens.AccessToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = slackTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprintf("%s_bot_user_id", slackTool.ToolName), slackTokens.BotUserId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	//slackTool.EmailAccount.TokenExpirationDateTime = time.Now().UTC().Add(time.Duration(msTokens.ExpiresIn) * time.Second).Format(time.RFC3339)
	persistStore = true

	toolResult = make(map[string]interface{})
	toolResult["login_status"] = "success"
	return toolResult, map[string]interface{}{"body": params}, persistStore, nil
}

func (slackTool *SlackTool) SendMessage(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SendMessage Execute - Start")

	messagePayload, messagePayloadOk := params["message_payload"]
	if !messagePayloadOk {
		err = errors.New("message_payload parameter is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	var messagePayloadStruct SlackMessagePayload
	messagePayloadBytes, err := json.Marshal(messagePayload)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = json.Unmarshal(messagePayloadBytes, &messagePayloadStruct)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = utils.ValidateStruct(ctx, messagePayloadStruct, "")
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	logs.WithContext(ctx).Info("Sending Slack message")

	url := fmt.Sprintf("%s/chat.postMessage", SLACK_BASE_URL)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", slackTool.getAccessToken(ctx, params)))
	headers.Set("Content-Type", "application/json")

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, messagePayloadStruct)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if resMap, resMapOk := res.(map[string]interface{}); resMapOk {
		toolResult["response"] = resMap
		if ok, okExists := resMap["ok"]; okExists && ok.(bool) {
			toolResult["status"] = "sent"
			if ts, tsExists := resMap["ts"]; tsExists {
				toolResult["message_ts"] = ts
			}
			if channel, channelExists := resMap["channel"]; channelExists {
				toolResult["channel"] = channel
			}
		} else {
			toolResult["status"] = "failed"
			if errorMsg, errorExists := resMap["error"]; errorExists {
				toolResult["error"] = errorMsg
			}
		}
	} else {
		toolResult["response"] = res
		toolResult["status"] = "sent"
	}

	return toolResult, map[string]interface{}{"body": messagePayloadStruct}, false, nil
}

func (slackTool *SlackTool) ReadMessages(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ReadMessages Execute - Start")

	// Validate required channel parameter
	channel, channelOk := params["channel"]
	if !channelOk || channel == "" {
		err = errors.New("channel parameter is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	url := fmt.Sprintf("%s/conversations.history", SLACK_BASE_URL)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", slackTool.getAccessToken(ctx, params)))
	headers.Set("Content-Type", "application/json")

	queryParams := map[string]string{
		"channel": fmt.Sprintf("%v", channel),
	}

	// Optional parameters
	if limit, limitOk := params["limit"]; limitOk {
		queryParams["limit"] = fmt.Sprintf("%v", limit)
	}
	if latest, latestOk := params["latest"]; latestOk {
		queryParams["latest"] = fmt.Sprintf("%v", latest)
	}
	if oldest, oldestOk := params["oldest"]; oldestOk {
		queryParams["oldest"] = fmt.Sprintf("%v", oldest)
	}
	if inclusive, inclusiveOk := params["inclusive"]; inclusiveOk {
		queryParams["inclusive"] = fmt.Sprintf("%v", inclusive)
	}
	if cursor, cursorOk := params["cursor"]; cursorOk {
		queryParams["cursor"] = fmt.Sprintf("%v", cursor)
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	// Check if we got a "not_in_channel" error
	if resMap, resMapOk := res.(map[string]interface{}); resMapOk {
		if ok, okExists := resMap["ok"]; okExists && !ok.(bool) {
			if errorMsg, errorExists := resMap["error"]; errorExists && errorMsg == "not_in_channel" {
				logs.WithContext(ctx).Info("Not in channel, attempting to join and retry")

				// Try to join the channel
				joinResult, _, _, joinErr := slackTool.JoinChannel(ctx, params)
				if joinErr != nil {
					logs.WithContext(ctx).Error(fmt.Sprintf("Failed to join channel: %v", joinErr))
					return nil, nil, false, fmt.Errorf("failed to join channel: %v", joinErr)
				}

				// Check if join was successful
				if joinResMap, joinResMapOk := joinResult["result"].(map[string]interface{}); joinResMapOk {
					if joinOk, joinOkExists := joinResMap["ok"]; joinOkExists && !joinOk.(bool) {
						logs.WithContext(ctx).Error("Failed to join channel")
						return nil, nil, false, errors.New("failed to join channel")
					}
				}

				// Retry reading messages
				logs.WithContext(ctx).Info("Retrying to read messages after joining channel")
				res, _, _, _, err = utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return nil, nil, false, err
				}
			}
		}
	}

	toolResult = make(map[string]interface{})
	toolResult["messages"] = res

	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (slackTool *SlackTool) JoinChannel(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("JoinChannel Execute - Start")

	// Validate required channel parameter
	channel, channelOk := params["channel"]
	if !channelOk || channel == "" {
		err = errors.New("channel parameter is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	url := fmt.Sprintf("%s/conversations.join", SLACK_BASE_URL)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", slackTool.getAccessToken(ctx, params)))
	headers.Set("Content-Type", "application/json")

	payload := map[string]interface{}{
		"channel": channel,
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, payload)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["result"] = res

	return toolResult, map[string]interface{}{"body": payload}, false, nil
}

func (slackTool *SlackTool) SubscribeWebhooks(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SubscribeWebhooks Execute - Start")

	webhookUrl := slackTool.GetToolCbUrl(projectId, tenantId)
	logs.WithContext(ctx).Info(fmt.Sprintf("Webhook URL: %s", webhookUrl))

	toolResult = make(map[string]interface{})
	toolResult["webhook_url"] = webhookUrl
	//toolResult["verification_token"] = slackTool.SlackAccount.WebhookVerifyToken
	toolResult["status"] = "configured"
	toolResult["instructions"] = "Configure this webhook URL in your Slack app's Event Subscriptions with the provided verification token"

	return toolResult, map[string]interface{}{"body": params}, false, nil
}

func (slackTool *SlackTool) ListChannels(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ListChannels Execute - Start")

	// Convert params to query parameters
	queryParams := map[string]string{}
	for k, v := range params {
		queryParams[k] = fmt.Sprintf("%v", v)
	}

	// Call recursively to get all channels
	consolidatedResponse, err := slackTool.getChannelsRecursive(ctx, queryParams, "")
	if err != nil {
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["channels"] = consolidatedResponse
	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (slackTool *SlackTool) getChannelsRecursive(ctx context.Context, queryParams map[string]string, cursor string) ([]interface{}, error) {
	logs.WithContext(ctx).Debug("getChannelsRecursive Execute - Start")
	var allChannels []interface{}

	url := fmt.Sprintf("%s/conversations.list", SLACK_BASE_URL)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", slackTool.getAccessToken(ctx, map[string]interface{}{})))
	headers.Set("Content-Type", "application/json")

	// Prepare query parameters based on whether cursor is provided
	currentQueryParams := make(map[string]string)
	if cursor != "" {
		// When cursor is provided, only pass the cursor parameter
		currentQueryParams["cursor"] = cursor
	} else {
		// For the first call, pass all original parameters
		for k, v := range queryParams {
			currentQueryParams[k] = v
		}
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, currentQueryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	// Parse response to extract channels and next cursor
	responseMap, ok := res.(map[string]interface{})
	if !ok {
		logs.WithContext(ctx).Error("Response is not a map")
		return nil, errors.New("invalid response format")
	}

	// Extract channels from current response
	if responseOk, exists := responseMap["ok"]; exists && responseOk.(bool) {
		if channelsData, exists := responseMap["channels"]; exists {
			if channelsList, ok := channelsData.([]interface{}); ok {
				allChannels = append(allChannels, channelsList...)
			}
		}

		// Check for next_cursor
		if responseCursor, exists := responseMap["response_metadata"]; exists {
			if metadataMap, ok := responseCursor.(map[string]interface{}); ok {
				if nextCursor, exists := metadataMap["next_cursor"]; exists {
					if nextCursorStr, ok := nextCursor.(string); ok && nextCursorStr != "" {
						logs.WithContext(ctx).Info(fmt.Sprintf("Found next_cursor: %s, making recursive call", nextCursorStr))
						// Recursive call with next_cursor
						nextChannels, err := slackTool.getChannelsRecursive(ctx, queryParams, nextCursorStr)
						if err != nil {
							return nil, err
						}
						allChannels = append(allChannels, nextChannels...)
					}
				}
			}
		}
	} else {
		// Handle error response
		if errorMsg, exists := responseMap["error"]; exists {
			logs.WithContext(ctx).Error(fmt.Sprintf("Slack API error: %v", errorMsg))
			return nil, fmt.Errorf("slack API error: %v", errorMsg)
		}
	}

	// No more next_cursor, return consolidated response
	logs.WithContext(ctx).Debug(fmt.Sprintf("No more next_cursor found. Total channels collected: %d", len(allChannels)))

	return allChannels, nil
}

func (slackTool *SlackTool) ListUsers(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ListUsers Execute - Start")

	// Convert params to query parameters
	queryParams := map[string]string{}
	for k, v := range params {
		queryParams[k] = fmt.Sprintf("%v", v)
	}

	// Call recursively to get all users
	consolidatedResponse, err := slackTool.getUsersRecursive(ctx, queryParams, "")
	if err != nil {
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["users"] = consolidatedResponse
	return toolResult, map[string]interface{}{"query": queryParams}, false, nil
}

func (slackTool *SlackTool) getUsersRecursive(ctx context.Context, queryParams map[string]string, cursor string) ([]interface{}, error) {
	logs.WithContext(ctx).Debug("getUsersRecursive Execute - Start")
	var allUsers []interface{}

	url := fmt.Sprintf("%s/users.list", SLACK_BASE_URL)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", slackTool.getAccessToken(ctx, map[string]interface{}{})))
	headers.Set("Content-Type", "application/json")

	// Prepare query parameters based on whether cursor is provided
	currentQueryParams := make(map[string]string)
	if cursor != "" {
		// When cursor is provided, only pass the cursor parameter
		currentQueryParams["cursor"] = cursor
	} else {
		// For the first call, pass all original parameters
		for k, v := range queryParams {
			currentQueryParams[k] = v
		}
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, currentQueryParams, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	// Parse response to extract users and next cursor
	responseMap, ok := res.(map[string]interface{})
	if !ok {
		logs.WithContext(ctx).Error("Response is not a map")
		return nil, errors.New("invalid response format")
	}

	// Extract users from current response
	if responseOk, exists := responseMap["ok"]; exists && responseOk.(bool) {
		if usersData, exists := responseMap["members"]; exists {
			if usersList, ok := usersData.([]interface{}); ok {
				allUsers = append(allUsers, usersList...)
			}
		}

		// Check for next_cursor
		if responseCursor, exists := responseMap["response_metadata"]; exists {
			if metadataMap, ok := responseCursor.(map[string]interface{}); ok {
				if nextCursor, exists := metadataMap["next_cursor"]; exists {
					if nextCursorStr, ok := nextCursor.(string); ok && nextCursorStr != "" {
						logs.WithContext(ctx).Info(fmt.Sprintf("Found next_cursor: %s, making recursive call", nextCursorStr))
						// Recursive call with next_cursor
						nextUsers, err := slackTool.getUsersRecursive(ctx, queryParams, nextCursorStr)
						if err != nil {
							return nil, err
						}
						allUsers = append(allUsers, nextUsers...)
					}
				}
			}
		}
	} else {
		// Handle error response
		if errorMsg, exists := responseMap["error"]; exists {
			logs.WithContext(ctx).Error(fmt.Sprintf("Slack API error: %v", errorMsg))
			return nil, fmt.Errorf("slack API error: %v", errorMsg)
		}
	}

	// No more next_cursor, return consolidated response
	logs.WithContext(ctx).Debug(fmt.Sprintf("No more next_cursor found. Total users collected: %d", len(allUsers)))

	return allUsers, nil
}

func (slackTool *SlackTool) CreateChannel(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("CreateChannel Execute - Start")

	channelPayload, channelPayloadOk := params["channel_payload"]
	if !channelPayloadOk {
		err = errors.New("channel_payload parameter is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	url := fmt.Sprintf("%s/conversations.create", SLACK_BASE_URL)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", slackTool.getAccessToken(ctx, params)))
	headers.Set("Content-Type", "application/json")

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, channelPayload)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["result"] = res

	return toolResult, map[string]interface{}{"body": channelPayload}, false, nil
}

func (slackTool *SlackTool) InviteToChannel(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("InviteToChannel Execute - Start")

	invitePayload, invitePayloadOk := params["invite_payload"]
	if !invitePayloadOk {
		err = errors.New("invite_payload parameter is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	url := fmt.Sprintf("%s/conversations.invite", SLACK_BASE_URL)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", slackTool.getAccessToken(ctx, params)))
	headers.Set("Content-Type", "application/json")

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, invitePayload)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	toolResult["result"] = res

	return toolResult, map[string]interface{}{"body": invitePayload}, false, nil
}

func (slackTool *SlackTool) UploadMedia(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("UploadMedia Execute - Start")

	mediaFile, mediaFileOk := params["file"]
	if !mediaFileOk {
		err = logs.Err(ctx, errors.New("file parameter is required (base64 encoded content)"), "")
		return nil, nil, false, err
	}
	mediaFileStr, mediaFileStrOk := mediaFile.(string)
	if !mediaFileStrOk {
		err = logs.Err(ctx, errors.New("file must be a base64 encoded string"), "")
		return nil, nil, false, err
	}

	mediaFileName, mediaFileNameOk := params["file_name"]
	if !mediaFileNameOk {
		err = logs.Err(ctx, errors.New("file_name parameter is required"), "")
		return nil, nil, false, err
	}
	mediaFileNameStr, mediaFileNameStrOk := mediaFileName.(string)
	if !mediaFileNameStrOk {
		err = logs.Err(ctx, errors.New("file_name must be a string"), "")
		return nil, nil, false, err
	}

	fileBytes, err := base64.StdEncoding.DecodeString(mediaFileStr)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to decode base64 file: %s", err.Error()), "")
		return nil, nil, false, err
	}
	if len(fileBytes) == 0 {
		err = logs.Err(ctx, errors.New("file is empty"), "")
		return nil, nil, false, err
	}

	mimeLimit := uint32(2000)
	if mimeLimitParam, mimeLimitParamOk := params["mime_limit"]; mimeLimitParamOk {
		if mimeLimitInt, mimeLimitIntOk := mimeLimitParam.(uint32); mimeLimitIntOk {
			mimeLimit = mimeLimitInt
		}
	}
	mimetype.SetLimit(mimeLimit)
	fMime := mimetype.Detect(fileBytes)
	logs.WithContext(ctx).Info(fmt.Sprintf("File MIME: %s", fMime.String()))

	accessToken := slackTool.getAccessToken(ctx, params)

	getUrlFormData := map[string]string{
		"filename": mediaFileNameStr,
		"length":   strconv.Itoa(len(fileBytes)),
	}
	//alt_txt is defined for images only - slack leaves a non-image file unprocessed when it is sent
	if altText, altTextOk := params["alt_text"]; altTextOk && strings.HasPrefix(fMime.String(), "image/") {
		getUrlFormData["alt_txt"] = fmt.Sprintf("%v", altText)
	}
	if snippetType, snippetTypeOk := params["snippet_type"]; snippetTypeOk {
		getUrlFormData["snippet_type"] = fmt.Sprintf("%v", snippetType)
	}

	getUrlHeaders := http.Header{}
	getUrlHeaders.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	getUrlHeaders.Set("Content-Type", "application/x-www-form-urlencoded")

	getUrlRes, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, fmt.Sprintf("%s/files.getUploadURLExternal", SLACK_BASE_URL), getUrlHeaders, getUrlFormData, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var uploadUrlResponse SlackUploadUrlResponse
	getUrlResBytes, err := json.Marshal(getUrlRes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = json.Unmarshal(getUrlResBytes, &uploadUrlResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if !uploadUrlResponse.Ok {
		err = logs.Err(ctx, fmt.Errorf("slack API error: %s", uploadUrlResponse.Error), "")
		return nil, nil, false, err
	}
	if uploadUrlResponse.UploadUrl == "" || uploadUrlResponse.FileId == "" {
		err = logs.Err(ctx, errors.New("slack did not return upload_url and file_id"), "")
		return nil, nil, false, err
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Uploading file to slack with file_id: %s", uploadUrlResponse.FileId))

	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadUrlResponse.UploadUrl, bytes.NewReader(fileBytes))
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create upload request: %s", err.Error()), "")
		return nil, nil, false, err
	}
	uploadReq.Header.Set("Content-Type", fMime.String())
	uploadReq.ContentLength = int64(len(fileBytes))

	uploadClient := &http.Client{}
	uploadResp, err := uploadClient.Do(uploadReq)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to upload file: %s", err.Error()), "")
		return nil, nil, false, err
	}
	uploadRespBody, _ := io.ReadAll(uploadResp.Body)
	uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusOK {
		err = logs.Err(ctx, fmt.Errorf("file upload failed with status %d: %s", uploadResp.StatusCode, string(uploadRespBody)), "")
		return nil, nil, false, err
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("Uploaded %d bytes to slack, response: %s", len(fileBytes), string(uploadRespBody)))

	title := mediaFileNameStr
	if titleParam, titleParamOk := params["title"]; titleParamOk {
		title = fmt.Sprintf("%v", titleParam)
	}

	channel := ""
	if channelParam, channelParamOk := params["channel"]; channelParamOk {
		channel = fmt.Sprintf("%v", channelParam)
	} else if channelParam, channelParamOk = params["channel_id"]; channelParamOk {
		channel = fmt.Sprintf("%v", channelParam)
	}

	filesArgBytes, err := json.Marshal([]map[string]interface{}{{"id": uploadUrlResponse.FileId, "title": title}})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	completeFormData := map[string]string{"files": string(filesArgBytes)}
	if channel != "" {
		completeFormData["channel_id"] = channel
	}
	if initialComment, initialCommentOk := params["initial_comment"]; initialCommentOk {
		completeFormData["initial_comment"] = fmt.Sprintf("%v", initialComment)
	}
	if threadTs, threadTsOk := params["thread_ts"]; threadTsOk {
		completeFormData["thread_ts"] = fmt.Sprintf("%v", threadTs)
	}

	completeUrl := fmt.Sprintf("%s/files.completeUploadExternal", SLACK_BASE_URL)
	completeHeaders := http.Header{}
	completeHeaders.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	completeHeaders.Set("Content-Type", "application/x-www-form-urlencoded")

	//slack shares a file only into channels the app is a member of - chat:write.public does not cover file shares
	if channel != "" {
		joinParams := map[string]interface{}{"channel": channel}
		if tokenType, tokenTypeOk := params["token_type"]; tokenTypeOk {
			joinParams["token_type"] = tokenType
		}
		joinResult, _, _, joinErr := slackTool.JoinChannel(ctx, joinParams)
		if joinErr != nil {
			logs.WithContext(ctx).Info(fmt.Sprintf("could not join channel %s before sharing file: %v", channel, joinErr))
		} else {
			logs.WithContext(ctx).Info(fmt.Sprint("join channel result: ", joinResult["result"]))
		}
	}

	completeResponse, err := slackTool.completeUpload(ctx, completeUrl, completeHeaders, completeFormData)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolRequest = map[string]interface{}{"body": map[string]interface{}{
		"file_name": mediaFileNameStr,
		"file_size": len(fileBytes),
		"mime_type": fMime.String(),
		"title":     title,
		"channel":   channel,
	}}

	toolResult = make(map[string]interface{})
	toolResult["response"] = completeResponse
	toolResult["file_id"] = uploadUrlResponse.FileId
	if !completeResponse.Ok {
		toolResult["status"] = "failed"
		toolResult["error"] = completeResponse.Error
		return toolResult, toolRequest, false, nil
	}

	toolResult["status"] = "uploaded"
	if len(completeResponse.Files) > 0 {
		uploadedFile := completeResponse.Files[0]
		toolResult["permalink"] = uploadedFile.Permalink
		toolResult["url_private"] = uploadedFile.UrlPrivate
		if channel != "" && !slackTool.isFileShared(ctx, uploadedFile) {
			logs.WithContext(ctx).Info(fmt.Sprintf("file %s not shared by slack - confirming with files.info", uploadedFile.Id))
			fileInfoResponse, fileInfoErr := slackTool.getFileInfo(ctx, accessToken, uploadedFile.Id)
			if fileInfoErr == nil && slackTool.isFileShared(ctx, fileInfoResponse.File) {
				logs.WithContext(ctx).Info("slack shared the file after all - no fallback needed")
			} else {
				logs.WithContext(ctx).Info(fmt.Sprintf("sharing file %s into %s by posting its permalink", uploadedFile.Id, channel))
				shareErr := slackTool.shareFileByPermalink(ctx, params, channel, uploadedFile)
				if shareErr != nil {
					toolResult["status"] = "uploaded_not_shared"
					toolResult["warning"] = fmt.Sprintf("file uploaded but could not be shared into %s : %s", channel, shareErr.Error())
					logs.WithContext(ctx).Error(fmt.Sprintf("file %s uploaded but not shared into %s", uploadedFile.Id, channel))
				} else {
					toolResult["status"] = "uploaded_shared_via_link"
				}
			}
		}
	}

	return toolResult, toolRequest, false, nil
}

func (slackTool *SlackTool) isFileShared(ctx context.Context, uploadedFile SlackUploadedFile) bool {
	logs.WithContext(ctx).Debug("isFileShared - Start")
	return len(uploadedFile.Channels) > 0 || len(uploadedFile.Groups) > 0 || len(uploadedFile.Ims) > 0
}

func (slackTool *SlackTool) shareFileByPermalink(ctx context.Context, params map[string]interface{}, channel string, uploadedFile SlackUploadedFile) (err error) {
	logs.WithContext(ctx).Debug("shareFileByPermalink - Start")

	if uploadedFile.Permalink == "" {
		err = logs.Err(ctx, errors.New("slack did not return a permalink for the file"), "")
		return err
	}

	text := uploadedFile.Permalink
	if initialComment, initialCommentOk := params["initial_comment"]; initialCommentOk {
		text = fmt.Sprint(initialComment, "\n", uploadedFile.Permalink)
	}

	messagePayload := map[string]interface{}{
		"channel": channel,
		"text":    text,
	}
	if threadTs, threadTsOk := params["thread_ts"]; threadTsOk {
		messagePayload["thread_ts"] = fmt.Sprintf("%v", threadTs)
	}

	shareParams := map[string]interface{}{"message_payload": messagePayload}
	if tokenType, tokenTypeOk := params["token_type"]; tokenTypeOk {
		shareParams["token_type"] = tokenType
	}

	shareResult, _, _, err := slackTool.SendMessage(ctx, shareParams)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if status, statusOk := shareResult["status"]; statusOk && status != "sent" {
		err = logs.Err(ctx, fmt.Errorf("failed to post file permalink : %v", shareResult["error"]), "")
		return err
	}
	return nil
}

func (slackTool *SlackTool) completeUpload(ctx context.Context, url string, headers http.Header, formData map[string]string) (completeResponse SlackCompleteUploadResponse, err error) {
	logs.WithContext(ctx).Debug("completeUpload - Start")

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, formData, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return completeResponse, err
	}

	resBytes, err := json.Marshal(res)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return completeResponse, err
	}
	err = json.Unmarshal(resBytes, &completeResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return completeResponse, err
	}
	return completeResponse, nil
}

func (slackTool *SlackTool) getFileInfo(ctx context.Context, accessToken string, fileId string) (fileInfoResponse SlackFileInfoResponse, err error) {
	logs.WithContext(ctx).Debug("getFileInfo - Start")

	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	headers.Set("Content-Type", "application/x-www-form-urlencoded")

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, fmt.Sprintf("%s/files.info", SLACK_BASE_URL), headers, map[string]string{}, []*http.Cookie{}, map[string]string{"file": fileId}, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return fileInfoResponse, err
	}

	resBytes, err := json.Marshal(res)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return fileInfoResponse, err
	}
	err = json.Unmarshal(resBytes, &fileInfoResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return fileInfoResponse, err
	}
	if !fileInfoResponse.Ok {
		err = logs.Err(ctx, fmt.Errorf("slack API error: %s", fileInfoResponse.Error), "")
		return fileInfoResponse, err
	}
	return fileInfoResponse, nil
}

func (slackTool *SlackTool) DownloadMedia(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("DownloadMedia Execute - Start")

	fileId := ""
	if fileIdParam, fileIdParamOk := params["file_id"]; fileIdParamOk {
		fileId = fmt.Sprintf("%v", fileIdParam)
	}
	downloadUrl := ""
	if urlParam, urlParamOk := params["url"]; urlParamOk {
		downloadUrl = fmt.Sprintf("%v", urlParam)
	}
	if fileId == "" && downloadUrl == "" {
		err = logs.Err(ctx, errors.New("file_id parameter is required"), "")
		return nil, nil, false, err
	}

	accessToken := slackTool.getAccessToken(ctx, params)

	fileInfo := SlackUploadedFile{}
	if fileId != "" {
		fileInfoResponse, fileInfoErr := slackTool.getFileInfo(ctx, accessToken, fileId)
		if fileInfoErr != nil {
			logs.WithContext(ctx).Error(fileInfoErr.Error())
			return nil, nil, false, fileInfoErr
		}
		fileInfo = fileInfoResponse.File
		if downloadUrl == "" {
			downloadUrl = fileInfo.UrlPrivateDownload
		}
		if downloadUrl == "" {
			downloadUrl = fileInfo.UrlPrivate
		}
	}
	if downloadUrl == "" {
		err = logs.Err(ctx, errors.New("slack did not return a download url for the file"), "")
		return nil, nil, false, err
	}

	logs.WithContext(ctx).Info(fmt.Sprintf("Downloading slack file: %s", fileId))

	downloadReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to create download request: %s", err.Error()), "")
		return nil, nil, false, err
	}
	downloadReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	client := &http.Client{}
	downloadResp, err := client.Do(downloadReq)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to download file: %s", err.Error()), "")
		return nil, nil, false, err
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		err = logs.Err(ctx, fmt.Errorf("file download failed with status %d", downloadResp.StatusCode), "")
		return nil, nil, false, err
	}

	if strings.HasPrefix(downloadResp.Header.Get("Content-Type"), "text/html") {
		err = logs.Err(ctx, errors.New("file download not authorized - verify files:read scope and that the app has access to the file"), "")
		return nil, nil, false, err
	}

	fileBytes, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to read downloaded file: %s", err.Error()), "")
		return nil, nil, false, err
	}
	if len(fileBytes) == 0 {
		err = logs.Err(ctx, errors.New("downloaded file is empty"), "")
		return nil, nil, false, err
	}

	mimeLimit := uint32(2000)
	if mimeLimitParam, mimeLimitParamOk := params["mime_limit"]; mimeLimitParamOk {
		if mimeLimitInt, mimeLimitIntOk := mimeLimitParam.(uint32); mimeLimitIntOk {
			mimeLimit = mimeLimitInt
		}
	}
	mimetype.SetLimit(mimeLimit)
	detectedMime := mimetype.Detect(fileBytes)

	mimeType := fileInfo.Mimetype
	if mimeType == "" {
		mimeType = detectedMime.String()
	}
	fileName := fileInfo.Name
	if fileName == "" {
		fileName = fmt.Sprint("media", detectedMime.Extension())
	}

	toolResult = make(map[string]interface{})
	toolResult["file_content"] = base64.StdEncoding.EncodeToString(fileBytes)
	toolResult["file_name"] = fileName
	toolResult["mime_type"] = mimeType
	toolResult["file_size"] = len(fileBytes)
	toolResult["file_id"] = fileId
	toolResult["title"] = fileInfo.Title
	toolResult["permalink"] = fileInfo.Permalink

	return toolResult, map[string]interface{}{"query": map[string]string{"file": fileId}}, false, nil
}

func (slackTool *SlackTool) GetToolCallback() tools.ToolCallback {
	return tools.ToolCallback{
		ResponseContentType: "application/json",
	}
}

func (slackTool *SlackTool) Callback(ctx context.Context, projectId string, tenantId string, actionName string, body map[string]interface{}, params map[string][]string) (callbackResult interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Callback Execute - Start")
	// This callback handles:
	// 1. URL verification challenges from Slack Events API
	// 2. Message events (message.channels, message.groups, message.im, message.mpim)
	// 3. App mention events (app_mention)
	// 4. Reaction events (reaction_added, reaction_removed)
	// 5. File events (file_created, file_deleted, file_shared)
	// 6. User/team events (team_join, user_change)
	// All events are stored in database and forwarded to eru-functions for processing

	// Handle Slack URL verification challenge
	if challenge, challengeOk := body["challenge"]; challengeOk {
		if challengeType, typeOk := body["type"]; typeOk && challengeType == "url_verification" {
			logs.WithContext(ctx).Info("Slack URL verification challenge received")
			return challenge, false, nil
		}
	}

	// Handle event callback
	if eventType, typeOk := body["type"]; typeOk && eventType == "event_callback" {
		gm := server.GetGlobalGoroutineManager(ctx)
		gm.SafeGoWithRestartBehavior("slack-event-callback", func(bgCtx context.Context) {
			if eruFuncBaseUrl, ok := ctx.Value("Erufuncbaseurl").(string); ok {
				bgCtx = context.WithValue(bgCtx, "Erufuncbaseurl", eruFuncBaseUrl)
			}

			bodyBytes, err := json.Marshal(body)
			if err != nil {
				logs.WithContext(bgCtx).Error(err.Error())
				return
			}

			paramBytes, err := json.Marshal(params)
			if err != nil {
				logs.WithContext(bgCtx).Error(err.Error())
				return
			}

			var insertQueries []*models.Queries
			insertQueryFuncAsync := models.Queries{}
			insertQueryFuncAsync.Query = slackTool.ToolDb.GetDbQuery(bgCtx, INSERT_FUNC_ASYNC_SLACK)
			insertQueryFuncAsync.Vals = append(insertQueryFuncAsync.Vals, projectId, tenantId, string(bodyBytes), string(paramBytes))
			insertQueryFuncAsync.Rank = 1
			insertQueries = append(insertQueries, &insertQueryFuncAsync)

			_, insertOutputErr := utils.ExecuteDbSave(bgCtx, slackTool.ToolDb.GetConn(), insertQueries)
			if insertOutputErr != nil {
				logs.WithContext(bgCtx).Error(insertOutputErr.Error())
				return
			}

			var eventPayload SlackEventPayload
			err = json.Unmarshal(bodyBytes, &eventPayload)
			if err != nil {
				logs.WithContext(bgCtx).Error(err.Error())
				return
			}

			// Process different event types
			if eventPayload.Event.Type != "" {
				logs.WithContext(bgCtx).Info(fmt.Sprintf("Received Slack event: %s in channel: %s", eventPayload.Event.Type, eventPayload.Event.Channel))

				// Structure event data based on type for consistent processing
				eventDetails := map[string]interface{}{
					"event_type":   eventPayload.Event.Type,
					"event_ts":     eventPayload.Event.EventTs,
					"user":         eventPayload.Event.User,
					"channel":      eventPayload.Event.Channel,
					"channel_type": eventPayload.Event.ChannelType,
					"team_id":      eventPayload.TeamId,
					"api_app_id":   eventPayload.ApiAppId,
					"event_id":     eventPayload.EventId,
					"event_time":   eventPayload.EventTime,
					"tenant_id":    tenantId,
					"project_id":   projectId,
				}

				// Add event-specific data based on event type
				switch eventPayload.Event.Type {
				case "message":
					eventDetails["message"] = map[string]interface{}{
						"text":          eventPayload.Event.Text,
						"ts":            eventPayload.Event.Ts,
						"client_msg_id": eventPayload.Event.ClientMsgId,
						"thread_ts":     eventPayload.Event.Thread_ts,
						"blocks":        eventPayload.Event.Blocks,
						"files":         eventPayload.Event.Files,
					}
					logs.WithContext(bgCtx).Info(fmt.Sprintf("Message from user %s: %s", eventPayload.Event.User, eventPayload.Event.Text))

				case "app_mention":
					eventDetails["mention"] = map[string]interface{}{
						"text":      eventPayload.Event.Text,
						"ts":        eventPayload.Event.Ts,
						"thread_ts": eventPayload.Event.Thread_ts,
						"blocks":    eventPayload.Event.Blocks,
						"files":     eventPayload.Event.Files,
					}
					logs.WithContext(bgCtx).Info(fmt.Sprintf("App mentioned by user %s: %s", eventPayload.Event.User, eventPayload.Event.Text))

				case "reaction_added", "reaction_removed":
					eventDetails["reaction"] = map[string]interface{}{
						"reaction":     eventPayload.Event.Reaction,
						"item_type":    eventPayload.Event.Item.Type,
						"item_channel": eventPayload.Event.Item.Channel,
						"item_ts":      eventPayload.Event.Item.Ts,
					}
					logs.WithContext(bgCtx).Info(fmt.Sprintf("Reaction %s %s by user %s", eventPayload.Event.Reaction, eventPayload.Event.Type, eventPayload.Event.User))

				case "file_created", "file_deleted", "file_shared":
					eventDetails["file"] = map[string]interface{}{
						"file_id":   eventPayload.Event.FileId,
						"file_name": eventPayload.Event.File.Name,
						"file_type": eventPayload.Event.File.Id,
					}
					logs.WithContext(bgCtx).Info(fmt.Sprintf("File event %s by user %s", eventPayload.Event.Type, eventPayload.Event.User))

				case "team_join":
					eventDetails["new_user"] = map[string]interface{}{
						"user_id": eventPayload.Event.User,
					}
					logs.WithContext(bgCtx).Info(fmt.Sprintf("New user joined team: %s", eventPayload.Event.User))

				default:
					// For other event types, include the full event payload
					eventDetails["raw_event"] = eventPayload.Event
					logs.WithContext(bgCtx).Info(fmt.Sprintf("Other event type: %s", eventPayload.Event.Type))
				}

				hookBody := map[string]interface{}{
					"type":       "slack_event",
					"event":      eventDetails,
					"tenant_id":  tenantId,
					"event_time": eventPayload.EventTime,
				}

				hookResult, err := slackTool.ExecuteHook(bgCtx, "clbk", "", projectId, tenantId, hookBody, params)
				if err != nil {
					logs.WithContext(bgCtx).Error(err.Error())
					return
				}
				logs.WithContext(bgCtx).Info(fmt.Sprint("Slack event callback result: ", hookResult))
			}
		}, server.ContinueOnMaxRetries)
	}

	return "OK", false, nil
}

func (slackTool *SlackTool) GetToolCbUrl(projectId string, tenantId string) string {
	return fmt.Sprint(slackTool.CallbackBaseUrl, "/", projectId, "/", tenantId, "/callback/tool/", slackTool.ToolName)
}

func (slackTool *SlackTool) SetPrivateAttributes(ctx context.Context, realTool tools.Tooling) (err error) {
	slackTool.SlackAccount.AuthedUserAccessToken = fmt.Sprintf("$SECRET_%s_authed_user_access_token", slackTool.ToolName)
	slackTool.SlackAccount.AuthedUserId = fmt.Sprintf("$SECRET_%s_authed_user_id", slackTool.ToolName)
	slackTool.SlackAccount.BotAccessToken = fmt.Sprintf("$SECRET_%s_bot_access_token", slackTool.ToolName)
	slackTool.SlackAccount.BotUserId = fmt.Sprintf("$SECRET_%s_bot_user_id", slackTool.ToolName)

	return nil
}

func (slackTool *SlackTool) GetBytes(ctx context.Context) ([]byte, error) {
	slackToolWithToken := slackToolWithToken{
		Tool: slackTool.Tool,
		SlackAccount: slackAccountWithToken{
			TeamId:                slackTool.SlackAccount.TeamId,
			BotUserId:             slackTool.SlackAccount.BotUserId,
			AuthedUserAccessToken: slackTool.SlackAccount.AuthedUserAccessToken,
			BotAccessToken:        slackTool.SlackAccount.BotAccessToken,
			AuthedUserId:          slackTool.SlackAccount.AuthedUserId,
			TeamName:              slackTool.SlackAccount.TeamName,
			Enterprise:            slackTool.SlackAccount.Enterprise,
			IsEnterpriseInstall:   slackTool.SlackAccount.IsEnterpriseInstall,
			AppId:                 slackTool.SlackAccount.AppId,
		},
		AuthName: slackTool.AuthName,
	}

	toolJson, err := json.Marshal(slackToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}

func (slackTool *SlackTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	slackToolWithToken := slackToolWithToken{}
	err := json.Unmarshal(toolObjJson, &slackToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}

	slackTool = &SlackTool{
		Tool: slackToolWithToken.Tool,
		SlackAccount: SlackAccount{
			TeamId:                slackToolWithToken.SlackAccount.TeamId,
			BotUserId:             slackToolWithToken.SlackAccount.BotUserId,
			AuthedUserAccessToken: slackToolWithToken.SlackAccount.AuthedUserAccessToken,
			BotAccessToken:        slackToolWithToken.SlackAccount.BotAccessToken,
			AuthedUserId:          slackToolWithToken.SlackAccount.AuthedUserId,
			TeamName:              slackToolWithToken.SlackAccount.TeamName,
			Enterprise:            slackToolWithToken.SlackAccount.Enterprise,
			IsEnterpriseInstall:   slackToolWithToken.SlackAccount.IsEnterpriseInstall,
			AppId:                 slackToolWithToken.SlackAccount.AppId,
		},
		AuthName: slackToolWithToken.AuthName,
	}
	return slackTool, nil
}

func init() {
	tools.RegisterTool("SLACK", func() tools.Tooling { return new(SlackTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       true,
		ToolType:     "SLACK",
		Category:     "Communication",
		Description:  "Slack integration for messaging, channels, and webhooks",
		Actions:      []tools.ActionInfo{{Name: SendMessage}, {Name: ReadMessages}, {Name: Login}, {Name: GetSsoUrl}, {Name: SubscribeWebhooks}, {Name: ListChannels}, {Name: ListUsers}, {Name: CreateChannel}, {Name: InviteToChannel}, {Name: JoinChannel}, {Name: UploadMedia}, {Name: DownloadMedia}, {Name: Callback}},
		OAuthEnabled: true,
		Icon:         "PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxZW0iIGhlaWdodD0iMWVtIiB2aWV3Qm94PSIwIDAgMjU2IDI1NiI+PHBhdGggZmlsbD0iI2UwMWU1YSIgZD0iTTUzLjg0MSAxNjEuMzJjMCAxNC44MzItMTEuOTg3IDI2LjgyLTI2LjgxOSAyNi44MlMuMjAzIDE3Ni4xNTIuMjAzIDE2MS4zMmMwLTE0LjgzMSAxMS45ODctMjYuODE4IDI2LjgyLTI2LjgxOEg1My44NHptMTMuNDEgMGMwLTE0LjgzMSAxMS45ODctMjYuODE4IDI2LjgxOS0yNi44MThzMjYuODE5IDExLjk4NyAyNi44MTkgMjYuODE5djY3LjA0N2MwIDE0LjgzMi0xMS45ODcgMjYuODItMjYuODIgMjYuODJjLTE0LjgzIDAtMjYuODE4LTExLjk4OC0yNi44MTgtMjYuODJ6Ii8+PHBhdGggZmlsbD0iIzM2YzVmMCIgZD0iTTk0LjA3IDUzLjYzOGMtMTQuODMyIDAtMjYuODItMTEuOTg3LTI2LjgyLTI2LjgxOVM3OS4yMzkgMCA5NC4wNyAwczI2LjgxOSAxMS45ODcgMjYuODE5IDI2LjgxOXYyNi44MnptMCAxMy42MTNjMTQuODMyIDAgMjYuODE5IDExLjk4NyAyNi44MTkgMjYuODE5cy0xMS45ODcgMjYuODE5LTI2LjgyIDI2LjgxOUgyNi44MkMxMS45ODcgMTIwLjg4OSAwIDEwOC45MDIgMCA5NC4wNjljMC0xNC44MyAxMS45ODctMjYuODE4IDI2LjgxOS0yNi44MTh6Ii8+PHBhdGggZmlsbD0iIzJlYjY3ZCIgZD0iTTIwMS41NSA5NC4wN2MwLTE0LjgzMiAxMS45ODctMjYuODIgMjYuODE4LTI2LjgyczI2LjgyIDExLjk4OCAyNi44MiAyNi44MnMtMTEuOTg4IDI2LjgxOS0yNi44MiAyNi44MTlIMjAxLjU1em0tMTMuNDEgMGMwIDE0LjgzMi0xMS45ODggMjYuODE5LTI2LjgyIDI2LjgxOWMtMTQuODMxIDAtMjYuODE4LTExLjk4Ny0yNi44MTgtMjYuODJWMjYuODJDMTM0LjUwMiAxMS45ODcgMTQ2LjQ4OSAwIDE2MS4zMiAwczI2LjgxOSAxMS45ODcgMjYuODE5IDI2LjgxOXoiLz48cGF0aCBmaWxsPSIjZWNiMjJlIiBkPSJNMTYxLjMyIDIwMS41NWMxNC44MzIgMCAyNi44MiAxMS45ODcgMjYuODIgMjYuODE4cy0xMS45ODggMjYuODItMjYuODIgMjYuODJjLTE0LjgzMSAwLTI2LjgxOC0xMS45ODgtMjYuODE4LTI2LjgyVjIwMS41NXptMC0xMy40MWMtMTQuODMxIDAtMjYuODE4LTExLjk4OC0yNi44MTgtMjYuODJjMC0xNC44MzEgMTEuOTg3LTI2LjgxOCAyNi44MTktMjYuODE4aDY3LjI1YzE0LjgzMiAwIDI2LjgyIDExLjk4NyAyNi44MiAyNi44MTlzLTExLjk4OCAyNi44MTktMjYuODIgMjYuODE5eiIvPjwvc3ZnPg==",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(SlackTool{}), []string{}),
	})
}
