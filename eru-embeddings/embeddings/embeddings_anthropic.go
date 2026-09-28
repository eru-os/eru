package embeddings

import (
	"context"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type AnthropicEmbeddings struct {
	Embeddings
}

func (ant *AnthropicEmbeddings) CreateEmbeddings(ctx context.Context, input []string, model string) (EmbeddingResponse, error) {
	logs.WithContext(ctx).Info("Creating Anthropic embeddings")
	
	return EmbeddingResponse{
		Model: model,
	}, nil
}

func (ant *AnthropicEmbeddings) GetModels(ctx context.Context) ([]ModelInfo, error) {
	logs.WithContext(ctx).Info("Getting Anthropic embedding models")
	
	models := []ModelInfo{
		{
			ID:          "claude-3-embeddings",
			Provider:    "anthropic",
			Dimensions:  1536,
			MaxTokens:   8192,
			Description: "Claude 3 embeddings model",
		},
	}
	
	return models, nil
}

func (ant *AnthropicEmbeddings) GetModelInfo(ctx context.Context, model string) (ModelInfo, error) {
	logs.WithContext(ctx).Info("Getting Anthropic model info")
	
	models, err := ant.GetModels(ctx)
	if err != nil {
		return ModelInfo{}, err
	}
	
	for _, m := range models {
		if m.ID == model {
			return m, nil
		}
	}
	
	return ModelInfo{
		ID:       model,
		Provider: "anthropic",
	}, nil
}

func (ant *AnthropicEmbeddings) ValidateModel(ctx context.Context, model string) error {
	logs.WithContext(ctx).Info("Validating Anthropic model")
	
	return nil
}