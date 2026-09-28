package embeddings

import (
	"context"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type OpenAIEmbeddings struct {
	Embeddings
}

func (oai *OpenAIEmbeddings) CreateEmbeddings(ctx context.Context, input []string, model string) (EmbeddingResponse, error) {
	logs.WithContext(ctx).Info("Creating OpenAI embeddings")
	
	return EmbeddingResponse{
		Model: model,
	}, nil
}

func (oai *OpenAIEmbeddings) GetModels(ctx context.Context) ([]ModelInfo, error) {
	logs.WithContext(ctx).Info("Getting OpenAI embedding models")
	
	models := []ModelInfo{
		{
			ID:          "text-embedding-ada-002",
			Provider:    "openai",
			Dimensions:  1536,
			MaxTokens:   8191,
			Description: "OpenAI Ada v2 embeddings model",
		},
		{
			ID:          "text-embedding-3-small",
			Provider:    "openai",
			Dimensions:  1536,
			MaxTokens:   8191,
			Description: "OpenAI small embeddings model",
		},
		{
			ID:          "text-embedding-3-large",
			Provider:    "openai",
			Dimensions:  3072,
			MaxTokens:   8191,
			Description: "OpenAI large embeddings model",
		},
	}
	
	return models, nil
}

func (oai *OpenAIEmbeddings) GetModelInfo(ctx context.Context, model string) (ModelInfo, error) {
	logs.WithContext(ctx).Info("Getting OpenAI model info")
	
	models, err := oai.GetModels(ctx)
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
		Provider: "openai",
	}, nil
}

func (oai *OpenAIEmbeddings) ValidateModel(ctx context.Context, model string) error {
	logs.WithContext(ctx).Info("Validating OpenAI model")
	
	return nil
}