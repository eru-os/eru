package chunking

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type ParagraphChunker struct{}

func (pc *ParagraphChunker) GetName() string { return "paragraph" }

var paragraphRe = regexp.MustCompile(`\n{2,}`) // 2+ newlines = paragraph break

func (pc *ParagraphChunker) ChunkText(text string, config ChunkingConfig) ([]string, error) {
	// Choose limit
	maxChars := config.MaxChars
	if maxChars <= 0 {
		if config.MaxTokens > 0 {
			maxChars = config.MaxTokens * 4 // heuristic
		} else {
			maxChars = DefaultChunkingConfig().MaxChars // sensible default
		}
	}
	overlapChars := config.OverlapChars
	if overlapChars < 0 {
		overlapChars = 0
	}

	// Normalize newlines
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// Split to paragraphs (keep meaningful content)
	rawParas := paragraphRe.Split(text, -1)
	paras := make([]string, 0, len(rawParas))
	for _, p := range rawParas {
		p = strings.TrimSpace(p)
		if p != "" {
			paras = append(paras, p)
		}
	}
	if len(paras) == 0 {
		return nil, nil
	}

	// Helper chunkers for fallback
	sent := &SentenceChunker{}

	// Ensure no single paragraph is above maxChars; if so, split it first.
	normalized := make([]string, 0, len(paras))
	for _, p := range paras {
		if utf8.RuneCountInString(p) <= maxChars {
			normalized = append(normalized, p)
			continue
		}
		// First try sentence-level splits within this paragraph
		chunks, err := sent.ChunkText(p, config)
		if err != nil || len(chunks) == 0 {
			normalized = append(normalized, p) // fallback to raw if something odd happens
			continue
		}
		normalized = append(normalized, chunks...)
	}

	// Pack paragraphs into chunks under the limit, with overlap
	var out []string
	var b strings.Builder
	curRunes := 0

	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, strings.TrimSpace(b.String()))
		b.Reset()
		curRunes = 0
	}

	for i := 0; i < len(normalized); i++ {
		p := normalized[i]
		psz := utf8.RuneCountInString(p)
		sep := 0
		if b.Len() > 0 {
			sep = 2 // "\n\n" between paragraphs
		}

		if curRunes+sep+psz <= maxChars || b.Len() == 0 {
			if sep == 2 {
				b.WriteString("\n\n")
				curRunes += 2
			}
			b.WriteString(p)
			curRunes += psz
			continue
		}

		// flush current chunk
		flush()

		// add overlap from previous chunk tail (by characters)
		if overlapChars > 0 && i > 0 {
			backRunes := 0
			j := i - 1
			// walk backwards over prior paragraphs to accumulate overlap
			for j >= 0 && backRunes < overlapChars {
				part := normalized[j]
				pr := utf8.RuneCountInString(part)
				// prepend in correct order later; collect indexes for simplicity
				j--
				backRunes += pr + 2 // approximate including sep
			}
			// write overlap slice [j+1 .. i-1]
			for k := j + 1; k <= i-1; k++ {
				if b.Len() > 0 {
					b.WriteString("\n\n")
					curRunes += 2
				}
				b.WriteString(normalized[k])
				curRunes += utf8.RuneCountInString(normalized[k])
			}
		}

		// now add current paragraph (ensure progress)
		if b.Len() > 0 {
			b.WriteString("\n\n")
			curRunes += 2
		}
		b.WriteString(p)
		curRunes += psz
	}
	flush()

	// Final safeguard: if any chunk somehow exceeds maxChars, sentence/word split it.
	final := make([]string, 0, len(out))
	for _, c := range out {
		if utf8.RuneCountInString(c) <= maxChars {
			final = append(final, c)
			continue
		}
		parts, err := sent.ChunkText(c, config)
		if err != nil || len(parts) == 0 {
			final = append(final, c)
		} else {
			final = append(final, parts...)
		}
	}
	return final, nil
}
