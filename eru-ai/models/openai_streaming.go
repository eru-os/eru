package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIToolLoopStreamRequest struct {
	OpenAIToolLoopRequest
	Stream        bool                 `json:"stream"`
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
}

type openAIStreamDeltaFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIStreamToolCallDelta struct {
	Index    int                       `json:"index"`
	Id       string                    `json:"id"`
	Type     string                    `json:"type"`
	Function openAIStreamDeltaFunction `json:"function"`
}

type openAIStreamDelta struct {
	Role      string                      `json:"role"`
	Content   string                      `json:"content"`
	Refusal   string                      `json:"refusal"`
	ToolCalls []openAIStreamToolCallDelta `json:"tool_calls"`
}

type openAIStreamChoice struct {
	Index        int               `json:"index"`
	Delta        openAIStreamDelta `json:"delta"`
	FinishReason string            `json:"finish_reason"`
}

type openAIStreamChunk struct {
	Id      string               `json:"id"`
	Object  string               `json:"object"`
	Choices []openAIStreamChoice `json:"choices"`
	Usage   *OpenAIChatUsage     `json:"usage"`
}

type openAIStreamResult struct {
	Content      string
	FinishReason string
	ToolCalls    []OpenAIResponseToolCall
	Usage        OpenAIChatUsage
}

func openAIUsageTokens(usage OpenAIChatUsage) (inputTokens int64, outputTokens int64, cachedTokens int64, reasoningTokens int64) {
	inputTokens = usage.PromptTokens
	if inputTokens == 0 {
		inputTokens = usage.InputTokens
	}
	outputTokens = usage.CompletionTokens
	if outputTokens == 0 {
		outputTokens = usage.OutputTokens
	}
	cachedTokens = int64(usage.PromptTokensDetails.CachedTokens)
	if cachedTokens == 0 {
		cachedTokens = usage.InputTokensDetails.CachedTokens
	}
	reasoningTokens = int64(usage.CompletionTokensDetails.ReasoningTokens)
	if reasoningTokens == 0 {
		reasoningTokens = usage.OutputTokensDetails.ReasoningTokens
	}
	return
}

func accumulateOpenAIUsage(acc *UsageAccumulator, usage OpenAIChatUsage) {
	inputTokens, outputTokens, cachedTokens, reasoningTokens := openAIUsageTokens(usage)
	acc.Add(inputTokens, outputTokens, cachedTokens, reasoningTokens)
}

func reasoningEffortForBudget(thinkingBudget int, configured string) string {
	if configured != "" {
		return configured
	}
	switch {
	case thinkingBudget <= 0:
		return "medium"
	case thinkingBudget < 2048:
		return "low"
	case thinkingBudget < 8192:
		return "medium"
	default:
		return "high"
	}
}

func (openaiModel *OpenAIModel) buildToolLoopRequest(messages []OpenAIToolLoopMessage, openAIRequestTools []OpenAIRequestTools, thinkingBudget int) OpenAIToolLoopRequest {
	serviceTier := openaiModel.ServiceTier
	if serviceTier == "" {
		serviceTier = "auto"
	}

	request := OpenAIToolLoopRequest{
		Model:       openaiModel.LLMName,
		Messages:    messages,
		ServiceTier: serviceTier,
	}
	if len(openAIRequestTools) > 0 {
		request.Tools = openAIRequestTools
		request.ToolChoice = "auto"
	}
	if openaiModel.MaxTokens > 0 {
		request.MaxCompletionTokens = openaiModel.MaxTokens
	}
	if openaiModel.isReasoningModel() {
		request.ReasoningEffort = reasoningEffortForBudget(thinkingBudget, openaiModel.ReasoningEffort)
	} else {
		temperature := openaiModel.Temprature
		request.Temperature = &temperature
	}
	return request
}

