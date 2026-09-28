package orchestrator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/eru-os/eru/eru-functions/functions"
)

// Delegation strategies. The planner parallelises by default - independent
// steps become siblings - so "adaptive" and "parallel" both mean that. Only
// "sequential" changes anything: one step at a time, in order, for work whose
// steps must not overlap (a rate-limited API, writes that depend on reads the
// planner cannot see).
const (
	StrategyAdaptive   = "adaptive"
	StrategySequential = "sequential"
	StrategyParallel   = "parallel"
)

const sequentialGuidance = `

DELEGATION STRATEGY: SEQUENTIAL - this overrides "parallel is the default" above.
This orchestrator runs one step at a time. Every func_steps map in the plan holds exactly ONE step: nest each step inside the one before it, in the order they must run. Never place two steps as siblings, and set "loop_in_parallel": false on every loop.`

// strategyGuidance is what the planning prompt adds for a strategy.
func strategyGuidance(strategy string) string {
	if strings.EqualFold(strategy, StrategySequential) {
		return sequentialGuidance
	}
	return ""
}

// validateSequentialPlan holds a plan to the sequential strategy. The prompt
// asks for it; this makes it true, because a planner told "parallel is the
// default" at length will still reach for siblings.
func validateSequentialPlan(plan map[string]interface{}) []planIssue {
	raw, err := json.Marshal(plan)
	if err != nil {
		return nil
	}
	var group functions.FuncGroup
	if err := json.Unmarshal(raw, &group); err != nil {
		return nil
	}
	var issues []planIssue
	var walk func(path string, steps map[string]*functions.FuncStep)
	walk = func(path string, steps map[string]*functions.FuncStep) {
		keys := make([]string, 0, len(steps))
		for k := range steps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 1 {
			where := path
			if where == "" {
				where = "func_steps"
			}
			issues = append(issues, planIssue{Field: where, Err: fmt.Sprintf(
				"steps %s are siblings and would run at the same time, but this orchestrator runs sequentially - nest each step inside the one it follows",
				strings.Join(keys, ", "))})
		}
		for _, k := range keys {
			step := steps[k]
			if step == nil {
				continue
			}
			stepPath := k
			if path != "" {
				stepPath = path + "." + k
			}
			if step.LoopInParallel {
				issues = append(issues, planIssue{StepPath: stepPath, Field: "loop_in_parallel",
					Err: "this orchestrator runs sequentially - set loop_in_parallel to false"})
			}
			walk(stepPath, step.FuncSteps)
		}
	}
	walk("", group.FuncSteps)
	return issues
}
