package module_server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eru-os/eru/eru-ai/agents"
	"github.com/eru-os/eru/eru-ai/models"
	"github.com/eru-os/eru/eru-ai/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	"github.com/eru-os/eru/eru-server/server"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/google/uuid"
)

const (
	MCPProtocolVersion = "2025-03-26"
	ServerName         = "eru-ai-mcp-server"
	ServerVersion      = "1.0.1"
	mcpNameSep         = "__"
	mcpActionSep       = "___"
)

var supportedMCPVersions = []string{"2025-06-18", "2025-03-26"}

func negotiateMCPVersion(clientVersion string) string {
	for _, v := range supportedMCPVersions {
		if v == clientVersion {
			return v
		}
	}
	return supportedMCPVersions[0]
}

type EruAIMCPServer struct {
	store        *module_store.StoreHolder
	capabilities server.MCPCapabilities
}

func NewEruAIMCPServer(store *module_store.StoreHolder) *EruAIMCPServer {
	return &EruAIMCPServer{
		store: store,
		capabilities: server.MCPCapabilities{
			Tools: &server.MCPToolsCapability{
				ListChanged: true,
			},
		},
	}
}

func (s *EruAIMCPServer) Initialize(ctx context.Context, params server.MCPInitializeParams) (server.MCPInitializeResult, error) {
	logs.WithContext(ctx).Info(fmt.Sprintf("MCP client initializing: %s v%s", params.ClientInfo.Name, params.ClientInfo.Version))

	return server.MCPInitializeResult{
		ProtocolVersion: negotiateMCPVersion(params.ProtocolVersion),
		Capabilities:    s.capabilities,
		ServerInfo: server.MCPServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
	}, nil
}

func (s *EruAIMCPServer) ListTools(ctx context.Context, projectId string, tenantId string) (server.MCPListToolsResult, error) {
	var mcpTools []server.MCPTool

	for _, projectInfo := range s.store.Store.GetProjectList(ctx) {
		projectName, ok := projectInfo["project_name"].(string)
		if !ok {
			continue
		}
		if projectId != "" && projectName != projectId {
			continue
		}
		project, err := s.store.Store.GetProjectConfig(ctx, projectName)
		if err != nil {
			continue
		}
		defaultTenantId := eru_utils.DefaultTenant(ctx)
		for tenantKey := range project.Tenants {
			if tenantId != "" && tenantKey != tenantId && tenantKey != projectId && tenantKey != defaultTenantId {
				continue
			}
			prefixed := projectId == tenantId && projectId != ""
			for _, dt := range s.store.Store.DiscoverTools(ctx, projectName, tenantKey, nil, s.store.Store) {
				mcpTools = append(mcpTools, s.toolToMCPTool(dt, prefixed))
			}
			for _, da := range s.store.Store.DiscoverAgents(ctx, projectName, tenantKey, "", nil, s.store.Store) {
				mcpTools = append(mcpTools, s.agentToMCPTool(da, prefixed))
			}
		}
	}

	if mcpTools == nil {
		mcpTools = []server.MCPTool{}
	}

	return server.MCPListToolsResult{
		Tools: mcpTools,
	}, nil
}

// toolToMCPTool advertises a tool action the way executeToolAction actually reads
// it: the action's own fields always sit inside a root "params" object.
func (s *EruAIMCPServer) toolToMCPTool(dt agents.DiscoveredTool, prefixed bool) server.MCPTool {
	name := dt.ToolName
	if prefixed {
		name = strings.Join([]string{dt.TenantId, dt.ToolName}, mcpNameSep)
	}
	if dt.ActionName != "" {
		name = name + mcpActionSep + dt.ActionName
	}
	mcpTool := server.MCPTool{
		Name:        name,
		Description: dt.Description,
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"params": s.jsonSchemaToMap(dt.InputSchema),
			},
			"required": []string{"params"},
		},
	}
	if dt.OutputSchema.Type != "" {
		mcpTool.OutputSchema = s.jsonSchemaToMap(dt.OutputSchema)
	}
	return mcpTool
}

