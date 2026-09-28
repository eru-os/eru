package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	chunking "github.com/eru-os/eru/eru-ai/chunking"
	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	utils "github.com/eru-os/eru/eru-utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const geminiDefaultMaxOutputTokens = 8192

type GeminiEmbeddingRequest struct {
	Model                string        `json:"model"`
	Content              GeminiContent `json:"content"`
	TaskType             string        `json:"taskType,omitempty"`
	OutputDimensionality *int          `json:"outputDimensionality,omitempty"`
}

type GeminiEmbeddingResponse struct {
	Embedding struct {
		Values []float64 `json:"values"`
	} `json:"embedding"`
}

func (geminiModel *GeminiModel) resolveMaxOutputTokens() int {
	if geminiModel.MaxTokens > 0 {
		return int(geminiModel.MaxTokens)
	}
	return geminiDefaultMaxOutputTokens
}

func (geminiModel *GeminiModel) authHeader() http.Header {
	reqHeader := http.Header{}
	reqHeader.Add("Content-Type", "application/json")
	reqHeader.Add("x-goog-api-key", geminiModel.LLMSecret)
	return reqHeader
}

func sanitizeGeminiSchema(schema eru_models.JSONSchema) eru_models.JSONSchema {
	sanitized := schema
	sanitized.AdditionalProperties = nil

	if len(schema.Properties) > 0 {
		properties := make(map[string]eru_models.JSONSchema, len(schema.Properties))
		for name, property := range schema.Properties {
			properties[name] = sanitizeGeminiSchema(property)
		}
		sanitized.Properties = properties
	}
	if schema.Items != nil {
		items := sanitizeGeminiSchema(*schema.Items)
		sanitized.Items = &items
	}
	return sanitized
}

func convertGeminiTools(ctx context.Context, toolsMap map[string]tools.Tooling) ([]GeminiTool, string) {
	var declarations []GeminiFunctionDeclaration
	toolPrompt := ""

	for toolKey, tool := range toolsMap {
		if tool == nil {
			continue
		}
		toolName := ToolNameForModel(ctx, toolKey, tool)
		if toolName == "" {
			continue
		}

		description := ""
		if descriptionAttr, derr := tool.GetAttribute(ctx, "description"); derr == nil {
			if descriptionStr, dok := descriptionAttr.(string); dok {
				description = descriptionStr
			}
		}
		if systemPromptAttr, serr := tool.GetAttribute(ctx, "system_prompt"); serr == nil {
			if systemPrompt, sok := systemPromptAttr.(string); sok && systemPrompt != "" {
				toolPrompt = fmt.Sprint(toolPrompt, "\n", systemPrompt)
			}
		}

		declarations = append(declarations, GeminiFunctionDeclaration{
			Name:        toolName,
			Description: description,
			Parameters:  sanitizeGeminiSchema(tool.GetParameters()),
		})
	}

	if len(declarations) == 0 {
		return nil, strings.TrimSpace(toolPrompt)
	}
	return []GeminiTool{{FunctionDeclarations: declarations}}, strings.TrimSpace(toolPrompt)
}

func accumulateGeminiUsage(acc *UsageAccumulator, usage *GeminiUsageMetadata) {
	if usage == nil {
		return
	}
	acc.Add(
		int64(usage.PromptTokenCount),
		int64(usage.CandidatesTokenCount+usage.ThoughtsTokenCount),
		int64(usage.CachedContentTokenCount),
		int64(usage.ThoughtsTokenCount),
	)
}

func (geminiModel *GeminiModel) buildGeminiRequest(ctx context.Context, chatRequest ChatRequest, geminiTools []GeminiTool, systemContent string, thinkingBudget int) GeminiChatRequest {
	maxOutputTokens := geminiModel.resolveMaxOutputTokens()
	temperature := geminiModel.Temprature

	request := GeminiChatRequest{
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     &temperature,
			MaxOutputTokens: &maxOutputTokens,
		},
	}
	if len(geminiTools) > 0 {
		request.Tools = geminiTools
	}
	if thinkingBudget > 0 {
		budget := thinkingBudget
		request.GenerationConfig.ThinkingConfig = &GeminiThinkingConfig{
			ThinkingBudget:  &budget,
			IncludeThoughts: true,
		}
	}
	if systemContent != "" {
		request.SystemInstruction = &GeminiContent{Parts: []GeminiPart{{Text: systemContent}}}
	}

	for _, message := range chatRequest.Messages {
		if message.Role == "system" {
			continue
		}
		request.Contents = append(request.Contents, GeminiContent{
			Role:  geminiModel.mapRoleToGemini(message.Role),
			Parts: geminiModel.makeGeminiParts(ctx, message),
		})
	}
	return request
}

