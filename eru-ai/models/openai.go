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

const (
	OpenAIApiUrl          = "https://api.openai.com/v1/chat/completions"
	OpenAIResponseApiUrl  = "https://api.openai.com/v1/responses"
	OpenAIEmbeddingApiUrl = "https://api.openai.com/v1/embeddings"
)

type OpenAIModel struct {
	Model
	ApiType         string `json:"api_type"`
	ServiceTier     string `json:"service_tier"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type OpenAIRequestMessage struct {
	Content    []OpenAIRequestMessageContent `json:"content" `
	Role       string                        `json:"role" `
	Name       string                        `json:"name"`
	ToolCallId string                        `json:"tool_call_id"`
	Refusal    string                        `json:"refusal"`
}

type OpenAIRequestMessageContent interface {
	GetType() string
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (t TextContent) GetType() string {
	return "text"
}

type FileContent struct {
	Type string                          `json:"type"`
	File OpenAIRequestMessageContentFile `json:"file"`
}

func (f FileContent) GetType() string {
	return "file"
}

type ImageContent struct {
	Type     string                           `json:"type"`
	ImageUrl OpenAIRequestMessageContentImage `json:"image_url"`
}

func (i ImageContent) GetType() string {
	return "image_url"
}

type OpenAIRequestMessageContentFile struct {
	FileId   string `json:"file_id,omitempty"`
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data,omitempty"`
}

type OpenAIRequestMessageContentImage struct {
	Url string `json:"url,omitempty"`
}

type OpenAIAudioRequestMessage struct {
	Content    string `json:"content" `
	Role       string `json:"role" `
	Name       string `json:"name"`
	ToolCallId string `json:"tool_call_id"`
	Refusal    string `json:"refusal"`
	Audio      struct {
		Id string `json:"id"`
	} `json:"audio"`
}

type OpenAIToolChoice struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ToolChoiceFunction struct {
	Name string `json:"name" eru:"required"`
}

type ToolFunction struct {
	Name        string                `json:"name" eru:"required"`
	Description string                `json:"description" eru:"required"`
	Parameters  eru_models.JSONSchema `json:"parameters" eru:"required"`
	Strict      bool                  `json:"strict"`
}

type OpenAIRequestToolChoice struct {
	Type     string             `json:"type" eru:"required"`
	Function ToolChoiceFunction `json:"function" eru:"required"`
}
type OpenAIRequestTools struct {
	Type     string       `json:"type" eru:"required"`
	Function ToolFunction `json:"function" eru:"required"`
}
type OpenAIChatRequest struct {
	MaxCompletionTokens int                    `json:"max_completion_tokens"`
	Messages            []OpenAIRequestMessage `json:"messages" eru:"required"`
	Model               string                 `json:"model" eru:"required"`
	Store               bool                   `json:"store"`
	ReasoningEffort     string                 `json:"reasoning_effort,omitempty"`
	Metadata            map[string]interface{} `json:"metadata"`
	FrequencyPenalty    float64                `json:"frequency_penalty"`
	LogitBias           map[string]interface{} `json:"logit_bias"`

	N          int      `json:"n"`
	Modalities []string `json:"modalities,omitempty"`

	PresencePenalty float64 `json:"presence_penalty"`

	Seed        int     `json:"seed"`
	ServiceTier string  `json:"service_tier"`
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
	User        string  `json:"user"`
}

type OpenAIChatToolRequest struct {
	OpenAIChatRequest
	//ToolChoice        OpenAIRequestToolChoice `json:"tool_choice"`
	ToolChoice        string               `json:"tool_choice"`
	Tools             []OpenAIRequestTools `json:"tools"`
	ParallelToolCalls bool                 `json:"parallel_tool_calls"`
}

type OpenAIToolLoopMessage struct {
	Role       string                   `json:"role"`
	Content    interface{}              `json:"content,omitempty"`
	ToolCalls  []OpenAIResponseToolCall `json:"tool_calls,omitempty"`
	ToolCallId string                   `json:"tool_call_id,omitempty"`
}

type OpenAIToolLoopRequest struct {
	Model               string                  `json:"model"`
	Messages            []OpenAIToolLoopMessage `json:"messages"`
	MaxCompletionTokens int64                   `json:"max_completion_tokens,omitempty"`
	Temperature         *float64                `json:"temperature,omitempty"`
	ReasoningEffort     string                  `json:"reasoning_effort,omitempty"`
	ServiceTier         string                  `json:"service_tier,omitempty"`
	Tools               []OpenAIRequestTools    `json:"tools,omitempty"`
	ToolChoice          string                  `json:"tool_choice,omitempty"`
}

type OpenAIChatAudioRequest struct {
	OpenAIAudioRequestMessage
	Audio struct {
		Voice  string `json:"voice"`
		Format string `json:"format"`
	} `json:"audio"`
}

