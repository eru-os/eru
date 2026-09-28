package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	utils "github.com/eru-os/eru/eru-utils"
)

const (
	GeminiApiUrl = "https://generativelanguage.googleapis.com/v1beta/models"
)

type GeminiModel struct {
	Model
}

// Request Types
type GeminiChatRequest struct {
	Model             string                  `json:"model,omitempty"`
	Contents          []GeminiContent         `json:"contents" eru:"required"`
	Tools             []GeminiTool            `json:"tools,omitempty"`
	ToolConfig        *GeminiToolConfig       `json:"toolConfig,omitempty"`
	SafetySettings    []GeminiSafetySetting   `json:"safetySettings,omitempty"`
	SystemInstruction *GeminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts" eru:"required"`
}

type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *GeminiInlineData       `json:"inlineData,omitempty"`
	FileData         *GeminiFileData         `json:"fileData,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

type GeminiInlineData struct {
	MimeType string `json:"mimeType" eru:"required"`
	Data     string `json:"data" eru:"required"`
}

type GeminiFileData struct {
	MimeType string `json:"mimeType" eru:"required"`
	FileUri  string `json:"fileUri" eru:"required"`
}

type GeminiFunctionCall struct {
	Id   string                 `json:"id,omitempty"`
	Name string                 `json:"name" eru:"required"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type GeminiFunctionResponse struct {
	Id       string                 `json:"id,omitempty"`
	Name     string                 `json:"name" eru:"required"`
	Response map[string]interface{} `json:"response" eru:"required"`
}

type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type GeminiFunctionDeclaration struct {
	Name        string                `json:"name" eru:"required"`
	Description string                `json:"description" eru:"required"`
	Parameters  eru_models.JSONSchema `json:"parameters,omitempty"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // ANY, AUTO, NONE
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type GeminiSafetySetting struct {
	Category  string `json:"category" eru:"required"`
	Threshold string `json:"threshold" eru:"required"`
}

type GeminiGenerationConfig struct {
	StopSequences   []string `json:"stopSequences,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	TopK            *int     `json:"topK,omitempty"`

	ThinkingConfig *GeminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type GeminiThinkingConfig struct {
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

// Response Types
type GeminiChatResponse struct {
	Candidates     []GeminiCandidate     `json:"candidates"`
	PromptFeedback *GeminiPromptFeedback `json:"promptFeedback,omitempty"`
	UsageMetadata  *GeminiUsageMetadata  `json:"usageMetadata,omitempty"`
}

type GeminiCandidate struct {
	Content       GeminiContent        `json:"content"`
	FinishReason  string               `json:"finishReason,omitempty"`
	Index         int                  `json:"index,omitempty"`
	SafetyRatings []GeminiSafetyRating `json:"safetyRatings,omitempty"`
}

type GeminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type GeminiPromptFeedback struct {
	BlockReason   string               `json:"blockReason,omitempty"`
	SafetyRatings []GeminiSafetyRating `json:"safetyRatings,omitempty"`
}

type GeminiUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount    int `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount         int `json:"totalTokenCount,omitempty"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
}

// Implementation of ModelI interface
func (geminiModel *GeminiModel) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &geminiModel)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (geminiModel *GeminiModel) QueryModel(ctx context.Context, chatRequest ChatRequest) (queryResponse Message, err error) {
	logs.WithContext(ctx).Debug("QueryModel - Start")

	geminiRequest, err := geminiModel.makeGeminiChatRequest(ctx, chatRequest)
	if err != nil {
		return
	}

	geminiResponse, err := geminiModel.queryModel(ctx, geminiRequest)
	if err != nil {
		return
	}

	// Combine all text parts from the first candidate
	var combinedContent string
	if len(geminiResponse.Candidates) > 0 {
		for _, part := range geminiResponse.Candidates[0].Content.Parts {
			if part.Text != "" {
				combinedContent += part.Text
			}
		}
	}

	queryResponse = Message{
		Content: combinedContent,
		Role:    "assistant",
	}
	return
}

func (geminiModel *GeminiModel) makeGeminiChatRequest(ctx context.Context, chatRequest ChatRequest) (geminiRequest GeminiChatRequest, err error) {
	logs.WithContext(ctx).Debug("makeGeminiChatRequest - Start")

	maxOutputTokens := geminiModel.resolveMaxOutputTokens()
	geminiRequest = GeminiChatRequest{
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     &geminiModel.Temprature,
			MaxOutputTokens: &maxOutputTokens,
		},
	}

	var systemParts []GeminiPart
	for _, message := range chatRequest.Messages {
		if message.Role == "system" {
			systemParts = append(systemParts, geminiModel.makeGeminiParts(ctx, message)...)
			continue
		}
		content := GeminiContent{
			Role:  geminiModel.mapRoleToGemini(message.Role),
			Parts: geminiModel.makeGeminiParts(ctx, message),
		}
		geminiRequest.Contents = append(geminiRequest.Contents, content)
	}
	if len(systemParts) > 0 {
		geminiRequest.SystemInstruction = &GeminiContent{Parts: systemParts}
	}

	return
}

func (geminiModel *GeminiModel) mapRoleToGemini(role string) string {
	switch role {
	case "assistant":
		return "model"
	case "user":
		return "user"
	case "system":
		return "user" // Gemini doesn't have a system role, map to user
	default:
		return "user"
	}
}

