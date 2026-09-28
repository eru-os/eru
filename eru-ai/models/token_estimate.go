package models

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/tiktoken-go/tokenizer"
)

type TokenCountResult struct {
	Tokens       int           `json:"tokens"`
	BaseTokens   int           `json:"base_tokens"`
	Provider     string        `json:"provider"`
	Model        string        `json:"model"`
	Encoding     string        `json:"encoding"`
	Multiplier   float64       `json:"multiplier"`
	Approximate  bool          `json:"approximate"`
	CostEstimate *CostEstimate `json:"cost_estimate,omitempty"`
}

type CostEstimate struct {
	Currency         string  `json:"currency"`
	Direction        string  `json:"direction"`
	PricePer1M       float64 `json:"price_per_1m"`
	InputPricePer1M  float64 `json:"input_price_per_1m"`
	OutputPricePer1M float64 `json:"output_price_per_1m"`
	EstimatedCost    float64 `json:"estimated_cost"`
	Note             string  `json:"note,omitempty"`
}

// providerTokenMultiplier scales the tiktoken (OpenAI) reference count to
// roughly approximate other providers' tokenizers. These are coarse factors
// good enough for a cost estimate only.
var providerTokenMultiplier = map[string]float64{
	"OPENAI":    1.0,
	"ANTHROPIC": 1.15,
	"GEMINI":    0.95,
	"BEDROCK":   1.10,
}

type modelPrice struct {
	inputPer1M  float64
	outputPer1M float64
}

// modelPricing is an approximate USD-per-1M-tokens table matched by longest
// model-name prefix. Prices change over time - edit here as needed.
var modelPricing = []struct {
	prefix string
	price  modelPrice
}{
	{"gpt-4o-mini", modelPrice{0.15, 0.60}},
	{"gpt-4o", modelPrice{2.50, 10.00}},
	{"gpt-4.1-mini", modelPrice{0.40, 1.60}},
	{"gpt-4.1-nano", modelPrice{0.10, 0.40}},
	{"gpt-4.1", modelPrice{2.00, 8.00}},
	{"gpt-4-turbo", modelPrice{10.00, 30.00}},
	{"gpt-4", modelPrice{30.00, 60.00}},
	{"gpt-3.5-turbo", modelPrice{0.50, 1.50}},
	{"o1-mini", modelPrice{1.10, 4.40}},
	{"o1", modelPrice{15.00, 60.00}},
	{"o3-mini", modelPrice{1.10, 4.40}},
	{"o3", modelPrice{2.00, 8.00}},
	{"o4-mini", modelPrice{1.10, 4.40}},
	// Anthropic Claude - base input/output per MTok, per
	// https://platform.claude.com/docs/en/about-claude/pricing (as of 2026-07-08).
	// Longest-prefix match wins, so specific IDs override the generic fallbacks.
	{"claude-fable-5", modelPrice{10.00, 50.00}},
	{"claude-mythos-5", modelPrice{10.00, 50.00}},
	{"claude-mythos", modelPrice{10.00, 50.00}},
	{"claude-opus-4-8", modelPrice{5.00, 25.00}},
	{"claude-opus-4-7", modelPrice{5.00, 25.00}},
	{"claude-opus-4-6", modelPrice{5.00, 25.00}},
	{"claude-opus-4-5", modelPrice{5.00, 25.00}},
	{"claude-opus-4-1", modelPrice{15.00, 75.00}},
	{"claude-opus-4-0", modelPrice{15.00, 75.00}},
	{"claude-opus-4", modelPrice{15.00, 75.00}},
	{"claude-3-opus", modelPrice{15.00, 75.00}},
	{"claude-opus", modelPrice{5.00, 25.00}},
	// Sonnet 5 standard pricing; intro $2/$10 applies through 2026-08-31.
	{"claude-sonnet-5", modelPrice{3.00, 15.00}},
	{"claude-sonnet-4-6", modelPrice{3.00, 15.00}},
	{"claude-sonnet-4-5", modelPrice{3.00, 15.00}},
	{"claude-sonnet-4", modelPrice{3.00, 15.00}},
	{"claude-3-7-sonnet", modelPrice{3.00, 15.00}},
	{"claude-3-5-sonnet", modelPrice{3.00, 15.00}},
	{"claude-3-sonnet", modelPrice{3.00, 15.00}},
	{"claude-sonnet", modelPrice{3.00, 15.00}},
	{"claude-haiku-4-5", modelPrice{1.00, 5.00}},
	{"claude-3-5-haiku", modelPrice{0.80, 4.00}},
	{"claude-3-haiku", modelPrice{0.25, 1.25}},
	{"claude-haiku", modelPrice{1.00, 5.00}},
	{"gemini-1.5-flash", modelPrice{0.075, 0.30}},
	{"gemini-1.5-pro", modelPrice{1.25, 5.00}},
	{"gemini-2.0-flash", modelPrice{0.10, 0.40}},
	{"gemini-2.5-pro", modelPrice{1.25, 10.00}},
	{"gemini-2.5-flash", modelPrice{0.30, 2.50}},
}

