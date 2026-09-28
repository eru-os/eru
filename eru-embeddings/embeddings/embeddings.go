package embeddings

import (
	"context"
	"strings"
)

type EmbeddingsI interface {
	CreateEmbeddings(ctx context.Context, input []string, model string) (EmbeddingResponse, error)
	GetModels(ctx context.Context) ([]ModelInfo, error)
	GetModelInfo(ctx context.Context, model string) (ModelInfo, error)
	ValidateModel(ctx context.Context, model string) error
}

type EmbeddingResponse struct {
	Embeddings []Embedding `json:"embeddings"`
	Model      string      `json:"model"`
	Usage      Usage       `json:"usage"`
}

type Embedding struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type Usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ModelInfo struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	Dimensions  int    `json:"dimensions"`
	MaxTokens   int    `json:"max_tokens"`
	Description string `json:"description,omitempty"`
}

func GetEmbeddingsProvider(providerType string) EmbeddingsI {
	switch strings.ToLower(providerType) {
	case "openai":
		return new(OpenAIEmbeddings)
	case "anthropic":
		return new(AnthropicEmbeddings)
	case "huggingface":
		return new(HuggingFaceEmbeddings)
	case "bedrock":
		return new(BedrockEmbeddings)
	default:
		return new(Embeddings)
	}
}

type Embeddings struct {
	Provider string
	APIKey   string
	BaseURL  string
}

func (e *Embeddings) CreateEmbeddings(ctx context.Context, input []string, model string) (EmbeddingResponse, error) {
	return EmbeddingResponse{}, nil
}

func (e *Embeddings) GetModels(ctx context.Context) ([]ModelInfo, error) {
	return []ModelInfo{}, nil
}

func (e *Embeddings) GetModelInfo(ctx context.Context, model string) (ModelInfo, error) {
	return ModelInfo{
		ID:       model,
		Provider: e.Provider,
	}, nil
}

func (e *Embeddings) ValidateModel(ctx context.Context, model string) error {
	return nil
}