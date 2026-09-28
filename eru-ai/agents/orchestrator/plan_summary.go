package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	agents "github.com/eru-os/eru/eru-ai/agents"
	models "github.com/eru-os/eru/eru-ai/models"
	functions "github.com/eru-os/eru/eru-functions/functions"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

type planStepSummary struct {
	Step        string            `json:"step"`
	AgentName   string            `json:"agent_name,omitempty"`
	ToolName    string            `json:"tool_name,omitempty"`
	ToolAction  string            `json:"tool_action,omitempty"`
	WaitFor     string            `json:"wait_for,omitempty"`
	Async       bool              `json:"async,omitempty"`
	Loop        bool              `json:"loop,omitempty"`
	Conditional bool              `json:"conditional,omitempty"`
	Steps       []planStepSummary `json:"steps,omitempty"`
}

type planSummary struct {
	FuncCategoryName string            `json:"func_category_name,omitempty"`
	FuncGroupName    string            `json:"func_group_name,omitempty"`
	StepCount        int               `json:"step_count"`
	Steps            []planStepSummary `json:"steps"`
}

// summarizePlan reduces a generated FuncGroup to the step graph the client needs
// for progress display: what runs, in what order, and whether siblings run in
// parallel. Templates, tenant ids and conditions are deliberately left out —
// they are internal orchestration detail. Built in code, no model call.
func summarizePlan(plan map[string]interface{}) (planSummary, error) {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return planSummary{}, err
	}
	var funcGroup functions.FuncGroup
	if err := json.Unmarshal(planJSON, &funcGroup); err != nil {
		return planSummary{}, err
	}
	steps := summarizeSteps(funcGroup.FuncSteps)
	return planSummary{
		FuncCategoryName: funcGroup.FuncCategoryName,
		FuncGroupName:    funcGroup.FuncGroupName,
		StepCount:        countSteps(steps),
		Steps:            steps,
	}, nil
}

func summarizeSteps(steps map[string]*functions.FuncStep) []planStepSummary {
	stepKeys := make([]string, 0, len(steps))
	for stepKey := range steps {
		stepKeys = append(stepKeys, stepKey)
	}
	sort.Strings(stepKeys)
	summaries := make([]planStepSummary, 0, len(stepKeys))
	for _, stepKey := range stepKeys {
		step := steps[stepKey]
		if step == nil {
			continue
		}
		summaries = append(summaries, planStepSummary{
			Step:        stepKey,
			AgentName:   step.AgentName,
			ToolName:    step.ToolName,
			ToolAction:  step.ToolAction,
			WaitFor:     step.WaitFor,
			Async:       step.Async,
			Loop:        step.LoopVariable != "",
			Conditional: step.Condition != "",
			Steps:       summarizeSteps(step.FuncSteps),
		})
	}
	return summaries
}

func countSteps(steps []planStepSummary) int {
	count := 0
	for _, step := range steps {
		count = count + 1 + countSteps(step.Steps)
	}
	return count
}

// clientTraces returns the traces to hand back to the caller. The
// structured_output tool input carries the whole FuncGroup (and, for sub-agents,
// their full structured answer, which is already delivered as an action), so it
// is dropped unless raw output was requested. The traces saved to the
// conversation keep everything.
func clientTraces(ctx context.Context, traces []models.StepTrace) []models.StepTrace {
	if len(traces) == 0 || agents.RawOutputEnabled(ctx) {
		return traces
	}
	sanitized := make([]models.StepTrace, len(traces))
	copy(sanitized, traces)
	for i := range sanitized {
		if sanitized[i].ToolName == models.TerminalToolStructuredOutput {
			sanitized[i].ToolInput = nil
		}
		sanitized[i].Thinking = truncateTraceText(sanitized[i].Thinking)
		sanitized[i].Content = truncateTraceText(sanitized[i].Content)
	}
	return sanitized
}

// traceTextLimit caps how much of a single trace's reasoning text goes on the
// wire. Merging sub-agent traces multiplies the volume of thinking text in a
// response, so the client copy is trimmed while the conversation keeps the whole
// thing; ?raw=true returns it untrimmed.
const traceTextLimit = 4000

const traceTruncationNote = "\n... [truncated - full text is kept in the saved conversation; call with ?raw=true for the untrimmed trace]"

func truncateTraceText(text string) string {
	if len(text) <= traceTextLimit {
		return text
	}
	return text[:traceTextLimit] + traceTruncationNote
}

// terminalStepName returns the name of the step that produced the FuncGroup's
// final response, so a forwarded output is attributed to the agent/tool that
// actually generated it rather than to the orchestrator. It returns "" when the
// plan has more than one leaf step, since the producer is then ambiguous.
func terminalStepName(ctx context.Context, plan map[string]interface{}) string {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return ""
	}
	var funcGroup functions.FuncGroup
	if err := json.Unmarshal(planJSON, &funcGroup); err != nil {
		return ""
	}
	leaves := collectLeafSteps(funcGroup.FuncSteps, nil)
	if len(leaves) != 1 {
		logs.WithContext(ctx).Info(fmt.Sprint("terminalStepName - plan has ", len(leaves), " leaf step(s), attributing output to the orchestrator"))
		return ""
	}
	return leaves[0]
}

func collectLeafSteps(steps map[string]*functions.FuncStep, leaves []string) []string {
	for _, stepKey := range sortedStepKeys(steps) {
		step := steps[stepKey]
		if step == nil {
			continue
		}
		if len(step.FuncSteps) == 0 {
			leaves = append(leaves, stepDisplayName(stepKey, step))
			continue
		}
		leaves = collectLeafSteps(step.FuncSteps, leaves)
	}
	return leaves
}

func stepDisplayName(stepKey string, step *functions.FuncStep) string {
	if step.AgentName != "" {
		return step.AgentName
	}
	if step.ToolName != "" {
		return step.ToolName
	}
	return stepKey
}
