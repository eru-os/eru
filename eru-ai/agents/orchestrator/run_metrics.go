package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// runMetrics is what one orchestration cost.
//
// Until now the only way to answer "how many repair rounds did that take, and
// where did the time go" was to read the raw log by hand and count lines. That
// makes every improvement unmeasurable: a change that halves plan repairs looks
// exactly like a change that does nothing. These are the numbers that would have
// told us, at a glance, that one prompt in two was dying in plan repair.
type runMetrics struct {
	mu sync.Mutex

	PlanAttempts  int
	PlanRepairs   int
	Replans       int
	ExecAttempts  int
	StepsPlanned  int
	PlanInvalid   []string
	stageStarted  map[string]time.Time
	StageDuration map[string]time.Duration
	started       time.Time
}

func newRunMetrics() *runMetrics {
	return &runMetrics{
		stageStarted:  map[string]time.Time{},
		StageDuration: map[string]time.Duration{},
		started:       time.Now(),
	}
}

func (m *runMetrics) stage(name string) func() {
	if m == nil {
		return func() {}
	}
	m.mu.Lock()
	m.stageStarted[name] = time.Now()
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if started, ok := m.stageStarted[name]; ok {
			m.StageDuration[name] += time.Since(started)
			delete(m.stageStarted, name)
		}
	}
}

func (m *runMetrics) countPlanAttempt() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PlanAttempts++
}

// recordPlanInvalid notes that a plan failed validation, keeping only the field
// each issue was about. The full text is already logged; what is worth counting
// across runs is which KIND of mistake keeps happening.
func (m *runMetrics) recordPlanInvalid(issues []planIssue) {
	if m == nil || len(issues) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PlanRepairs++
	for _, issue := range issues {
		field := issue.Field
		if field == "" {
			field = "plan"
		}
		m.PlanInvalid = append(m.PlanInvalid, field)
	}
}

func (m *runMetrics) countReplan() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Replans++
}

func (m *runMetrics) countExecAttempt(steps int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ExecAttempts++
	if steps > m.StepsPlanned {
		m.StepsPlanned = steps
	}
}

// report writes the run's numbers as one structured line, so a run can be
// compared with the one before it without re-reading the log.
func (m *runMetrics) report(ctx context.Context, agentName string, outcome string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	stages := make([]string, 0, len(m.StageDuration))
	for name, duration := range m.StageDuration {
		stages = append(stages, fmt.Sprintf("%s=%dms", name, duration.Milliseconds()))
	}
	sortStrings(stages)

	invalid := ""
	if len(m.PlanInvalid) > 0 {
		invalid = fmt.Sprintf(" plan_invalid_fields=[%s]", strings.Join(dedupeCounted(m.PlanInvalid), " "))
	}

	logs.WithContext(ctx).Info(fmt.Sprintf(
		"orchestrator run metrics agent=%s outcome=%s total_ms=%d plan_attempts=%d plan_repairs=%d replans=%d exec_attempts=%d steps=%d stages=[%s]%s",
		agentName, outcome, time.Since(m.started).Milliseconds(),
		m.PlanAttempts, m.PlanRepairs, m.Replans, m.ExecAttempts, m.StepsPlanned,
		strings.Join(stages, " "), invalid))
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// dedupeCounted collapses repeats into "field xN", so one badly-worded template
// reported on six steps does not read as six different problems.
func dedupeCounted(values []string) []string {
	counts := map[string]int{}
	order := []string{}
	for _, value := range values {
		if counts[value] == 0 {
			order = append(order, value)
		}
		counts[value]++
	}
	out := make([]string, 0, len(order))
	for _, value := range order {
		if counts[value] == 1 {
			out = append(out, value)
			continue
		}
		out = append(out, fmt.Sprintf("%s x%d", value, counts[value]))
	}
	return out
}

type runMetricsKey struct{}

func withRunMetrics(ctx context.Context, metrics *runMetrics) context.Context {
	return context.WithValue(ctx, runMetricsKey{}, metrics)
}

// runMetricsFrom returns this run's metrics, or nil. Every method is nil-safe,
// so a caller outside a run never has to check.
func runMetricsFrom(ctx context.Context) *runMetrics {
	if metrics, ok := ctx.Value(runMetricsKey{}).(*runMetrics); ok {
		return metrics
	}
	return nil
}

// countPlanSteps counts the steps a plan runs, nested ones included, so "how big
// was the plan" is part of the record rather than a guess from its text.
func countPlanSteps(plan map[string]interface{}) int {
	steps, ok := plan["func_steps"].(map[string]interface{})
	if !ok {
		return 0
	}
	return countStepMap(steps)
}

func countStepMap(steps map[string]interface{}) int {
	total := 0
	for _, raw := range steps {
		step, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		total++
		if nested, ok := step["func_steps"].(map[string]interface{}); ok {
			total += countStepMap(nested)
		}
	}
	return total
}
