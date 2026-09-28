package chunking

import (
	"strings"
	"unicode/utf8"
)

// Expect your config to have these (add if missing):
// type ChunkingConfig struct {
//   MaxTokens     int // optional
//   MaxChars      int // preferred
//   OverlapChars  int // optional (e.g., 200-300)
//   MinChars      int // optional (e.g., 400-600) to avoid tiny tail
// }

type FixedSizeChunker struct{}

func (fsc *FixedSizeChunker) GetName() string { return "fixed" }

func (fsc *FixedSizeChunker) ChunkText(text string, chunkingConfig ChunkingConfig) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	// Decide the working limit
	maxChars := chunkingConfig.MaxChars
	if maxChars <= 0 {
		if chunkingConfig.MaxTokens > 0 {
			maxChars = chunkingConfig.MaxTokens * 4 // heuristic
		} else {
			maxChars = DefaultChunkingConfig().MaxChars // sensible default
		}
	}
	overlap := chunkingConfig.OverlapChars
	if overlap < 0 {
		overlap = 0
	}
	minChars := chunkingConfig.MinChars
	if minChars < 0 {
		minChars = 0
	}

	// Precompute rune start byte indices so we only cut on rune boundaries
	runeStarts := make([]int, 0, utf8.RuneCountInString(text))
	for i := range text {
		runeStarts = append(runeStarts, i)
	}
	// Append end sentinel to simplify slicing
	runeStarts = append(runeStarts, len(text))

	var chunks []string
	r := 0 // rune index

	for r < len(runeStarts)-1 {
		// Find byte end for ~maxChars runes (approx by runes==chars; safer than bytes)
		// We’ll soft-wrap at whitespace below.
		targetRunes := advanceToApproxChars(text, runeStarts, r, maxChars)

		startByte := runeStarts[r]
		endByte := runeStarts[targetRunes]

		// Only soft-wrap when the text is actually being cut short of its end
		softEnd := endByte
		if endByte < len(text) {
			softEnd = backtrackToWhitespace(text, startByte, endByte)
		}
		if softEnd > startByte {
			endByte = softEnd
			// align endByte to rune boundary (already safe because we only moved left across ASCII space)
		}

		chunk := strings.TrimSpace(text[startByte:endByte])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}

		if endByte == len(text) {
			break
		}

		// Advance with overlap (by characters/runes)
		if overlap > 0 {
			// Move r forward to the rune that corresponds to endByte,
			// then step back by ~overlap chars.
			endRune := findRuneIndex(runeStarts, endByte)
			backRunes := retreatByApproxChars(text, runeStarts, endRune, overlap)
			if backRunes <= r {
				r = endRune // ensure progress
			} else {
				r = backRunes
			}
		} else {
			// No overlap
			r = findRuneIndex(runeStarts, endByte)
		}
	}

	// Merge tiny tail if needed
	if minChars > 0 && len(chunks) >= 2 {
		last := chunks[len(chunks)-1]
		if utf8.RuneCountInString(last) < minChars {
			chunks[len(chunks)-2] = strings.TrimSpace(chunks[len(chunks)-2] + " " + last)
			chunks = chunks[:len(chunks)-1]
		}
	}

	return chunks, nil
}

// advanceToApproxChars moves forward from rune index 'start' until about maxChars are covered.
func advanceToApproxChars(text string, runeStarts []int, start, maxChars int) int {
	if maxChars <= 0 {
		return len(runeStarts) - 1
	}
	cur := start
	runes := 0
	for cur+1 < len(runeStarts) && runes < maxChars {
		seg := text[runeStarts[cur]:runeStarts[cur+1]]
		runes += utf8.RuneCountInString(seg) // 1, but safe for robustness
		cur++
	}
	return cur
}

// retreatByApproxChars walks backward ~overlap chars from rune index 'end'.
func retreatByApproxChars(text string, runeStarts []int, end, overlap int) int {
	if overlap <= 0 {
		return end
	}
	cur := end
	runes := 0
	for cur-1 >= 0 && runes < overlap {
		seg := text[runeStarts[cur-1]:runeStarts[cur]]
		runes += utf8.RuneCountInString(seg)
		cur--
	}
	return cur
}

// backtrackToWhitespace looks left from endByte to the nearest whitespace within a small window.
func backtrackToWhitespace(s string, startByte, endByte int) int {
	const window = 200 // don’t scan too far; tune as needed
	if endByte-startByte <= 1 {
		return endByte
	}
	i := endByte - 1
	limit := startByte
	if i-window > limit {
		limit = i - window
	}
	for ; i > limit; i-- {
		switch s[i] {
		case ' ', '\n', '\t', '\r':
			return i
		}
	}
	return endByte
}

// findRuneIndex finds the rune index whose byte start equals b (or nearest greater).
func findRuneIndex(runeStarts []int, b int) int {
	// binary search
	lo, hi := 0, len(runeStarts)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if runeStarts[mid] < b {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}
