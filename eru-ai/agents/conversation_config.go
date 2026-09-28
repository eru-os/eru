package agents

import (
	"strings"
	"time"
)

// ConversationConfig holds configuration for conversation management
type ConversationConfig struct {
	MaxRecentMessages   int           `json:"max_recent_messages"`  // Number of recent messages to keep in full detail
	MaxTokens           int           `json:"max_tokens"`           // Maximum tokens for the entire conversation
	SummaryThreshold    int           `json:"summary_threshold"`    // Token threshold to trigger summarization
	EnableSummarization bool          `json:"enable_summarization"` // Whether to enable conversation summarization
	SummaryModel        string        `json:"summary_model"`        // Model to use for summarization (optional, uses main model if empty)
	MaxConversationAge  time.Duration `json:"max_conversation_age"` // Maximum age before forcing summarization
}

// DefaultConversationConfig returns default configuration
func DefaultConversationConfig(modelName string) *ConversationConfig {

	config := ConversationConfig{
		MaxRecentMessages:   20,             // Keep last 20 messages in full detail
		MaxTokens:           100000,         // Conservative limit for most models
		SummaryThreshold:    80000,          // Start summarizing when approaching 80k tokens
		EnableSummarization: true,           // Enable summarization by default
		SummaryModel:        "",             // Use main model for summarization
		MaxConversationAge:  24 * time.Hour, // Force summarization after 24 hours
	}
	config.GetModelSpecificConfig(modelName)
	return &config
}

func (config *ConversationConfig) GetModelSpecificConfig(modelName string) {
	modelName = strings.ToLower(modelName)
	// Adjust limits based on model capabilities
	switch {
	case strings.Contains(modelName, "gpt-4o"):
		config.MaxTokens = 120000 // GPT-4o has 128k context window
		config.SummaryThreshold = 100000
	case strings.Contains(modelName, "gpt-4"):
		config.MaxTokens = 120000 // GPT-4 has 128k context window
		config.SummaryThreshold = 100000
	case strings.Contains(modelName, "claude-3.5-sonnet"):
		config.MaxTokens = 180000 // Claude 3.5 Sonnet has 200k context window
		config.SummaryThreshold = 160000
	case strings.Contains(modelName, "claude-3-opus"):
		config.MaxTokens = 180000 // Claude 3 Opus has 200k context window
		config.SummaryThreshold = 160000
	case strings.Contains(modelName, "claude-3-haiku"):
		config.MaxTokens = 180000 // Claude 3 Haiku has 200k context window
		config.SummaryThreshold = 160000
	case strings.Contains(modelName, "gpt-3.5"):
		config.MaxTokens = 12000 // GPT-3.5 has 16k context window
		config.SummaryThreshold = 10000
		config.MaxRecentMessages = 10 // Fewer recent messages for smaller context
	default:
		// Use conservative defaults for unknown models
		config.MaxTokens = 8000
		config.SummaryThreshold = 6000
		config.MaxRecentMessages = 10
	}
}
