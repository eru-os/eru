package reflex_agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agents "github.com/eru-os/eru/eru-ai/agents"
	models "github.com/eru-os/eru/eru-ai/models"
	tools "github.com/eru-os/eru/eru-ai/tools"
	utility "github.com/eru-os/eru/eru-ai/tools/utility"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
)

type ReflexAgent struct {
	agents.Agent
}

func (reflex_agent *ReflexAgent) GetSpec() agents.AgentI {
	return reflex_agent
}

func (reflex_agent *ReflexAgent) GetSystemPrompt() string {
	const systemPrompt = `

	`
	return systemPrompt
}

func (reflex_agent *ReflexAgent) GetOutputSchema(ctx context.Context) eru_models.JSONSchema {
	return eru_models.JSONSchema{}
}

func (reflex_agent *ReflexAgent) Execute(ctx context.Context, agentMessage agents.AgentMessage, conversationId string, projectId string, tenantId string) (agents.AgentMessage, error) {
	logs.WithContext(ctx).Debug("Agent Execute - Start")
	chatRequest, conversation, err := reflex_agent.LoadConversations(ctx, conversationId, agentMessage, projectId, tenantId)
	if err != nil {
		return agents.AgentMessage{}, err
	}
	if codeMsg := reflex_agent.buildCodeMessage(ctx, agentMessage.Params); codeMsg != nil {
		chatRequest.Messages = append(chatRequest.Messages, *codeMsg)
	}

	response, err := reflex_agent.execute(ctx, chatRequest, reflex_agent.AgentTools, 1, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to execute agent: %v", err))
		return agents.AgentMessage{}, err
	}

	response.MessageId = agentMessage.MessageId
	conversation.Messages = append(conversation.Messages, response)
	conversation.NewMessages = append(conversation.NewMessages, response)
	err = reflex_agent.SaveConversation(ctx, conversation, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to save conversation: %v", err))
		return agents.AgentMessage{}, err
	}
	return response, nil
}

