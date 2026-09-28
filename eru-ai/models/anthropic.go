package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type AnthropicModel struct {
	Model
	client anthropic.Client
	inited bool
}

func (m *AnthropicModel) getClient() anthropic.Client {
	if !m.inited {
		m.client = anthropic.NewClient(option.WithAPIKey(m.LLMSecret))
		m.inited = true
	}
	return m.client
}

func (m *AnthropicModel) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, m)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (m *AnthropicModel) QueryModel(ctx context.Context, chatRequest ChatRequest) (Message, error) {
	logs.WithContext(ctx).Debug("QueryModel - Start")
	client := m.getClient()

	messages := convertMessages(ctx, chatRequest.Messages)

	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       m.LLMName,
		MaxTokens:   int64(1024),
		Temperature: anthropic.Float(m.Temprature),
		Messages:    messages,
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Message{}, err
	}

	var combinedContent string
	for _, block := range msg.Content {
		if block.Type == "text" {
			combinedContent += block.AsText().Text
		}
	}

	return Message{Content: combinedContent, Role: "assistant"}, nil
}

func (m *AnthropicModel) QueryModelWithTool(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentName string, agentPrompt string) (JsonMessage, error) {
	logs.WithContext(ctx).Debug("QueryModelWithTool - Start")
	client := m.getClient()

	sdkTools, toolPrompt := convertTools(ctx, toolsMap)

	systemContent := strings.TrimSpace(fmt.Sprint(agentPrompt, "\n", toolPrompt))

	var messages []anthropic.MessageParam
	messages = append(messages, anthropic.NewAssistantMessage(
		anthropic.NewTextBlock(systemContent),
	))
	for _, message := range chatRequest.Messages {
		messages = append(messages, convertSingleMessage(ctx, message))
	}

	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       m.LLMName,
		MaxTokens:   int64(1024),
		Temperature: anthropic.Float(m.Temprature),
		Messages:    messages,
		Tools:       sdkTools,
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{Type: "any"},
		},
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return JsonMessage{}, err
	}

	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			toolUse := block.AsToolUse()
			outputJson := make(map[string]interface{})
			if err := json.Unmarshal(toolUse.Input, &outputJson); err != nil {
				logs.WithContext(ctx).Error(err.Error())
				outputJson = map[string]interface{}{"raw": string(toolUse.Input)}
			}
			return JsonMessage{Content: outputJson, Role: "assistant"}, nil
		}
	}

	return JsonMessage{}, errors.New("no tool_use block in response")
}

func (m *AnthropicModel) QueryModelStreaming(ctx context.Context, chatRequest ChatRequest, callback func(chunk string)) (Message, error) {
	logs.WithContext(ctx).Debug("QueryModelStreaming - Start")
	client := m.getClient()

	messages := convertMessages(ctx, chatRequest.Messages)

	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:       m.LLMName,
		MaxTokens:   int64(1024),
		Temperature: anthropic.Float(m.Temprature),
		Messages:    messages,
	})

	var message anthropic.Message
	for stream.Next() {
		event := stream.Current()
		message.Accumulate(event)
		if event.Type == "content_block_delta" {
			delta := event.AsContentBlockDelta()
			if delta.Delta.Type == "text_delta" {
				callback(delta.Delta.AsTextDelta().Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Message{}, err
	}

	var content string
	for _, block := range message.Content {
		if block.Type == "text" {
			content += block.AsText().Text
		}
	}
	return Message{Content: content, Role: "assistant"}, nil
}

func (m *AnthropicModel) QueryModelWithReasoning(ctx context.Context, chatRequest ChatRequest, thinkingBudget int) (Message, string, error) {
	logs.WithContext(ctx).Debug("QueryModelWithReasoning - Start")
	client := m.getClient()

	messages := convertMessages(ctx, chatRequest.Messages)

	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     m.LLMName,
		MaxTokens: int64(16000),
		Messages:  messages,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{
				Type:         "enabled",
				BudgetTokens: int64(thinkingBudget),
			},
		},
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Message{}, "", err
	}

	var content, thinking string
	for _, block := range msg.Content {
		if block.Type == "text" {
			content += block.AsText().Text
		}
		if block.Type == "thinking" {
			thinking += block.AsThinking().Thinking
		}
	}
	return Message{Content: content, Role: "assistant"}, thinking, nil
}