func (geminiModel *GeminiModel) makeGeminiParts(ctx context.Context, message Message) []GeminiPart {
	var parts []GeminiPart

	if message.Content != "" {
		parts = append(parts, GeminiPart{
			Text: message.Content,
		})
	}

	if len(message.Files) > 0 {
		for _, file := range message.Files {
			if file.FileData != "" {
				parts = append(parts, GeminiPart{
					InlineData: &GeminiInlineData{
						MimeType: file.FileType,
						Data:     file.FileData,
					},
				})
			} else if file.FileId != "" {
				parts = append(parts, GeminiPart{
					FileData: &GeminiFileData{
						MimeType: file.FileType,
						FileUri:  file.FileId, // Assuming FileId contains the URI
					},
				})
			} else if file.ImageData != "" {
				parts = append(parts, GeminiPart{
					InlineData: &GeminiInlineData{
						MimeType: file.FileType,
						Data:     file.ImageData,
					},
				})
			}
		}
	}

	return parts
}

func (geminiModel *GeminiModel) queryModel(ctx context.Context, chatRequest GeminiChatRequest) (geminiResponse GeminiChatResponse, err error) {
	logs.WithContext(ctx).Debug("queryModel - Start")

	url := fmt.Sprintf("%s/%s:generateContent", GeminiApiUrl, geminiModel.LLMName)

	reqHeader := http.Header{}
	reqHeader.Add("Content-Type", "application/json")

	queryParams := map[string]string{
		"key": geminiModel.LLMSecret,
	}

	logs.WithContext(ctx).Info(fmt.Sprint(chatRequest))

	response, _, _, _, err := utils.CallHttp(ctx, "POST", url, reqHeader, nil, nil, queryParams, chatRequest)
	if err != nil {
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

	logs.WithContext(ctx).Info(fmt.Sprint(geminiResponse))
	return
}

func (geminiModel *GeminiModel) QueryModelWithTool(ctx context.Context, chatRequest ChatRequest, tools map[string]tools.Tooling, agentName string, agentPrompt string) (queryResponse JsonMessage, err error) {
	logs.WithContext(ctx).Debug("QueryModelWithTool - Start")

	geminiChatToolRequest, err := geminiModel.makeGeminiChatToolRequest(ctx, chatRequest, tools, agentName, agentPrompt)
	if err != nil {
		return
	}

	geminiChatResponse, err := geminiModel.queryModelTool(ctx, geminiChatToolRequest)
	if err != nil {
		return
	}

	outputJson := make(map[string]interface{})

	// Extract function call from response
	if len(geminiChatResponse.Candidates) > 0 {
		for _, part := range geminiChatResponse.Candidates[0].Content.Parts {
			if part.FunctionCall != nil {
				outputJson = part.FunctionCall.Args
				break
			}
		}
	}

	if len(outputJson) == 0 {
		outputJson["raw"] = geminiChatResponse
	}

	queryResponse = JsonMessage{
		Content: outputJson,
		Role:    "assistant",
	}
	return
}

func (geminiModel *GeminiModel) makeGeminiChatToolRequest(ctx context.Context, chatRequest ChatRequest, tools map[string]tools.Tooling, agentName string, agentPrompt string) (geminiRequest GeminiChatRequest, err error) {
	logs.WithContext(ctx).Debug("makeGeminiChatToolRequest - Start")

	geminiTools, toolPrompt := convertGeminiTools(ctx, tools)

	maxOutputTokens := geminiModel.resolveMaxOutputTokens()
	geminiRequest = GeminiChatRequest{
		Tools: geminiTools,
		ToolConfig: &GeminiToolConfig{
			FunctionCallingConfig: &GeminiFunctionCallingConfig{
				Mode: "ANY",
			},
		},
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     &geminiModel.Temprature,
			MaxOutputTokens: &maxOutputTokens,
		},
	}

	systemContent := strings.TrimSpace(fmt.Sprint(agentPrompt, "\n", toolPrompt))
	geminiRequest.SystemInstruction = &GeminiContent{
		Parts: []GeminiPart{{Text: systemContent}},
	}

	for _, message := range chatRequest.Messages {
		content := GeminiContent{
			Role:  geminiModel.mapRoleToGemini(message.Role),
			Parts: geminiModel.makeGeminiParts(ctx, message),
		}
		geminiRequest.Contents = append(geminiRequest.Contents, content)
	}

	return
}

func (geminiModel *GeminiModel) queryModelTool(ctx context.Context, toolRequest GeminiChatRequest) (geminiResponse GeminiChatResponse, err error) {
	logs.WithContext(ctx).Debug("queryModelTool - Start")

	url := fmt.Sprintf("%s/%s:generateContent", GeminiApiUrl, geminiModel.LLMName)

	reqHeader := http.Header{}
	reqHeader.Add("Content-Type", "application/json")

	queryParams := map[string]string{
		"key": geminiModel.LLMSecret,
	}

	logs.WithContext(ctx).Info(fmt.Sprint(toolRequest))

	response, _, _, _, err := utils.CallHttp(ctx, "POST", url, reqHeader, nil, nil, queryParams, toolRequest)
	if err != nil {
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

	logs.WithContext(ctx).Info(fmt.Sprint(geminiResponse))
	return
}