func (reflex_agent *ReflexAgent) execute(ctx context.Context, chatRequest models.ChatRequest, agentTools []agents.AgentTools, currentTry int, projectId string, tenantId string) (agentOutput agents.AgentMessage, err error) {
	logs.WithContext(ctx).Debug("execute - Start")

	toolResults, err := reflex_agent.ExecuteTools(ctx, chatRequest, agentTools, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return agents.AgentMessage{}, err
	}

	sp := ""
	if reflex_agent.GetProvider() != nil {
		sp = reflex_agent.GetProvider().GetSystemPrompt()
	}
	sp += "\n" + reflex_agent.SystemPrompt
	sp += reflex_agent.ExecutionContextSection(projectId, tenantId)
	sp += reflex_agent.GuardrailSection()

	chatRequest.Messages = append([]models.Message{
		{Role: "assistant", Content: sp, Name: reflex_agent.AgentName},
	}, chatRequest.Messages...)

	if len(toolResults) > 0 {
		toolResultsBytes, err := json.Marshal(toolResults)
		if err != nil {
			chatRequest.Messages = append([]models.Message{
				{Role: "assistant",
					Content: fmt.Sprintf("Tool results: %+v", toolResults),
					Name:    reflex_agent.AgentName,
				}}, chatRequest.Messages...)
		} else {
			contentStr := `Tool results is as given below
		
		`
			chatRequest.Messages = append([]models.Message{{
				Role: "assistant",
				Content: fmt.Sprint(contentStr, string(toolResultsBytes), `
			`),
				Name: reflex_agent.AgentName,
			}}, chatRequest.Messages...)
		}
	}
	agentResponse := make(map[string]interface{})
	response := models.Message{}
	outputSchema := reflex_agent.OutputSchema
	if reflex_agent.GetProvider() != nil {
		providerSchema := reflex_agent.GetProvider().GetOutputSchema(ctx)
		logs.WithContext(ctx).Info(fmt.Sprintf("Provider schema: %+v", providerSchema))
		if providerSchema.Type != "" {
			outputSchema = providerSchema
		}
	}
	if outputSchema.Type != "" {
		outputTool := utility.StructuredOutputTool{}
		outputTool.SetAttribute(ctx, "output_schema", outputSchema)
		outputTool.SetAttribute(ctx, "parameters", outputSchema)
		outputTool.SetAttribute(ctx, "description", "Output the result")
		outputTool.SetAttribute(ctx, "tool_name", "structured_output")
		outputTool.SetAttribute(ctx, "tool_type", "STRUCTURED_OUTPUT")
		outputTool.SetToolAction("structured_output")
		agentResponse, err = reflex_agent.ExecuteTools(ctx, chatRequest, []agents.AgentTools{{Tool: &outputTool, ToolOutputType: "json"}}, projectId, tenantId)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return agents.AgentMessage{}, err
		}
	} else {
		response, err = reflex_agent.Model.QueryModel(ctx, chatRequest)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return agents.AgentMessage{}, err
		}
		responseMap := map[string]interface{}{}
		err = json.Unmarshal([]byte(response.Content), &responseMap)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			agentResponse["output"] = response.Content
		} else {
			agentResponse = responseMap
		}
	}

	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		logs.WithContext(ctx).Info(fmt.Sprintf("%+v", response.Content))
		if currentTry < reflex_agent.RetryCount {
			errMsgString := fmt.Sprintf("Error in the json string. Please try again. \n Error: %s \n Erroneous JSON Code generated in previous try: %s", err.Error(), response.Content)
			msg := models.Message{
				Role:    "user",
				Content: errMsgString,
				Name:    reflex_agent.AgentName,
				Files:   []models.FileMessage{},
			}
			chatRequest.Messages = append(chatRequest.Messages, msg)
			return reflex_agent.execute(ctx, chatRequest, reflex_agent.AgentTools, currentTry+1, projectId, tenantId)
		}
		return agents.AgentMessage{}, err
	}
	agentOutputAction := agents.AgentOutputAction{
		ActionName: reflex_agent.AgentName,
		Action:     agentResponse,
	}
	agentOutputActions := []agents.AgentOutputAction{agentOutputAction}

	agentOutput = agents.AgentMessage{
		Role:             "assistant",
		Actions:          agentOutputActions,
		MessageTimestamp: time.Now(),
	}
	agentOutput.RetryCount = currentTry
	return agentOutput, err
}

func (reflex_agent *ReflexAgent) buildCodeMessage(ctx context.Context, params map[string]interface{}) *models.Message {
	if params == nil {
		return nil
	}
	codeRaw, codeOk := params["code"]
	if !codeOk {
		logs.WithContext(ctx).Info("code is not present in the params")
		return nil
	}
	codeStr, codeStrOk := codeRaw.(string)
	if !codeStrOk {
		logs.WithContext(ctx).Info("code is not a string")
		return nil
	}
	content := fmt.Sprintf("This is the existing structured output (e.g. JSON or SQL) generated in a previous attempt and you need to build on top of this incorporating the user's new instructions and improvize. If this code is blank, generate a fresh output. \n\n %s \n\n", codeStr)
	return &models.Message{
		Role:    "system",
		Content: content,
		Name:    reflex_agent.AgentName,
	}
}

func (reflex_agent *ReflexAgent) validate(ctx context.Context, jsonString string) error {
	logs.WithContext(ctx).Debug("validate - Start")
	logs.WithContext(ctx).Info(fmt.Sprintf("jsonString: %+v", jsonString))
	return nil
}

func (reflex_agent *ReflexAgent) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &reflex_agent)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
func (reflex_agent *ReflexAgent) callTool(ctx context.Context, projectId string, tenantId string, tool tools.Tooling, params map[string]interface{}) (map[string]interface{}, bool, error) {
	logs.WithContext(ctx).Debug("callTool - Start")
	return tool.Execute(ctx, projectId, tenantId, "", params)
}

func (reflex_agent *ReflexAgent) callModel(ctx context.Context, model models.ModelI, params map[string]interface{}) (map[string]interface{}, error) {
	logs.WithContext(ctx).Debug("callModel - Start")
	return nil, nil
}

func init() {
	agents.RegisterAgentType("REFLEX", func() agents.AgentI { return new(ReflexAgent) })
}