func (m *AnthropicModel) RunToolLoop(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor) (Message, []StepTrace, error) {
	logs.WithContext(ctx).Debug("RunToolLoop - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "Anthropic.RunToolLoop",
		oteltrace.WithAttributes(attribute.String("model", m.LLMName), attribute.Int("max_iterations", maxIterations)),
	)
	defer span.End()
	client := m.getClient()

	sdkTools, toolPrompt := convertTools(ctx, toolsMap)
	systemBlocks := cachedSystemBlocks(agentPrompt, toolPrompt)

	messages := convertMessages(ctx, chatRequest.Messages)

	var traces []StepTrace
	var usageAcc UsageAccumulator

	maxTokens := resolveMaxTokens(m.MaxTokens, thinkingBudget)

	for iteration := 1; iteration <= maxIterations; iteration++ {
		params := anthropic.MessageNewParams{
			Model:        m.LLMName,
			MaxTokens:    maxTokens,
			Messages:     messages,
			Tools:        sdkTools,
			System:       systemBlocks,
			CacheControl: anthropic.CacheControlEphemeralParam{TTL: anthropic.CacheControlEphemeralTTLTTL5m},
		}
		if thinkingBudget > 0 {
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfEnabled: &anthropic.ThinkingConfigEnabledParam{
					Type:         "enabled",
					BudgetTokens: int64(thinkingBudget),
				},
			}
		} else {
			params.Temperature = anthropic.Float(m.Temprature)
		}

		msg, err := client.Messages.New(ctx, params, option.WithRequestTimeout(nonStreamingTimeout(maxTokens)))
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return Message{}, traces, err
		}

		logCacheUsage(ctx, iteration, msg.Usage)
		accumulateUsage(&usageAcc, msg.Usage)

		trace := StepTrace{
			Iteration: iteration,
			Timestamp: time.Now(),
		}

		var assistantBlocks []anthropic.ContentBlockParamUnion
		var toolUseBlocks []anthropic.ToolUseBlock
		var finalContent string

		for _, block := range msg.Content {
			switch block.Type {
			case "thinking":
				trace.Thinking = block.AsThinking().Thinking
				assistantBlocks = append(assistantBlocks, anthropic.ContentBlockParamUnion{
					OfThinking: &anthropic.ThinkingBlockParam{
						Type:      "thinking",
						Thinking:  block.AsThinking().Thinking,
						Signature: block.AsThinking().Signature,
					},
				})
			case "text":
				finalContent += block.AsText().Text
				assistantBlocks = append(assistantBlocks, anthropic.NewTextBlock(block.AsText().Text))
			case "tool_use":
				tu := block.AsToolUse()
				toolUseBlocks = append(toolUseBlocks, tu)
				assistantBlocks = append(assistantBlocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tu.ID,
						Name:  tu.Name,
						Input: tu.Input,
						Type:  "tool_use",
					},
				})
			}
		}

		messages = append(messages, anthropic.MessageParam{
			Role:    "assistant",
			Content: assistantBlocks,
		})

		if string(msg.StopReason) == "max_tokens" {
			logs.WithContext(ctx).Error(fmt.Sprintf("Anthropic response truncated: stop_reason=max_tokens, max_tokens=%d. Increase model.max_tokens or reduce thinking_budget.", maxTokens))
		}

		if len(toolUseBlocks) == 0 {
			trace.Content = finalContent
			traces = append(traces, trace)
			return usageAcc.Attach(Message{Content: finalContent, Role: "assistant"}), traces, nil
		}

		for _, tu := range toolUseBlocks {
			inputMap := make(map[string]interface{})
			if uerr := json.Unmarshal(tu.Input, &inputMap); uerr != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("tool_use %s input parse failed (likely truncated by max_tokens=%d): %v; raw=%s", tu.Name, maxTokens, uerr, string(tu.Input)))
			}

			if tu.Name == "structured_output" {
				trace.ToolName = tu.Name
				trace.ToolInput = inputMap
				traces = append(traces, trace)
				if len(inputMap) == 0 {
					return Message{}, traces, fmt.Errorf("structured_output tool input was empty or truncated; raise model.max_tokens (current=%d) or lower thinking_budget", maxTokens)
				}
				resultBytes, _ := json.Marshal(inputMap)
				return usageAcc.Attach(Message{Content: string(resultBytes), Role: "assistant", TerminalTool: TerminalToolStructuredOutput}), traces, nil
			}

			if tu.Name == TerminalToolAskUser {
				trace.ToolName = tu.Name
				trace.ToolInput = inputMap
				traces = append(traces, trace)
				if len(inputMap) == 0 {
					return Message{}, traces, fmt.Errorf("ask_user tool input was empty or truncated; raise model.max_tokens (current=%d) or lower thinking_budget", maxTokens)
				}
				resultBytes, _ := json.Marshal(inputMap)
				return usageAcc.Attach(Message{Content: string(resultBytes), Role: "assistant", TerminalTool: TerminalToolAskUser}), traces, nil
			}

			trace.ToolName = tu.Name
			trace.ToolInput = inputMap
		}

		var toolResultBlocks []anthropic.ContentBlockParamUnion
		for _, tu := range toolUseBlocks {
			inputMap := make(map[string]interface{})
			json.Unmarshal(tu.Input, &inputMap)

			result, execErr := toolExecutor(ctx, tu.Name, inputMap)
			if execErr != nil {
				toolResultBlocks = append(toolResultBlocks, anthropic.NewToolResultBlock(tu.ID, fmt.Sprintf("Error: %s", execErr.Error()), true))
				trace.ToolResult = map[string]interface{}{"error": execErr.Error()}
			} else {
				resultBytes, _ := json.Marshal(result)
				toolResultBlocks = append(toolResultBlocks, anthropic.NewToolResultBlock(tu.ID, string(resultBytes), false))
				trace.ToolResult = result
			}
		}
		traces = append(traces, trace)

		messages = append(messages, anthropic.NewUserMessage(toolResultBlocks...))
	}

	return usageAcc.Attach(Message{Content: "max iterations reached", Role: "assistant"}), traces, nil
}