type OpenAIChatStreamRequest struct {
	OpenAIChatRequest
	Stream        bool `json:"stream"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

// mostly not to be used as  we will using tools
//ResponseFormat  OpenAIResponseFormat `json:"response_format"`

// To decide its usage
//Logprobs         bool                   `json:"logprobs"`
//	TopLogprobs      int                    `json:"top_logprobs"`

type OpenAIRequestPrediction struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type OpenAIResponseFormat struct {
	Type       string            `json:"type" eru:"required"`
	JsonSchema map[string]string `json:"json_schema"`
}
type JsonSchema struct {
	Description string            `json:"description" `
	Name        string            `json:"name" eru:"required"`
	Schema      map[string]string `json:"schema" `
	Strict      bool              `json:"strict"`
}

// OpenAI Request Response //
// @docs https://platform.openai.com/docs/api-reference/chat/create

// OpenAI Response //
type OpenAIResponseFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIResponseToolCall struct {
	Id       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function OpenAIResponseFunction `json:"function"`
}

type OpenAIResponseMessage struct {
	Content   string                     `json:"content" eru:"required"`
	Refusal   string                     `json:"refusal"`
	Role      string                     `json:"role" eru:"required"`
	ToolCalls []OpenAIResponseToolCall   `json:"tool_calls"`
	Audio     OpenAIResponseMessageAudio `json:"audio"`
}

type OpenAIResponseMessageAudio struct {
	Id         string `json:"id"`
	Transcript string `json:"transcript"`
	Data       string `json:"data"`
	ExpiresAt  int    `json:"expires_at"`
}

type OpenAIChatResponse struct {
	Id                string                     `json:"id"`
	Object            string                     `json:"object"`
	Created           int                        `json:"created"`
	Model             string                     `json:"model"`
	Choices           []OpenAIChatResponseChoice `json:"choices"`
	ServiceTier       string                     `json:"service_tier"`
	SystemFingerprint string                     `json:"system_fingerprint"`
	Usage             OpenAIChatUsage            `json:"usage"`
}
type OpenAIChatResponseChoice struct {
	FinishReason string                `json:"finish_reason" enum:"stop,length,tool_calls,content_filter,function_call"`
	Index        int                   `json:"index"`
	Message      OpenAIResponseMessage `json:"message"`
	Logprobs     OpenAILogprobs        `json:"logprobs"`
}

type OpenAILogprobs struct {
	Content OpenAILogprobsContent `json:"content"`
	Refusal OpenAILogprobsContent `json:"refusal"`
}
type OpenAILogprobsContentObj struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
	Bytes   []byte  `json:"bytes"`
}
type OpenAILogprobsContent struct {
	OpenAILogprobsContentObj
	TopLogprobs []OpenAILogprobsContentObj `json:"top_logprobs"`
}
type OpenAIChatUsage struct {
	InputTokens        int64 `json:"input_tokens,omitempty"`
	InputTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details,omitempty"`
	OutputTokens        int64 `json:"output_tokens,omitempty"`
	OutputTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details,omitempty"`
	PromptTokens            int64                         `json:"prompt_tokens,omitempty"`
	CompletionTokens        int64                         `json:"completion_tokens,omitempty"`
	TotalTokens             int64                         `json:"total_tokens,omitempty"`
	PromptTokensDetails     OpenAIPromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails OpenAICompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}
type OpenAICompletionTokensDetails struct {
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
	AudioTokens              int `json:"audio_tokens"`
	ReasoningTokens          int `json:"reasoning_tokens"`
}
type OpenAIPromptTokensDetails struct {
	AudioTokens  int `json:"audio_tokens"`
	CachedTokens int `json:"cached_tokens"`
}

// OpenAI Embedding Request
type OpenAIEmbeddingRequest struct {
	Input          interface{} `json:"input" eru:"required"`
	Model          string      `json:"model" eru:"required"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
	User           string      `json:"user,omitempty"`
}

// OpenAI Embedding Response
type OpenAIEmbeddingResponse struct {
	Object string                `json:"object"`
	Data   []OpenAIEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Usage  OpenAIEmbeddingUsage  `json:"usage"`
}

type OpenAIEmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// OpenAI Embedding Usage struct
type OpenAIEmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

const (
	OpenAIResponsesIncludeWebSearchCallActionSources = "web_search_call.action.sources"
	OpenAIResponsesIncludeCodeInterpreterCallOutputs = "code_interpreter_call.outputs"
	OpenAIResponsesIncludeComputerCallOutputImageUrl = "computer_call_output.output.image_url"
	OpenAIResponsesIncludeFileSearchCallResults      = "file_search_call.results"
	OpenAIResponsesIncludeMessageInputImageImageUrl  = "message.input_image.image_url"
	OpenAIResponsesIncludeMessageOutputTextLogprobs  = "message.output_text.logprobs"
	OpenAIResponsesIncludeReasoningEncryptedContent  = "reasoning.encrypted_content"
)

// OpenAI Responses API Request/Response //
type OpenAIResponsesRequest struct {
	Background           bool                                      `json:"background,omitempty"`
	ContextManagement    []OpenAIResponsesRequestContextManagement `json:"context_management,omitempty"`
	Conversation         string                                    `json:"conversation,omitempty"`
	Include              []string                                  `json:"include,omitempty"`
	Input                []OpenAIResponsesInput                    `json:"input"`
	Instructions         string                                    `json:"instructions,omitempty"`
	MaxOutputTokens      int64                                     `json:"max_output_tokens,omitempty"`
	MaxToolCalls         int64                                     `json:"max_tool_calls,omitempty"`
	Metadata             map[string]string                         `json:"metadata,omitempty"`
	Model                string                                    `json:"model"`
	ParallelToolCalls    bool                                      `json:"parallel_tool_calls,omitempty"`
	PreviousResponseId   string                                    `json:"previous_response_id,omitempty"`
	Prompt               *OpenAIResponsesRequestPrompt             `json:"prompt,omitempty"`
	PromptCacheKey       string                                    `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string                                    `json:"prompt_cache_retention,omitempty"`
	Reasoning            *OpenAIResponsesRequestReasoning          `json:"reasoning,omitempty"`
	SafetyIdentifier     string                                    `json:"safety_identifier,omitempty"`
	ServiceTier          string                                    `json:"service_tier,omitempty"` //flex, auto, default, auto
	Store                bool                                      `json:"store,omitempty"`
	Stream               bool                                      `json:"stream,omitempty"`
	StreamOptions        *struct {
		IncludeObfuscation bool `json:"include_obfuscation,omitempty"`
	} `json:"stream_options,omitempty"`
	Temperature int64                       `json:"temperature,omitempty"`
	Text        *OpenAIResponsesRequestText `json:"text,omitempty"`
	Verbosity   string                      `json:"verbosity,omitempty"`   //low, medium, and high
	ToolChoice  string                      `json:"tool_choice,omitempty"` //none, auto, or required
	Tools       []interface{}               `json:"tools,omitempty"`
	TopLogprobs int64                       `json:"top_logprobs,omitempty"`
	TopP        int64                       `json:"top_p,omitempty"`
	Truncation  string                      `json:"truncation,omitempty"` //auto ,disabled
}