func (openaiModel *OpenAIModel) streamChatCompletion(ctx context.Context, request OpenAIToolLoopRequest, onTextDelta func(chunk string), onToolInputDelta func(toolName string, blockIndex int, partial string)) (openAIStreamResult, error) {
	result := openAIStreamResult{}

	streamRequest := openAIToolLoopStreamRequest{
		OpenAIToolLoopRequest: request,
		Stream:                true,
		StreamOptions:         &openAIStreamOptions{IncludeUsage: true},
	}

	reqHeader := http.Header{}
	reqHeader.Add("Authorization", "Bearer "+openaiModel.LLMSecret)
	reqHeader.Add("Content-Type", "application/json")

	req, err := newJsonRequest(ctx, http.MethodPost, OpenAIApiUrl, reqHeader, streamRequest)
	if err != nil {
		return result, err
	}

	accumulator := newOpenAIStreamAccumulator()

	err = streamSSE(ctx, req, func(data []byte) (bool, error) {
		var chunk openAIStreamChunk
		if uerr := json.Unmarshal(data, &chunk); uerr != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("openai stream chunk parse failed: ", uerr.Error(), " raw=", string(data)))
			return false, nil
		}
		accumulator.consume(chunk, onTextDelta, onToolInputDelta)
		return false, nil
	})
	if err != nil {
		return result, err
	}

	return accumulator.result(), nil
}

type openAIStreamAccumulator struct {
	content       strings.Builder
	finishReason  string
	toolCalls     map[int]*OpenAIResponseToolCall
	toolArguments map[int]*strings.Builder
	usage         OpenAIChatUsage
}

func newOpenAIStreamAccumulator() *openAIStreamAccumulator {
	return &openAIStreamAccumulator{
		toolCalls:     map[int]*OpenAIResponseToolCall{},
		toolArguments: map[int]*strings.Builder{},
	}
}

func (a *openAIStreamAccumulator) consume(chunk openAIStreamChunk, onTextDelta func(chunk string), onToolInputDelta func(toolName string, blockIndex int, partial string)) {
	if chunk.Usage != nil {
		a.usage = *chunk.Usage
	}

	for _, choice := range chunk.Choices {
		if choice.FinishReason != "" {
			a.finishReason = choice.FinishReason
		}
		if choice.Delta.Content != "" {
			a.content.WriteString(choice.Delta.Content)
			if onTextDelta != nil {
				onTextDelta(choice.Delta.Content)
			}
		}
		for _, toolDelta := range choice.Delta.ToolCalls {
			existing, found := a.toolCalls[toolDelta.Index]
			if !found {
				existing = &OpenAIResponseToolCall{Type: "function"}
				a.toolCalls[toolDelta.Index] = existing
				a.toolArguments[toolDelta.Index] = &strings.Builder{}
			}
			if toolDelta.Id != "" {
				existing.Id = toolDelta.Id
			}
			if toolDelta.Type != "" {
				existing.Type = toolDelta.Type
			}
			if toolDelta.Function.Name != "" {
				existing.Function.Name = toolDelta.Function.Name
			}
			if toolDelta.Function.Arguments != "" {
				a.toolArguments[toolDelta.Index].WriteString(toolDelta.Function.Arguments)
				if onToolInputDelta != nil {
					onToolInputDelta(existing.Function.Name, toolDelta.Index, toolDelta.Function.Arguments)
				}
			}
		}
	}
}

func (a *openAIStreamAccumulator) result() openAIStreamResult {
	result := openAIStreamResult{
		Content:      a.content.String(),
		FinishReason: a.finishReason,
		Usage:        a.usage,
	}

	indexes := make([]int, 0, len(a.toolCalls))
	for index := range a.toolCalls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		toolCall := a.toolCalls[index]
		toolCall.Function.Arguments = a.toolArguments[index].String()
		result.ToolCalls = append(result.ToolCalls, *toolCall)
	}
	return result
}