// agentToMCPTool advertises an agent using the same input contract the
// orchestrator plans against, plus the transport-only conversation_id and code
// arguments that only the MCP entry point accepts.
func (s *EruAIMCPServer) agentToMCPTool(da agents.DiscoveredAgent, prefixed bool) server.MCPTool {
	agentPrefix := "agent"
	if prefixed {
		agentPrefix = strings.Join([]string{"agent", da.TenantId}, mcpNameSep)
	}

	inputSchema := s.jsonSchemaToMap(da.InputSchema)
	properties, _ := inputSchema["properties"].(map[string]interface{})
	if properties == nil {
		properties = map[string]interface{}{}
		inputSchema["properties"] = properties
	}
	properties["conversation_id"] = map[string]interface{}{
		"type":        "string",
		"description": "Optional conversation id to continue an existing conversation. When provided, the agent loads the prior conversation history for this id and appends the new message to it. Omit (or leave empty) to start a fresh conversation — the agent will generate a new id and return it in the response.",
	}
	if da.HasStructuredOutput() {
		properties["code"] = map[string]interface{}{
			"type":        "string",
			"description": "Existing structured output (as a JSON string) that the agent should build on top of. Provide this when modifying or extending an output produced earlier — especially when the prior conversation history is not available to the caller. The agent will treat this as the baseline and apply the new instruction on top of it.",
		}
	}

	mcpTool := server.MCPTool{
		Name:        strings.Join([]string{agentPrefix, da.AgentName}, mcpNameSep),
		Description: agentMCPDescription(da),
		InputSchema: inputSchema,
	}
	if da.HasStructuredOutput() {
		mcpTool.OutputSchema = s.jsonSchemaToMap(da.OutputSchema)
	}
	return mcpTool
}

// agentMCPDescription tells the calling client what the agent can do beyond its
// configured one-liner, so it can choose between agents without calling them.
func agentMCPDescription(da agents.DiscoveredAgent) string {
	var sb strings.Builder
	sb.WriteString(da.Description)
	if len(da.Tools) > 0 {
		sb.WriteString(fmt.Sprint("\n\nCan call these tools itself: ", strings.Join(da.Tools, ", "), "."))
	}
	if keys := da.ParamKeys(); len(keys) > 0 {
		sb.WriteString(fmt.Sprint("\nReads these params keys: ", strings.Join(keys, ", "), ". Any other params key is ignored."))
	}
	if da.SupportsClarification {
		sb.WriteString("\nMay reply with a clarifying question instead of an answer.")
	}
	if da.IsOrchestrator {
		sb.WriteString("\nOrchestrates other agents and tools to complete the task.")
	}
	switch {
	case da.HasStructuredOutput():
		sb.WriteString("\nResponds with structured output described by outputSchema, wrapped as {\"actions\":[{\"action\":{...}}]}.")
	case da.IsOrchestrator:
		sb.WriteString("\nResponds with one action per sub-step it ran; the field names depend on the plan it builds.")
	default:
		sb.WriteString("\nResponds with free text, wrapped as {\"actions\":[{\"action\":{\"output\":\"...\"}}]}.")
	}
	return sb.String()
}

func (s *EruAIMCPServer) CallTool(ctx context.Context, conversationId string, params server.MCPCallToolParams, projectId string, tenantId string) (server.MCPCallToolResult, error) {
	mcpName := params.Name
	actionName := ""
	if idx := strings.Index(mcpName, mcpActionSep); idx != -1 {
		actionName = mcpName[idx+len(mcpActionSep):]
		mcpName = mcpName[:idx]
	}

	parts := s.parseToolName(mcpName)
	isAgent := false
	toolAgentName := ""
	switch {
	case len(parts) == 1:
		toolAgentName = parts[0]
	case len(parts) == 2 && parts[0] == "agent":
		isAgent = true
		toolAgentName = parts[1]
	case len(parts) == 2:
		toolAgentName = parts[1]
	case len(parts) == 3 && parts[0] == "agent":
		isAgent = true
		toolAgentName = parts[2]
	default:
		return server.MCPCallToolResult{}, fmt.Errorf("invalid tool name format: %s", params.Name)
	}
	if isAgent {
		return s.executeAgent(ctx, conversationId, projectId, tenantId, toolAgentName, params.Arguments)
	}
	return s.executeToolAction(ctx, conversationId, projectId, tenantId, toolAgentName, actionName, params.Arguments)
}