func (m *AnthropicModel) RunToolLoopStreaming(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor, streamCb StreamEventCallback) (Message, []StepTrace, error) {
	logs.WithContext(ctx).Debug("RunToolLoopStreaming - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "Anthropic.RunToolLoopStreaming",
		oteltrace.WithAttributes(attribute.String("model", m.LLMName), attribute.Int("max_iterations", maxIterations)),
	)
	defer span.End()
	client := m.getClient()

	sdkTools, toolPrompt := convertTools(ctx, toolsMap)
	systemBlocks := cachedSystemBlocks(agentPrompt, toolPrompt)

	messages := convertMessages(ctx, chatRequest.Messages)
	var traces []StepTrace
	var usageAcc UsageAccumulator

	maxTokens := resolveMaxTokens(m.MaxTokens, thinkingBudget)

	for iteration := 1; iteration <= maxIterations; iteration++ {
		params := anthropic.MessageNewParams{
			Model:        m.LLMName,
			MaxTokens:    maxTokens,
			Messages:     messages,
			Tools:        sdkTools,
			System:       systemBlocks,
			CacheControl: anthropic.CacheControlEphemeralParam{TTL: anthropic.CacheControlEphemeralTTLTTL5m},
		}
		if thinkingBudget > 0 {
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfEnabled: &anthropic.ThinkingConfigEnabledParam{
					Type:         "enabled",
					BudgetTokens: int64(thinkingBudget),
				},
			}
		} else {
			params.Temperature = anthropic.Float(m.Temprature)
		}

		stream := client.Messages.NewStreaming(ctx, params)

		trace := StepTrace{
			Iteration: iteration,
			Timestamp: time.Now(),
		}

		var msg anthropic.Message
		var currentThinking string
		// Which tool each content block belongs to, so an argument delta can say
		// what it is arguments for.
		toolBlockNames := map[int64]string{}

		for stream.Next() {
			event := stream.Current()
			msg.Accumulate(event)

			if event.Type == "content_block_start" {
				start := event.AsContentBlockStart()
				if start.ContentBlock.Type == "tool_use" {
					toolBlockNames[start.Index] = start.ContentBlock.Name
				}
			}

			if event.Type == "content_block_delta" {
				delta := event.AsContentBlockDelta()
				switch delta.Delta.Type {
				case "input_json_delta":
					// The arguments of a tool call as they are written. For a
					// structured-output agent this is the answer itself arriving,
					// which is what lets a client render before the model is done.
					if streamCb != nil {
						streamCb(ModelStreamEvent{
							Type:       StreamToolInputDelta,
							Content:    delta.Delta.AsInputJSONDelta().PartialJSON,
							ToolName:   toolBlockNames[delta.Index],
							Iteration:  iteration,
							BlockIndex: int(delta.Index),
						})
					}
				case "thinking_delta":
					chunk := delta.Delta.AsThinkingDelta().Thinking
					currentThinking += chunk
					if streamCb != nil {
						streamCb(ModelStreamEvent{
							Type:      StreamThinking,
							Content:   chunk,
							Iteration: iteration,
						})
					}
				case "text_delta":
					chunk := delta.Delta.AsTextDelta().Text
					if streamCb != nil {
						streamCb(ModelStreamEvent{
							Type:      StreamTextDelta,
							Content:   chunk,
							Iteration: iteration,
						})
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return Message{}, traces, err
		}

		logCacheUsage(ctx, iteration, msg.Usage)
		accumulateUsage(&usageAcc, msg.Usage)

		trace.Thinking = currentThinking

		var assistantBlocks []anthropic.ContentBlockParamUnion
		var toolUseBlocks []anthropic.ToolUseBlock
		var finalContent string

		for _, block := range msg.Content {
			switch block.Type {
			case "thinking":
				assistantBlocks = append(assistantBlocks, anthropic.ContentBlockParamUnion{
					OfThinking: &anthropic.ThinkingBlockParam{
						Type:      "thinking",
						Thinking:  block.AsThinking().Thinking,
						Signature: block.AsThinking().Signature,
					},
				})
			case "text":
				finalContent += block.AsText().Text
				assistantBlocks = append(assistantBlocks, anthropic.NewTextBlock(block.AsText().Text))
			case "tool_use":
				tu := block.AsToolUse()
				toolUseBlocks = append(toolUseBlocks, tu)
				assistantBlocks = append(assistantBlocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tu.ID,
						Name:  tu.Name,
						Input: tu.Input,
						Type:  "tool_use",
					},
				})
			}
		}

		messages = append(messages, anthropic.MessageParam{
			Role:    "assistant",
			Content: assistantBlocks,
		})

		if string(msg.StopReason) == "max_tokens" {
			logs.WithContext(ctx).Error(fmt.Sprintf("Anthropic streaming response truncated: stop_reason=max_tokens, max_tokens=%d. Increase model.max_tokens or reduce thinking_budget.", maxTokens))
		}

		if len(toolUseBlocks) == 0 {
			trace.Content = finalContent
			traces = append(traces, trace)
			return usageAcc.Attach(Message{Content: finalContent, Role: "assistant"}), traces, nil
		}

		for _, tu := range toolUseBlocks {
			inputMap := make(map[string]interface{})
			if uerr := json.Unmarshal(tu.Input, &inputMap); uerr != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("tool_use %s input parse failed (likely truncated by max_tokens=%d): %v; raw=%s", tu.Name, maxTokens, uerr, string(tu.Input)))
			}

			if tu.Name == "structured_output" {
				trace.ToolName = tu.Name
				trace.ToolInput = inputMap
				traces = append(traces, trace)
				if len(inputMap) == 0 {
					return Message{}, traces, fmt.Errorf("structured_output tool input was empty or truncated; raise model.max_tokens (current=%d) or lower thinking_budget", maxTokens)
				}
				resultBytes, _ := json.Marshal(inputMap)
				return usageAcc.Attach(Message{Content: string(resultBytes), Role: "assistant", TerminalTool: TerminalToolStructuredOutput}), traces, nil
			}

			if tu.Name == TerminalToolAskUser {
				trace.ToolName = tu.Name
				trace.ToolInput = inputMap
				traces = append(traces, trace)
				if len(inputMap) == 0 {
					return Message{}, traces, fmt.Errorf("ask_user tool input was empty or truncated; raise model.max_tokens (current=%d) or lower thinking_budget", maxTokens)
				}
				if streamCb != nil {
					streamCb(ModelStreamEvent{
						Type:      StreamQuestion,
						ToolName:  tu.Name,
						ToolInput: inputMap,
						Iteration: iteration,
					})
				}
				resultBytes, _ := json.Marshal(inputMap)
				return usageAcc.Attach(Message{Content: string(resultBytes), Role: "assistant", TerminalTool: TerminalToolAskUser}), traces, nil
			}

			if streamCb != nil {
				streamCb(ModelStreamEvent{
					Type:      StreamToolUse,
					ToolName:  tu.Name,
					ToolInput: inputMap,
					Iteration: iteration,
				})
			}
			trace.ToolName = tu.Name
			trace.ToolInput = inputMap
		}

		var toolResultBlocks []anthropic.ContentBlockParamUnion
		for _, tu := range toolUseBlocks {
			inputMap := make(map[string]interface{})
			json.Unmarshal(tu.Input, &inputMap)

			result, execErr := toolExecutor(ctx, tu.Name, inputMap)
			if execErr != nil {
				toolResultBlocks = append(toolResultBlocks, anthropic.NewToolResultBlock(tu.ID, fmt.Sprintf("Error: %s", execErr.Error()), true))
				trace.ToolResult = map[string]interface{}{"error": execErr.Error()}
				if streamCb != nil {
					streamCb(ModelStreamEvent{Type: StreamToolResult, Content: execErr.Error(), ToolName: tu.Name, Iteration: iteration})
				}
			} else {
				resultBytes, _ := json.Marshal(result)
				toolResultBlocks = append(toolResultBlocks, anthropic.NewToolResultBlock(tu.ID, string(resultBytes), false))
				trace.ToolResult = result
				if streamCb != nil {
					streamCb(ModelStreamEvent{Type: StreamToolResult, Content: string(resultBytes), ToolName: tu.Name, Iteration: iteration})
				}
			}
		}
		traces = append(traces, trace)
		messages = append(messages, anthropic.NewUserMessage(toolResultBlocks...))
	}

	return usageAcc.Attach(Message{Content: "max iterations reached", Role: "assistant"}), traces, nil
}

