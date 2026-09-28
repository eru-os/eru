package models

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	chunking "github.com/eru-os/eru/eru-ai/chunking"
	tools "github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// ToolNameForModel is the name a tool is presented to an LLM under.
//
// It is the key of the agent's tool map - the AgentTools entry's tool_key - and
// not the tool's own name. One physical tool attached under several actions
// appears in the map once per action, and every one of those answers
// GetAttribute("tool_name") with the same string. Presented that way the
// provider rejects the request outright ("tool names must be unique"), and if it
// did not, every call would resolve to whichever action happened to be first -
// a silent wrong write rather than an error.
//
// The map key is already unique, and the agent's tool executor resolves the
// model's chosen name back through the same key. An agent that sets no tool_key
// keeps its old name, because the key falls back to the tool name when the map
// is built.
func ToolNameForModel(ctx context.Context, key string, tool tools.Tooling) string {
	if key != "" {
		return key
	}
	if nameI, err := tool.GetAttribute(ctx, "tool_name"); err == nil {
		if name, ok := nameI.(string); ok {
			return name
		}
	}
	return ""
}

type StepTrace struct {
	Iteration  int                    `json:"iteration"`
	Thinking   string                 `json:"thinking,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolInput  map[string]interface{} `json:"tool_input,omitempty"`
	ToolResult map[string]interface{} `json:"tool_result,omitempty"`
	Content    string                 `json:"content,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Agent      string                 `json:"agent,omitempty"`
	Step       string                 `json:"step,omitempty"`
}

type ToolExecutor func(ctx context.Context, toolName string, input map[string]interface{}) (map[string]interface{}, error)

type StreamEventType string

const (
	StreamThinking   StreamEventType = "thinking"
	StreamToolUse    StreamEventType = "tool_use"
	StreamToolResult StreamEventType = "tool_result"
	StreamTextDelta  StreamEventType = "text_delta"
	StreamDone       StreamEventType = "done"
	StreamQuestion   StreamEventType = "question"
	// StreamToolInputDelta carries the tool arguments as the model writes them,
	// before the tool call is complete. It is what makes progressive rendering
	// possible for a structured-output agent: the answer IS the tool input, so
	// without these deltas the whole answer arrives in one lump at the end.
	StreamToolInputDelta StreamEventType = "tool_input_delta"
)

const TerminalToolAskUser = "ask_user"
const TerminalToolStructuredOutput = "structured_output"

type ModelStreamEvent struct {
	Type      StreamEventType        `json:"type"`
	Content   string                 `json:"content,omitempty"`
	ToolName  string                 `json:"tool_name,omitempty"`
	ToolInput map[string]interface{} `json:"tool_input,omitempty"`
	Iteration int                    `json:"iteration,omitempty"`
	// BlockIndex identifies the content block a delta belongs to, so
	// concurrent tool calls in one response can be told apart.
	BlockIndex int `json:"block_index,omitempty"`
}

type StreamEventCallback func(event ModelStreamEvent)

type ModelI interface {
	GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error)
	MakeFromJson(ctx context.Context, rj *json.RawMessage) (err error)
	PerformPreSaveTask(ctx context.Context) (err error)
	PerformPreDeleteTask(ctx context.Context) (err error)
	QueryModel(ctx context.Context, chatRequest ChatRequest) (response Message, err error)
	QueryModelWithTool(ctx context.Context, chatRequest ChatRequest, tools map[string]tools.Tooling, agentName string, agentPrompt string) (response JsonMessage, err error)
	RunToolLoop(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor) (response Message, traces []StepTrace, err error)
	GenerateEmbeddings(ctx context.Context, inputs []EmbeddingInput, config chunking.ChunkingConfig, dimension int) (outputs []EmbeddingOutput, err error)
}

type StreamingModelI interface {
	ModelI
	QueryModelStreaming(ctx context.Context, chatRequest ChatRequest, callback func(chunk string)) (Message, error)
	RunToolLoopStreaming(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor, streamCb StreamEventCallback) (response Message, traces []StepTrace, err error)
}

type ReasoningModelI interface {
	ModelI
	QueryModelWithReasoning(ctx context.Context, chatRequest ChatRequest, thinkingBudget int) (Message, string, error)
}

type Model struct {
	Provider   string  `json:"provider" eru:"required"`
	LLMName    string  `json:"llm_name" eru:"required"`
	ModelName  string  `json:"model_name" eru:"required"`
	LLMSecret  string  `json:"llm_secret" eru:"required"`
	Temprature float64 `json:"temprature"`
	MaxTokens  int64   `json:"max_tokens,omitempty"`
}
type ChatRequest struct {
	Messages []Message `json:"messages"`
}
type TokenUsage struct {
	InputTokens     int64 `json:"input_tokens,omitempty"`
	OutputTokens    int64 `json:"output_tokens,omitempty"`
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"`
	CachedTokens    int64 `json:"cached_tokens,omitempty"`
	TotalTokens     int64 `json:"total_tokens,omitempty"`
}

type AgentPrompt struct {
	Static  string
	Dynamic string
}

func StaticAgentPrompt(prompt string) AgentPrompt {
	return AgentPrompt{Static: prompt}
}