func (openaiModel *OpenAIModel) QueryModelStreaming(ctx context.Context, chatRequest ChatRequest, callback func(chunk string)) (Message, error) {
	logs.WithContext(ctx).Debug("QueryModelStreaming - Start")

	var messages []OpenAIToolLoopMessage
	for _, message := range chatRequest.Messages {
		messages = append(messages, OpenAIToolLoopMessage{
			Role:    message.Role,
			Content: openaiModel.makeOpenAIChatRequestContent(ctx, message),
		})
	}

	request := openaiModel.buildToolLoopRequest(messages, nil, 0)

	result, err := openaiModel.streamChatCompletion(ctx, request, callback, nil)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Message{}, err
	}

	var usageAcc UsageAccumulator
	accumulateOpenAIUsage(&usageAcc, result.Usage)
	return usageAcc.Attach(Message{Content: result.Content, Role: "assistant"}), nil
}

func (openaiModel *OpenAIModel) QueryModelWithReasoning(ctx context.Context, chatRequest ChatRequest, thinkingBudget int) (Message, string, error) {
	logs.WithContext(ctx).Debug("QueryModelWithReasoning - Start")

	var messages []OpenAIToolLoopMessage
	for _, message := range chatRequest.Messages {
		messages = append(messages, OpenAIToolLoopMessage{
			Role:    message.Role,
			Content: openaiModel.makeOpenAIChatRequestContent(ctx, message),
		})
	}

	request := openaiModel.buildToolLoopRequest(messages, nil, thinkingBudget)
	if !openaiModel.isReasoningModel() {
		logs.WithContext(ctx).Info(fmt.Sprint("model ", openaiModel.LLMName, " is not a reasoning model - answering without reasoning"))
	}

	openAIChatResponse, err := openaiModel.queryToolLoop(ctx, request)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Message{}, "", err
	}
	if len(openAIChatResponse.Choices) == 0 {
		err = errors.New("no choices in OpenAI response")
		logs.WithContext(ctx).Error(err.Error())
		return Message{}, "", err
	}

	var usageAcc UsageAccumulator
	accumulateOpenAIUsage(&usageAcc, openAIChatResponse.Usage)
	message := usageAcc.Attach(Message{Content: openAIChatResponse.Choices[0].Message.Content, Role: "assistant"})
	return message, "", nil
}

