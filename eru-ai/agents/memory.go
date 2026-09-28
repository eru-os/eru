package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// MemoryRecord is one thing an agent remembers.
type MemoryRecord struct {
	Id       string                 `json:"id"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Score    float64                `json:"score,omitempty"`
}

// MemoryStore is long-term memory: text saved in one conversation and found
// again by meaning in another.
//
// It is an interface rather than a vector store because the vector store alone
// cannot do the job. Embedding happens in the module store, which knows the
// tenant's embedding model; an agent handed the raw store saved records with no
// vectors and searched with a key the embedder never reads, so memory "worked"
// in unit tests and never once returned anything. The module store supplies the
// implementation when it fetches the agent.
type MemoryStore interface {
	Search(ctx context.Context, namespace string, query string, topK int) ([]MemoryRecord, error)
	Save(ctx context.Context, namespace string, record MemoryRecord) error
}

// SetMemory attaches long-term memory.
func (agent *Agent) SetMemory(store MemoryStore) {
	agent.Memory = store
}

// HasMemory reports whether this agent can recall and save.
func (agent *Agent) HasMemory() bool {
	return agent.Memory != nil
}

// memoryNamespace is where this agent's memories live. Agents that share a
// namespace share what they remember.
func (agent *Agent) memoryNamespace() string {
	if ns := strings.TrimSpace(agent.MemoryNamespace); ns != "" {
		return ns
	}
	return agent.AgentName
}

// RecallMemory finds what the agent remembered that is closest in meaning to
// the query.
func (agent *Agent) RecallMemory(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if agent.Memory == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}
	records, err := agent.Memory.Search(ctx, agent.memoryNamespace(), query, topK)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to recall memory: %v", err))
		return nil, err
	}
	return records, nil
}

// SaveToMemory stores one durable fact.
func (agent *Agent) SaveToMemory(ctx context.Context, content string, metadata map[string]interface{}) error {
	if agent.Memory == nil {
		return nil
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["agent_name"] = agent.AgentName
	metadata["created_at"] = time.Now().UTC().Format(time.RFC3339)
	record := MemoryRecord{
		Id:       fmt.Sprintf("mem_%s_%d", agent.AgentName, time.Now().UnixNano()),
		Content:  content,
		Metadata: metadata,
	}
	if err := agent.Memory.Save(ctx, agent.memoryNamespace(), record); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to save to memory: %v", err))
		return err
	}
	return nil
}

// MemoryGuidance is appended to the system prompt of an agent with memory.
const MemoryGuidance = `

LONG-TERM MEMORY:
You have memory that lasts beyond this conversation. Use recall_memory at the start of a task when something learned before could matter - a preference, an earlier decision, a correction someone made. Use save_memory for facts worth keeping: stable preferences, decisions and their reasons, corrections to your earlier answers. Save one fact per call, written so it makes sense on its own later. Never save secrets, credentials, or personal data the user did not ask you to keep, and never save what you only guessed.`
