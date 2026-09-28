package chunking

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ---- Precompiled patterns (package-level) ----
var (
	// Protect common abbreviations & initials (case-insensitive).
	abbrRe = regexp.MustCompile(`(?i)\b(?:Mr|Mrs|Ms|Dr|Prof|Sr|Jr|St|Mt|No|Inc|Ltd|Co|vs|etc|i\.e|e\.g|U\.S|U\.K|A\.M|P\.M|B\.C|A\.D|Ph\.D|M\.B\.A|B\.A|M\.A|M\.D|B\.S|M\.S)\.`)

	// Protect single-letter initials (e.g., "A. B. Smith") and acronyms like "U.S."
	initialRe = regexp.MustCompile(`\b([A-Z])\.`)

	// Sentence finder that KEEPS the delimiter and trailing quotes/parens.
	// It matches minimally until a sentence end punctuation, then optional quotes/parens, then whitespace or EoS.
	sentRe = regexp.MustCompile(`(?s)(.*?[.!?]+(?:['")\]]+)?)(\s+|$)`)
)

// ---- Types ----
type SentenceChunker struct{}

func (sc *SentenceChunker) GetName() string { return "sentence" }

// ---- Public API ----
func (sc *SentenceChunker) ChunkText(text string, chunkingConfig ChunkingConfig) ([]string, error) {
	maxChars := chunkingConfig.MaxChars
	if maxChars <= 0 {
		// fallback heuristic
		if chunkingConfig.MaxTokens > 0 {
			maxChars = chunkingConfig.MaxTokens * 4
		} else {
			maxChars = DefaultChunkingConfig().MaxChars // safe default
		}
	}
	overlapChars := chunkingConfig.OverlapChars
	if overlapChars < 0 {
		overlapChars = 0
	}

	sentences := splitIntoSentencesKeepDelim(text)

	// Pack sentences up to maxChars (rune-aware), then apply overlap.
	var chunks []string
	var b strings.Builder
	runes := 0
	startIdx := 0 // sentence index where chunk starts

	for i, s := range sentences {
		sz := utf8.RuneCountInString(s)
		if runes+sz <= maxChars || b.Len() == 0 {
			b.WriteString(s)
			runes += sz
			continue
		}
		// flush chunk
		chunks = append(chunks, strings.TrimSpace(b.String()))

		// compute overlap by sentences first, else by chars
		overlapStart := i
		if chunkingConfig.OverlapSentences > 0 {
			overlapStart = max(i-chunkingConfig.OverlapSentences, startIdx)
		} else if overlapChars > 0 {
			// walk backwards until overlapChars (approx by runes)
			overlapRunes := 0
			j := i - 1
			for j >= startIdx && overlapRunes < overlapChars {
				overlapRunes += utf8.RuneCountInString(sentences[j])
				j--
			}
			overlapStart = j + 1
		}

		// reset builder with overlap slice
		b.Reset()
		runes = 0
		for k := overlapStart; k < i; k++ {
			b.WriteString(sentences[k])
			runes += utf8.RuneCountInString(sentences[k])
		}
		startIdx = overlapStart

		// add current sentence (may push over, but ensures progress)
		if runes+sz > maxChars && b.Len() > 0 {
			// if the overlap itself already fills the chunk, flush and start clean
			chunks = append(chunks, strings.TrimSpace(b.String()))
			b.Reset()
			runes = 0
			startIdx = i
		}
		b.WriteString(s)
		runes += sz
	}

	if b.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(b.String()))
	}

	// Final safeguard: if any chunk still exceeds maxChars (rare), split by words (rune-aware).
	final := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if utf8.RuneCountInString(c) <= maxChars {
			final = append(final, c)
		} else {
			final = append(final, splitIntoWordChunksRuneAware(c, maxChars)...)
		}
	}
	return final, nil
}

// ---- Helpers ----

func splitIntoSentencesKeepDelim(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	// Protect abbreviations and initials to avoid false breaks.
	protected := abbrRe.ReplaceAllString(text, "${0}_ABBR_") // mark the dot
	protected = initialRe.ReplaceAllStringFunc(protected, func(m string) string {
		// "A." -> "A._ABBR_"
		return m + "_ABBR_"
	})

	matches := sentRe.FindAllStringSubmatchIndex(protected, -1)
	if len(matches) == 0 {
		// no obvious sentence boundaries -> return whole text
		return []string{text}
	}

	out := make([]string, 0, len(matches))
	for _, idx := range matches {
		// idx[2]: start of group 1 (the sentence inc. delimiter), idx[3]: end
		s := protected[idx[2]:idx[3]]
		s = strings.ReplaceAll(s, "._ABBR_", ".") // restore protected dots
		out = append(out, s)
	}
	return out
}

func splitIntoWordChunksRuneAware(s string, maxChars int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}

	var chunks []string
	var b strings.Builder
	runes := 0

	for i, w := range words {
		need := utf8.RuneCountInString(w)
		sep := 0
		if b.Len() > 0 {
			sep = 1 // space
		}
		if runes+sep+need <= maxChars || b.Len() == 0 {
			if sep == 1 {
				b.WriteByte(' ')
				runes++
			}
			b.WriteString(w)
			runes += need
		} else {
			chunks = append(chunks, b.String())
			b.Reset()
			runes = 0
			b.WriteString(w)
			runes = need
		}
		// ensure progress even for a single very long word
		if i == len(words)-1 && b.Len() > 0 {
			chunks = append(chunks, b.String())
		}
	}
	return chunks
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
