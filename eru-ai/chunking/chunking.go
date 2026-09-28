package chunking

import (
	"context"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// ChunkingStrategy defines the interface for different chunking methods
type ChunkingStrategy interface {
	ChunkText(text string, config ChunkingConfig) ([]string, error)
	GetName() string
}

// ChunkingConfig holds configuration for chunking behavior
type ChunkingConfig struct {
	Strategy  string `json:"strategy"`   // "fixed", "recursive", "sentence", "paragraph", "semantic"
	MaxTokens int    `json:"max_tokens"` // Maximum tokens per chunk
	MaxChars  int    `json:"max_chars"`  // Maximum characters per chunk
	MinChars  int    `json:"min_chars"`  // Minimum characters per chunk
	//OverlapTokens     int      `json:"overlap_tokens"`     // Token overlap between chunks
	OverlapChars      int      `json:"overlap_chars"`      // Character overlap between chunks
	MinChunkSize      int      `json:"min_chunk_size"`     // Minimum chunk size in tokens
	PreserveStructure bool     `json:"preserve_structure"` // Whether to preserve document structure
	Separators        []string `json:"separators"`         // Custom separators for recursive chunking
	Language          string   `json:"language"`           // Language for language-specific rules
	OverlapSentences  int      `json:"overlap_sentences"`  // Overlap sentences between chunks
}

// DefaultChunkingConfig returns sensible defaults
func DefaultChunkingConfig() ChunkingConfig {
	return ChunkingConfig{
		Strategy:          "recursive",
		MaxTokens:         4000,
		MaxChars:          1000,
		MinChars:          100,
		OverlapChars:      0,
		PreserveStructure: true,
		Separators:        []string{"\n\n", "\n", ". ", " ", ""},
		Language:          "en",
	}
}

// ChunkingFactory creates chunking strategies based on configuration
type ChunkingFactory struct{}

// GetChunkingStrategy returns the appropriate chunking strategy
func (cf *ChunkingFactory) GetChunkingStrategy(config ChunkingConfig) (ChunkingStrategy, error) {
	switch strings.ToLower(config.Strategy) {
	case "fixed":
		return &FixedSizeChunker{}, nil
	case "recursive":
		return &RecursiveChunker{}, nil
	case "sentence":
		return &SentenceChunker{}, nil
	case "paragraph":
		return &ParagraphChunker{}, nil
	case "semantic":
		return &SemanticChunker{}, nil
	default:
		return nil, logs.Err(context.Background(), fmt.Errorf("unknown chunking strategy: %s", config.Strategy), "")
	}
}