type OpenAIResponsesRequestText struct {
	Format OpenAIResponsesRequestTextFormat `json:"format,omitempty"`
}

type OpenAIResponsesRequestTextFormat struct {
	Type        string                `json:"type" eru:"required"` //json_schema, text, json_object
	Name        string                `json:"name,omitempty"`
	Schema      eru_models.JSONSchema `json:"schema,omitempty"`
	Description string                `json:"description,omitempty"`
	Strict      bool                  `json:"strict,omitempty"`
}

type OpenAIResponsesRequestReasoning struct {
	Effort  string `json:"effort,omitempty"`  //none, minimal, low, medium, high, and xhigh (differs from model to model)
	Summary string `json:"summary,omitempty"` //auto, concise, or detailed.
}

type OpenAIResponsesRequestPrompt struct {
	Id        string                 `json:"id" eru:"required"`
	Version   string                 `json:"version,omitempty"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}
type OpenAIResponsesRequestContextManagement struct {
	Type             string `json:"type" eru:"required"` //compaction
	CompactThreshold int64  `json:"compact_threshold,omitempty"`
}

type OpenAIResponsesInput struct {
	Type    string                   `json:"type,omitempty"`      //always "message"
	Role    string                   `json:"role" eru:"required"` //user, assistant, system, or developer.
	Content []OpenAIResponsesContent `json:"content" eru:"required"`
}

type OpenAIResponsesContent struct {
	Type     string `json:"type" eru:"required"` //input_text, input_image, tool_call, tool_output, or reasoning
	Text     string `json:"text,omitempty"`
	Details  string `json:"details,omitempty"` //high, low, or auto. Defaults to auto
	FileId   string `json:"file_id,omitempty"`
	ImageUrl string `json:"image_url,omitempty"`
	FileData string `json:"file_data,omitempty"`
	FileUrl  string `json:"file_url,omitempty"`
	FileName string `json:"file_name,omitempty"`
}

type OpenAIResponsesResponse struct {
	Background   bool    `json:"background,omitempty"`
	CompletedAt  float64 `json:"completed_at,omitempty"`
	Conversation struct {
		Id string `json:"id,omitempty"`
	} `json:"conversation,omitempty"`
	CreatedAt int64 `json:"created_at,omitempty"`
	Error     struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	Id                string `json:"id"`
	IncompleteDetails struct {
		Reason string `json:"reason,omitempty"`
	} `json:"incomplete_details,omitempty"`
	Instructions         interface{}                     `json:"instructions,omitempty"`
	MaxOutputTokens      int64                           `json:"max_output_tokens,omitempty"`
	MaxToolCalls         int64                           `json:"max_tool_calls,omitempty"`
	Metadata             map[string]string               `json:"metadata,omitempty"`
	Model                string                          `json:"model,omitempty"`
	Object               string                          `json:"object,omitempty"` //response
	Status               string                          `json:"status"`
	Output               []OpenAIResponsesOutput         `json:"output"`
	ParallelToolCalls    bool                            `json:"parallel_tool_calls,omitempty"`
	PreviousResponseId   string                          `json:"previous_response_id,omitempty"`
	Prompt               OpenAIResponsesRequestPrompt    `json:"prompt,omitempty"`
	PromptCacheKey       string                          `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string                          `json:"prompt_cache_retention,omitempty"`
	Reasoning            OpenAIResponsesRequestReasoning `json:"reasoning,omitempty"`
	SafetyIdentifier     string                          `json:"safety_identifier,omitempty"`
	ServiceTier          string                          `json:"service_tier,omitempty"`
	Temperature          int64                           `json:"temperature,omitempty"`
	Text                 OpenAIResponsesRequestText      `json:"text,omitempty"`
	ToolChoice           string                          `json:"tool_choice,omitempty"`
	Tools                []interface{}                   `json:"tools,omitempty"`
	TopLogprobs          int64                           `json:"top_logprobs,omitempty"`
	TopP                 int64                           `json:"top_p,omitempty"`
	Truncation           string                          `json:"truncation,omitempty"`
	Usage                OpenAIChatUsage                 `json:"usage"`
}

