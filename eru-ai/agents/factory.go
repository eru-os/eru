package agents

import (
	"fmt"
	"sort"
	"sync"
)

var (
	factoryMu sync.RWMutex
	factory   = map[string]func() AgentI{}
)

func RegisterAgentType(agentType string, newAgent func() AgentI) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	if _, exists := factory[agentType]; exists {
		panic(fmt.Sprintf("agent type %s registered twice", agentType))
	}
	factory[agentType] = newAgent
}

func NewAgent(agentType string) AgentI {
	factoryMu.RLock()
	newAgent, ok := factory[agentType]
	factoryMu.RUnlock()
	if !ok {
		return new(Agent)
	}
	return newAgent()
}

func AgentTypes() []string {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	agentTypes := make([]string, 0, len(factory))
	for agentType := range factory {
		agentTypes = append(agentTypes, agentType)
	}
	sort.Strings(agentTypes)
	return agentTypes
}
