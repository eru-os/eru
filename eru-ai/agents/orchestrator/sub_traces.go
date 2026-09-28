package orchestrator

import (
	"encoding/json"
	"sort"

	models "github.com/eru-os/eru/eru-ai/models"
	functions "github.com/eru-os/eru/eru-functions/functions"
)

// collectSubAgentTraces pulls each completed step's own reasoning trace out of its
// response body. A sub-agent returns its traces in the body it sends back, which
// lands in .ResVars.<step>.Body.traces - without this they are dropped and the
// final message shows the orchestrator's reasoning with a hole where every
// sub-agent ran. This is the durable counterpart to the live stream: it is
// complete, it survives into the saved conversation, and it is the only copy a
// non-streaming caller (plain POST, MCP, A2A) ever sees.
func collectSubAgentTraces(funcVarsMap map[string]functions.FuncTemplateVars) []models.StepTrace {
	// Every entry in funcVarsMap carries the whole ResVars map, so a step would
	// otherwise be harvested once per entry.
	seen := make(map[string]bool)
	var collected []models.StepTrace
	for _, fv := range funcVarsMap {
		for stepName, tv := range fv.ResVars {
			if stepName == "" || seen[stepName] || tv == nil || tv.Body == nil {
				continue
			}
			seen[stepName] = true
			traces, agentName := stepTraces(tv.Body)
			for i := range traces {
				traces[i].Step = stepName
				if traces[i].Agent == "" {
					if agentName != "" {
						traces[i].Agent = agentName
					} else {
						traces[i].Agent = stepName
					}
				}
			}
			collected = append(collected, traces...)
		}
	}
	sort.SliceStable(collected, func(i, j int) bool {
		return collected[i].Timestamp.Before(collected[j].Timestamp)
	})
	return collected
}

func stepTraces(body interface{}) ([]models.StepTrace, string) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, ""
	}
	rawTraces, ok := bodyMap["traces"]
	if !ok {
		return nil, ""
	}
	traceBytes, err := json.Marshal(rawTraces)
	if err != nil {
		return nil, ""
	}
	var traces []models.StepTrace
	if err := json.Unmarshal(traceBytes, &traces); err != nil {
		return nil, ""
	}
	return traces, bodyActionName(bodyMap)
}

func bodyActionName(bodyMap map[string]interface{}) string {
	actions, ok := bodyMap["actions"].([]interface{})
	if !ok || len(actions) == 0 {
		return ""
	}
	action, ok := actions[0].(map[string]interface{})
	if !ok {
		return ""
	}
	name, _ := action["action_name"].(string)
	return name
}

// labelOwnTraces attributes the traces the orchestrator produced itself; sub-agent
// traces arrive already labelled and are left alone.
func labelOwnTraces(traces []models.StepTrace, agentName string) []models.StepTrace {
	for i := range traces {
		if traces[i].Agent == "" {
			traces[i].Agent = agentName
		}
	}
	return traces
}
