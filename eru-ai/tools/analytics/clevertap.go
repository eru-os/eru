package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	FetchEvents = "fetch_events"
)

type ClevertapTool struct {
	tools.Tool
	ClevertapAccount ClevertapAccount `json:"clevertap_account"`
}

type ClevertapAccount struct {
	BaseUrl   string `json:"base_url" eru:"required"`
	AccountId string `json:"account_id" eru:"required"`
	Passcode  string `json:"passcode" secret:"true" eru:"required"`
}

type FetchEventsParams struct {
	EventName string `json:"event_name" eru:"required"`
	From      int    `json:"from" eru:"required"`
	To        int    `json:"to" eru:"required"`
	BatchSize int    `json:"batch_size"`
	AllEvents bool   `json:"all_events"`
}

func (clevertapTool *ClevertapTool) GetActionsList() []tools.ActionInfo {
	return []tools.ActionInfo{
		{Name: FetchEvents},
	}
}

func (clevertapTool *ClevertapTool) GetSpec() tools.Tooling {
	return clevertapTool
}

func (clevertapTool *ClevertapTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &clevertapTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (clevertapTool *ClevertapTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("ClevertapTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case FetchEvents:
		toolResult, toolRequest, persistStore, err = clevertapTool.ExecuteFetchEvents(ctx, params)
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

		hookResult, err := clevertapTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (clevertapTool *ClevertapTool) ExecuteFetchEvents(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("FetchEvents Execute - Start")

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to marshal params: %s", err.Error()), "failed to marshal params")
		return nil, nil, false, err
	}

	fetchEventsParams := FetchEventsParams{
		BatchSize: 50,
	}
	err = json.Unmarshal(paramsBytes, &fetchEventsParams)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to unmarshal fetch events params: %s", err.Error()), "failed to unmarshal fetch events params")
		return nil, nil, false, err
	}

	err = utils.ValidateStruct(ctx, fetchEventsParams, "")
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("invalid fetch events params: %s", err.Error()), fmt.Sprintf("invalid fetch events params: %s", err.Error()))
		return nil, nil, false, err
	}

	url := fmt.Sprintf("%s/1/events.json", clevertapTool.ClevertapAccount.BaseUrl)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-CleverTap-Account-Id", clevertapTool.ClevertapAccount.AccountId)
	headers.Set("X-CleverTap-Passcode", clevertapTool.ClevertapAccount.Passcode)

	payload := map[string]interface{}{
		"event_name": fetchEventsParams.EventName,
		"from":       fetchEventsParams.From,
		"to":         fetchEventsParams.To,
	}

	queryParams := map[string]string{
		"batch_size": fmt.Sprintf("%d", fetchEventsParams.BatchSize),
	}

	res, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, payload)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to fetch events: %s", err.Error()), "failed to fetch events")
		return nil, nil, false, err
	}

	resMap, ok := res.(map[string]interface{})
	if !ok {
		toolResult = map[string]interface{}{"response": res}
		return toolResult, payload, false, nil
	}

	cursor, _ := resMap["cursor"].(string)
	if cursor == "" {
		if !fetchEventsParams.AllEvents {
			if records, rOk := resMap["records"].([]interface{}); rOk {
				resMap["records"] = filterRecordEvents(records, fetchEventsParams.EventName)
			}
		}
		return resMap, payload, false, nil
	}

	var allRecords []interface{}

	for cursor != "" {
		decodedCursor, decErr := neturl.QueryUnescape(cursor)
		if decErr != nil {
			decodedCursor = cursor
		}
		cursorParams := map[string]string{
			"cursor": decodedCursor,
		}

		cursor_payload := map[string]interface{}{}

		cursorRes, _, _, _, cursorErr := utils.CallHttp(ctx, http.MethodPost, url, headers, map[string]string{}, []*http.Cookie{}, cursorParams, cursor_payload)
		if cursorErr != nil {
			err = logs.Err(ctx, fmt.Errorf("failed to fetch events with cursor: %s", cursorErr.Error()), "failed to fetch events with cursor")
			return nil, nil, false, err
		}

		cursorResMap, cursorOk := cursorRes.(map[string]interface{})
		if !cursorOk {
			break
		}

		if records, rOk := cursorResMap["records"].([]interface{}); rOk {
			allRecords = append(allRecords, records...)
		}

		nextCursor, _ := cursorResMap["next_cursor"].(string)
		cursor = nextCursor
	}

	if !fetchEventsParams.AllEvents {
		allRecords = filterRecordEvents(allRecords, fetchEventsParams.EventName)
	}

	toolResult = map[string]interface{}{
		"records": allRecords,
		"status":  resMap["status"],
	}

	return toolResult, payload, false, nil
}

func filterRecordEvents(records []interface{}, eventName string) []interface{} {
	if eventName == "" {
		return records
	}
	for _, r := range records {
		rec, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		prof, ok := rec["profile"].(map[string]interface{})
		if !ok {
			continue
		}
		events, ok := prof["events"].(map[string]interface{})
		if !ok {
			continue
		}
		filtered := map[string]interface{}{}
		if ev, exists := events[eventName]; exists {
			filtered[eventName] = ev
		}
		prof["events"] = filtered
	}
	return records
}

func (clevertapTool *ClevertapTool) GetBytes(ctx context.Context) ([]byte, error) {
	toolJson, err := json.Marshal(clevertapTool)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}

func (clevertapTool *ClevertapTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	newTool := &ClevertapTool{}
	err := json.Unmarshal(toolObjJson, newTool)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return newTool, nil
}

func init() {
	tools.RegisterTool("CLEVERTAP", func() tools.Tooling { return new(ClevertapTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       true,
		ToolType:     "CLEVERTAP",
		Category:     "Analytics",
		Description:  "CleverTap analytics integration for fetching events",
		Actions:      []tools.ActionInfo{{Name: FetchEvents}},
		OAuthEnabled: false,
		Icon:         "",
		IconType:     "svg",
	})
}