type OpenAIResponsesOutput struct {
	Id      string                           `json:"id,omitempty"`
	Role    string                           `json:"role,omitempty"`
	Type    string                           `json:"type,omitempty"`
	Status  string                           `json:"status,omitempty"`
	Content []OpenAIResponsesResponseContent `json:"content,omitempty"`
	Queries []string                         `json:"queries,omitempty"`
	Results []struct {
		Attributes map[string]string `json:"attributes,omitempty"`
		FileId     string            `json:"file_id,omitempty"`
		Filename   string            `json:"filename,omitempty"`
		Score      int64             `json:"score,omitempty"`
		Text       string            `json:"text,omitempty"`
	} `json:"results,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallId    string `json:"call_id,omitempty"`
	Summary   []struct {
		Text string `json:"text,omitempty"`
		Type string `json:"type,omitempty"` //summary_text
	} `json:"summary,omitempty"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

type OpenAIResponsesResponseContent struct {
	Annotations []struct {
		FileId      string `json:"file_id,omitempty"`
		Filename    string `json:"filename,omitempty"`
		Index       int64  `json:"index,omitempty"`
		Type        string `json:"type,omitempty"` //file_citation, url_citation, container_file_citation, file_path
		EndIndex    int64  `json:"end_index,omitempty"`
		StartIndex  int64  `json:"start_index,omitempty"`
		Title       string `json:"title,omitempty"`
		Url         string `json:"url,omitempty"`
		ContainerId string `json:"container_id,omitempty"`
	} `json:"annotations,omitempty"`
	Logprobs []interface{} `json:"logprobs,omitempty"`
	Text     string        `json:"text,omitempty"`
	Type     string        `json:"type,omitempty"` //refusal, output_text
	Refusal  string        `json:"refusal,omitempty"`
}

func (openaiModel *OpenAIModel) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &openaiModel)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
func (openaiModel *OpenAIModel) isReasoningModel() bool {
	return strings.HasPrefix(openaiModel.LLMName, "o1") ||
		strings.HasPrefix(openaiModel.LLMName, "o3") ||
		strings.HasPrefix(openaiModel.LLMName, "o4")
}

func (openaiModel *OpenAIModel) QueryModel(ctx context.Context, chatRequest ChatRequest) (queryResponse Message, err error) {
	logs.WithContext(ctx).Debug("QueryModel - Start")
	if openaiModel.ApiType == "RESPONSES" {
		return openaiModel.queryModelResponses(ctx, chatRequest)
	}
	openAIChatRequest, err := openaiModel.makeOpenAIChatRequest(ctx, chatRequest)
	//_, err = openaiModel.makeOpenAIChatRequest(ctx, chatRequest)

	if err != nil {
		return
	}
	openAIChatResponse, err := openaiModel.queryModel(ctx, openAIChatRequest)
	if err != nil {
		return
	}
	queryResponse = Message{
		Content: openAIChatResponse.Choices[0].Message.Content,
		Role:    "assistant",
	}
	return
}
func (openaiModel *OpenAIModel) queryModelResponses(ctx context.Context, chatRequest ChatRequest) (queryResponse Message, err error) {
	logs.WithContext(ctx).Debug("queryModelResponses - Start")
	openAIResponsesRequest := OpenAIResponsesRequest{
		Model: openaiModel.LLMName,
	}

	for _, msg := range chatRequest.Messages {
		input := OpenAIResponsesInput{
			Type: "message",
			Role: msg.Role,
			Content: []OpenAIResponsesContent{
				{
					Type: "text",
					Text: msg.Content,
				},
			},
		}
		openAIResponsesRequest.Input = append(openAIResponsesRequest.Input, input)
	}

	reqHeader := http.Header{}
	reqHeader.Add("Authorization", "Bearer "+openaiModel.LLMSecret)
	reqHeader.Add("Content-Type", "application/json")

	response, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, OpenAIResponseApiUrl, reqHeader, nil, nil, nil, openAIResponsesRequest)
	if err != nil {
		return
	}

	responseJson, err := json.Marshal(response)
	if err != nil {
		return
	}

	var openAIResponsesResponse OpenAIResponsesResponse
	err = json.Unmarshal(responseJson, &openAIResponsesResponse)
	if err != nil {
		return
	}

	var combinedContent string
	for _, out := range openAIResponsesResponse.Output {
		if out.Type == "message" {
			for _, content := range out.Content {
				combinedContent += content.Text
			}
		}
	}

	queryResponse = Message{
		Content: combinedContent,
		Role:    "assistant",
	}
	return
}
func (openaiModel *OpenAIModel) makeOpenAIChatRequestContent(ctx context.Context, message Message) (openAIRequestMessageContent []OpenAIRequestMessageContent) {
	logs.WithContext(ctx).Debug("makeOpenAIChatRequestContent - Start")
	openAIRequestMessageContent = append(openAIRequestMessageContent, TextContent{
		Type: "text",
		Text: message.Content,
	})
	if len(message.Files) > 0 {
		for _, file := range message.Files {
			if file.FileData != "" {
				openAIRequestMessageContent = append(openAIRequestMessageContent, FileContent{
					Type: "file",
					File: OpenAIRequestMessageContentFile{
						Filename: file.FileName,
						FileData: file.FileData,
					},
				})
			} else if file.FileId != "" {
				openAIRequestMessageContent = append(openAIRequestMessageContent, FileContent{
					Type: "file",
					File: OpenAIRequestMessageContentFile{
						FileId: file.FileId,
					},
				})
			} else if file.ImageData != "" {
				openAIRequestMessageContent = append(openAIRequestMessageContent, ImageContent{
					Type: "image_url",
					ImageUrl: OpenAIRequestMessageContentImage{
						Url: fmt.Sprintf("data:%s;base64,%s", file.FileType, file.ImageData),
					},
				})
			}
		}
	}

	return
}
func (openaiModel *OpenAIModel) makeOpenAIChatRequest(ctx context.Context, chatRequest ChatRequest) (openAIChatRequest OpenAIChatRequest, err error) {
	logs.WithContext(ctx).Debug("makeOpenAIChatRequest - Start")
	serviceTier := openaiModel.ServiceTier
	if serviceTier == "" {
		serviceTier = "auto"
	}
	openAIChatRequest = OpenAIChatRequest{
		Model:               openaiModel.LLMName,
		N:                   1,
		MaxCompletionTokens: 150,
		ServiceTier:         serviceTier,
	}
	if openaiModel.isReasoningModel() {
		effort := openaiModel.ReasoningEffort
		if effort == "" {
			effort = "medium"
		}
		openAIChatRequest.ReasoningEffort = effort
		openAIChatRequest.MaxCompletionTokens = 16000
	} else {
		openAIChatRequest.Temperature = openaiModel.Temprature
		openAIChatRequest.TopP = 1
	}

	for _, message := range chatRequest.Messages {
		openAIChatRequest.Messages = append(openAIChatRequest.Messages, OpenAIRequestMessage{
			Role:    message.Role,
			Content: openaiModel.makeOpenAIChatRequestContent(ctx, message),
			Name:    message.Name,
		})
	}
	return
}