func (geminiModel *GeminiModel) generateContent(ctx context.Context, request GeminiChatRequest) (geminiResponse GeminiChatResponse, err error) {
	url := fmt.Sprintf("%s/%s:generateContent", GeminiApiUrl, geminiModel.LLMName)

	response, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, geminiModel.authHeader(), nil, nil, nil, request)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	responseJson, err := json.Marshal(response)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	err = json.Unmarshal(responseJson, &geminiResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (geminiModel *GeminiModel) streamGenerateContent(ctx context.Context, request GeminiChatRequest, onChunk func(candidate GeminiCandidate)) (GeminiChatResponse, error) {
	aggregate := GeminiChatResponse{}
	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse", GeminiApiUrl, geminiModel.LLMName)

	req, err := newJsonRequest(ctx, http.MethodPost, url, geminiModel.authHeader(), request)
	if err != nil {
		return aggregate, err
	}

	var aggregatedParts []GeminiPart
	finishReason := ""

	err = streamSSE(ctx, req, func(data []byte) (bool, error) {
		var chunk GeminiChatResponse
		if uerr := json.Unmarshal(data, &chunk); uerr != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("gemini stream chunk parse failed: ", uerr.Error(), " raw=", string(data)))
			return false, nil
		}

		if chunk.UsageMetadata != nil {
			aggregate.UsageMetadata = chunk.UsageMetadata
		}
		if chunk.PromptFeedback != nil {
			aggregate.PromptFeedback = chunk.PromptFeedback
		}

		for _, candidate := range chunk.Candidates {
			if candidate.FinishReason != "" {
				finishReason = candidate.FinishReason
			}
			if onChunk != nil {
				onChunk(candidate)
			}
			aggregatedParts = append(aggregatedParts, candidate.Content.Parts...)
		}
		return false, nil
	})
	if err != nil {
		return aggregate, err
	}

	aggregate.Candidates = []GeminiCandidate{{
		Content:      GeminiContent{Role: "model", Parts: mergeGeminiTextParts(aggregatedParts)},
		FinishReason: finishReason,
	}}
	return aggregate, nil
}

func mergeGeminiTextParts(parts []GeminiPart) []GeminiPart {
	var merged []GeminiPart
	var textBuilder strings.Builder
	var thoughtBuilder strings.Builder

	flush := func() {
		if thoughtBuilder.Len() > 0 {
			merged = append(merged, GeminiPart{Text: thoughtBuilder.String(), Thought: true})
			thoughtBuilder.Reset()
		}
		if textBuilder.Len() > 0 {
			merged = append(merged, GeminiPart{Text: textBuilder.String()})
			textBuilder.Reset()
		}
	}

	for _, part := range parts {
		switch {
		case part.FunctionCall != nil:
			flush()
			merged = append(merged, part)
		case part.Text != "" && part.Thought:
			thoughtBuilder.WriteString(part.Text)
		case part.Text != "":
			textBuilder.WriteString(part.Text)
		}
	}
	flush()
	return merged
}

func geminiCandidateText(candidate GeminiCandidate) (content string, thinking string) {
	var contentBuilder strings.Builder
	var thinkingBuilder strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Text == "" {
			continue
		}
		if part.Thought {
			thinkingBuilder.WriteString(part.Text)
			continue
		}
		contentBuilder.WriteString(part.Text)
	}
	return contentBuilder.String(), thinkingBuilder.String()
}

