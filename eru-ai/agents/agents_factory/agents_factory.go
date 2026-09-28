package agents_factory

import (
	agents "github.com/eru-os/eru/eru-ai/agents"
	_ "github.com/eru-os/eru/eru-ai/agents/orchestrator"
	_ "github.com/eru-os/eru/eru-ai/agents/reasoning_agents"
	_ "github.com/eru-os/eru/eru-ai/agents/reflex_agents"
)

func GetAgent(agentType string) agents.AgentI {
	return agents.NewAgent(agentType)
}