func (openaiModel *OpenAIModel) makeOpenAIChatToolRequest(ctx context.Context, chatRequest ChatRequest, tools map[string]tools.Tooling, agentName string, agentPrompt string) (openAIChatToolRequest OpenAIChatToolRequest, err error) {
	logs.WithContext(ctx).Debug("makeOpenAIChatToolRequest - Start")

	var openAIRequestTools []OpenAIRequestTools
	toolPrompt := ""
	for toolKey, tool := range tools {
		toolDescriptionI, _ := tool.GetAttribute(ctx, "description")
		//toolParametersI, _ := tool.GetAttribute(ctx, "parameters")
		toolSystemPromptI, _ := tool.GetAttribute(ctx, "system_prompt")
		toolName := ToolNameForModel(ctx, toolKey, tool)
		toolDescription := toolDescriptionI.(string)
		//toolParameters := toolParametersI.(eru_models.JSONSchema)

		toolParameters := tool.GetParameters()

		toolPrompt += fmt.Sprint("Tool prompt for Tool ", toolName, " is as follows :\n", toolSystemPromptI.(string))
		reqTool := OpenAIRequestTools{
			Type: "function",
			Function: ToolFunction{
				Name:        toolName,
				Description: toolDescription,
				Parameters:  toolParameters,
			},
		}
		openAIRequestTools = append(openAIRequestTools, reqTool)
	}
	serviceTier := openaiModel.ServiceTier
	if serviceTier == "" {
		serviceTier = "auto"
	}
	openAIChatToolRequest = OpenAIChatToolRequest{
		OpenAIChatRequest: OpenAIChatRequest{
			Model:       openaiModel.LLMName,
			N:           1,
			Temperature: openaiModel.Temprature,
			TopP:        1,
			//Modalities:          []string{"text"},
			MaxCompletionTokens: 1024,
			ServiceTier:         serviceTier,
		},

		ToolChoice: "required",
		/* ToolChoice: OpenAIRequestToolChoice{
			Type: "function",
			Function: ToolChoiceFunction{
				Name: toolName,
			},
		}, */
		Tools: openAIRequestTools,
	}
	if agentName == "" {
		agentName = "Agent"
	}
	openAIChatToolRequest.Messages = append(openAIChatToolRequest.Messages, OpenAIRequestMessage{
		Role: "system",
		Content: openaiModel.makeOpenAIChatRequestContent(ctx, Message{
			Content: fmt.Sprint(toolPrompt, "\n", agentPrompt),
			//Content: toolPrompt,
		}),
		Name: agentName,
	})
	for _, message := range chatRequest.Messages {

		openAIChatToolRequest.Messages = append(openAIChatToolRequest.Messages, OpenAIRequestMessage{
			Role:    message.Role,
			Content: openaiModel.makeOpenAIChatRequestContent(ctx, message),
			Name:    message.Name,
		})
	}
	return
}

func (openaiModel *OpenAIModel) queryModel(ctx context.Context, chatRequest OpenAIChatRequest) (openAIChatResponse OpenAIChatResponse, err error) {
	logs.WithContext(ctx).Debug("QueryModel - Start")
	reqHeader := http.Header{}
	reqHeader.Add("Authorization", "Bearer "+openaiModel.LLMSecret)
	reqHeader.Add("Content-Type", "application/json")

	logs.WithContext(ctx).Info(fmt.Sprint(chatRequest))
	//response, respHeaders, respCookies, statusCode, err := utils.CallHttp(ctx, "POST", OpenAIApiUrl, reqHeader, nil, nil, nil, postBody)
	response, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, OpenAIApiUrl, reqHeader, nil, nil, nil, chatRequest)

	if err != nil {
		return
	}
	// logs.WithContext(ctx).Info(fmt.Sprint(response))
	// logs.WithContext(ctx).Info(fmt.Sprint(respHeaders))
	// logs.WithContext(ctx).Info(fmt.Sprint(respCookies))
	// logs.WithContext(ctx).Info(fmt.Sprint(statusCode))

	responseJson, err := json.Marshal(response)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	err = json.Unmarshal(responseJson, &openAIChatResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint(openAIChatResponse.Choices[0].Message))
	return
}

