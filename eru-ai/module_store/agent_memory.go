package module_store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agents "github.com/eru-os/eru/eru-ai/agents"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	vectorstore "github.com/eru-os/eru/eru-vectorstore/vectorstore"
)

// memoryTextKey is the search input the vector stores embed.
const memoryTextKey = "##text##"

// agentMemory is an agent's long-term memory, backed by a tenant vector store
// through SaveVectors and SearchVectors - which is where the tenant's embedding
// model is applied. Handing the agent the raw vector store skipped that step.
type agentMemory struct {
	store     ModuleStoreI
	projectId string
	tenantId  string
	name      string
}

func newAgentMemory(store ModuleStoreI, projectId string, tenantId string, name string) *agentMemory {
	return &agentMemory{store: store, projectId: projectId, tenantId: tenantId, name: name}
}

// namespace keeps memories apart per tenant even when the vector store itself
// is shared from the project: an agent's namespace is its own name by default,
// and the same agent name exists in every tenant.
func (m *agentMemory) namespace(ns string) string {
	return m.tenantId + "__" + ns
}

// embedField is the metadata key the store embeds on save.
func (m *agentMemory) embedField(ctx context.Context) string {
	vs, err := m.store.GetVectorStore(ctx, m.projectId, m.tenantId, m.name, m.store)
	if err == nil && vs != nil {
		if embed, eErr := vs.GetEmbed(ctx); eErr == nil && strings.TrimSpace(embed.Field) != "" {
			return embed.Field
		}
	}
	return "content"
}

func (m *agentMemory) Save(ctx context.Context, ns string, record agents.MemoryRecord) error {
	if strings.TrimSpace(record.Content) == "" {
		return errors.New("nothing to remember: content is empty")
	}
	metadata := map[string]interface{}{}
	for k, v := range record.Metadata {
		metadata[k] = v
	}
	metadata["content"] = record.Content
	metadata[m.embedField(ctx)] = record.Content
	return m.store.SaveVectors(ctx, vectorstore.VectorRecords{
		Namespace: m.namespace(ns),
		Vectors:   []vectorstore.Vector{{Id: record.Id, Metadata: metadata}},
	}, m.name, m.projectId, m.tenantId, m.store)
}

func (m *agentMemory) Search(ctx context.Context, ns string, query string, topK int) ([]agents.MemoryRecord, error) {
	results, err := m.store.SearchVectors(ctx, vectorstore.VectorRecordsSearch{
		Namespace:      m.namespace(ns),
		TopK:           topK,
		ReturnMetadata: true,
		Inputs:         map[string]string{memoryTextKey: query},
	}, m.name, m.projectId, m.tenantId, m.store)
	if err != nil {
		return nil, err
	}
	out := make([]agents.MemoryRecord, 0, len(results.Records))
	for _, r := range results.Records {
		rec := agents.MemoryRecord{Id: r.Id, Metadata: r.Metadata}
		if c, ok := r.Metadata["content"].(string); ok {
			rec.Content = c
		}
		switch score := r.Metadata["score"].(type) {
		case float64:
			rec.Score = score
		case float32:
			rec.Score = float64(score)
		}
		out = append(out, rec)
	}
	return out, nil
}

// attachMemory resolves an agent's memory_store into a working memory. An agent
// that names a store that does not exist runs without memory and says so in the
// log, rather than failing every request.
func (ms *ModuleStore) attachMemory(ctx context.Context, projectId string, tenantId string, agent agents.AgentI, s ModuleStoreI) {
	nameI, err := agent.GetAttribute(ctx, "memory_store")
	if err != nil {
		return
	}
	name, _ := nameI.(string)
	if strings.TrimSpace(name) == "" {
		return
	}
	if _, err := ms.GetVectorStore(ctx, projectId, tenantId, name, s); err != nil {
		logs.WithContext(ctx).Info(fmt.Sprintf("agent memory store %q not found, running without long-term memory: %v", name, err))
		return
	}
	if holder, ok := agent.(interface{ SetMemory(agents.MemoryStore) }); ok {
		holder.SetMemory(newAgentMemory(s, projectId, tenantId, name))
	}
}
