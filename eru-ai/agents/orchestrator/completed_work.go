package orchestrator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	functions "github.com/eru-os/eru/eru-functions/functions"
)

// When a plan fails part way, the steps that already ran are still good work.
// Re-planning used to throw all of it away: the new plan re-ran every step from
// the top, which on a page-building plan means paying for the whole generation
// again to fix something that failed after it. The outputs are right there in
// the executor's ResVars.
//
// So a replan is told what already succeeded and asked to plan only what is
// left, and those outputs are handed to the next execution so the references
// still resolve.

// completedWork is the steps of a failed run that produced a usable result.
type completedWork struct {
	// Steps are the step keys that succeeded, in a stable order.
	Steps []string
	// ResVars are their results, in the shape the executor takes back.
	ResVars map[string]*functions.TemplateVars
}

func (c completedWork) any() bool { return len(c.Steps) > 0 }

// collectCompletedWork picks out the steps that finished with a body and no
// error. A step that asked a question is deliberately NOT treated as complete -
// it is paused, and the clarification path owns it.
func collectCompletedWork(plan map[string]interface{}, funcVarsMap map[string]functions.FuncTemplateVars) completedWork {
	resVars := extractResVars(funcVarsMap)
	planSteps, _ := plan["func_steps"].(map[string]interface{})

	done := completedWork{ResVars: map[string]*functions.TemplateVars{}}
	for stepKey, vars := range resVars {
		if vars == nil || vars.Body == nil {
			continue
		}
		if _, isPlanStep := planStep(planSteps, stepKey); !isPlanStep {
			continue
		}
		if _, paused := questionInBody(vars.Body); paused {
			continue
		}
		if bodyLooksFailed(vars.Body) {
			continue
		}
		done.Steps = append(done.Steps, stepKey)
		done.ResVars[stepKey] = vars
	}
	sort.Strings(done.Steps)
	return done
}

// bodyLooksFailed spots a step whose body is an error envelope rather than a
// result. A step that returned an error has not done any work worth keeping.
func bodyLooksFailed(body interface{}) bool {
	object, ok := body.(map[string]interface{})
	if !ok {
		return false
	}
	for _, key := range []string{"error", "err"} {
		switch value := object[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" && strings.TrimSpace(value) != "-" {
				return true
			}
		case map[string]interface{}:
			if len(value) > 0 {
				return true
			}
		}
	}
	return false
}

// briefing is what the replanner is told about work it does not need to redo.
func (c completedWork) briefing() string {
	if !c.any() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nTHESE STEPS ALREADY RAN SUCCESSFULLY - DO NOT PLAN THEM AGAIN:\n")
	for _, step := range c.Steps {
		fmt.Fprintf(&b, "- %s\n", step)
		if preview := previewBody(c.ResVars[step]); preview != "" {
			fmt.Fprintf(&b, "    its result: %s\n", preview)
		}
	}
	b.WriteString("\nTheir results are already available to the new plan - read them with " +
		".ResVars.<step> exactly as if that step had just run, or with {\"from\": \"<step>.<field>\"} in a request. " +
		"Leave them out of func_steps entirely and plan only the work that still has to happen. " +
		"Re-running them costs the user the same time and money a second time, for a result that is already in hand.\n")
	return b.String()
}

// previewBody shows enough of a result for the planner to reference it without
// pasting a whole page of JSON into the prompt.
func previewBody(vars *functions.TemplateVars) string {
	if vars == nil || vars.Body == nil {
		return ""
	}
	encoded, err := json.Marshal(vars.Body)
	if err != nil {
		return ""
	}
	const max = 400
	if len(encoded) <= max {
		return string(encoded)
	}
	return string(encoded[:max]) + "... (truncated)"
}
