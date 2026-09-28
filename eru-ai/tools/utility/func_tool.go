package utiltiy

import (
	"context"
	"encoding/json"

	tools "github.com/eru-os/eru/eru-ai/tools"
	eru_models "github.com/eru-os/eru/eru-models"
)

// FuncTool is a built-in tool whose behaviour is a Go function. It is for an
// agent's own reference lookups - things computed in-process from data the
// agent already holds - where writing a full tool type per lookup would be
// ceremony. It is never saved or configured, so it carries no JSON of its own.
type FuncTool struct {
	tools.Tool
	Fn func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) `json:"-"`
}

// NewFuncTool builds a FuncTool ready to hand to the tool loop.
func NewFuncTool(ctx context.Context, name string, description string, parameters eru_models.JSONSchema,
	fn func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error)) *FuncTool {
	tool := &FuncTool{Fn: fn}
	_ = tool.SetAttribute(ctx, "tool_name", name)
	_ = tool.SetAttribute(ctx, "tool_type", "Func")
	_ = tool.SetAttribute(ctx, "description", description)
	_ = tool.SetAttribute(ctx, "parameters", parameters)
	tool.SetToolAction(name)
	return tool
}

func (ft *FuncTool) GetSpec() tools.Tooling {
	return ft
}

func (ft *FuncTool) GetActionsList() []tools.ActionInfo {
	name, _ := ft.GetAttribute(context.Background(), "tool_name")
	description, _ := ft.GetAttribute(context.Background(), "description")
	n, _ := name.(string)
	d, _ := description.(string)
	return []tools.ActionInfo{{Name: n, Description: d}}
}

func (ft *FuncTool) BytesToTool(ctx context.Context, toolObjJson []byte) (tools.Tooling, error) {
	clone := &FuncTool{Fn: ft.Fn}
	if err := json.Unmarshal(toolObjJson, clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func (ft *FuncTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (map[string]interface{}, bool, error) {
	result, err := ft.Fn(ctx, params)
	return result, false, err
}

func IsEruqlTool(ctx context.Context, tool tools.Tooling) bool {
	if tool == nil {
		return false
	}
	for _, action := range tool.GetActionsList() {
		if action.Name == "execute_query" {
			return true
		}
	}
	return false
}
