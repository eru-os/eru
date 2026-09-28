package embeddings

import (
	"context"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type HuggingFaceEmbeddings struct {
	Embeddings
}

func (hf *HuggingFaceEmbeddings) CreateEmbeddings(ctx context.Context, input []string, model string) (EmbeddingResponse, error) {
	logs.WithContext(ctx).Info("Creating HuggingFace embeddings")
	
	return EmbeddingResponse{
		Model: model,
	}, nil
}

func (hf *HuggingFaceEmbeddings) GetModels(ctx context.Context) ([]ModelInfo, error) {
	logs.WithContext(ctx).Info("Getting HuggingFace embedding models")
	
	models := []ModelInfo{
		{
			ID:          "sentence-transformers/all-MiniLM-L6-v2",
			Provider:    "huggingface",
			Dimensions:  384,
			MaxTokens:   256,
			Description: "All MiniLM L6 v2 sentence transformer",
		},
		{
			ID:          "sentence-transformers/all-mpnet-base-v2",
			Provider:    "huggingface",
			Dimensions:  768,
			MaxTokens:   384,
			Description: "All MPNet base v2 sentence transformer",
		},
		{
			ID:          "sentence-transformers/paraphrase-MiniLM-L6-v2",
			Provider:    "huggingface",
			Dimensions:  384,
			MaxTokens:   128,
			Description: "Paraphrase MiniLM L6 v2 sentence transformer",
		},
	}
	
	return models, nil
}

func (hf *HuggingFaceEmbeddings) GetModelInfo(ctx context.Context, model string) (ModelInfo, error) {
	logs.WithContext(ctx).Info("Getting HuggingFace model info")
	
	models, err := hf.GetModels(ctx)
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
		Provider: "huggingface",
	}, nil
}

func (hf *HuggingFaceEmbeddings) ValidateModel(ctx context.Context, model string) error {
	logs.WithContext(ctx).Info("Validating HuggingFace model")
	
	return nil
}