func (p AgentPrompt) String() string {
	return p.Static + p.Dynamic
}

func (p AgentPrompt) IsEmpty() bool {
	return p.Static == "" && p.Dynamic == ""
}

type UsageAccumulator struct {
	InputTokens     int64
	OutputTokens    int64
	ReasoningTokens int64
	CachedTokens    int64
}

func (u *UsageAccumulator) Add(inputTokens int64, outputTokens int64, cachedTokens int64, reasoningTokens int64) {
	u.InputTokens += inputTokens
	u.OutputTokens += outputTokens
	u.CachedTokens += cachedTokens
	u.ReasoningTokens += reasoningTokens
}

func (u *UsageAccumulator) Attach(message Message) Message {
	message.Usage = u.TokenUsage()
	return message
}

func (u *UsageAccumulator) TokenUsage() *TokenUsage {
	return &TokenUsage{
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		ReasoningTokens: u.ReasoningTokens,
		CachedTokens:    u.CachedTokens,
		TotalTokens:     u.InputTokens + u.OutputTokens,
	}
}

type Message struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	Name         string        `json:"name"`
	Files        []FileMessage `json:"files,omitempty"`
	Usage        *TokenUsage   `json:"usage,omitempty"`
	TerminalTool string        `json:"terminal_tool,omitempty"`
}
type FileMessage struct {
	FileData  string `json:"file_data,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	FileId    string `json:"file_id,omitempty"`
	ImageData string `json:"image_data,omitempty"`
	FileType  string `json:"file_type,omitempty"`
}
type JsonMessage struct {
	Role    string                 `json:"role"`
	Content map[string]interface{} `json:"content"`
	Name    string                 `json:"name"`
}

type EmbeddingInputRequest struct {
	Inputs      []EmbeddingInput        `json:"inputs" eru:"required"`
	ChunkConfig chunking.ChunkingConfig `json:"chunk_config"`
	Dimension   int                     `json:"dimension"`
}

// Batch embedding input/output types
type EmbeddingInput struct {
	Id   string `json:"id"`
	Text string `json:"text"`
}

type EmbeddingOutput struct {
	Id     string    `json:"id"`
	Text   string    `json:"text"`
	Vector []float64 `json:"vector"`
}

func (model *Model) GetAttribute(ctx context.Context, attributeName string) (attributeValue interface{}, err error) {
	switch attributeName {
	case "provider":
		return model.Provider, nil
	case "model_name":
		return model.ModelName, nil
	case "llm_name":
		return model.LLMName, nil
	case "llm_secret":
		return model.LLMSecret, nil
	case "temprature":
		return model.Temprature, nil
	default:
		err := errors.New("attribute not found")
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
}

func GetModel(provider string) ModelI {
	switch provider {
	case "OPENAI":
		return new(OpenAIModel)
	case "ANTHROPIC":
		return new(AnthropicModel)
	case "BEDROCK":
		return new(BedrockModel)
	case "GEMINI":
		return new(GeminiModel)
	default:
		return new(Model)
	}
}

func (model *Model) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	err := errors.New("MakeFromJson Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return err
}

func (model *Model) PerformPreSaveTask(ctx context.Context) (err error) {
	err = errors.New("PerformPreSaveTask Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (model *Model) PerformPreDeleteTask(ctx context.Context) (err error) {
	err = errors.New("PerformPreDeleteTask Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (model *Model) QueryModel(ctx context.Context, chatRequest ChatRequest) (response Message, err error) {
	err = errors.New("QueryModel Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (model *Model) QueryModelWithTool(ctx context.Context, chatRequest ChatRequest, tools map[string]tools.Tooling, agentName string, agentPrompt string) (response JsonMessage, err error) {
	err = errors.New("QueryModelWithTool Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (model *Model) RunToolLoop(ctx context.Context, chatRequest ChatRequest, toolsMap map[string]tools.Tooling, agentPrompt AgentPrompt, maxIterations int, thinkingBudget int, toolExecutor ToolExecutor) (response Message, traces []StepTrace, err error) {
	err = errors.New("RunToolLoop Method not implemented for provider " + model.Provider)
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (model *Model) GenerateEmbedding(ctx context.Context, text string) (embedding []float64, err error) {
	err = errors.New("GenerateEmbedding Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

func (model *Model) GenerateEmbeddings(ctx context.Context, inputs []EmbeddingInput, config chunking.ChunkingConfig, dimension int) (outputs []EmbeddingOutput, err error) {
	err = errors.New("GenerateEmbeddings Method not implemented")
	logs.WithContext(ctx).Error(err.Error())
	return
}

// averageEmbeddings averages multiple embeddings
func (model *Model) averageEmbeddings(embeddings [][]float64) []float64 {
	if len(embeddings) == 0 {
		return nil
	}

	if len(embeddings) == 1 {
		return embeddings[0]
	}

	// All embeddings should have the same dimension
	dimension := len(embeddings[0])
	result := make([]float64, dimension)

	for _, embedding := range embeddings {
		for i, value := range embedding {
			result[i] += value
		}
	}

	// Average the values
	for i := range result {
		result[i] /= float64(len(embeddings))
	}

	return result
}
