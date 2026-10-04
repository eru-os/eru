package messengers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	models "github.com/eru-os/eru/eru-models"
	server "github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
	"google.golang.org/api/option"
)

const (
	SaveFirebaseToken = "save_firebase_token"
	SendNotification  = "send_notification"
)

const (
	UPSERT_FIREBASE_TOKEN  = "insert into eruai_firebase_tokens (project_id, tenant_id, user_id, device_id, firebase_token, created_at, updated_at) values ($1, $2, $3, $4, $5, now(), now()) on conflict (project_id, tenant_id, user_id, device_id) do update set firebase_token = excluded.firebase_token, updated_at = now()"
	SELECT_FIREBASE_TOKENS = "select device_id, firebase_token from eruai_firebase_tokens where project_id = $1 and tenant_id = $2 and user_id = $3"
	DELETE_FIREBASE_TOKEN  = "delete from eruai_firebase_tokens where project_id = $1 and tenant_id = $2 and user_id = $3 and device_id = $4"
)

type FirebaseAccount struct {
	ProjectId      string `json:"project_id" eru:"required" desc:"Firebase project id"`
	ServiceAccount string `json:"service_account" secret:"true" eru:"required" desc:"Firebase service account"`
}

type FirebaseTool struct {
	tools.Tool
	FirebaseAccount FirebaseAccount `json:"firebase_account"`
}

type FirebaseSaveTokenParams struct {
	UserId        string `json:"user_id" eru:"required" desc:"user id owning the device"`
	DeviceId      string `json:"device_id" eru:"required" desc:"device id"`
	FirebaseToken string `json:"firebase_token" eru:"required" desc:"FCM registration token for the device"`
}

type FirebaseSendNotificationParams struct {
	UserId string            `json:"user_id" eru:"required" desc:"target user id"`
	Title  string            `json:"title" eru:"required" desc:"notification title"`
	Body   string            `json:"body" eru:"required" desc:"notification body"`
	Data   map[string]string `json:"data" desc:"optional data payload"`
}

var firebaseToolActions = []tools.ToolAction{
	{
		ActionName:   SaveFirebaseToken,
		Description:  "Upsert a Firebase device token for a user",
		SystemPrompt: "Upsert a Firebase device token for a user",
		OutputSchema: eru_models.JSONSchema{},
		Parameters:   eru_models.JSONSchema{},
		GetParameters: func() eru_models.JSONSchema {
			return utils.StructToJSONSchema(reflect.TypeOf(FirebaseSaveTokenParams{}), []string{})
		},
	},
	{
		ActionName:   SendNotification,
		Description:  "Send a Firebase push notification to all devices of a user",
		SystemPrompt: "Send a Firebase push notification to all devices of a user",
		OutputSchema: eru_models.JSONSchema{},
		Parameters:   eru_models.JSONSchema{},
		GetParameters: func() eru_models.JSONSchema {
			return utils.StructToJSONSchema(reflect.TypeOf(FirebaseSendNotificationParams{}), []string{})
		},
	},
}

func (f *FirebaseTool) GetActionsList() []tools.ActionInfo {
	infos := make([]tools.ActionInfo, len(firebaseToolActions))
	for i, a := range firebaseToolActions {
		infos[i] = tools.ActionInfo{Name: a.ActionName, Description: a.Description}
	}
	return infos
}

func (f *FirebaseTool) GetActions() []tools.ToolAction {
	return firebaseToolActions
}

func (f *FirebaseTool) GetSpec() tools.Tooling {
	return f
}

func (f *FirebaseTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	if err := json.Unmarshal(*rj, &f); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (f *FirebaseTool) SetToolAction(actionName string) {
	for _, a := range firebaseToolActions {
		if a.ActionName == actionName {
			f.ToolAction = a
			return
		}
	}
	f.ToolAction = tools.ToolAction{}
}

func (f *FirebaseTool) GetBytes(ctx context.Context) ([]byte, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return nil, logs.Err(ctx, err, "")
	}
	return b, nil
}

func (f *FirebaseTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	if err := json.Unmarshal(toolObjJson, &f); err != nil {
		return nil, logs.Err(ctx, err, "")
	}
	return &FirebaseTool{
		Tool: f.Tool,
		FirebaseAccount: FirebaseAccount{
			ProjectId:      f.FirebaseAccount.ProjectId,
			ServiceAccount: f.FirebaseAccount.ServiceAccount,
		},
	}, nil
}