func convertMessages(ctx context.Context, messages []Message) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	for _, msg := range messages {
		result = append(result, convertSingleMessage(ctx, msg))
	}
	return result
}

func convertSingleMessage(ctx context.Context, msg Message) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion

	if msg.Content != "" {
		blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
	}

	for _, file := range msg.Files {
		if file.ImageData != "" {
			blocks = append(blocks, anthropic.NewImageBlockBase64(file.FileType, file.ImageData))
		} else if file.FileData != "" {
			if file.FileType == "application/pdf" {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfDocument: &anthropic.DocumentBlockParam{
						Source: anthropic.DocumentBlockParamSourceUnion{
							OfBase64: &anthropic.Base64PDFSourceParam{
								Data: file.FileData,
							},
						},
					},
				})
			} else {
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfDocument: &anthropic.DocumentBlockParam{
						Source: anthropic.DocumentBlockParamSourceUnion{
							OfText: &anthropic.PlainTextSourceParam{
								Data: file.FileData,
							},
						},
					},
				})
			}
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, anthropic.NewTextBlock(""))
	}

	switch msg.Role {
	case "user":
		return anthropic.NewUserMessage(blocks...)
	case "assistant":
		return anthropic.NewAssistantMessage(blocks...)
	default:
		return anthropic.NewUserMessage(blocks...)
	}
}

