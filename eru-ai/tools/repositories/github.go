package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	repos "github.com/eru-os/eru/eru-repos/repos"
	server "github.com/eru-os/eru/eru-server/server"
)

type GithubTool struct {
	tools.Tool
	Repo repos.GithubRepo `json:"repo"`
}

func (ghTool *GithubTool) GetSpec() tools.Tooling {
	return ghTool
}

func (ghTool *GithubTool) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &ghTool)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (ghTool *GithubTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (toolResult map[string]interface{}, persistStore bool, err error) {
	logs.WithContext(ctx).Debug("GithubTool Execute - Start")
	logs.WithContext(ctx).Info("ghTool.Executed")
	var toolRequest interface{}
	toolRequest = params
	contents := ""
	fileName := ""
	if contentI, contentIOk := params["contents"]; contentIOk {
		contents = contentI.(string)
	} else {
		err = errors.New("contents attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, false, err
	}
	if fileNameI, fileNameIOk := params["file_name"]; fileNameIOk {
		fileName = fileNameI.(string)
	} else {
		err = errors.New("file_name attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, false, err
	}

	err = ghTool.Repo.Commit(ctx, []byte(contents), fileName)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, false, err
	}
	toolResult = map[string]interface{}{"content": "Commit successful"}

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

		hookResult, err := ghTool.ExecuteHook(bgCtx, "poex", actionName, projectId, tenantId, body, nil)
		if err != nil {
			logs.WithContext(bgCtx).Error(err.Error())
			return
		}
		logs.WithContext(bgCtx).Info(fmt.Sprint(hookResult))
	}, server.ContinueOnMaxRetries)

	return toolResult, false, nil
}

func init() {
	tools.RegisterTool("GITHUB", func() tools.Tooling { return new(GithubTool) })
}
