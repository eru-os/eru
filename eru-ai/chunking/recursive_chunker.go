package chunking

import "strings"

// RecursiveChunker splits text using separators in priority order
type RecursiveChunker struct{}

func (rc *RecursiveChunker) GetName() string {
	return "recursive"
}

func (rc *RecursiveChunker) ChunkText(text string, chunkingConfig ChunkingConfig) ([]string, error) {
	separators := chunkingConfig.Separators
	if len(separators) == 0 {
		separators = []string{"\n\n", "\n", ". ", " ", ""}
	}

	maxChars := chunkingConfig.MaxChars
	if maxChars <= 0 {
		// fallback heuristic
		if chunkingConfig.MaxTokens > 0 {
			maxChars = chunkingConfig.MaxTokens * 4
		} else {
			maxChars = DefaultChunkingConfig().MaxChars // safe default
		}
	}

	return rc.recursiveSplit(text, separators, maxChars, 0)
}

func (rc *RecursiveChunker) recursiveSplit(text string, separators []string, maxChars, depth int) ([]string, error) {
	if depth >= len(separators) {
		// Fall back to character-level splitting
		return rc.characterSplit(text, maxChars), nil
	}

	separator := separators[depth]
	if separator == "" {
		return rc.characterSplit(text, maxChars), nil
	}

	parts := strings.Split(text, separator)
	var chunks []string
	currentChunk := ""

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Add separator back if it's not the last part
		if currentChunk != "" && separator != "" {
			part = separator + part
		}

		if len(currentChunk)+len(part) <= maxChars {
			currentChunk += part
		} else {
			if currentChunk != "" {
				chunks = append(chunks, strings.TrimSpace(currentChunk))
			}
			currentChunk = part
		}
	}

	if currentChunk != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	// If chunks are still too long, try next separator
	var finalChunks []string
	for _, chunk := range chunks {
		if len(chunk) <= maxChars {
			finalChunks = append(finalChunks, chunk)
		} else {
			subChunks, err := rc.recursiveSplit(chunk, separators, maxChars, depth+1)
			if err != nil {
				return nil, err
			}
			finalChunks = append(finalChunks, subChunks...)
		}
	}

	return finalChunks, nil
}

func (rc *RecursiveChunker) characterSplit(text string, maxChars int) []string {
	var chunks []string
	for i := 0; i < len(text); i += maxChars {
		end := i + maxChars
		if end > len(text) {
			end = len(text)
		}
		chunk := strings.TrimSpace(text[i:end])
		if len(chunk) > 0 {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}