func (s *EruAIMCPServer) executeAgent(ctx context.Context, conversationId, project, tenant, agentName string, arguments map[string]interface{}) (server.MCPCallToolResult, error) {
	agent, err := s.store.Store.GetAgent(ctx, project, tenant, "", agentName, s.store.Store)
	if err != nil {
		return server.MCPCallToolResult{}, err
	}

	ctx = module_store.WithProjectContext(ctx, project, s.store.Store)

	content, ok := arguments["content"].(string)
	if !ok {
		return server.MCPCallToolResult{}, fmt.Errorf("content parameter required")
	}

	params, _ := arguments["params"].(map[string]interface{})
	if params == nil {
		params = make(map[string]interface{})
	}

	var files []models.FileMessage
	if filesArg, ok := arguments["files"].([]interface{}); ok {
		for _, fileData := range filesArg {
			if fileMap, ok := fileData.(map[string]interface{}); ok {
				file := models.FileMessage{}

				if name, ok := fileMap["name"].(string); ok {
					file.FileName = name
				}

				if content, ok := fileMap["content"].(string); ok {
					file.FileData = content
				}

				if mimeType, ok := fileMap["mime_type"].(string); ok {
					file.FileType = mimeType
				}

				files = append(files, file)
			}
		}
	}

	code, _ := arguments["code"].(string)

	if argConvId, ok := arguments["conversation_id"].(string); ok && argConvId != "" {
		conversationId = argConvId
	}
	if conversationId == "" {
		conversationId = uuid.New().String()
	}

	agentMessage := agents.AgentMessage{
		Content: content,
		Code:    code,
		Params:  params,
		Files:   files,
	}

	streamCb := agents.StreamCallback(func(event agents.StreamEvent) {
		eventBytes, mErr := json.Marshal(event)
		if mErr != nil {
			logs.WithContext(ctx).Debug(fmt.Sprintf("mcp agent stream event: %s (iter %d)", event.Event, event.Iteration))
			return
		}
		logs.WithContext(ctx).Debug(fmt.Sprintf("mcp agent stream event: %s", string(eventBytes)))
	})
	ctx = agents.WithStreamCallback(ctx, streamCb)

	result, err := agent.Execute(ctx, agentMessage, conversationId, project, tenant)
	// The framework's own check that the answer handed over is the answer the
	// loop judged. It lives here, and at the other invocation site, precisely
	// because an agent type cannot remove it.
	if err == nil {
		_ = agents.VerifyDelivery(ctx, agent.GetProvider(), result)
	}
	result.ConversationId = conversationId
	if err != nil {
		return server.MCPCallToolResult{
			Content: []server.MCPContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Agent execution failed: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	resultText := s.formatResult(result)
	return server.MCPCallToolResult{
		Content: []server.MCPContent{
			{
				Type: "text",
				Text: resultText,
			},
		},
	}, nil
}

func (s *EruAIMCPServer) executeToolAction(ctx context.Context, conversationId, project, tenant, toolName, actionName string, arguments map[string]interface{}) (server.MCPCallToolResult, error) {
	tool, err := s.store.Store.GetTool(ctx, project, tenant, toolName, "", s.store.Store)
	if err != nil {
		return server.MCPCallToolResult{}, err
	}

	ctx = module_store.WithProjectContext(ctx, project, s.store.Store)

	toolParams := map[string]interface{}{}
	if wrapped, ok := arguments["params"].(map[string]interface{}); ok {
		toolParams = wrapped
	}

	result, _, err := tool.Execute(ctx, project, tenant, actionName, toolParams)
	if err != nil {
		return server.MCPCallToolResult{
			Content: []server.MCPContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Tool execution failed: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	resultText := s.formatResult(result)
	return server.MCPCallToolResult{
		Content: []server.MCPContent{
			{
				Type: "text",
				Text: resultText,
			},
		},
	}, nil
}

func (s *EruAIMCPServer) GetCapabilities() server.MCPCapabilities {
	return s.capabilities
}

func (s *EruAIMCPServer) GetServerInfo() server.MCPServerInfo {
	return server.MCPServerInfo{
		Name:    ServerName,
		Version: ServerVersion,
	}
}

// parseToolName splits an MCP tool name on the __ separator.
// Tool names use the format: project__tenant__toolname
// Agent names use the format: project__tenant__agent__agentname
func (s *EruAIMCPServer) parseToolName(toolName string) []string {
	return strings.Split(toolName, mcpNameSep)
}

func (s *EruAIMCPServer) jsonSchemaToMap(jsonSchema eru_models.JSONSchema) map[string]interface{} {
	out := map[string]interface{}{}

	if jsonSchema.Type != "" {
		out["type"] = jsonSchema.Type
	} else {
		out["type"] = "object"
	}

	if jsonSchema.Description != "" {
		out["description"] = jsonSchema.Description
	}
	if jsonSchema.Format != "" {
		out["format"] = jsonSchema.Format
	}
	if len(jsonSchema.Enum) > 0 {
		out["enum"] = jsonSchema.Enum
	}

	if len(jsonSchema.Properties) > 0 {
		properties := map[string]interface{}{}
		for name, prop := range jsonSchema.Properties {
			properties[name] = s.jsonSchemaToMap(prop)
		}
		out["properties"] = properties
	} else if jsonSchema.Type == "object" || jsonSchema.Type == "" {
		out["properties"] = map[string]interface{}{}
	}

	if len(jsonSchema.Required) > 0 {
		out["required"] = jsonSchema.Required
	}

	if jsonSchema.Items != nil {
		out["items"] = s.jsonSchemaToMap(*jsonSchema.Items)
	}

	if jsonSchema.AdditionalProperties != nil {
		if apSchema, ok := jsonSchema.AdditionalProperties.(eru_models.JSONSchema); ok {
			out["additionalProperties"] = s.jsonSchemaToMap(apSchema)
		} else {
			out["additionalProperties"] = jsonSchema.AdditionalProperties
		}
	}

	return out
}

func (s *EruAIMCPServer) formatResult(result interface{}) string {
	if result == nil {
		return "Operation completed successfully"
	}

	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("Result: %v", result)
	}

	return string(resultBytes)
}
