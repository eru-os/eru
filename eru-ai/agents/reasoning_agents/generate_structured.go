package reasoning_agents

import (
	"context"
	"fmt"

	agents "github.com/eru-os/eru/eru-ai/agents"
	models "github.com/eru-os/eru/eru-ai/models"
	tools "github.com/eru-os/eru/eru-ai/tools"
	utility "github.com/eru-os/eru/eru-ai/tools/utility"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
)

// GenerateStructured runs one structured generation and returns its payload.
//
// It is deliberately not Execute: no conversation is loaded or saved, no
// agent_started/finished pair is emitted, and nothing is added to the agent's
// history. That makes it the right primitive for a step INSIDE one answer - the
// page plan an agent draws before it writes a page, or one of several pages that
// answer together - where the user asked one question and should see one answer.
//
// Tool assembly is kept separate from Execute's on purpose: this runs on a path
// that did not exist before, and sharing the setup would put a new caller's
// mistakes into the path every existing answer takes.
func (ra *ReasoningAgent) GenerateStructured(
	ctx context.Context,
	systemPrompt string,
	content string,
	schema eru_models.JSONSchema,
	projectId string,
	tenantId string,
	offerTools bool,
) (map[string]interface{}, []models.StepTrace, error) {
	if schema.Type == "" {
		return nil, nil, fmt.Errorf("GenerateStructured needs an output schema")
	}
	if ra.Model == nil {
		return nil, nil, fmt.Errorf("agent %s has no model configured", ra.AgentName)
	}

	toolsMap := map[string]tools.Tooling{}
	if offerTools {
		for _, at := range ra.AgentTools {
			if at.Tool != nil {
				key := at.ToolKey
				if key == "" {
					key = at.ToolName
				}
				toolsMap[key] = at.Tool
			}
		}
		if provider, ok := ra.GetProvider().(agents.ExtraToolProvider); ok && provider != nil {
			for name, tool := range provider.ExtraTools(ctx) {
				if tool == nil {
					continue
				}
				if _, configured := toolsMap[name]; configured {
					continue
				}
				toolsMap[name] = tool
			}
		}
	}

	outputTool := &utility.StructuredOutputTool{}
	_ = outputTool.SetAttribute(ctx, "output_schema", schema)
	_ = outputTool.SetAttribute(ctx, "parameters", schema)
	_ = outputTool.SetAttribute(ctx, "description", "Output the final result as structured JSON. Call this tool when you have your final answer ready.")
	_ = outputTool.SetAttribute(ctx, "tool_name", "structured_output")
	_ = outputTool.SetAttribute(ctx, "tool_type", "STRUCTURED_OUTPUT")
	outputTool.SetToolAction("structured_output")
	toolsMap["structured_output"] = outputTool

	toolExecutor := func(ctx context.Context, toolName string, input map[string]interface{}) (map[string]interface{}, error) {
		for _, at := range ra.AgentTools {
			if at.Tool == nil {
				continue
			}
			tnI, _ := at.Tool.GetAttribute(ctx, "tool_name")
			if tn, ok := tnI.(string); ok && tn == toolName {
				result, _, execErr := at.Tool.Execute(ctx, projectId, tenantId, at.ActionName, input)
				return result, execErr
			}
		}
		if tool, ok := toolsMap[toolName]; ok && tool != nil {
			result, _, execErr := tool.Execute(ctx, projectId, tenantId, toolName, input)
			return result, execErr
		}
		return nil, fmt.Errorf("tool %s not found", toolName)
	}

	chatRequest := models.ChatRequest{
		Messages: []models.Message{{Role: "user", Content: content, Name: ra.AgentName}},
	}

	response, traces, err := ra.Model.RunToolLoop(ctx, chatRequest, toolsMap, models.StaticAgentPrompt(systemPrompt), ra.MaxIterations, ra.ThinkingBudget, toolExecutor)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, traces, err
	}

	payload := parseAgentResponse(response.Content)
	if normalized, ok := deepUnstringifyJSON(payload, "").(map[string]interface{}); ok {
		payload = normalized
	}
	payload = unwrapSingleEnvelope(payload, schema)
	if err := validateRootKeys(payload, schema); err != nil {
		return payload, traces, err
	}
	if err := validateAgainstSchema(payload, schema, ""); err != nil {
		return payload, traces, err
	}
	return payload, traces, nil
}

// unwrapSingleEnvelope undoes a model that wrapped its answer in one key.
//
// "output", "result", "data" - a model that has been handed a schema sometimes
// returns the object one level down, and rejecting that costs a retry over
// something we can simply read. Only a single-key object is unwrapped, and only
// when the thing inside looks like the schema asked for, so a real answer that
// happens to have one field is never mistaken for a wrapper.
func unwrapSingleEnvelope(payload map[string]interface{}, schema eru_models.JSONSchema) map[string]interface{} {
	if len(payload) != 1 || len(schema.Required) == 0 {
		return payload
	}
	for _, required := range schema.Required {
		if _, present := payload[required]; present {
			return payload
		}
	}
	for _, value := range payload {
		inner, ok := value.(map[string]interface{})
		if !ok {
			return payload
		}
		for _, required := range schema.Required {
			if _, present := inner[required]; !present {
				return payload
			}
		}
		return inner
	}
	return payload
}
