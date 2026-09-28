package agentspec

import (
	"fmt"

	"github.com/eru-os/eru/eru-ai/agents/ruleset"
)

// Rule outcomes on a scorecard.
const (
	RulePass = "pass"
	RuleFail = "fail"
	// RuleIdle means the rule never applied: its subjects were absent from the
	// answer, or its arming conditions never held. Not a pass - nothing was
	// checked - and worth seeing, because a rule that is always idle is usually
	// pointed at the wrong path.
	RuleIdle = "idle"
	// RuleNeedsRun is a from_evidence rule, which can only be judged against
	// what a real run observed.
	RuleNeedsRun = "needs_run"
)

// RuleScore is one rule's outcome against one answer.
type RuleScore struct {
	Set      int            `json:"set"`
	Rule     int            `json:"rule"`
	Code     string         `json:"code"`
	Kind     string         `json:"kind"`
	Severity string         `json:"severity"`
	Status   string         `json:"status"`
	Checked  int            `json:"checked"`
	Findings []ScoreFinding `json:"findings"`
}

// ScoreFinding is one place a rule failed, as the model would be told it.
type ScoreFinding struct {
	Path     string `json:"path"`
	Property string `json:"property"`
	Message  string `json:"message"`
}

// Scorecard judges every rule against an answer and reports each one, passed
// or not. The agent loop only ever reports failures; an author tuning rules
// needs to see the ones that held and the ones that never ran as well.
func Scorecard(sets []ruleset.RuleSet, answer map[string]interface{}) []RuleScore {
	out := []RuleScore{}
	for i, set := range sets {
		subjects := set.SubjectsIn(answer)
		for j, rule := range set.Rules {
			score := RuleScore{Set: i, Rule: j, Code: rule.Code, Kind: string(rule.Kind), Severity: string(rule.Severity), Findings: []ScoreFinding{}}
			if score.Severity == "" {
				score.Severity = "error"
			}
			if rule.Kind == ruleset.KindFromEvidence {
				score.Status = RuleNeedsRun
				out = append(out, score)
				continue
			}
			for _, subject := range subjects {
				if !rule.Armed(subject) {
					continue
				}
				score.Checked++
				for _, finding := range rule.Check(subject) {
					score.Findings = append(score.Findings, ScoreFinding{Path: finding.Path, Property: finding.Property, Message: finding.Message})
				}
			}
			switch {
			case score.Checked == 0:
				score.Status = RuleIdle
			case len(score.Findings) > 0:
				score.Status = RuleFail
			default:
				score.Status = RulePass
			}
			out = append(out, score)
		}
	}
	return out
}

// ScoreSummary counts a scorecard by status.
func ScoreSummary(scores []RuleScore) string {
	counts := map[string]int{}
	for _, score := range scores {
		counts[score.Status]++
	}
	return fmt.Sprintf("%d passed, %d failed, %d idle, %d need a run", counts[RulePass], counts[RuleFail], counts[RuleIdle], counts[RuleNeedsRun])
}