func (openaiModel *OpenAIModel) RunToolLoopStreaming(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor, streamCb StreamEventCallback) (Message, []StepTrace, error) {
	logs.WithContext(ctx).Debug("RunToolLoopStreaming - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "OpenAI.RunToolLoopStreaming",
		oteltrace.WithAttributes(attribute.String("model", openaiModel.LLMName), attribute.Int("max_iterations", maxIterations)),
	)
	defer span.End()

	openAIRequestTools, toolPrompt := convertOpenAITools(ctx, toolsMap)
	systemContent := agentPrompt.String()
	if toolPrompt != "" {
		systemContent = strings.TrimSpace(fmt.Sprint(systemContent, "\n", toolPrompt))
	}

	var messages []OpenAIToolLoopMessage
	messages = append(messages, OpenAIToolLoopMessage{Role: "system", Content: systemContent})
	for _, message := range chatRequest.Messages {
		messages = append(messages, OpenAIToolLoopMessage{
			Role:    message.Role,
			Content: openaiModel.makeOpenAIChatRequestContent(ctx, message),
		})
	}

	var traces []StepTrace
	var usageAcc UsageAccumulator

	for iteration := 1; iteration <= maxIterations; iteration++ {
		request := openaiModel.buildToolLoopRequest(messages, openAIRequestTools, thinkingBudget)

		currentIteration := iteration
		result, err := openaiModel.streamChatCompletion(ctx, request,
			func(chunk string) {
				if streamCb != nil {
					streamCb(ModelStreamEvent{
						Type:      StreamTextDelta,
						Content:   chunk,
						Iteration: currentIteration,
					})
				}
			},
			func(toolName string, blockIndex int, partial string) {
				if streamCb != nil {
					streamCb(ModelStreamEvent{
						Type:       StreamToolInputDelta,
						Content:    partial,
						ToolName:   toolName,
						Iteration:  currentIteration,
						BlockIndex: blockIndex,
					})
				}
			},
		)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return Message{}, traces, err
		}

		accumulateOpenAIUsage(&usageAcc, result.Usage)

		trace := StepTrace{
			Iteration: iteration,
			Timestamp: time.Now(),
		}

		if result.FinishReason == "length" {
			logs.WithContext(ctx).Error(fmt.Sprintf("OpenAI response truncated: finish_reason=length, max_completion_tokens=%d. Increase model.max_tokens.", request.MaxCompletionTokens))
		}

		assistantMessage := OpenAIToolLoopMessage{Role: "assistant", ToolCalls: result.ToolCalls}
		if result.Content != "" {
			assistantMessage.Content = result.Content
		}
		messages = append(messages, assistantMessage)

		if len(result.ToolCalls) == 0 {
			trace.Content = result.Content
			traces = append(traces, trace)
			return usageAcc.Attach(Message{Content: result.Content, Role: "assistant"}), traces, nil
		}

		for _, toolCall := range result.ToolCalls {
			inputMap := make(map[string]interface{})
			if uerr := json.Unmarshal([]byte(toolCall.Function.Arguments), &inputMap); uerr != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("tool_call %s arguments parse failed (likely truncated by max_completion_tokens=%d): %v; raw=%s", toolCall.Function.Name, request.MaxCompletionTokens, uerr, toolCall.Function.Arguments))
			}

			if toolCall.Function.Name == TerminalToolStructuredOutput {
				trace.ToolName = toolCall.Function.Name
				trace.ToolInput = inputMap
				traces = append(traces, trace)
				if len(inputMap) == 0 {
					return Message{}, traces, fmt.Errorf("structured_output tool input was empty or truncated; raise model.max_tokens (current=%d)", request.MaxCompletionTokens)
				}
				resultBytes, _ := json.Marshal(inputMap)
				return usageAcc.Attach(Message{Content: string(resultBytes), Role: "assistant", TerminalTool: TerminalToolStructuredOutput}), traces, nil
			}

			if toolCall.Function.Name == TerminalToolAskUser {
				trace.ToolName = toolCall.Function.Name
				trace.ToolInput = inputMap
				traces = append(traces, trace)
				if len(inputMap) == 0 {
					return Message{}, traces, fmt.Errorf("ask_user tool input was empty or truncated; raise model.max_tokens (current=%d)", request.MaxCompletionTokens)
				}
				if streamCb != nil {
					streamCb(ModelStreamEvent{
						Type:      StreamQuestion,
						ToolName:  toolCall.Function.Name,
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
					ToolName:  toolCall.Function.Name,
					ToolInput: inputMap,
					Iteration: iteration,
				})
			}
			trace.ToolName = toolCall.Function.Name
			trace.ToolInput = inputMap
		}

		for _, toolCall := range result.ToolCalls {
			inputMap := make(map[string]interface{})
			json.Unmarshal([]byte(toolCall.Function.Arguments), &inputMap)

			execResult, execErr := toolExecutor(ctx, toolCall.Function.Name, inputMap)
			toolMessage := OpenAIToolLoopMessage{Role: "tool", ToolCallId: toolCall.Id}
			if execErr != nil {
				toolMessage.Content = fmt.Sprintf("Error: %s", execErr.Error())
				trace.ToolResult = map[string]interface{}{"error": execErr.Error()}
				if streamCb != nil {
					streamCb(ModelStreamEvent{Type: StreamToolResult, Content: execErr.Error(), ToolName: toolCall.Function.Name, Iteration: iteration})
				}
			} else {
				resultBytes, _ := json.Marshal(execResult)
				toolMessage.Content = string(resultBytes)
				trace.ToolResult = execResult
				if streamCb != nil {
					streamCb(ModelStreamEvent{Type: StreamToolResult, Content: string(resultBytes), ToolName: toolCall.Function.Name, Iteration: iteration})
				}
			}
			messages = append(messages, toolMessage)
		}
		traces = append(traces, trace)
	}

	return usageAcc.Attach(Message{Content: "max iterations reached", Role: "assistant"}), traces, nil
}