func priceForModel(model string) (modelPrice, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return modelPrice{}, false
	}
	best := modelPrice{}
	bestLen := 0
	found := false
	for _, mp := range modelPricing {
		if strings.Contains(m, mp.prefix) && len(mp.prefix) > bestLen {
			best = mp.price
			bestLen = len(mp.prefix)
			found = true
		}
	}
	return best, found
}

func encodingForModel(model string) tokenizer.Encoding {
	m := strings.ToLower(model)
	if strings.HasPrefix(m, "gpt-3.5") || strings.HasPrefix(m, "gpt-35") ||
		(strings.HasPrefix(m, "gpt-4") && !strings.Contains(m, "4o") && !strings.HasPrefix(m, "gpt-4.1")) {
		return tokenizer.Cl100kBase
	}
	return tokenizer.O200kBase
}

func collectStrings(v interface{}, sb *strings.Builder) {
	switch t := v.(type) {
	case string:
		sb.WriteString(t)
		sb.WriteByte('\n')
	case json.Number:
		sb.WriteString(t.String())
		sb.WriteByte('\n')
	case bool:
		if t {
			sb.WriteString("true\n")
		} else {
			sb.WriteString("false\n")
		}
	case []interface{}:
		for _, e := range t {
			collectStrings(e, sb)
		}
	case map[string]interface{}:
		for _, e := range t {
			collectStrings(e, sb)
		}
	}
}

// EstimateTokens counts tokens in an arbitrary JSON payload using a tiktoken
// reference tokenizer, scales it per provider, and derives an approximate cost.
// provider, model and direction ("input" default, or "output") are optional.
func EstimateTokens(ctx context.Context, payload []byte, provider string, model string, direction string) (result TokenCountResult, err error) {
	logs.WithContext(ctx).Debug("EstimateTokens - Start")

	var data interface{}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	if err = dec.Decode(&data); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	var sb strings.Builder
	collectStrings(data, &sb)

	enc := encodingForModel(model)
	codec, cErr := tokenizer.Get(enc)
	if cErr != nil {
		err = cErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	base, cntErr := codec.Count(sb.String())
	if cntErr != nil {
		err = cntErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}

	provider = strings.ToUpper(strings.TrimSpace(provider))
	mult, ok := providerTokenMultiplier[provider]
	if !ok {
		mult = 1.0
	}
	tokens := int(math.Round(float64(base) * mult))

	result = TokenCountResult{
		Tokens:      tokens,
		BaseTokens:  base,
		Provider:    provider,
		Model:       model,
		Encoding:    string(enc),
		Multiplier:  mult,
		Approximate: true,
	}

	if price, found := priceForModel(model); found {
		if direction != "output" {
			direction = "input"
		}
		rate := price.inputPer1M
		if direction == "output" {
			rate = price.outputPer1M
		}
		result.CostEstimate = &CostEstimate{
			Currency:         "USD",
			Direction:        direction,
			PricePer1M:       rate,
			InputPricePer1M:  price.inputPer1M,
			OutputPricePer1M: price.outputPer1M,
			EstimatedCost:    float64(tokens) / 1_000_000.0 * rate,
			Note:             "approximate cost for estimation only",
		}
	}

	return
}