func (openaiModel *OpenAIModel) queryModelTool(ctx context.Context, chatToolRequest OpenAIChatToolRequest) (openAIChatResponse OpenAIChatResponse, err error) {
	logs.WithContext(ctx).Debug("QueryModelTool - Start")
	reqHeader := http.Header{}
	reqHeader.Add("Authorization", "Bearer "+openaiModel.LLMSecret)
	reqHeader.Add("Content-Type", "application/json")

	//logs.WithContext(ctx).Info(fmt.Sprint(chatToolRequest))
	//response, respHeaders, respCookies, statusCode, err := utils.CallHttp(ctx, "POST", OpenAIApiUrl, reqHeader, nil, nil, nil, postBody)
	response, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, OpenAIApiUrl, reqHeader, nil, nil, nil, chatToolRequest)

	if err != nil {
		return
	}
	// logs.WithContext(ctx).Info(fmt.Sprint(response))
	// logs.WithContext(ctx).Info(fmt.Sprint(respHeaders))
	// logs.WithContext(ctx).Info(fmt.Sprint(respCookies))
	// logs.WithContext(ctx).Info(fmt.Sprint(statusCode))

	responseJson, err := json.Marshal(response)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	err = json.Unmarshal(responseJson, &openAIChatResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint(openAIChatResponse.Choices[0].Message))
	return
}

func (openaiModel *OpenAIModel) QueryModelWithTool(ctx context.Context, chatRequest ChatRequest, tools map[string]tools.Tooling, agentName string, agentPrompt string) (queryResponse JsonMessage, err error) {
	logs.WithContext(ctx).Debug("QueryModelWithTool - Start")
	if openaiModel.ApiType == "RESPONSES" {
		return openaiModel.queryModelResponsesWithTool(ctx, chatRequest, tools, agentName, agentPrompt)
	}
	openAIChatToolRequest, err := openaiModel.makeOpenAIChatToolRequest(ctx, chatRequest, tools, agentName, agentPrompt)
	if err != nil {
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint(openAIChatToolRequest))
	openAIChatResponse, err := openaiModel.queryModelTool(ctx, openAIChatToolRequest)
	if err != nil {
		return
	}
	outputJson := make(map[string]interface{})
	err = json.Unmarshal([]byte(openAIChatResponse.Choices[0].Message.ToolCalls[0].Function.Arguments), &outputJson)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		outputJson["raw"] = openAIChatResponse.Choices[0].Message.ToolCalls[0].Function.Arguments
	}
	//outputJson["raw"] = openAIChatResponse
	queryResponse = JsonMessage{
		Content: outputJson,
		Role:    "assistant",
	}
	return
}

// GenerateEmbeddings allows users to specify chunking strategy for batch processing
func (openaiModel *OpenAIModel) GenerateEmbeddings(ctx context.Context, inputs []EmbeddingInput, config chunking.ChunkingConfig, dimension int) (outputs []EmbeddingOutput, err error) {
	logs.WithContext(ctx).Debug(fmt.Sprintf("GenerateEmbeddings - Start with %d inputs using %s strategy", len(inputs), config.Strategy))

	if len(inputs) == 0 {
		return []EmbeddingOutput{}, nil
	}
	if config.Strategy == "" {
		config = chunking.DefaultChunkingConfig()
	}

	// Process each input
	for _, input := range inputs {
		embedding, err := openaiModel.GenerateEmbedding(ctx, input.Text, config, dimension)
		if err != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("Failed to generate embedding for ID %s: %v", input.Id, err))
			return nil, err
		}

		output := EmbeddingOutput{
			Id:     input.Id,
			Text:   input.Text,
			Vector: embedding,
		}
		outputs = append(outputs, output)
	}

	logs.WithContext(ctx).Debug(fmt.Sprintf("Generated %d embeddings successfully using %s strategy", len(outputs), config.Strategy))
	return outputs, nil
}

// GenerateEmbedding allows users to specify chunking strategy and configuration
func (openaiModel *OpenAIModel) GenerateEmbedding(ctx context.Context, text string, config chunking.ChunkingConfig, dimension int) (embedding []float64, err error) {
	logs.WithContext(ctx).Debug("GenerateEmbedding - Start")

	// Get the appropriate chunking strategy
	factory := &chunking.ChunkingFactory{}
	chunker, err := factory.GetChunkingStrategy(config)
	if err != nil {
		logs.Err(ctx, fmt.Errorf("Failed to get chunking strategy: %v", err), "")
		return nil, err
	}

	// Apply chunking based on user's strategy
	chunkedTexts, err := chunker.ChunkText(text, config)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to chunk text: %v", err))
		return nil, err
	}

	if len(chunkedTexts) == 1 {
		// Single chunk, proceed normally
		return openaiModel.generateSingleEmbedding(ctx, chunkedTexts[0], dimension)
	}

	// Multiple chunks, average the embeddings
	var allEmbeddings [][]float64
	for _, chunk := range chunkedTexts {
		chunkEmbedding, err := openaiModel.generateSingleEmbedding(ctx, chunk, dimension)
		if err != nil {
			return nil, err
		}
		allEmbeddings = append(allEmbeddings, chunkEmbedding)
	}

	// Average the embeddings from all chunks
	embedding = openaiModel.averageEmbeddings(allEmbeddings)
	logs.WithContext(ctx).Debug(fmt.Sprintf("Generated averaged embedding from %d chunks using %s strategy with %d dimensions",
		len(allEmbeddings), chunker.GetName(), len(embedding)))
	return embedding, nil
}