func convertTools(ctx context.Context, toolsMap map[string]tools.Tooling) ([]anthropic.ToolUnionParam, string) {
	var sdkTools []anthropic.ToolUnionParam
	var toolPrompt string

	for toolKey, tool := range toolsMap {
		toolDescI, _ := tool.GetAttribute(ctx, "description")
		toolSysPromptI, _ := tool.GetAttribute(ctx, "system_prompt")

		toolName := ToolNameForModel(ctx, toolKey, tool)
		toolDesc := toolDescI.(string)
		toolParams := tool.GetParameters()
		toolSysPrompt := toolSysPromptI.(string)

		toolPrompt += fmt.Sprint("Tool prompt for Tool ", toolName, " is as follows :\n", toolSysPrompt)

		sdkTools = append(sdkTools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        toolName,
				Description: anthropic.String(toolDesc),
				InputSchema: jsonSchemaToSDK(toolParams),
			},
		})
	}
	return sdkTools, toolPrompt
}

func jsonSchemaToSDK(schema eru_models.JSONSchema) anthropic.ToolInputSchemaParam {
	properties := make(map[string]any)
	for name, prop := range schema.Properties {
		properties[name] = jsonSchemaPropertyToMap(prop)
	}
	param := anthropic.ToolInputSchemaParam{
		Type:       "object",
		Properties: properties,
		Required:   schema.Required,
	}
	if schema.AdditionalProperties != nil {
		param.ExtraFields = map[string]any{"additionalProperties": schema.AdditionalProperties}
	}
	return param
}

