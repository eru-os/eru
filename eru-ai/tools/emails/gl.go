package emails

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	models "github.com/eru-os/eru/eru-models"
	"github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
	"google.golang.org/api/idtoken"
)

const (
	GlBaseUrl              = "https://gmail.googleapis.com"
	INSERT_FUNC_ASYNC_GL   = "insert into eruai_cb_glemail (project_id, tenant_id, request_body, request_params) values ($1, $2, $3, $4)"
	SELECT_LAST_HISTORY_GL = "select request_body->'notification'->>'history_id' as history_id from eruai_cb_glemail where project_id = $1 and tenant_id = $2 and request_body->'notification'->>'email_address' = $3 order by created_date desc limit 1"
	GlParamPaginate        = "paginate"
	GlParamMaxMessages     = "max_messages"
	glMaxResultsPerPage    = 500
)

var (
	glIdTokenValidator     *idtoken.Validator
	glIdTokenValidatorOnce sync.Once
	glIdTokenValidatorErr  error
)

func glGetIdTokenValidator(ctx context.Context) (*idtoken.Validator, error) {
	glIdTokenValidatorOnce.Do(func() {
		glIdTokenValidator, glIdTokenValidatorErr = idtoken.NewValidator(ctx)
	})
	return glIdTokenValidator, glIdTokenValidatorErr
}

func glGcpSubscriptionId(projectId string, tenantId string, toolName string) string {
	id := fmt.Sprintf("eru-%s-%s-%s-sub", projectId, tenantId, toolName)
	return glSanitizeGcpName(id)
}

func glSanitizeGcpName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == '~', r == '+', r == '%':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	if len(out) > 255 {
		out = out[:255]
	}
	return out
}

func glCallEventSubscribe(ctx context.Context, projectId string, eventName string, subscriptionId string, pushEndpoint string) (map[string]interface{}, error) {
	efurl, ok := ctx.Value(tools.EruFuncBaseUrlKey).(string)
	if !ok || efurl == "" {
		return nil, errors.New("erufuncbaseurl not found in context")
	}
	url := fmt.Sprint(efurl, "/store/", projectId, "/event/subscribe/", eventName)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	body := map[string]interface{}{
		"subscription_id": subscriptionId,
		"push_endpoint":   pushEndpoint,
	}
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, body)
	if err != nil {
		return nil, err
	}
	resMap, ok := res.(map[string]interface{})
	if !ok {
		return nil, errors.New("subscribe response is not a map")
	}
	return resMap, nil
}

func glCallEventUnsubscribe(ctx context.Context, projectId string, eventName string, subscriptionId string) error {
	efurl, ok := ctx.Value(tools.EruFuncBaseUrlKey).(string)
	if !ok || efurl == "" {
		return errors.New("erufuncbaseurl not found in context")
	}
	url := fmt.Sprint(efurl, "/store/", projectId, "/event/unsubscribe/", eventName, "/", subscriptionId)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	_, _, _, _, err := utils.CallHttp(ctx, http.MethodDelete, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	return err
}

func (glEmailTool *GlEmailTool) verifyOidcPush(ctx context.Context) error {
	if strings.EqualFold(os.Getenv("ERUAI_SKIP_OIDC_VERIFY"), "true") {
		logs.WithContext(ctx).Info("ERUAI_SKIP_OIDC_VERIFY=true - skipping oidc push verification")
		return nil
	}
	if glEmailTool.OidcServiceAccount == "" {
		return nil
	}
	authVal, _ := ctx.Value(tools.RequestAuthorizationKey).(string)
	if authVal == "" {
		return errors.New("missing Authorization header on push")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authVal, "Bearer "))
	if token == "" {
		return errors.New("empty bearer token on push")
	}
	audience := glEmailTool.OidcAudience
	validator, vErr := glGetIdTokenValidator(ctx)
	if vErr != nil {
		return fmt.Errorf("idtoken validator init failed: %w", vErr)
	}
	payload, vErr := validator.Validate(ctx, token, audience)
	if vErr != nil {
		return fmt.Errorf("oidc token validation failed: %w", vErr)
	}
	email, _ := payload.Claims["email"].(string)
	if email != glEmailTool.OidcServiceAccount {
		return fmt.Errorf("oidc email mismatch: got %q want %q", email, glEmailTool.OidcServiceAccount)
	}
	return nil
}

type GlPubSubPush struct {
	Message      GlPubSubMessage `json:"message"`
	Subscription string          `json:"subscription"`
}