// generateSingleEmbedding handles the actual API call for a single text
func (openaiModel *OpenAIModel) generateSingleEmbedding(ctx context.Context, text string, dimension int) (embedding []float64, err error) {
	// Create embedding request
	embeddingRequest := OpenAIEmbeddingRequest{
		Input: text,
		Model: openaiModel.LLMName, // eg: text-embedding-ada-002
	}
	if openaiModel.supportsDimensions() {
		embeddingRequest.Dimensions = getExpectedDimensions(openaiModel.LLMName, dimension)
	}

	// Make HTTP request to OpenAI embedding API
	reqHeader := http.Header{}
	reqHeader.Add("Authorization", "Bearer "+openaiModel.LLMSecret)
	reqHeader.Add("Content-Type", "application/json")

	response, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, OpenAIEmbeddingApiUrl, reqHeader, nil, nil, nil, embeddingRequest)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to call OpenAI embedding API: %v", err))
		return nil, err
	}

	// Parse response
	responseJson, err := json.Marshal(response)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to marshal response: %v", err))
		return nil, err
	}

	var embeddingResponse OpenAIEmbeddingResponse
	err = json.Unmarshal(responseJson, &embeddingResponse)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to unmarshal embedding response: %v", err))
		return nil, err
	}

	// Check if we have embedding data
	if len(embeddingResponse.Data) == 0 {
		err = fmt.Errorf("no embedding data received from OpenAI")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}

	embedding = embeddingResponse.Data[0].Embedding
	logs.WithContext(ctx).Debug(fmt.Sprintf("Generated single embedding with %d dimensions", len(embedding)))
	return embedding, nil
}

// getExpectedDimensions returns the expected dimensions for a given OpenAI embedding model
func getExpectedDimensions(modelName string, dimension int) int {
	switch modelName {
	case "text-embedding-ada-002":
		return 1536
	case "text-embedding-3-small":
		if dimension == 1536 || dimension == 3072 {
			return dimension
		}
		return 1536
	case "text-embedding-3-large":
		if dimension == 1536 || dimension == 3072 || dimension == 4096 {
			return dimension
		}
		return 3072
	default:
		return 0 // Unknown model, will use actual response dimensions
	}
}

// supportsDimensions checks if the model supports custom dimensions
func (openaiModel *OpenAIModel) supportsDimensions() bool {
	switch openaiModel.LLMName {
	case "text-embedding-3-small", "text-embedding-3-large":
		return true
	case "text-embedding-ada-002":
		return false
	default:
		return false // Conservative default
	}
}

func convertOpenAITools(ctx context.Context, toolsMap map[string]tools.Tooling) ([]OpenAIRequestTools, string) {
	var openAIRequestTools []OpenAIRequestTools
	toolPrompt := ""
	for toolKey, tool := range toolsMap {
		toolDescriptionI, _ := tool.GetAttribute(ctx, "description")
		toolSystemPromptI, _ := tool.GetAttribute(ctx, "system_prompt")
		toolName := ToolNameForModel(ctx, toolKey, tool)
		toolDescription := toolDescriptionI.(string)
		toolParameters := tool.GetParameters()

		if toolSystemPrompt, ok := toolSystemPromptI.(string); ok && toolSystemPrompt != "" {
			toolPrompt += fmt.Sprint("Tool prompt for Tool ", toolName, " is as follows :\n", toolSystemPrompt)
		}

		openAIRequestTools = append(openAIRequestTools, OpenAIRequestTools{
			Type: "function",
			Function: ToolFunction{
				Name:        toolName,
				Description: toolDescription,
				Parameters:  toolParameters,
			},
		})
	}
	return openAIRequestTools, toolPrompt
}

func (openaiModel *OpenAIModel) queryToolLoop(ctx context.Context, request OpenAIToolLoopRequest) (openAIChatResponse OpenAIChatResponse, err error) {
	logs.WithContext(ctx).Debug("queryToolLoop - Start")
	reqHeader := http.Header{}
	reqHeader.Add("Authorization", "Bearer "+openaiModel.LLMSecret)
	reqHeader.Add("Content-Type", "application/json")

	response, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, OpenAIApiUrl, reqHeader, nil, nil, nil, request)
	if err != nil {
		return
	}

	responseJson, err := json.Marshal(response)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	err = json.Unmarshal(responseJson, &openAIChatResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}

func (openaiModel *OpenAIModel) RunToolLoop(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor) (Message, []StepTrace, error) {
	logs.WithContext(ctx).Debug("RunToolLoop - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "OpenAI.RunToolLoop",
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

		openAIChatResponse, err := openaiModel.queryToolLoop(ctx, request)
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			return Message{}, traces, err
		}
		if len(openAIChatResponse.Choices) == 0 {
			err = errors.New("no choices in OpenAI response")
			logs.WithContext(ctx).Error(err.Error())
			return Message{}, traces, err
		}
		accumulateOpenAIUsage(&usageAcc, openAIChatResponse.Usage)

		choice := openAIChatResponse.Choices[0]

		trace := StepTrace{
			Iteration: iteration,
			Timestamp: time.Now(),
		}

		if choice.FinishReason == "length" {
			logs.WithContext(ctx).Error(fmt.Sprintf("OpenAI response truncated: finish_reason=length, max_completion_tokens=%d. Increase model.max_tokens.", request.MaxCompletionTokens))
		}

		assistantMessage := OpenAIToolLoopMessage{Role: "assistant", ToolCalls: choice.Message.ToolCalls}
		if choice.Message.Content != "" {
			assistantMessage.Content = choice.Message.Content
		}
		messages = append(messages, assistantMessage)

		if len(choice.Message.ToolCalls) == 0 {
			trace.Content = choice.Message.Content
			traces = append(traces, trace)
			return usageAcc.Attach(Message{Content: choice.Message.Content, Role: "assistant"}), traces, nil
		}

		for _, toolCall := range choice.Message.ToolCalls {
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
				resultBytes, _ := json.Marshal(inputMap)
				return usageAcc.Attach(Message{Content: string(resultBytes), Role: "assistant", TerminalTool: TerminalToolAskUser}), traces, nil
			}

			trace.ToolName = toolCall.Function.Name
			trace.ToolInput = inputMap
		}

		for _, toolCall := range choice.Message.ToolCalls {
			inputMap := make(map[string]interface{})
			json.Unmarshal([]byte(toolCall.Function.Arguments), &inputMap)

			result, execErr := toolExecutor(ctx, toolCall.Function.Name, inputMap)
			toolMessage := OpenAIToolLoopMessage{Role: "tool", ToolCallId: toolCall.Id}
			if execErr != nil {
				toolMessage.Content = fmt.Sprintf("Error: %s", execErr.Error())
				trace.ToolResult = map[string]interface{}{"error": execErr.Error()}
			} else {
				resultBytes, _ := json.Marshal(result)
				toolMessage.Content = string(resultBytes)
				trace.ToolResult = result
			}
			messages = append(messages, toolMessage)
		}
		traces = append(traces, trace)
	}

	return usageAcc.Attach(Message{Content: "max iterations reached", Role: "assistant"}), traces, nil
}

