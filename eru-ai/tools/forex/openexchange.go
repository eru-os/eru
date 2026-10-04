package forex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server "github.com/eru-os/eru/eru-server/server"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	GetExchangeRate = "get_exchange_rate"
)

const (
	OpenExchangeBaseUrl  = "https://openexchangerates.org/api"
	OpenExchangePlanFree = "free"
	OpenExchangePlanPaid = "paid"
	OpenExchangeFreeBase = "USD"
	rateDateLayout       = "2006-01-02"
)

type OpenExchangeAccount struct {
	AppId    string `json:"app_id" secret:"true" eru:"required"`
	PlanType string `json:"plan_type" eru:"required"`
}

type OpenExchangeTool struct {
	tools.Tool
	OpenExchangeAccount OpenExchangeAccount `json:"openexchange_account"`
}

type GetExchangeRateParams struct {
	BaseCcy  string `json:"base_ccy"`
	RateDate string `json:"rate_date"`
}

func (oxTool *OpenExchangeTool) GetActionsList() []tools.ActionInfo {
	return []tools.ActionInfo{
		{Name: GetExchangeRate, Description: "Get exchange rates of all currencies against base_ccy (default USD), for rate_date (yyyy-mm-dd) or latest if rate_date is not given"},
	}
}

func (oxTool *OpenExchangeTool) GetSpec() tools.Tooling {
	return oxTool
}

func (oxTool *OpenExchangeTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &oxTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (oxTool *OpenExchangeTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("OpenExchangeTool Execute - Start")
	var toolRequest interface{}
	switch actionName {
	case GetExchangeRate:
		toolResult, toolRequest, persistStore, err = oxTool.GetExchangeRate(ctx, params)
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

		hookResult, err := oxTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, persistStore, err
}

func (oxTool *OpenExchangeTool) GetExchangeRate(ctx context.Context, params map[string]interface{}) (toolResult map[string]interface{}, toolRequest interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GetExchangeRate Execute - Start")

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to marshal params: %s", err.Error()), "failed to marshal params")
		return nil, nil, false, err
	}
	rateParams := GetExchangeRateParams{}
	err = json.Unmarshal(paramsBytes, &rateParams)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to unmarshal get exchange rate params: %s", err.Error()), "failed to unmarshal get exchange rate params")
		return nil, nil, false, err
	}

	baseCcy := strings.ToUpper(strings.TrimSpace(rateParams.BaseCcy))
	if baseCcy == "" {
		baseCcy = OpenExchangeFreeBase
	}
	rateDate := strings.TrimSpace(rateParams.RateDate)

	url := fmt.Sprint(OpenExchangeBaseUrl, "/latest.json")
	if rateDate != "" {
		if _, pErr := time.Parse(rateDateLayout, rateDate); pErr != nil {
			err = logs.Err(ctx, fmt.Errorf("invalid rate_date %s, expected yyyy-mm-dd", rateDate), "")
			return nil, nil, false, err
		}
		url = fmt.Sprint(OpenExchangeBaseUrl, "/historical/", rateDate, ".json")
	}

	planType := strings.ToLower(strings.TrimSpace(oxTool.OpenExchangeAccount.PlanType))
	callBase := baseCcy
	switch planType {
	case OpenExchangePlanFree:
		callBase = OpenExchangeFreeBase
	case OpenExchangePlanPaid:
	default:
		err = logs.Err(ctx, fmt.Errorf("invalid plan_type %s, expected %s or %s", oxTool.OpenExchangeAccount.PlanType, OpenExchangePlanFree, OpenExchangePlanPaid), "")
		return nil, nil, false, err
	}

	queryParams := map[string]string{
		"base":   callBase,
		"app_id": oxTool.OpenExchangeAccount.AppId,
	}
	toolRequest = map[string]interface{}{"base_ccy": baseCcy, "rate_date": rateDate}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	res, _, _, _, err := utils.CallHttp(ctx, http.MethodGet, url, headers, map[string]string{}, []*http.Cookie{}, queryParams, nil)
	if err != nil {
		err = logs.Err(ctx, fmt.Errorf("failed to fetch exchange rates: %s", err.Error()), "failed to fetch exchange rates")
		return nil, toolRequest, false, err
	}

	resMap, ok := res.(map[string]interface{})
	if !ok {
		err = logs.Err(ctx, errors.New("invalid response format from openexchangerates"), "")
		return nil, toolRequest, false, err
	}
	if isErr, _ := resMap["error"].(bool); isErr {
		err = logs.Err(ctx, fmt.Errorf("openexchangerates error : %v - %v", resMap["message"], resMap["description"]), "")
		return nil, toolRequest, false, err
	}

	if callBase != baseCcy {
		rates, rOk := resMap["rates"].(map[string]interface{})
		if !rOk {
			err = logs.Err(ctx, errors.New("rates not found in openexchangerates response"), "")
			return nil, toolRequest, false, err
		}
		resMap["rates"], err = deriveRates(rates, baseCcy)
		if err != nil {
			err = logs.Err(ctx, err, "")
			return nil, toolRequest, false, err
		}
		resMap["base"] = baseCcy
	}

	if ts, tsOk := resMap["timestamp"].(float64); tsOk {
		resMap["rate_date"] = time.Unix(int64(ts), 0).UTC().Format(rateDateLayout)
	}

	return resMap, toolRequest, false, nil
}

func deriveRates(rates map[string]interface{}, baseCcy string) (derived map[string]interface{}, err error) {
	baseRate, ok := rates[baseCcy].(float64)
	if !ok || baseRate == 0 {
		return nil, fmt.Errorf("rate for base currency %s not found", baseCcy)
	}
	derived = make(map[string]interface{}, len(rates))
	for ccy, v := range rates {
		rate, rOk := v.(float64)
		if !rOk {
			continue
		}
		derived[ccy] = rate / baseRate
	}
	derived[baseCcy] = float64(1)
	return derived, nil
}

func (oxTool *OpenExchangeTool) GetBytes(ctx context.Context) ([]byte, error) {
	toolJson, err := json.Marshal(oxTool)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return toolJson, nil
}

func (oxTool *OpenExchangeTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	newTool := &OpenExchangeTool{}
	err := json.Unmarshal(toolObjJson, newTool)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return nil, err
	}
	return newTool, nil
}

func init() {
	tools.RegisterTool("OPENEXCHANGE", func() tools.Tooling { return new(OpenExchangeTool) })
	tools.RegisterToolCatalog(tools.ToolCatalogEntry{
		Public:       true,
		ToolType:     "OPENEXCHANGE",
		Category:     "Finance",
		Description:  "Open Exchange Rates integration for latest and historical currency exchange rates",
		Actions:      []tools.ActionInfo{{Name: GetExchangeRate}},
		OAuthEnabled: false,
		Icon:         "",
		IconType:     "svg",
		ToolSchema:   utils.StructToJSONSchema(reflect.TypeOf(OpenExchangeTool{}), []string{}),
	})
}