func geminiFunctionCalls(candidate GeminiCandidate) []GeminiFunctionCall {
	var calls []GeminiFunctionCall
	for _, part := range candidate.Content.Parts {
		if part.FunctionCall != nil {
			calls = append(calls, *part.FunctionCall)
		}
	}
	return calls
}

func checkGeminiResponse(ctx context.Context, response GeminiChatResponse) error {
	if response.PromptFeedback != nil && response.PromptFeedback.BlockReason != "" {
		err := fmt.Errorf("gemini blocked the prompt: %s", response.PromptFeedback.BlockReason)
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if len(response.Candidates) == 0 {
		err := errors.New("no candidates in gemini response")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	if response.Candidates[0].FinishReason == "MAX_TOKENS" {
		logs.WithContext(ctx).Error("gemini response truncated: finishReason=MAX_TOKENS. Increase model.max_tokens.")
	}
	if response.Candidates[0].FinishReason == "SAFETY" {
		err := errors.New("gemini stopped the response for safety reasons")
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (geminiModel *GeminiModel) QueryModelStreaming(ctx context.Context, chatRequest ChatRequest, callback func(chunk string)) (Message, error) {
	logs.WithContext(ctx).Debug("QueryModelStreaming - Start")

	systemContent := geminiSystemContent(chatRequest)
	request := geminiModel.buildGeminiRequest(ctx, chatRequest, nil, systemContent, 0)

	response, err := geminiModel.streamGenerateContent(ctx, request, func(candidate GeminiCandidate) {
		content, _ := geminiCandidateText(candidate)
		if content != "" && callback != nil {
			callback(content)
		}
	})
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Message{}, err
	}
	if cerr := checkGeminiResponse(ctx, response); cerr != nil {
		return Message{}, cerr
	}

	content, _ := geminiCandidateText(response.Candidates[0])

	var usageAcc UsageAccumulator
	accumulateGeminiUsage(&usageAcc, response.UsageMetadata)
	return usageAcc.Attach(Message{Content: content, Role: "assistant"}), nil
}

func (geminiModel *GeminiModel) QueryModelWithReasoning(ctx context.Context, chatRequest ChatRequest, thinkingBudget int) (Message, string, error) {
	logs.WithContext(ctx).Debug("QueryModelWithReasoning - Start")

	systemContent := geminiSystemContent(chatRequest)
	request := geminiModel.buildGeminiRequest(ctx, chatRequest, nil, systemContent, thinkingBudget)

	response, err := geminiModel.generateContent(ctx, request)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return Message{}, "", err
	}
	if cerr := checkGeminiResponse(ctx, response); cerr != nil {
		return Message{}, "", cerr
	}

	content, thinking := geminiCandidateText(response.Candidates[0])

	var usageAcc UsageAccumulator
	accumulateGeminiUsage(&usageAcc, response.UsageMetadata)
	return usageAcc.Attach(Message{Content: content, Role: "assistant"}), thinking, nil
}

func geminiSystemContent(chatRequest ChatRequest) string {
	var builder strings.Builder
	for _, message := range chatRequest.Messages {
		if message.Role == "system" && message.Content != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(message.Content)
		}
	}
	return builder.String()
}

func (geminiModel *GeminiModel) RunToolLoop(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor) (Message, []StepTrace, error) {
	logs.WithContext(ctx).Debug("RunToolLoop - Start")
	return geminiModel.runToolLoop(ctx, chatRequest, toolsMap, agentPrompt, maxIterations, thinkingBudget, toolExecutor, nil, false)
}

func (geminiModel *GeminiModel) RunToolLoopStreaming(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor, streamCb StreamEventCallback) (Message, []StepTrace, error) {
	logs.WithContext(ctx).Debug("RunToolLoopStreaming - Start")
	return geminiModel.runToolLoop(ctx, chatRequest, toolsMap, agentPrompt, maxIterations, thinkingBudget, toolExecutor, streamCb, true)
}

func (geminiModel *GeminiModel) runToolLoop(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor, streamCb StreamEventCallback, streaming bool) (Message, []StepTrace, error) {
	spanName := "Gemini.RunToolLoop"
	if streaming {
		spanName = "Gemini.RunToolLoopStreaming"
	}
	ctx, span := otel.Tracer("eru-ai").Start(ctx, spanName,
		oteltrace.WithAttributes(attribute.String("model", geminiModel.LLMName), attribute.Int("max_iterations", maxIterations)),
	)
	defer span.End()

	geminiTools, toolPrompt := convertGeminiTools(ctx, toolsMap)
	systemContent := agentPrompt.String()
	if toolPrompt != "" {
		systemContent = strings.TrimSpace(fmt.Sprint(systemContent, "\n", toolPrompt))
	}
	if existing := geminiSystemContent(chatRequest); existing != "" {
		systemContent = strings.TrimSpace(fmt.Sprint(systemContent, "\n", existing))
	}

	request := geminiModel.buildGeminiRequest(ctx, chatRequest, geminiTools, systemContent, thinkingBudget)

	var traces []StepTrace
	var usageAcc UsageAccumulator

	for iteration := 1; iteration <= maxIterations; iteration++ {
		var response GeminiChatResponse
		var err error

		if streaming {
			currentIteration := iteration
			response, err = geminiModel.streamGenerateContent(ctx, request, func(candidate GeminiCandidate) {
				content, thinking := geminiCandidateText(candidate)
				if streamCb == nil {
					return
				}
				if thinking != "" {
					streamCb(ModelStreamEvent{Type: StreamThinking, Content: thinking, Iteration: currentIteration})
				}
				if content != "" {
					streamCb(ModelStreamEvent{Type: StreamTextDelta, Content: content, Iteration: currentIteration})
				}
			})
		} else {
			response, err = geminiModel.generateContent(ctx, request)
		}
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return Message{}, traces, err
		}
		if cerr := checkGeminiResponse(ctx, response); cerr != nil {
			return Message{}, traces, cerr
		}

		accumulateGeminiUsage(&usageAcc, response.UsageMetadata)

		candidate := response.Candidates[0]
		content, thinking := geminiCandidateText(candidate)
		functionCalls := geminiFunctionCalls(candidate)

		trace := StepTrace{
			Iteration: iteration,
			Thinking:  thinking,
			Timestamp: time.Now(),
		}

		request.Contents = append(request.Contents, GeminiContent{
			Role:  "model",
			Parts: candidate.Content.Parts,
		})

		if len(functionCalls) == 0 {
			trace.Content = content
			traces = append(traces, trace)
			return usageAcc.Attach(Message{Content: content, Role: "assistant"}), traces, nil
		}

		for _, functionCall := range functionCalls {
			inputMap := functionCall.Args
			if inputMap == nil {
				inputMap = map[string]interface{}{}
			}

			if functionCall.Name == TerminalToolStructuredOutput {
				trace.ToolName = functionCall.Name
				trace.ToolInput = inputMap
				traces = append(traces, trace)
				if len(inputMap) == 0 {
					return Message{}, traces, fmt.Errorf("structured_output tool input was empty or truncated; raise model.max_tokens (current=%d)", geminiModel.resolveMaxOutputTokens())
				}
				resultBytes, _ := json.Marshal(inputMap)
				return usageAcc.Attach(Message{Content: string(resultBytes), Role: "assistant", TerminalTool: TerminalToolStructuredOutput}), traces, nil
			}

			if functionCall.Name == TerminalToolAskUser {
				trace.ToolName = functionCall.Name
				trace.ToolInput = inputMap
				traces = append(traces, trace)
				if len(inputMap) == 0 {
					return Message{}, traces, fmt.Errorf("ask_user tool input was empty or truncated; raise model.max_tokens (current=%d)", geminiModel.resolveMaxOutputTokens())
				}
				if streamCb != nil {
					streamCb(ModelStreamEvent{
						Type:      StreamQuestion,
						ToolName:  functionCall.Name,
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
					ToolName:  functionCall.Name,
					ToolInput: inputMap,
					Iteration: iteration,
				})
			}
			trace.ToolName = functionCall.Name
			trace.ToolInput = inputMap
		}

		var responseParts []GeminiPart
		for _, functionCall := range functionCalls {
			inputMap := functionCall.Args
			if inputMap == nil {
				inputMap = map[string]interface{}{}
			}

			execResult, execErr := toolExecutor(ctx, functionCall.Name, inputMap)
			functionResponse := &GeminiFunctionResponse{
				Id:   functionCall.Id,
				Name: functionCall.Name,
			}
			if execErr != nil {
				functionResponse.Response = map[string]interface{}{"error": execErr.Error()}
				trace.ToolResult = map[string]interface{}{"error": execErr.Error()}
				if streamCb != nil {
					streamCb(ModelStreamEvent{Type: StreamToolResult, Content: execErr.Error(), ToolName: functionCall.Name, Iteration: iteration})
				}
			} else {
				functionResponse.Response = execResult
				trace.ToolResult = execResult
				if streamCb != nil {
					resultBytes, _ := json.Marshal(execResult)
					streamCb(ModelStreamEvent{Type: StreamToolResult, Content: string(resultBytes), ToolName: functionCall.Name, Iteration: iteration})
				}
			}
			responseParts = append(responseParts, GeminiPart{FunctionResponse: functionResponse})
		}

		request.Contents = append(request.Contents, GeminiContent{Role: "user", Parts: responseParts})
		traces = append(traces, trace)
	}

	return usageAcc.Attach(Message{Content: "max iterations reached", Role: "assistant"}), traces, nil
}

func (geminiModel *GeminiModel) GenerateEmbeddings(ctx context.Context, inputs []EmbeddingInput, config chunking.ChunkingConfig, dimension int) (outputs []EmbeddingOutput, err error) {
	logs.WithContext(ctx).Debug(fmt.Sprintf("GenerateEmbeddings - Start with %d inputs", len(inputs)))

	if len(inputs) == 0 {
		return []EmbeddingOutput{}, nil
	}
	if config.Strategy == "" {
		config = chunking.DefaultChunkingConfig()
	}

	for _, input := range inputs {
		embedding, eerr := geminiModel.GenerateEmbedding(ctx, input.Text, config, dimension)
		if eerr != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("Failed to generate embedding for ID %s: %v", input.Id, eerr))
			return nil, eerr
		}
		outputs = append(outputs, EmbeddingOutput{
			Id:     input.Id,
			Text:   input.Text,
			Vector: embedding,
		})
	}
	return outputs, nil
}