func (openaiModel *OpenAIModel) queryModelResponsesWithTool(ctx context.Context, chatRequest ChatRequest, tools map[string]tools.Tooling, agentName string, agentPrompt string) (queryResponse JsonMessage, err error) {
	logs.WithContext(ctx).Debug("queryModelResponsesWithTool - Start")
	var openAIResponsesRequestText OpenAIResponsesRequestText
	var openAIRequestTools []OpenAIRequestTools
	toolPrompt := ""
	for toolKey, tool := range tools {
		toolType, _ := tool.GetAttribute(ctx, "tool_type")
		toolDescriptionI, _ := tool.GetAttribute(ctx, "description")
		toolSystemPromptI, _ := tool.GetAttribute(ctx, "system_prompt")
		toolName := ToolNameForModel(ctx, toolKey, tool)
		toolDescription := toolDescriptionI.(string)
		toolParameters := tool.GetParameters()

		toolPrompt += fmt.Sprint("Tool prompt for Tool ", toolName, " is as follows :\n", toolSystemPromptI.(string))

		if toolType.(string) == "STRUCTURED_OUTPUT" {
			toolParametersBytes, _ := json.Marshal(toolParameters)
			logs.WithContext(ctx).Info(string(toolParametersBytes))
			openAIResponsesRequestText = OpenAIResponsesRequestText{
				Format: OpenAIResponsesRequestTextFormat{
					Type:        "json_schema",
					Name:        toolName,
					Schema:      toolParameters,
					Description: toolDescription,
					Strict:      true,
				},
			}
		} else {
			reqTool := OpenAIRequestTools{
				Type: "function",
				Function: ToolFunction{
					Name:        toolName,
					Description: toolDescription,
					Parameters:  toolParameters,
				},
			}
			openAIRequestTools = append(openAIRequestTools, reqTool)
		}
	}
	/* var toolsInterface []interface{}
	for _, tool := range openAIRequestTools {
		toolsInterface = append(toolsInterface, tool)
	} */
	serviceTier := openaiModel.ServiceTier
	if serviceTier == "" {
		serviceTier = "auto"
	}
	openAIResponsesRequest := OpenAIResponsesRequest{
		Model:       openaiModel.LLMName,
		Tools:       nil,
		ServiceTier: serviceTier,
		Text:        &openAIResponsesRequestText,
	}

	if agentName == "" {
		agentName = "Agent"
	}

	// In Responses API, system instruction usually goes in the input array as an item
	// Mapping current logic to input items
	openAIResponsesRequest.Input = append(openAIResponsesRequest.Input, OpenAIResponsesInput{
		Type: "message",
		Role: "system",
		Content: []OpenAIResponsesContent{
			{
				Type: "input_text",
				Text: fmt.Sprint(toolPrompt, "\n", agentPrompt),
			},
		},
	})

	for _, msg := range chatRequest.Messages {
		if msg.Content != "" {
			typeStr := "input_text"
			switch msg.Role {
			case "user":
				typeStr = "input_text"
			case "assistant":
				typeStr = "output_text"
			}
			openAIResponsesRequest.Input = append(openAIResponsesRequest.Input, OpenAIResponsesInput{
				Type: "message",
				Role: msg.Role,
				Content: []OpenAIResponsesContent{
					{
						Type: typeStr,
						Text: msg.Content,
					},
				},
			})
		}
	}

	reqHeader := http.Header{}
	reqHeader.Add("Authorization", "Bearer "+openaiModel.LLMSecret)
	reqHeader.Add("Content-Type", "application/json")

	response, _, _, _, err := utils.CallHttp(ctx, http.MethodPost, OpenAIResponseApiUrl, reqHeader, nil, nil, nil, openAIResponsesRequest)
	if err != nil {
		return
	}

	responseJson, err := json.Marshal(response)
	if err != nil {
		return
	}

	var openAIResponsesResponse OpenAIResponsesResponse
	err = json.Unmarshal(responseJson, &openAIResponsesResponse)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	outputJson := make(map[string]interface{})
	for _, out := range openAIResponsesResponse.Output {
		if out.Type == "function_call" {
			err = json.Unmarshal([]byte(out.Arguments), &outputJson)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				outputJson["raw"] = out.Arguments
			}
			break
		} else if out.Type == "message" {
			err = json.Unmarshal([]byte(out.Content[0].Text), &outputJson)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				outputJson["raw"] = out.Content[0].Text
			}
			break
		}
	}

	queryResponse = JsonMessage{
		Content: outputJson,
		Role:    "assistant",
	}
	return
}
