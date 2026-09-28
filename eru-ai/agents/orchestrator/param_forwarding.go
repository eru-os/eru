package orchestrator

import (
	"regexp"
	"sort"
	"strings"

	"github.com/eru-os/eru/eru-ai/agents"
)

var paramsObjectPattern = regexp.MustCompile(`"params"\s*:\s*\{`)

func autoForwardParams(plan map[string]interface{}, allowedAgents []agents.DiscoveredAgent, cc codeContext) {
	if len(cc.ForwardParams) == 0 || len(allowedAgents) == 0 {
		return
	}
	byName := make(map[string]agents.DiscoveredAgent, len(allowedAgents))
	for _, agent := range allowedAgents {
		byName[agent.AgentName] = agent
	}
	steps, ok := plan["func_steps"].(map[string]interface{})
	if !ok {
		return
	}
	forwardIntoSteps(steps, byName, cc.ForwardParams)
}

func forwardIntoSteps(steps map[string]interface{}, byName map[string]agents.DiscoveredAgent, forward []string) {
	names := make([]string, 0, len(steps))
	for name := range steps {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		step, ok := steps[name].(map[string]interface{})
		if !ok {
			continue
		}
		agentName, _ := step["agent_name"].(string)
		if agent, known := byName[agentName]; known {
			template, _ := step["transform_request"].(string)
			if injectable(template) {
				for _, param := range forward {
					if !containsString(agent.ParamKeys(), param) {
						continue
					}
					if strings.Contains(template, param) {
						continue
					}
					template = injectParam(template, param)
				}
				step["transform_request"] = template
			}
		}
		if nested, ok := step["func_steps"].(map[string]interface{}); ok {
			forwardIntoSteps(nested, byName, forward)
		}
	}
}

func injectable(template string) bool {
	trimmed := strings.TrimSpace(template)
	if len(trimmed) < 2 || strings.HasPrefix(trimmed, "{{") {
		return false
	}
	return strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")
}

func injectParam(template string, param string) string {
	entry := `"` + param + `": {{stringify ` + userRequestRoot + `.params.` + param + `}}`

	if location := paramsObjectPattern.FindStringIndex(template); location != nil {
		insertAt := location[1]
		rest := strings.TrimLeft(template[insertAt:], " \t\n")
		if strings.HasPrefix(rest, "}") {
			return template[:insertAt] + entry + template[insertAt:]
		}
		return template[:insertAt] + entry + ", " + template[insertAt:]
	}

	closingBrace := strings.LastIndex(template, "}")
	if closingBrace < 0 {
		return template
	}
	body := strings.TrimSpace(template[strings.Index(template, "{")+1 : closingBrace])
	separator := ", "
	if body == "" {
		separator = ""
	}
	return template[:closingBrace] + separator + `"params": {` + entry + `}` + template[closingBrace:]
}