type GlPubSubMessage struct {
	Data        string            `json:"data"`
	MessageId   string            `json:"messageId"`
	PublishTime string            `json:"publishTime"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

type GlNotification struct {
	EmailAddress string `json:"emailAddress"`
	HistoryId    uint64 `json:"historyId"`
}

type GlWatchResponse struct {
	HistoryId  string `json:"historyId"`
	Expiration string `json:"expiration"`
}

type GlHistoryListResponse struct {
	History       []GlHistoryRecord `json:"history"`
	HistoryId     string            `json:"historyId"`
	NextPageToken string            `json:"nextPageToken,omitempty"`
}

type GlHistoryRecord struct {
	Id            string                 `json:"id"`
	Messages      []GlHistoryMessageRef  `json:"messages,omitempty"`
	MessagesAdded []GlHistoryMessageItem `json:"messagesAdded,omitempty"`
}

type GlHistoryMessageItem struct {
	Message GlHistoryMessageRef `json:"message"`
}

type GlHistoryMessageRef struct {
	Id       string   `json:"id"`
	ThreadId string   `json:"threadId"`
	LabelIds []string `json:"labelIds,omitempty"`
}

type GlTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IdToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

type GlEmailTool struct {
	tools.Tool
	EmailAccount       EmailAccount `json:"email_account"`
	AuthName           string       `json:"auth_name"`
	EventName          string       `json:"event_name"`
	TopicName          string       `json:"topic_name,omitempty"`
	OidcAudience       string       `json:"oidc_audience,omitempty"`
	OidcServiceAccount string       `json:"oidc_service_account,omitempty"`
}

type glEmailToolWithToken struct {
	tools.Tool
	EmailAccount       emailAccountWithToken
	AuthName           string
	EventName          string
	TopicName          string
	OidcAudience       string
	OidcServiceAccount string
}

func (glEmailTool *GlEmailTool) GetActionsList() []tools.ActionInfo {
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
		{Name: ReadHistoryRange},
	}
}

func (glEmailTool *GlEmailTool) GetSpec() tools.Tooling {
	return glEmailTool
}

func (glEmailTool *GlEmailTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &glEmailTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (glEmailTool *GlEmailTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GlEmailTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case ReadEmail:
		toolResult, toolRequest, persistStore, err = glEmailTool.ReadEmail(ctx, params)
	case SendEmail:
		toolResult, toolRequest, persistStore, err = glEmailTool.SendEmail(ctx, params)
	case SubscribeEmail:
		toolResult, toolRequest, persistStore, err = glEmailTool.SubscribeEmail(ctx, projectId, tenantId, params, false)
	case ReadMessage:
		toolResult, toolRequest, persistStore, err = glEmailTool.ReadMessage(ctx, params)
	case GetSsoUrl:
		toolResult, toolRequest, persistStore, err = glEmailTool.GetSsoUrl(ctx, projectId, tenantId, params)
	case Login:
		toolResult, toolRequest, persistStore, err = glEmailTool.Login(ctx, projectId, tenantId, params, "")
	case RenewToken:
		toolResult, toolRequest, persistStore, err = glEmailTool.RenewToken(ctx, projectId, tenantId, params)
	case RenewSubscription:
		toolResult, toolRequest, persistStore, err = glEmailTool.RenewSubscription(ctx, projectId, tenantId, params)
	case StopAutoRenew:
		toolResult, toolRequest, persistStore, err = glEmailTool.StopAutoRenew(ctx, projectId, tenantId, params)
	case StopSubscription:
		toolResult, toolRequest, persistStore, err = glEmailTool.StopSubscription(ctx, projectId, tenantId, params)
	case ReadConversation:
		toolResult, toolRequest, persistStore, err = glEmailTool.ReadConversation(ctx, params)
	case ReadHistoryRange:
		toolResult, toolRequest, persistStore, err = glEmailTool.ReadHistoryRange(ctx, params)
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

		hookResult, err := glEmailTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (glEmailTool *GlEmailTool) ReadEmail(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ReadEmail Execute - Start")
	paginate := true
	switch v := params[GlParamPaginate].(type) {
	case bool:
		paginate = v
	case string:
		paginate = !(v == "false" || v == "0")
	}
	maxMessages := 0
	if v, ok := params[GlParamMaxMessages]; ok && v != nil {
		if parsed, pErr := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v))); pErr == nil && parsed > 0 {
			maxMessages = parsed
		}
	}

	qVals := url.Values{}
	pageToken := ""
	for k, v := range params {
		if k == GlParamPaginate || k == GlParamMaxMessages {
			continue
		}
		if k == "pageToken" {
			if v != nil {
				pageToken = strings.TrimSpace(fmt.Sprint(v))
			}
			continue
		}
		switch vv := v.(type) {
		case []interface{}:
			for _, item := range vv {
				if item == nil {
					continue
				}
				qVals.Add(k, fmt.Sprint(item))
			}
		case []string:
			for _, item := range vv {
				qVals.Add(k, item)
			}
		case nil:
			continue
		default:
			qVals.Add(k, fmt.Sprint(vv))
		}
	}

	pageSize := glMaxResultsPerPage
	if mr := qVals.Get("maxResults"); mr != "" {
		if parsed, pErr := strconv.Atoi(strings.TrimSpace(mr)); pErr == nil && parsed > 0 && parsed < glMaxResultsPerPage {
			pageSize = parsed
		}
	}
	if maxMessages > 0 && maxMessages < pageSize {
		pageSize = maxMessages
	}
	qVals.Set("maxResults", strconv.Itoa(pageSize))

	baseUrl := fmt.Sprint(GlBaseUrl, "/gmail/v1/users/me/messages")
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", glEmailTool.EmailAccount.AccessToken))
	headers.Set("Content-Type", "application/json")

	allMessages := make([]interface{}, 0)
	seen := make(map[string]bool)
	pages := 0
	resultSizeEstimate := float64(0)
	nextPageToken := ""
	for {
		if pageToken != "" {
			qVals.Set("pageToken", pageToken)
		} else {
			qVals.Del("pageToken")
		}
		reqUrl := baseUrl
		if encoded := qVals.Encode(); encoded != "" {
			reqUrl = fmt.Sprint(reqUrl, "?", encoded)
		}
		logs.WithContext(ctx).Info(fmt.Sprint("read_email url: ", reqUrl))
		res, _, _, _, callErr := utils.CallHttp(ctx, http.MethodGet, reqUrl, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
		if callErr != nil {
			logs.WithContext(ctx).Error(callErr.Error())
			return nil, nil, false, callErr
		}
		resMap, ok := res.(map[string]interface{})
		if !ok {
			err = errors.New("read_email result is not a map")
			logs.WithContext(ctx).Error(err.Error())
			return nil, nil, false, err
		}
		pages = pages + 1
		if rse, ok := resMap["resultSizeEstimate"].(float64); ok && rse > resultSizeEstimate {
			resultSizeEstimate = rse
		}
		msgs, _ := resMap["messages"].([]interface{})
		truncated := false
		for _, m := range msgs {
			if mm, ok := m.(map[string]interface{}); ok {
				if id, _ := mm["id"].(string); id != "" {
					if seen[id] {
						continue
					}
					seen[id] = true
				}
			}
			if maxMessages > 0 && len(allMessages) >= maxMessages {
				truncated = true
				break
			}
			allMessages = append(allMessages, m)
		}
		nextPageToken, _ = resMap["nextPageToken"].(string)
		if !paginate || truncated || nextPageToken == "" || nextPageToken == pageToken {
			break
		}
		if maxMessages > 0 && len(allMessages) >= maxMessages {
			break
		}
		pageToken = nextPageToken
	}
	logs.WithContext(ctx).Info(fmt.Sprint("read_email pages fetched: ", pages, " messages: ", len(allMessages)))

	if float64(len(allMessages)) > resultSizeEstimate {
		resultSizeEstimate = float64(len(allMessages))
	}
	emails := map[string]interface{}{
		"messages":           allMessages,
		"resultSizeEstimate": resultSizeEstimate,
	}
	if nextPageToken != "" && (!paginate || (maxMessages > 0 && len(allMessages) >= maxMessages)) {
		emails["nextPageToken"] = nextPageToken
	}
	toolResult = make(map[string]interface{})
	toolResult["emails"] = emails
	toolResult["count"] = len(allMessages)
	toolResult["pages"] = pages
	qVals.Del("pageToken")
	return toolResult, map[string]interface{}{"query": qVals.Encode(), "pages": pages, "count": len(allMessages)}, false, nil
}

func glFetchAttachmentBytes(ctx context.Context, accessToken string, messageId string, attachmentId string) (dataStd string, size int64, err error) {
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	headers.Set("Content-Type", "application/json")
	attUrl := fmt.Sprint(GlBaseUrl, "/gmail/v1/users/me/messages/", messageId, "/attachments/", attachmentId)
	attRes, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, attUrl, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		return "", 0, err
	}
	attMap, ok := attRes.(map[string]interface{})
	if !ok {
		return "", 0, errors.New("attachment result is not a map")
	}
	if s, ok := attMap["size"].(float64); ok {
		size = int64(s)
	}
	dataB64Url, _ := attMap["data"].(string)
	dataStd = dataB64Url
	if dataBytes, decErr := base64.RawURLEncoding.DecodeString(dataB64Url); decErr == nil {
		dataStd = base64.StdEncoding.EncodeToString(dataBytes)
	} else if dataBytes, decErr := base64.URLEncoding.DecodeString(dataB64Url); decErr == nil {
		dataStd = base64.StdEncoding.EncodeToString(dataBytes)
	}
	return dataStd, size, nil
}

func glInlineAttachments(ctx context.Context, accessToken string, messageId string, part map[string]interface{}) {
	if body, ok := part["body"].(map[string]interface{}); ok {
		if aid, _ := body["attachmentId"].(string); aid != "" {
			data, size, err := glFetchAttachmentBytes(ctx, accessToken, messageId, aid)
			if err != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("attachment fetch failed for ", aid, ": ", err.Error()))
			} else {
				body["data"] = data
				if size > 0 {
					body["size"] = size
				}
			}
		}
	}
	if parts, ok := part["parts"].([]interface{}); ok {
		for _, p := range parts {
			if pm, ok := p.(map[string]interface{}); ok {
				glInlineAttachments(ctx, accessToken, messageId, pm)
			}
		}
	}
}

func (glEmailTool *GlEmailTool) ReadMessage(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ReadMessage Execute - Start")
	messageId := params["message_id"].(string)
	format := "full"
	if f, ok := params["format"].(string); ok && f != "" {
		format = f
	}
	expandAttachments := false
	switch v := params["attachments"].(type) {
	case bool:
		expandAttachments = v
	case string:
		expandAttachments = v == "true" || v == "1"
	}
	url := fmt.Sprint(GlBaseUrl, "/gmail/v1/users/me/messages/", messageId, "?format=", format)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", glEmailTool.EmailAccount.AccessToken))
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
	if expandAttachments {
		if payload, ok := toolResult["payload"].(map[string]interface{}); ok {
			glInlineAttachments(ctx, glEmailTool.EmailAccount.AccessToken, messageId, payload)
		}
	}
	return toolResult, map[string]interface{}{"query": map[string]interface{}{"message_id": messageId, "format": format, "attachments": expandAttachments}}, false, nil
}

func (glEmailTool *GlEmailTool) fetchEmailsSince(ctx context.Context, startHistoryId string, maxHistoryId uint64) (mails []map[string]interface{}, latestHistoryId string, err error) {
	logs.WithContext(ctx).Debug("fetchEmailsSince - Start")
	if startHistoryId == "" {
		err = errors.New("startHistoryId is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, "", err
	}
	historyUrl := fmt.Sprint(GlBaseUrl, "/gmail/v1/users/me/history")
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", glEmailTool.EmailAccount.AccessToken))
	headers.Set("Content-Type", "application/json")

	seen := make(map[string]bool)
	pageToken := ""
	bounded := false
	for {
		qParams := map[string]string{
			"startHistoryId": startHistoryId,
			"historyTypes":   "messageAdded",
		}
		if pageToken != "" {
			qParams["pageToken"] = pageToken
		}
		res, _, _, _, callErr := utils.CallHttp(ctx, http.MethodGet, historyUrl, headers, map[string]string{}, []*http.Cookie{}, qParams, nil)
		if callErr != nil {
			logs.WithContext(ctx).Error(callErr.Error())
			return nil, latestHistoryId, callErr
		}
		resBytes, _ := json.Marshal(res)
		var list GlHistoryListResponse
		if uErr := json.Unmarshal(resBytes, &list); uErr != nil {
			logs.WithContext(ctx).Error(uErr.Error())
			return nil, latestHistoryId, uErr
		}
		if list.HistoryId != "" {
			latestHistoryId = list.HistoryId
		}

		for _, h := range list.History {
			if maxHistoryId > 0 {
				if recId, pErr := strconv.ParseUint(h.Id, 10, 64); pErr == nil && recId > maxHistoryId {
					bounded = true
					continue
				}
			}
			for _, ma := range h.MessagesAdded {
				if seen[ma.Message.Id] {
					continue
				}
				seen[ma.Message.Id] = true
				readMsg, _, _, readErr := glEmailTool.ReadMessage(ctx, map[string]interface{}{"message_id": ma.Message.Id, "attachments": true})
				if readErr != nil {
					logs.WithContext(ctx).Error(readErr.Error())
					continue
				}
				mails = append(mails, readMsg)
			}
		}

		if bounded || list.NextPageToken == "" {
			break
		}
		pageToken = list.NextPageToken
	}
	return mails, latestHistoryId, nil
}

func (glEmailTool *GlEmailTool) ReadHistoryRange(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ReadHistoryRange Execute - Start")
	fromVal, fromOk := params["from_history_id"]
	if !fromOk || fromVal == nil {
		err = errors.New("from_history_id is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	fromStr := strings.TrimSpace(fmt.Sprint(fromVal))
	if fromStr == "" {
		err = errors.New("from_history_id is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	var maxHid uint64
	if toVal, ok := params["to_history_id"]; ok && toVal != nil {
		toStr := strings.TrimSpace(fmt.Sprint(toVal))
		if toStr != "" {
			maxHid, err = strconv.ParseUint(toStr, 10, 64)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, nil, false, fmt.Errorf("to_history_id invalid: %w", err)
			}
		}
	}
	mails, latest, fetchErr := glEmailTool.fetchEmailsSince(ctx, fromStr, maxHid)
	if fetchErr != nil {
		return nil, nil, false, fetchErr
	}
	toolResult = map[string]interface{}{
		"emails":     mails,
		"count":      len(mails),
		"history_id": latest,
	}
	return toolResult, map[string]interface{}{"from_history_id": fromStr, "to_history_id": maxHid}, false, nil
}

func (glEmailTool *GlEmailTool) SubscribeEmail(ctx context.Context, projectId string, tenantId string, params map[string]interface{}, unsubscribe bool) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SubscribeEmail Execute - Start")

	if glEmailTool.EventName == "" {
		err = errors.New("event_name is required on tool config (must point to a GCP_PUBSUB event)")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	subscriptionId := glGcpSubscriptionId(projectId, tenantId, glEmailTool.ToolName)

	if unsubscribe {
		if delErr := glCallEventUnsubscribe(ctx, projectId, glEmailTool.EventName, subscriptionId); delErr != nil {
			logs.WithContext(ctx).Error(delErr.Error())
		}
		gmailHeaders := http.Header{}
		gmailHeaders.Set("Content-Type", "application/json")
		gmailHeaders.Set("Authorization", fmt.Sprintf("Bearer %s", glEmailTool.EmailAccount.AccessToken))
		stopUrl := fmt.Sprint(GlBaseUrl, "/gmail/v1/users/me/stop")
		_, _, _, _, err = utils.CallHttp(ctx, http.MethodPost, stopUrl, gmailHeaders, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, nil, false, err
		}
		jobName := fmt.Sprint(glEmailTool.ToolName, "_", glEmailTool.Tool.Hooks.ARSU, "_", tenantId)
		err = glEmailTool.Scheduler.Unschedule(ctx, "", jobName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		toolResult = make(map[string]interface{})
		glEmailTool.EmailAccount.SubscriptionId = ""
		glEmailTool.EmailAccount.SubscriptionExpirationDateTime = ""
		glEmailTool.EmailAccount.HistoryId = ""
		glEmailTool.TopicName = ""
		glEmailTool.OidcAudience = ""
		glEmailTool.OidcServiceAccount = ""
		persistStore = true
		toolResult["unsubscription_status"] = "success"
		return toolResult, map[string]interface{}{"body": nil}, persistStore, nil
	}

	pushEndpoint := glEmailTool.GetToolCbUrl(projectId, tenantId)
	subRes, subErr := glCallEventSubscribe(ctx, projectId, glEmailTool.EventName, subscriptionId, pushEndpoint)
	if subErr != nil {
		err = subErr
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	topicName, _ := subRes["topic_name"].(string)
	if topicName == "" {
		err = errors.New("topic_name not returned from event subscribe")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	glEmailTool.TopicName = topicName
	if v, ok := subRes["oidc_audience"].(string); ok {
		glEmailTool.OidcAudience = v
	}
	if v, ok := subRes["oidc_service_account"].(string); ok {
		glEmailTool.OidcServiceAccount = v
	}

	labelIds := []string{"INBOX"}
	if li, ok := params["label_ids"].([]interface{}); ok && len(li) > 0 {
		labelIds = labelIds[:0]
		for _, l := range li {
			if ls, ok := l.(string); ok {
				labelIds = append(labelIds, ls)
			}
		}
	}

	subPost := map[string]interface{}{
		"topicName":           topicName,
		"labelIds":            labelIds,
		"labelFilterBehavior": "INCLUDE",
	}

	gmailHeaders := http.Header{}
	gmailHeaders.Set("Content-Type", "application/json")
	gmailHeaders.Set("Authorization", fmt.Sprintf("Bearer %s", glEmailTool.EmailAccount.AccessToken))
	watchUrl := fmt.Sprint(GlBaseUrl, "/gmail/v1/users/me/watch")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, watchUrl, gmailHeaders, map[string]string{}, []*http.Cookie{}, map[string]string{}, subPost)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	logs.WithContext(ctx).Info(fmt.Sprint(res))

	resBytes, _ := json.Marshal(res)
	var watchRes GlWatchResponse
	err = json.Unmarshal(resBytes, &watchRes)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	expirationTime := ""
	if watchRes.Expiration != "" {
		if expMs, parseErr := strconv.ParseInt(watchRes.Expiration, 10, 64); parseErr == nil {
			expirationTime = time.Unix(0, expMs*int64(time.Millisecond)).UTC().Format(time.RFC3339)
		}
	}

	glEmailTool.EmailAccount.SubscriptionId = subscriptionId
	glEmailTool.EmailAccount.SubscriptionExpirationDateTime = expirationTime
	glEmailTool.EmailAccount.HistoryId = watchRes.HistoryId
	persistStore = true

	if glEmailTool.Tool.Hooks.ARSU != "" {
		hookBody := map[string]interface{}{
			"Vars": map[string]interface{}{
				"Body": map[string]interface{}{
					"tool_name": glEmailTool.ToolName,
					"tenant_id": tenantId,
				},
				"OrgBody": map[string]interface{}{
					"tool_name": glEmailTool.ToolName,
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
		jobName := fmt.Sprint(glEmailTool.ToolName, "_", glEmailTool.Tool.Hooks.ARSU, "_", tenantId)
		err = glEmailTool.Scheduler.Unschedule(ctx, "", jobName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}
		cronStr := utils.GetCronStr(ctx, time.Now().UTC().Add(24*time.Hour))
		schedulerCommand := fmt.Sprint("CALL schedule_procedure('", glEmailTool.Tool.Hooks.ARSU, "','", string(hookBodyBytes), "','", glEmailTool.Scheduler.GetSchedulerName(), "')")
		jobId, err := glEmailTool.Scheduler.Schedule(ctx, jobName, schedulerCommand, cronStr)
		if err != nil {
			return nil, nil, persistStore, err
		}
		logs.WithContext(ctx).Info(fmt.Sprint("jobId: ", jobId))
	}
	toolResult = make(map[string]interface{})
	toolResult["subscription_status"] = "success"
	toolResult["history_id"] = watchRes.HistoryId
	toolResult["expiration"] = expirationTime
	return toolResult, map[string]interface{}{"body": subPost}, persistStore, nil
}

func emailNormalizeRecipients(v interface{}) []string {
	switch vv := v.(type) {
	case string:
		if vv == "" {
			return nil
		}
		return []string{vv}
	case []string:
		return vv
	case []interface{}:
		out := make([]string, 0, len(vv))
		for _, x := range vv {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func (glEmailTool *GlEmailTool) SendEmail(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("SendEmail Execute - Start")

	to := emailNormalizeRecipients(params["to"])
	cc := emailNormalizeRecipients(params["cc"])
	bcc := emailNormalizeRecipients(params["bcc"])
	replyTo, _ := params["reply_to"].(string)
	subject, _ := params["subject"].(string)
	body, _ := params["body"].(string)
	bodyType, _ := params["body_type"].(string)
	threadId, _ := params["thread_id"].(string)
	inReplyTo, _ := params["in_reply_to"].(string)
	references := strings.Join(emailNormalizeRecipients(params["references"]), " ")

	if len(to) == 0 && len(cc) == 0 && len(bcc) == 0 {
		err = errors.New("at least one of to, cc or bcc is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if subject == "" {
		err = errors.New("subject is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if bodyType == "" {
		bodyType = "html"
	}
	contentType := "text/html"
	if bodyType == "text" {
		contentType = "text/plain"
	}

	var attachments []map[string]interface{}
	if rawAtts, ok := params["attachments"].([]interface{}); ok {
		for _, a := range rawAtts {
			if am, ok := a.(map[string]interface{}); ok {
				attachments = append(attachments, am)
			}
		}
	}

	var msg bytes.Buffer
	if len(to) > 0 {
		fmt.Fprintf(&msg, "To: %s\r\n", strings.Join(to, ", "))
	}
	if len(cc) > 0 {
		fmt.Fprintf(&msg, "Cc: %s\r\n", strings.Join(cc, ", "))
	}
	if len(bcc) > 0 {
		fmt.Fprintf(&msg, "Bcc: %s\r\n", strings.Join(bcc, ", "))
	}
	if replyTo != "" {
		fmt.Fprintf(&msg, "Reply-To: %s\r\n", replyTo)
	}
	if inReplyTo != "" {
		fmt.Fprintf(&msg, "In-Reply-To: %s\r\n", inReplyTo)
	}
	if references != "" {
		fmt.Fprintf(&msg, "References: %s\r\n", references)
	}
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	if hdrs, ok := params["headers"].(map[string]interface{}); ok {
		for hk, hv := range hdrs {
			if hk == "" || hv == nil || strings.EqualFold(hk, "From") {
				continue
			}
			fmt.Fprintf(&msg, "%s: %s\r\n", hk, fmt.Sprint(hv))
		}
	}
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")

	if len(attachments) == 0 {
		fmt.Fprintf(&msg, "Content-Type: %s; charset=UTF-8\r\n\r\n", contentType)
		msg.WriteString(body)
	} else {
		boundary := fmt.Sprintf("erub_%d", time.Now().UnixNano())
		fmt.Fprintf(&msg, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", boundary)
		fmt.Fprintf(&msg, "--%s\r\n", boundary)
		fmt.Fprintf(&msg, "Content-Type: %s; charset=UTF-8\r\n\r\n", contentType)
		msg.WriteString(body)
		msg.WriteString("\r\n")
		for _, att := range attachments {
			filename, _ := att["filename"].(string)
			mimeType, _ := att["mime_type"].(string)
			data, _ := att["data"].(string)
			if filename == "" || data == "" {
				continue
			}
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			fmt.Fprintf(&msg, "--%s\r\n", boundary)
			fmt.Fprintf(&msg, "Content-Type: %s\r\n", mimeType)
			fmt.Fprintf(&msg, "Content-Disposition: attachment; filename=%q\r\n", filename)
			fmt.Fprintf(&msg, "Content-Transfer-Encoding: base64\r\n\r\n")
			for i := 0; i < len(data); i += 76 {
				end := i + 76
				if end > len(data) {
					end = len(data)
				}
				msg.WriteString(data[i:end])
				msg.WriteString("\r\n")
			}
		}
		fmt.Fprintf(&msg, "--%s--\r\n", boundary)
	}

	raw := base64.URLEncoding.EncodeToString(msg.Bytes())
	sendBody := map[string]interface{}{"raw": raw}
	if threadId != "" {
		sendBody["threadId"] = threadId
	}

	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", glEmailTool.EmailAccount.AccessToken))
	headers.Set("Content-Type", "application/json")
	sendUrl := fmt.Sprint(GlBaseUrl, "/gmail/v1/users/me/messages/send")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, sendUrl, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, sendBody)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	toolResult = make(map[string]interface{})
	if rm, ok := res.(map[string]interface{}); ok {
		toolResult = rm
	} else {
		toolResult["result"] = res
	}
	return toolResult, map[string]interface{}{"body": params}, false, nil
}

func (glEmailTool *GlEmailTool) GetToolCallback() tools.ToolCallback {
	return tools.ToolCallback{
		ResponseContentType: "application/json",
	}
}

func (glEmailTool *GlEmailTool) Callback(ctx context.Context, projectId string, tenantId string, actionName string, body map[string]interface{}, params map[string][]string) (callbackResult interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Callback Execute - Start")

	if verifyErr := glEmailTool.verifyOidcPush(ctx); verifyErr != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("oidc verify failed: ", verifyErr.Error()))
		return "", false, nil
	}

	gm := server.GetGlobalGoroutineManager(ctx)
	gm.SafeGoWithRestartBehavior("gl-email-callback", func(bgCtx context.Context) {
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

		var push GlPubSubPush
		err = json.Unmarshal(bodyBytes, &push)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}

		if push.Message.Data == "" {
			logs.WithContext(bgCtx).Info("pubsub message has no data, skipping")
			return
		}

		dataBytes, err := base64.StdEncoding.DecodeString(push.Message.Data)
		if err != nil {
			dataBytes, err = base64.URLEncoding.DecodeString(push.Message.Data)
			if err != nil {
				logs.WithContext(bgCtx).Error(err.Error())
				return
			}
		}

		var notification GlNotification
		err = json.Unmarshal(dataBytes, &notification)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}

		startHistoryId := ""
		if notification.EmailAddress != "" {
			selectQ := models.Queries{}
			selectQ.Query = glEmailTool.ToolDb.GetDbQuery(bgCtx, SELECT_LAST_HISTORY_GL)
			selectQ.Vals = append(selectQ.Vals, projectId, tenantId, notification.EmailAddress)
			rows, selectErr := utils.ExecuteDbFetch(bgCtx, glEmailTool.ToolDb.GetConn(), selectQ)
			if selectErr != nil {
				logs.WithContext(bgCtx).Error(selectErr.Error())
			} else if len(rows) > 0 {
				if hid := rows[0]["history_id"]; hid != nil {
					switch v := hid.(type) {
					case string:
						startHistoryId = v
					case []byte:
						startHistoryId = string(v)
					default:
						startHistoryId = fmt.Sprint(v)
					}
				}
			}
		}

		enrichedBody := map[string]interface{}{
			"envelope": body,
			"notification": map[string]interface{}{
				"email_address": notification.EmailAddress,
				"history_id":    notification.HistoryId,
			},
		}
		enrichedBytes, err := json.Marshal(enrichedBody)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}

		var insertQueries []*models.Queries
		insertQueryFuncAsync := models.Queries{}
		insertQueryFuncAsync.Query = glEmailTool.ToolDb.GetDbQuery(bgCtx, INSERT_FUNC_ASYNC_GL)
		insertQueryFuncAsync.Vals = append(insertQueryFuncAsync.Vals, projectId, tenantId, string(enrichedBytes), string(paramBytes))
		insertQueryFuncAsync.Rank = 1
		insertQueries = append(insertQueries, &insertQueryFuncAsync)
		_, insertOutputErr := utils.ExecuteDbSave(bgCtx, glEmailTool.ToolDb.GetConn(), insertQueries)
		if insertOutputErr != nil {
			logs.WithContext(bgCtx).Error(insertOutputErr.Error())
			return
		}

		if startHistoryId == "" {
			startHistoryId = glEmailTool.EmailAccount.HistoryId
		}
		if startHistoryId == "" {
			logs.WithContext(bgCtx).Info("no prior history id and no watch fallback, skipping fetch")
			return
		}

		mails, _, fetchErr := glEmailTool.fetchEmailsSince(bgCtx, startHistoryId, 0)
		if fetchErr != nil {
			logs.WithContext(bgCtx).Error(fetchErr.Error())
			return
		}
		for _, mail := range mails {
			hookBody := map[string]interface{}{
				"mail":                  mail,
				"tenant_id":             tenantId,
				"subscription_email_id": notification.EmailAddress,
			}
			hookResult, hookErr := glEmailTool.ExecuteHook(bgCtx, "clbk", "", projectId, tenantId, hookBody, params)
			if hookErr != nil {
				logs.WithContext(bgCtx).Error(hookErr.Error())
				continue
			}
			logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
		}

	}, server.ContinueOnMaxRetries)

	return "", false, nil
}

func (glEmailTool *GlEmailTool) GetToolCbUrl(projectId string, tenantId string) string {
	return fmt.Sprint(glEmailTool.CallbackBaseUrl, "/", projectId, "/callback/", tenantId, "/tool/", glEmailTool.ToolName)
}

func (glEmailTool *GlEmailTool) GetSsoUrl(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetSsoUrl Execute - Start")
	if glEmailTool.AuthName == "" {
		err = errors.New("auth name is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", glEmailTool.AuthName, "/getssourl")
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

func (glEmailTool *GlEmailTool) RenewToken(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	params["refresh_token"] = glEmailTool.EmailAccount.RefreshToken
	return glEmailTool.Login(ctx, projectId, tenantId, params, "/renew")
}

func (glEmailTool *GlEmailTool) RenewSubscription(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("RenewSubscription Execute - Start")
	return glEmailTool.SubscribeEmail(ctx, projectId, tenantId, params, false)
}

func (glEmailTool *GlEmailTool) Login(ctx context.Context, projectId string, tenantId string, params map[string]interface{}, renewStr string) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("Login Execute - Start")
	if glEmailTool.AuthName == "" {
		err = errors.New("auth name is required")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	eruauthUrl := ctx.Value("eruauthbaseurl").(string)
	url := fmt.Sprint(eruauthUrl, "/", projectId, "/", glEmailTool.AuthName, "/idptoken", renewStr)
	logs.WithContext(ctx).Info(fmt.Sprint("url: ", url))
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, params)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	var glTokens GlTokens
	resBytes, _ := json.Marshal(res)
	err = json.Unmarshal(resBytes, &glTokens)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	err = glEmailTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(glEmailTool.ToolName, "_access_token"), glTokens.AccessToken)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if glTokens.RefreshToken != "" {
		err = glEmailTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(glEmailTool.ToolName, "_refresh_token"), glTokens.RefreshToken)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, nil, false, err
		}
	}
	if glTokens.IdToken != "" {
		err = glEmailTool.SaveTenantSecret(ctx, projectId, tenantId, fmt.Sprint(glEmailTool.ToolName, "_id_token"), glTokens.IdToken)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return nil, nil, false, err
		}
	}

	glEmailTool.EmailAccount.TokenExpirationDateTime = time.Now().UTC().Add(time.Duration(glTokens.ExpiresIn) * time.Second).Format(time.RFC3339)
	persistStore = true

	if glEmailTool.Tool.Hooks.ARRT != "" {
		hookBody := map[string]interface{}{
			"Vars": map[string]interface{}{
				"Body": map[string]interface{}{
					"tool_name": glEmailTool.ToolName,
					"tenant_id": tenantId,
				},
				"OrgBody": map[string]interface{}{
					"tool_name": glEmailTool.ToolName,
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
		jobName := fmt.Sprint(glEmailTool.ToolName, "_", glEmailTool.Tool.Hooks.ARRT, "_", tenantId)
		err = glEmailTool.Scheduler.Unschedule(ctx, "", jobName)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
		}

		schedulerCommand := fmt.Sprint("CALL schedule_procedure('", glEmailTool.Tool.Hooks.ARRT, "','", string(hookBodyBytes), "','", glEmailTool.Scheduler.GetSchedulerName(), "')")

		cronStr := utils.GetCronStr(ctx, time.Now().UTC().Add(45*time.Minute))
		jobId, err := glEmailTool.Scheduler.Schedule(ctx, jobName, schedulerCommand, cronStr)
		if err != nil {
			return nil, nil, persistStore, err
		}
		logs.WithContext(ctx).Info(fmt.Sprint("jobId: ", jobId))
	}
	toolResult = make(map[string]interface{})
	toolResult["login_status"] = "success"
	return toolResult, map[string]interface{}{"body": params}, persistStore, nil
}

func (glEmailTool *GlEmailTool) SetPrivateAttributes(ctx context.Context, realTool tools.Tooling) (err error) {
	glEmailTool.EmailAccount.AccessToken = fmt.Sprint("$SECRET_", glEmailTool.ToolName, "_access_token")
	glEmailTool.EmailAccount.RefreshToken = fmt.Sprint("$SECRET_", glEmailTool.ToolName, "_refresh_token")
	return nil
}

func (glEmailTool *GlEmailTool) GetBytes(ctx context.Context) ([]byte, error) {

	glEmailToolWithToken := glEmailToolWithToken{
		Tool: glEmailTool.Tool,
		EmailAccount: emailAccountWithToken{
			DisplayName:                    glEmailTool.EmailAccount.DisplayName,
			AccessToken:                    glEmailTool.EmailAccount.AccessToken,
			RefreshToken:                   glEmailTool.EmailAccount.RefreshToken,
			SubscriptionId:                 glEmailTool.EmailAccount.SubscriptionId,
			SubscriptionExpirationDateTime: glEmailTool.EmailAccount.SubscriptionExpirationDateTime,
			TokenExpirationDateTime:        glEmailTool.EmailAccount.TokenExpirationDateTime,
			HistoryId:                      glEmailTool.EmailAccount.HistoryId,
		},
		AuthName:           glEmailTool.AuthName,
		EventName:          glEmailTool.EventName,
		TopicName:          glEmailTool.TopicName,
		OidcAudience:       glEmailTool.OidcAudience,
		OidcServiceAccount: glEmailTool.OidcServiceAccount,
	}

	toolJson, err := json.Marshal(glEmailToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}

func (glEmailTool *GlEmailTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	glEmailToolWithToken := glEmailToolWithToken{}
	err := json.Unmarshal(toolObjJson, &glEmailToolWithToken)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	glEmailTool = &GlEmailTool{
		Tool: glEmailToolWithToken.Tool,
		EmailAccount: EmailAccount{
			DisplayName:                    glEmailToolWithToken.EmailAccount.DisplayName,
			AccessToken:                    glEmailToolWithToken.EmailAccount.AccessToken,
			RefreshToken:                   glEmailToolWithToken.EmailAccount.RefreshToken,
			SubscriptionId:                 glEmailToolWithToken.EmailAccount.SubscriptionId,
			SubscriptionExpirationDateTime: glEmailToolWithToken.EmailAccount.SubscriptionExpirationDateTime,
			TokenExpirationDateTime:        glEmailToolWithToken.EmailAccount.TokenExpirationDateTime,
			HistoryId:                      glEmailToolWithToken.EmailAccount.HistoryId,
		},
		AuthName:           glEmailToolWithToken.AuthName,
		EventName:          glEmailToolWithToken.EventName,
		TopicName:          glEmailToolWithToken.TopicName,
		OidcAudience:       glEmailToolWithToken.OidcAudience,
		OidcServiceAccount: glEmailToolWithToken.OidcServiceAccount,
	}
	return glEmailTool, nil
}

func (glEmailTool *GlEmailTool) StopAutoRenew(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	if glEmailTool.Scheduler == nil {
		err = errors.New("scheduler not defined")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	glEmailTool.Scheduler.Unschedule(ctx, "", fmt.Sprint(glEmailTool.ToolName, "_", glEmailTool.Tool.Hooks.ARRT, "_", tenantId))
	toolResult = make(map[string]interface{})
	toolResult["stop_auto_renew_status"] = "success"
	glEmailTool.EmailAccount.TokenExpirationDateTime = ""
	persistStore = true
	return toolResult, map[string]interface{}{"body": params}, persistStore, nil
}

func (glEmailTool *GlEmailTool) StopSubscription(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("StopSubscription Execute - Start")
	return glEmailTool.SubscribeEmail(ctx, projectId, tenantId, params, true)
}

func (glEmailTool *GlEmailTool) ReadConversation(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ReadConversation Execute - Start")

	type rcParams struct {
		ConversationId string `json:"conversation_id"`
		Format         string `json:"format"`
	}
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
	if rcParamsObj.ConversationId == "" {
		err = errors.New("conversation_id is required")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if rcParamsObj.Format == "" {
		rcParamsObj.Format = "full"
	}

	url := fmt.Sprint(GlBaseUrl, "/gmail/v1/users/me/threads/", rcParamsObj.ConversationId, "?format=", rcParamsObj.Format)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", glEmailTool.EmailAccount.AccessToken))
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, map[string]string{}, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	threadMap, ok := res.(map[string]interface{})
	if !ok {
		err = errors.New("thread result is not a map")
		logs.Err(ctx, err, "")
		return nil, nil, false, err
	}
	messages, _ := threadMap["messages"].([]interface{})

	toolResult = make(map[string]interface{})
	toolResult["conversation_messages"] = messages
	toolResult["total_messages"] = len(messages)
	toolResult["thread"] = threadMap

	return toolResult, map[string]interface{}{"body": params}, false, nil
}

func init() {
	tools.RegisterTool("GL_EMAIL", func() tools.Tooling { return new(GlEmailTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       true,
		ToolType:     "GL_EMAIL",
		Category:     "Communication",
		Description:  "Google (Gmail) email integration for reading, sending, and subscribing to emails via Gmail API and Pub/Sub",
		Actions:      []tools.ActionInfo{{Name: ReadEmail}, {Name: SendEmail}, {Name: SubscribeEmail}, {Name: ReadMessage}, {Name: GetSsoUrl}, {Name: Login}, {Name: RenewToken}, {Name: RenewSubscription}, {Name: ReadHistoryRange}},
		OAuthEnabled: true,
		Icon:         "PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxLjMzZW0iIGhlaWdodD0iMWVtIiB2aWV3Qm94PSIwIDAgMjU2IDE5MyI+PHBhdGggZmlsbD0iIzQyODVmNCIgZD0iTTU4LjE4MiAxOTIuMDVWOTMuMTRMMjcuNTA3IDY1LjA3N0wwIDQ5LjUwNHYxMjUuMDkxYzAgOS42NTggNy44MjUgMTcuNDU1IDE3LjQ1NSAxNy40NTV6Ii8+PHBhdGggZmlsbD0iIzM0YTg1MyIgZD0iTTE5Ny44MTggMTkyLjA1aDQwLjcyN2M5LjY1OSAwIDE3LjQ1NS03LjgyNiAxNy40NTUtMTcuNDU1VjQ5LjUwNWwtMzEuMTU2IDE3LjgzN2wtMjcuMDI2IDI1Ljc5OHoiLz48cGF0aCBmaWxsPSIjZWE0MzM1IiBkPSJtNTguMTgyIDkzLjE0bC00LjE3NC0zOC42NDdsNC4xNzQtMzYuOTg5TDEyOCA2OS44NjhsNjkuODE4LTUyLjM2NGw0LjY2OSAzNC45OTJsLTQuNjY5IDQwLjY0NEwxMjggMTQ1LjUwNHoiLz48cGF0aCBmaWxsPSIjZmJiYzA0IiBkPSJNMTk3LjgxOCAxNy41MDRWOTMuMTRMMjU2IDQ5LjUwNFYyNi4yMzFjMC0yMS41ODUtMjQuNjQtMzMuODktNDEuODktMjAuOTQ1eiIvPjxwYXRoIGZpbGw9IiNjNTIyMWYiIGQ9Im0wIDQ5LjUwNGwyNi43NTkgMjAuMDdMNTguMTgyIDkzLjE0VjE3LjUwNEw0MS44OSA1LjI4NkMyNC42MS03LjY2IDAgNC42NDYgMCAyNi4yM3oiLz48L3N2Zz4=",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(GlEmailTool{}), []string{}),
	})
}