func (geminiModel *GeminiModel) GenerateEmbedding(ctx context.Context, text string, config chunking.ChunkingConfig, dimension int) (embedding []float64, err error) {
	logs.WithContext(ctx).Debug("GenerateEmbedding - Start")

	factory := &chunking.ChunkingFactory{}
	chunker, err := factory.GetChunkingStrategy(config)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	chunkedTexts, err := chunker.ChunkText(text, config)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	if len(chunkedTexts) == 1 {
		return geminiModel.generateSingleEmbedding(ctx, chunkedTexts[0], dimension)
	}

	var allEmbeddings [][]float64
	for _, chunk := range chunkedTexts {
		chunkEmbedding, cerr := geminiModel.generateSingleEmbedding(ctx, chunk, dimension)
		if cerr != nil {
			return nil, cerr
		}
		allEmbeddings = append(allEmbeddings, chunkEmbedding)
	}
	return geminiModel.averageEmbeddings(allEmbeddings), nil
}

func (geminiModel *GeminiModel) generateSingleEmbedding(ctx context.Context, text string, dimension int) (embedding []float64, err error) {
	request := GeminiEmbeddingRequest{
		Model:   fmt.Sprint("models/", geminiModel.LLMName),
		Content: GeminiContent{Parts: []GeminiPart{{Text: text}}},
	}
	if dimension > 0 {
		request.OutputDimensionality = &dimension
	}

	url := fmt.Sprintf("%s/%s:embedContent", GeminiApiUrl, geminiModel.LLMName)

	response, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, url, geminiModel.authHeader(), nil, nil, nil, request)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	responseJson, err := json.Marshal(response)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	var embeddingResponse GeminiEmbeddingResponse
	if err = json.Unmarshal(responseJson, &embeddingResponse); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	if len(embeddingResponse.Embedding.Values) == 0 {
		err = errors.New("no embedding values returned by gemini")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	return embeddingResponse.Embedding.Values, nil
}
