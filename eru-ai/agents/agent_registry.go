package agents

import (
	"context"
	"sync"
)

type AgentRegistry struct {
	mu     sync.RWMutex
	byName map[string]AgentI
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{byName: make(map[string]AgentI)}
}

func (r *AgentRegistry) Register(_ context.Context, name string, agent AgentI) {
	r.mu.Lock()
	r.byName[name] = agent
	r.mu.Unlock()
}

func (r *AgentRegistry) Get(_ context.Context, name string) AgentI {
	r.mu.RLock()
	a := r.byName[name]
	r.mu.RUnlock()
	return a
}

func (r *AgentRegistry) List(_ context.Context) []string {
	r.mu.RLock()
	names := make([]string, 0, len(r.byName))
	for k := range r.byName {
		names = append(names, k)
	}
	r.mu.RUnlock()
	return names
}