func (f *FirebaseTool) GetAttribute(ctx context.Context, attributeName string) (interface{}, error) {
	switch attributeName {
	case "tool_name":
		return f.ToolName, nil
	case "tool_type":
		return f.ToolType, nil
	case "system_prompt":
		return f.SystemPrompt, nil
	case "output_schema":
		return f.OutputSchema, nil
	case "parameters":
		return f.Parameters, nil
	case "description":
		return f.Description, nil
	case "project_id":
		return f.FirebaseAccount.ProjectId, nil
	default:
		err := errors.New("attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

func (f *FirebaseTool) SetAttribute(ctx context.Context, attributeName string, attributeValue interface{}) error {
	switch attributeName {
	case "tool_name":
		f.ToolName = attributeValue.(string)
	case "tool_type":
		f.ToolType = attributeValue.(string)
	case "system_prompt":
		f.SystemPrompt = attributeValue.(string)
	case "output_schema":
		f.OutputSchema = attributeValue.(eru_models.JSONSchema)
	case "parameters":
		f.Parameters = attributeValue.(eru_models.JSONSchema)
	case "description":
		f.Description = attributeValue.(string)
	case "project_id":
		f.FirebaseAccount.ProjectId = attributeValue.(string)
	case "service_account":
		f.FirebaseAccount.ServiceAccount = attributeValue.(string)
	default:
		err := errors.New("attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (f *FirebaseTool) unmarshalParams(ctx context.Context, params map[string]interface{}, target interface{}) error {
	b, err := json.Marshal(params)
	if err != nil {
		return logs.Err(ctx, err, "")
	}
	if err := json.Unmarshal(b, target); err != nil {
		return logs.Err(ctx, err, "")
	}
	return nil
}

func (f *FirebaseTool) getMessagingClient(ctx context.Context) (*messaging.Client, error) {
	if f.FirebaseAccount.ServiceAccount == "" {
		return nil, errors.New("service_account is not configured")
	}
	if f.FirebaseAccount.ProjectId == "" {
		return nil, errors.New("project_id is not configured")
	}
	sa, err := base64.StdEncoding.DecodeString(f.FirebaseAccount.ServiceAccount)
	if err != nil {
		return nil, logs.Err(ctx, err, "")
	}
	opt := option.WithCredentialsJSON(sa)
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: f.FirebaseAccount.ProjectId}, opt)
	if err != nil {
		return nil, logs.Err(ctx, err, "")
	}
	return app.Messaging(ctx)
}

func (f *FirebaseTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("FirebaseTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case SaveFirebaseToken:
		toolResult, toolRequest, persistStore, err = f.SaveFirebaseToken(ctx, projectId, tenantId, params)
	case SendNotification:
		toolResult, toolRequest, persistStore, err = f.SendNotification(ctx, projectId, tenantId, params)
	default:
		return nil, false, fmt.Errorf("action %s not found", actionName)
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
		hookResult, hookErr := f.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if hookErr != nil {
			logs.WithContext(bgCtx).Error(hookErr.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (f *FirebaseTool) SaveFirebaseToken(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (map[string]interface{}, interface{}, bool, error) {
	logs.WithContext(ctx).Debug("FirebaseTool SaveFirebaseToken - Start")
	p := FirebaseSaveTokenParams{}
	if err := f.unmarshalParams(ctx, params, &p); err != nil {
		return nil, nil, false, err
	}
	if err := utils.ValidateStruct(ctx, p, ""); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if f.ToolDb == nil {
		err := errors.New("tool db not configured")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	q := models.Queries{}
	q.Query = f.ToolDb.GetDbQuery(ctx, UPSERT_FIREBASE_TOKEN)
	q.Vals = append(q.Vals, projectId, tenantId, p.UserId, p.DeviceId, p.FirebaseToken)
	q.Rank = 1
	if _, err := utils.ExecuteDbSave(ctx, f.ToolDb.GetConn(), []*models.Queries{&q}); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	toolResult := map[string]interface{}{
		"save_status": "success",
		"user_id":     p.UserId,
		"device_id":   p.DeviceId,
	}
	return toolResult, map[string]interface{}{"body": p}, false, nil
}

func (f *FirebaseTool) SendNotification(ctx context.Context, projectId string, tenantId string, params map[string]interface{}) (map[string]interface{}, interface{}, bool, error) {
	logs.WithContext(ctx).Debug("FirebaseTool SendNotification - Start")
	p := FirebaseSendNotificationParams{}
	if err := f.unmarshalParams(ctx, params, &p); err != nil {
		return nil, nil, false, err
	}
	if err := utils.ValidateStruct(ctx, p, ""); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if f.ToolDb == nil {
		err := errors.New("tool db not configured")
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	selectQ := models.Queries{}
	selectQ.Query = f.ToolDb.GetDbQuery(ctx, SELECT_FIREBASE_TOKENS)
	selectQ.Vals = append(selectQ.Vals, projectId, tenantId, p.UserId)
	rows, err := utils.ExecuteDbFetch(ctx, f.ToolDb.GetConn(), selectQ)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}
	if len(rows) == 0 {
		return map[string]interface{}{
			"sent":    0,
			"failed":  0,
			"deleted": 0,
			"message": "no devices registered for user",
		}, map[string]interface{}{"body": p}, false, nil
	}

	client, err := f.getMessagingClient(ctx)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, nil, false, err
	}

	sent := 0
	failed := 0
	deleted := 0
	var failures []map[string]interface{}
	var deleteQueries []*models.Queries
	rank := 1
	for _, row := range rows {
		deviceId, _ := row["device_id"].(string)
		token, _ := row["firebase_token"].(string)
		if token == "" {
			continue
		}
		msg := &messaging.Message{
			Token: token,
			Notification: &messaging.Notification{
				Title: p.Title,
				Body:  p.Body,
			},
			Data: p.Data,
		}
		msgId, sendErr := client.Send(ctx, msg)
		if sendErr != nil {
			failed++
			failures = append(failures, map[string]interface{}{
				"device_id": deviceId,
				"error":     sendErr.Error(),
			})
			logs.WithContext(ctx).Error(fmt.Sprintf("fcm send failed for device %s: %s", deviceId, sendErr.Error()))
			if messaging.IsUnregistered(sendErr) || messaging.IsInvalidArgument(sendErr) || messaging.IsSenderIDMismatch(sendErr) {
				dq := models.Queries{}
				dq.Query = f.ToolDb.GetDbQuery(ctx, DELETE_FIREBASE_TOKEN)
				dq.Vals = append(dq.Vals, projectId, tenantId, p.UserId, deviceId)
				dq.Rank = rank
				rank++
				deleteQueries = append(deleteQueries, &dq)
				deleted++
			}
			continue
		}
		logs.WithContext(ctx).Info(fmt.Sprintf("fcm sent device=%s message_id=%s", deviceId, msgId))
		sent++
	}

	if len(deleteQueries) > 0 {
		if _, delErr := utils.ExecuteDbSave(ctx, f.ToolDb.GetConn(), deleteQueries); delErr != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("failed to delete invalid tokens: %s", delErr.Error()))
		}
	}

	toolResult := map[string]interface{}{
		"sent":     sent,
		"failed":   failed,
		"deleted":  deleted,
		"total":    len(rows),
		"failures": failures,
	}
	return toolResult, map[string]interface{}{"body": p}, false, nil
}

func init() {
	tools.RegisterTool("FIREBASE", func() tools.Tooling { return new(FirebaseTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:      true,
		ToolType:    "FIREBASE",
		Category:    "Communication",
		Description: "Firebase Cloud Messaging tool to register device tokens and send push notifications to a user's devices",
		Actions: func() []tools.ActionInfo {
			infos := make([]tools.ActionInfo, len(firebaseToolActions))
			for i, a := range firebaseToolActions {
				infos[i] = tools.ActionInfo{Name: a.ActionName, Description: a.Description}
			}
			return infos
		}(),
		OAuthEnabled: false,
		Icon:         "PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIzLjc3ZW0iIGhlaWdodD0iMWVtIiB2aWV3Qm94PSIwIDAgNTEyIDEzNiI+PHBhdGggZmlsbD0iIzVlNWU1ZSIgZD0iTTQ4Ny41MyAxMTAuNzMxcS03LjUwNSAwLTEzLjU3MS0zLjQ5NnEtNS45NjMtMy40OTYtOS4zNTYtOS42NjRxLTMuMzkzLTYuMjcyLTMuMzkyLTEzLjk4M3EwLTcuNDAxIDMuMTg3LTEzLjU3MXEzLjE4Ni02LjA3NSA4Ljc4Ni05LjY0MmwuMzY0LS4yMjhxNS44Ni0zLjcgMTMuMzY1LTMuN3E3LjcxMSAwIDEzLjM2NiAzLjM5MnE1Ljc1NyAzLjM5MyA4Ljc0IDkuNDU5cTIuOTggNS45NjIgMi45ODEgMTMuNzc3cTAgLjkyNS0uMTAzIDEuODVsLS4wOTMuODg3cS0uMDEuMTEtLjAxLjE0MUg0NzIuNTJxLjUxNCA2Ljk5MiA1LjAzOCAxMC43OTVxNC41MjQgMy44MDUgMTAuMjggMy44MDRxOC44NDMgMCAxMy41NzItOC4yMjVsOS42NjUgNC42MjdxLTMuMzkzIDYuMjcyLTkuNDYgMTAuMDc2cS01Ljk2MyAzLjctMTQuMDg1IDMuNzAxbTEyLjk1NS0zMy40MTRxLS4yMDYtMi40NjctMS42NDUtNS4wMzhxLTEuNDQtMi41Ny00LjUyNC00LjMxOHEtMi45ODItMS44NS03LjUwNS0xLjg1cS01LjAzOCAwLTguNzQgMy4wODRxLTMuNTk3IDMuMDg0LTQuOTM0IDguMTIyem0tNjQuMDk2IDMzLjQxNHEtOC43MzkgMC0xNC40OTYtNC4wMXEtNS42NTUtNC4xMTItOC4wMi0xMC4yODFsMTAuMDc2LTQuMzE4cTEuNzQ4IDQuMjE1IDUuMDM4IDYuMzc0cTMuMzkzIDIuMTYgNy40MDIgMi4xNnE0LjIxNSAwIDYuODg5LTEuNTQzcTIuNjczLTEuNjQ1IDIuNjczLTQuMjE1cTAtMi4zNjUtMS45ODMtMy44OGwtLjE3Ny0uMTNxLTIuMTU5LTEuNjQ0LTcuMDkzLTIuODc5bC02Ljk5Mi0xLjY0NHEtNS43NTctMS4zMzgtOS45NzItNC45MzVxLTQuMTEzLTMuNzAyLTQuMTEzLTkuNTYycTAtNi45NDMgNS42MTQtMTEuMDNsLjI0Ny0uMTc3cTUuODYtNC4yMTUgMTQuMzkzLTQuMjE1cTcuMDk0IDAgMTIuNTQ0IDIuOTgycTUuNTUxIDIuOTggOC4wMTggOC42MzZsLTkuOTcyIDQuMjE1cS0xLjQ0LTMuMTg2LTQuNDIxLTQuNzNxLTIuOTgyLTEuNTQtNi40NzctMS41NDFxLTMuMjI0IDAtNS43MDQgMS4zOTFsLS4yNi4xNXEtMi41NyAxLjQ0LTIuNTcgMy45MDhxMCAyLjE1OSAxLjc0OCAzLjQ5NnExLjc3IDEuMjc4IDUuNDIyIDIuMThsNy45NDQgMS45MzJxNy43MSAxLjk1MyAxMS41MTUgNS45NjNxMy44MDMgMy45MDggMy44MDQgOS40NTlxMCA0LjM2MS0yLjU4MiA3Ljk2bC0uMTk0LjI2NXEtMi42NzMgMy43LTcuNTA1IDUuODZxLTQuNzMgMi4xNi0xMC43OTYgMi4xNm0tNTQuMTA0IDBxLTUuNDUgMC05Ljg3LTIuMTU5cS00LjMxNy0yLjI2Mi02Ljg4OS02LjE2OXEtMi40NjYtNC4wMS0yLjQ2Ny05LjA0N3EwLTguMDIgNS45NjMtMTIuNjQ2cTYuMDY1LTQuNjI3IDE1LjMyLTQuNjI3cTguMTIgMCAxMy44OCAyLjc3NlY3Ni43cTAtNC40ODYtMy40ODItNy40MjVsLS4yMi0uMTgzcS0zLjU5OS0zLjA4NC04Ljc0LTMuMDg0cS03LjgxMyAwLTEyLjU0MiA2LjM3NGwtOC42MzYtNS45NjNxMy41OTgtNC44MzMgOC45NDQtNy40MDNxNS40NS0yLjU3IDEyLjIzNS0yLjU3cTEwLjk5IDAgMTcuMDM3IDUuNDRsLjIzNS4yMTVxNi4xNjggNS41NTEgNi4xNjkgMTUuNzN2MzEuMjU1aC0xMXYtNi4yNzFoLS42MThxLTIuNDY3IDMuNDk2LTYuMjcyIDUuNzU3cS0zLjgwMyAyLjE2LTkuMDQ3IDIuMTZtMS44NS05LjM1NnEzLjgwNSAwIDYuOTkyLTEuODVxMy4yOS0xLjk1NCA1LjE0LTUuMDM4cTEuOTU0LTMuMTg3IDEuOTU0LTYuNzg2cS01LjQ0OS0yLjk4Mi0xMS42MTgtMi45ODJxLTUuNjU1IDAtOC44NDEgMi40NjhxLTMuMTg3IDIuNDY4LTMuMTg3IDYuMjcxcTAgMy40OTYgMi43NzUgNS43NThxMi44OCAyLjE2IDYuNzg2IDIuMTU5bS00OS4zIDkuMzU2cS01LjU1MyAwLTkuOTczLTIuMzY1cS00LjMxOS0yLjM2NC02LjQ3Ny01Ljk2M2gtLjYxN3Y2LjY4M2gtMTAuNjkzVjM1LjQ3MmgxMS4zMXYyMi4xMDVsLS42MTcgNy4wOTRoLjYxN3EyLjE1OS0zLjQ5NiA2LjQ3Ny01Ljg2cTQuNDItMi4zNjUgOS45NzMtMi4zNjVxNi41OCAwIDEyLjEzMSAzLjQ5NnE1LjQ3NCAzLjM4MyA4LjczIDkuMzY1bC4yMTUuNDAycTMuMzkzIDYuMTY3IDMuMzkzIDEzLjg4cTAgNy40Ny0zLjE4NCAxMy40OTJsLS4yMDkuMzg3cS0zLjI5IDYuMTY4LTguOTQ1IDkuNzY3cS01LjU1MSAzLjQ5Ni0xMi4xMzEgMy40OTZtLTEuOTU0LTEwLjM4NHE0LjAxIDAgNy40MDItMi4wNTZxMy40OTUtMi4wNTcgNS41NTItNS44NnEyLjE2LTMuOTA3IDIuMTYtOC44NDNxMC00LjkzNS0yLjE2LTguNzM4cS0yLjA1NS0zLjkwOC01LjU1Mi01Ljk2NHEtMy4zOTItMi4wNTYtNy40MDItMi4wNTZ0LTcuNTA1IDIuMDU2cS0zLjM5MyAyLjA1Ni01LjU1MiA1Ljg2cS0yLjA1NSAzLjgwNS0yLjA1NiA4Ljg0MnEwIDUuMDM5IDIuMDU2IDguODQycTIuMTYgMy44MDQgNS41NTIgNS44NnEzLjQ5NSAyLjA1OCA3LjUwNSAyLjA1N20tNTcuNjI4LTQzLjlxNy43MSAwIDEzLjM2NSAzLjM5MnE1Ljc1OCAzLjM5MyA4Ljc0IDkuNDU5cTIuOTggNS45NjIgMi45ODEgMTMuNzc3cTAgLjkyNS0uMTAyIDEuODVsLS4wODIuNzU4YTQgNCAwIDAgMC0uMDIxLjI3SDI2MC44NnEuNTE0IDYuOTkyIDUuMDM4IDEwLjc5NXE0LjUyNCAzLjgwNSAxMC4yODEgMy44MDRxOC44NDMgMCAxMy41NzEtOC4yMjVsOS42NjUgNC42MjdxLTMuMzkzIDYuMjcyLTkuNDU5IDEwLjA3NnEtNS45NjIgMy43LTE0LjA4NSAzLjcwMXEtNy41MDUgMC0xMy41NzEtMy40OTZxLTUuOTY0LTMuNDk2LTkuMzU2LTkuNjY0cS0zLjM5My02LjI3Mi0zLjM5My0xMy45ODNxMC03LjQwMSAzLjE4Ny0xMy41NzFxMy4yOS02LjI3IDkuMTUtOS44N3E1Ljg2MS0zLjcgMTMuMzY2LTMuN20tMzAuOTc2LjEwMnEzLjcwMSAwIDYuMDY2IDEuMDI4djExLjgyNHEtMy40OTYtMS43NDgtNy44MTQtMS43NDhxLTUuNTUyIDAtOS4zNTYgNC4zMThxLTMuNyA0LjIxNS0zLjcwMSAxMC4zODR2MjYuNzNoLTExLjMxVjU4LjA5MmgxMC42OTNWNjUuN2guNjE3cTEuNTg3LTMuNzY3IDUuNjU4LTYuMjkybC4zMDUtLjE4NXE0LjMxOC0yLjY3MyA4Ljg0Mi0yLjY3M20zMC44NzMgOS41NjFxLTUuMDM4IDAtOC43MzkgMy4wODVxLTMuNTk4IDMuMDg0LTQuOTM1IDguMTIyaDI3LjM0OHEtLjIwNS0yLjQ2Ny0xLjY0NS01LjAzOHEtMS40NC0yLjU3LTQuNTIzLTQuMzE4cS0yLjk4Mi0xLjg1LTcuNTA2LTEuODVtLTczLjEyOC0xNi42NTVxLTMuMTg2IDAtNS40NDktMi4xNnEtMi4xNi0yLjI2MS0yLjE1OS01LjQ0OHEwLTMuMTg3IDIuMTU5LTUuMzQ3cTIuMjYyLTIuMjYgNS40NS0yLjI2MXEzLjE4NiAwIDUuMzQ2IDIuMjYxcTIuMjYyIDIuMTYgMi4yNjIgNS4zNDdxMCAzLjE4Ni0yLjI2MiA1LjQ0OXEtMi4xNiAyLjE2LTUuMzQ3IDIuMTU5bS01LjY1NCA4LjYzNmgxMS4zMDl2NTAuOTk1aC0xMS4zMXptLTUxLjUxNC0yMi42MTloNDQuNzIzdjExLjAwMWgtMzMuMjA5djIxLjE4aDI5LjkxOVY3OC41NWgtMjkuOTE5djMwLjUzNWgtMTEuNTE0eiIvPjxwYXRoIGZpbGw9IiNmZjkxMDAiIGQ9Ik0zMy43IDEzMS4yNTZhNTMuOCA1My44IDAgMCAwIDE4LjIzIDMuODVhNTMuNiA1My42IDAgMCAwIDI0LjQxOC00LjkyYTc2IDc2IDAgMCAxLTIzLjgwNC0xNC45NDdBNDAuNzYgNDAuNzYgMCAwIDEgMzMuNyAxMzEuMjU2Ii8+PHBhdGggZmlsbD0iI2ZmYzQwMCIgZD0iTTUyLjU0MiAxMTUuMjQyQzM2Ljg2MiAxMDAuNzQgMjcuMzUgNzkuNzUgMjguMTU0IDU2LjcyOWMuMDI2LS43NDguMDY2LTEuNDk1LjExMi0yLjI0MmE0MC42IDQwLjYgMCAwIDAtMjEuMjA2LjIyMkE1My41NSA1My41NSAwIDAgMCAuMDMzIDc5LjQ2NWMtLjgxMiAyMy4yNTYgMTMuMjYyIDQzLjU3NiAzMy42NjUgNTEuNzkzYTQwLjcgNDAuNyAwIDAgMCAxOC44NDQtMTYuMDE2Ii8+PHBhdGggZmlsbD0iI2ZmOTEwMCIgZD0iTTUyLjU0MyAxMTUuMjRhNDAuMzYgNDAuMzYgMCAwIDAgNi4xMTMtMjAuMDQyYy42NzctMTkuMzg0LTEyLjM1NC0zNi4wNTgtMzAuMzktNDAuNzExYTg0IDg0IDAgMCAwLS4xMTEgMi4yNDFjLS44MDQgMjMuMDIyIDguNzA4IDQ0LjAxMSAyNC4zODggNTguNTEzIi8+PHBhdGggZmlsbD0iI2RkMmMwMCIgZD0iTTU2LjY0IDBDNDYuMzY2IDguMjI5IDM4LjI1NSAxOS4wOCAzMy4zNDggMzEuNThhNzUuOSA3NS45IDAgMCAwLTUuMDkgMjIuOTExYzE4LjAzNSA0LjY1MyAzMS4wNjYgMjEuMzI4IDMwLjM4OSA0MC43MTJhNDAuNDUgNDAuNDUgMCAwIDEtNi4xMTMgMjAuMDQzYTc1LjkgNzUuOSAwIDAgMCAyMy44MDQgMTQuOTQ1YzE3LjgzOC04LjI0NSAzMC40OTMtMjUuOTg3IDMxLjIyNi00Ni45NzNjLjQ3NS0xMy41OTctNC43NS0yNS43MTUtMTIuMTMxLTM1Ljk0NEM4Ny42MzggMzYuNDU2IDU2LjYzOSAwIDU2LjYzOSAwIi8+PC9zdmc+",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(FirebaseTool{}), []string{}),
	})
}
