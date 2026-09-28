package chunking

import (
	"strings"
	"unicode"
)

// SemanticChunker is a placeholder for future semantic chunking implementation
type SemanticChunker struct{}

func (sc *SemanticChunker) GetName() string {
	return "semantic"
}

func (sc *SemanticChunker) ChunkText(text string, config ChunkingConfig) ([]string, error) {
	// For now, fall back to recursive chunking
	// Future implementation could use embeddings to detect topic shifts
	recursiveChunker := &RecursiveChunker{}
	return recursiveChunker.ChunkText(text, config)
}

// Utility functions for text analysis
func estimateTokens(text string, language string) int {
	// Basic token estimation based on language
	switch strings.ToLower(language) {
	case "zh", "ja", "ko": // Chinese, Japanese, Korean
		// CJK characters are roughly 1 token per character
		return len([]rune(text))
	case "th": // Thai
		// Thai has complex word boundaries
		return len([]rune(text)) / 2
	default:
		// English and most European languages: ~4 characters per token
		return len(text) / 4
	}
}

func detectLanguage(text string) string {
	// Simple language detection based on character sets
	var (
		cjkCount      int
		thaiCount     int
		arabicCount   int
		hebrewCount   int
		cyrillicCount int
	)

	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			cjkCount++
		case unicode.Is(unicode.Thai, r):
			thaiCount++
		case unicode.Is(unicode.Arabic, r):
			arabicCount++
		case unicode.Is(unicode.Hebrew, r):
			hebrewCount++
		case unicode.Is(unicode.Cyrillic, r):
			cyrillicCount++
		}
	}

	totalRunes := len([]rune(text))
	if totalRunes == 0 {
		return "en"
	}

	// If more than 30% of characters are from a specific script, use that language
	threshold := float64(totalRunes) * 0.3

	if float64(cjkCount) > threshold {
		return "zh" // Default to Chinese
	}
	if float64(thaiCount) > threshold {
		return "th"
	}
	if float64(arabicCount) > threshold {
		return "ar"
	}
	if float64(hebrewCount) > threshold {
		return "he"
	}
	if float64(cyrillicCount) > threshold {
		return "ru"
	}

	return "en" // Default to English
}
