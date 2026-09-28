package reasoning_agents

import (
	"context"
	"fmt"
	"strings"

	tools "github.com/eru-os/eru/eru-ai/tools"
	utility "github.com/eru-os/eru/eru-ai/tools/utility"
	eru_models "github.com/eru-os/eru/eru-models"
)

const (
	RecallMemoryToolName = "recall_memory"
	SaveMemoryToolName   = "save_memory"
	maxRecall            = 10
)

// memoryTools are what an agent with a memory_store is given to use it. Memory
// is the model's to consult and to write, not something done to every turn
// behind its back: saving everything fills the store with noise, and recalling
// on every request puts stale facts in front of questions they have nothing to
// do with.
func (ra *ReasoningAgent) memoryTools(ctx context.Context) map[string]tools.Tooling {
	return map[string]tools.Tooling{
		RecallMemoryToolName: utility.NewFuncTool(ctx, RecallMemoryToolName,
			"Search your long-term memory for things learned in earlier conversations, by meaning. Returns the closest memories with their content.",
			eru_models.JSONSchema{
				Type: "object",
				Properties: map[string]eru_models.JSONSchema{
					"query": {Type: "string", Description: "What you want to remember, in plain words."},
					"top_k": {Type: "integer", Description: fmt.Sprintf("How many memories to return, at most %d. Default 5.", maxRecall)},
				},
				Required: []string{"query"},
			},
			func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
				query, _ := params["query"].(string)
				if strings.TrimSpace(query) == "" {
					return nil, fmt.Errorf("recall_memory needs a query")
				}
				topK := 5
				if n, ok := params["top_k"].(float64); ok && n > 0 {
					topK = int(n)
				}
				if topK > maxRecall {
					topK = maxRecall
				}
				records, err := ra.RecallMemory(ctx, query, topK)
				if err != nil {
					return nil, err
				}
				memories := make([]map[string]interface{}, 0, len(records))
				for _, r := range records {
					entry := map[string]interface{}{"content": r.Content}
					if at, ok := r.Metadata["created_at"]; ok {
						entry["saved_at"] = at
					}
					memories = append(memories, entry)
				}
				return map[string]interface{}{"memories": memories, "count": len(memories)}, nil
			}),
		SaveMemoryToolName: utility.NewFuncTool(ctx, SaveMemoryToolName,
			"Save one durable fact to long-term memory so it is available in later conversations. One fact per call, written to make sense on its own.",
			eru_models.JSONSchema{
				Type: "object",
				Properties: map[string]eru_models.JSONSchema{
					"content": {Type: "string", Description: "The fact, in one or two self-contained sentences."},
					"topic":   {Type: "string", Description: "A short label for what it is about, e.g. customer preference."},
				},
				Required: []string{"content"},
			},
			func(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
				content, _ := params["content"].(string)
				metadata := map[string]interface{}{}
				if topic, ok := params["topic"].(string); ok && topic != "" {
					metadata["topic"] = topic
				}
				if err := ra.SaveToMemory(ctx, content, metadata); err != nil {
					return nil, err
				}
				return map[string]interface{}{"saved": true}, nil
			}),
	}
}