func jsonSchemaPropertyToMap(schema eru_models.JSONSchema) map[string]any {
	m := map[string]any{}
	if schema.Type != "" {
		m["type"] = schema.Type
	}
	if schema.Description != "" {
		m["description"] = schema.Description
	}
	if len(schema.Enum) > 0 {
		m["enum"] = schema.Enum
	}
	if schema.Format != "" {
		m["format"] = schema.Format
	}
	if len(schema.Properties) > 0 {
		props := make(map[string]any)
		for k, v := range schema.Properties {
			props[k] = jsonSchemaPropertyToMap(v)
		}
		m["properties"] = props
	}
	if len(schema.Required) > 0 {
		m["required"] = schema.Required
	}
	if schema.Items != nil {
		m["items"] = jsonSchemaPropertyToMap(*schema.Items)
	}
	if schema.AdditionalProperties != nil {
		m["additionalProperties"] = schema.AdditionalProperties
	}
	return m
}

func accumulateUsage(acc *UsageAccumulator, usage anthropic.Usage) {
	acc.Add(
		usage.InputTokens+usage.CacheReadInputTokens+usage.CacheCreationInputTokens,
		usage.OutputTokens,
		usage.CacheReadInputTokens,
		0,
	)
}

func logCacheUsage(ctx context.Context, iteration int, usage anthropic.Usage) {
	logs.WithContext(ctx).Info(fmt.Sprint(
		"anthropic cache usage iteration=", iteration,
		" input=", usage.InputTokens,
		" cache_write=", usage.CacheCreationInputTokens,
		" cache_read=", usage.CacheReadInputTokens,
		" output=", usage.OutputTokens,
	))
}

func cachedSystemBlocks(agentPrompt AgentPrompt, toolPrompt string) []anthropic.TextBlockParam {
	staticContent := agentPrompt.Static
	if toolPrompt != "" {
		staticContent = strings.TrimSpace(fmt.Sprint(staticContent, "\n", toolPrompt))
	}

	blocks := []anthropic.TextBlockParam{
		{
			Text:         staticContent,
			CacheControl: anthropic.CacheControlEphemeralParam{TTL: anthropic.CacheControlEphemeralTTLTTL5m},
		},
	}
	if agentPrompt.Dynamic != "" {
		blocks = append(blocks, anthropic.TextBlockParam{Text: agentPrompt.Dynamic})
	}
	return blocks
}

// resolveMaxTokens picks a sensible MaxTokens for a Messages request.
//
// Anthropic counts thinking tokens against MaxTokens, so a small MaxTokens
// combined with a non-trivial thinking_budget can leave too few tokens for
// the actual response — typically observed as a truncated tool_use input
// that fails to parse. We give the user-configured value priority and add
// a generous default that always reserves at least 16k of headroom over
// the thinking budget.
func resolveMaxTokens(configured int64, thinkingBudget int) int64 {
	const defaultBase int64 = 32000
	const headroom int64 = 16000

	if configured > 0 {
		return configured
	}
	if thinkingBudget <= 0 {
		return defaultBase
	}
	if int64(thinkingBudget)+headroom > defaultBase {
		return int64(thinkingBudget) + headroom
	}
	return defaultBase
}

// nonStreamingTimeout returns an explicit request timeout for non-streaming
// Messages.New calls. Setting any request timeout bypasses the SDK guard that
// otherwise rejects non-streaming requests whose max_tokens could take longer
// than 10 minutes (anthropic-sdk-go CalculateNonStreamingTimeout). We scale the
// timeout with max_tokens using the SDK's own 1h/128k token estimate, clamped
// to the SDK's 1-hour ceiling and a 10-minute floor.
func nonStreamingTimeout(maxTokens int64) time.Duration {
	const maximumTime = time.Hour
	const defaultTime = 10 * time.Minute
	expected := time.Duration(float64(maximumTime) * float64(maxTokens) / 128000.0)
	if expected < defaultTime {
		return defaultTime
	}
	if expected > maximumTime {
		return maximumTime
	}
	return expected
}
