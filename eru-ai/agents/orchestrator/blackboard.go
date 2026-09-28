package orchestrator

import (
	"context"
	"sync"
	"time"
)

type contextKey string

const BlackboardContextKey contextKey = "orchestrator_blackboard"

type StateChange struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	AgentName string      `json:"agent_name"`
	Timestamp time.Time   `json:"timestamp"`
}

type Blackboard struct {
	mu      sync.RWMutex
	state   map[string]interface{}
	changes []StateChange
}

func NewBlackboard() *Blackboard {
	return &Blackboard{
		state:   make(map[string]interface{}),
		changes: []StateChange{},
	}
}

func (bb *Blackboard) Set(key string, value interface{}, agentName string) {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	bb.state[key] = value
	bb.changes = append(bb.changes, StateChange{
		Key:       key,
		Value:     value,
		AgentName: agentName,
		Timestamp: time.Now(),
	})
}

func (bb *Blackboard) Get(key string) (interface{}, bool) {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	val, ok := bb.state[key]
	return val, ok
}

func (bb *Blackboard) GetAll() map[string]interface{} {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	snapshot := make(map[string]interface{}, len(bb.state))
	for k, v := range bb.state {
		snapshot[k] = v
	}
	return snapshot
}

func (bb *Blackboard) GetChanges() []StateChange {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	out := make([]StateChange, len(bb.changes))
	copy(out, bb.changes)
	return out
}

func (bb *Blackboard) Delete(key string, agentName string) {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	delete(bb.state, key)
	bb.changes = append(bb.changes, StateChange{
		Key:       key,
		Value:     nil,
		AgentName: agentName,
		Timestamp: time.Now(),
	})
}

func WithBlackboard(ctx context.Context, bb *Blackboard) context.Context {
	return context.WithValue(ctx, BlackboardContextKey, bb)
}

func GetBlackboard(ctx context.Context) *Blackboard {
	if bb, ok := ctx.Value(BlackboardContextKey).(*Blackboard); ok {
		return bb
	}
	return nil
}
