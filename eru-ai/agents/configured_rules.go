package agents

import (
	"context"
	"fmt"
	"strings"

	ruleset "github.com/eru-os/eru/eru-ai/agents/ruleset"
)

// The configured half of the inner loop.
//
// Everything the loop does for eru_studio - reject an answer that breaks a
// rule, feed the reason back, judge whether the result is any good - it now
// does for an agent that declares those rules in JSON. The mechanisms were
// already generic; this is the wiring that was missing, and without it the rule
// engine was a library nobody outside this repository could reach.
//
// The three entry points below are called by the reasoning loop in the same
// places it calls an agent type's Go implementations, and they compose: an agent
// may have both, and both are enforced.

// maxReportedConfiguredIssues caps what goes back to the model. An answer that
// is wrong everywhere produces the same complaint many times over, and the model
// only needs the shape of the mistake to fix all of them.
const maxReportedConfiguredIssues = 25

// CheckConfiguredRules enforces the error-severity rules an agent declares.
// Returns nil when there are none, which is the common case and must stay free.
func (agent *Agent) CheckConfiguredRules(ctx context.Context, answer map[string]interface{}) error {
	if answer == nil || (len(agent.ValidationRules) == 0 && len(agent.Claims) == 0) {
		return nil
	}
	findings := ruleset.CheckSetsWith(agent.ValidationRules, answer, ruleset.SeverityError, EvidenceFrom(ctx))
	problems := ruleset.Describe(findings, maxReportedConfiguredIssues)

	// A claim the run cannot support is a different kind of wrong from a broken
	// rule - the answer is well formed and says something untrue - so it is
	// reported in its own words rather than folded in as another finding.
	if unsupported := VerifyClaims(ctx, agent.Claims, answer, ruleset.SeverityError); len(unsupported) > 0 {
		claims := "- " + strings.Join(unsupported, "\n- ")
		if problems == "" {
			problems = claims
		} else {
			problems = problems + "\n" + claims
		}
	}
	if problems == "" {
		return nil
	}
	return fmt.Errorf("the answer breaks rules this agent declares:\n%s", problems)
}

// JudgeConfiguredQuality is the quality gate's verdict from declared rules.
//
// Quality rules are rated poor or good and nothing between: a declared rule
// either holds or it does not, so there is no judgement of degree to express.
// "Acceptable" belongs to a model judge, which can hold an opinion a table
// cannot.
func (agent *Agent) JudgeConfiguredQuality(ctx context.Context, answer map[string]interface{}) (QualityVerdict, bool) {
	if len(agent.ValidationRules) == 0 || answer == nil {
		return QualityVerdict{}, false
	}
	hasQualityRule := false
	for _, set := range agent.ValidationRules {
		if len(ruleset.Filter(set.Rules, ruleset.SeverityQuality)) > 0 {
			hasQualityRule = true
			break
		}
	}
	if !hasQualityRule && len(agent.Claims) == 0 {
		// No opinion declared is not the same as "this is good".
		return QualityVerdict{}, false
	}

	findings := ruleset.CheckSetsWith(agent.ValidationRules, answer, ruleset.SeverityQuality, EvidenceFrom(ctx))
	reason := ruleset.Describe(findings, maxReportedConfiguredIssues)
	if unsupported := VerifyClaims(ctx, agent.Claims, answer, ruleset.SeverityQuality); len(unsupported) > 0 {
		claims := "- " + strings.Join(unsupported, "\n- ")
		if reason == "" {
			reason = claims
		} else {
			reason = reason + "\n" + claims
		}
	}
	if reason == "" {
		return QualityVerdict{Rating: QualityGood}, true
	}
	return QualityVerdict{Rating: QualityPoor, Reason: reason}, true
}

// ConfiguredRulesPrompt states the declared rules before the fact.
//
// A gate that only ever speaks afterwards pays for a second model call to say
// something the first could have been told for free, and a validator that only
// ever rejects teaches the model nothing about what it wanted.
func (agent *Agent) ConfiguredRulesPrompt() string {
	if len(agent.ValidationRules) == 0 {
		return ""
	}
	out := ruleset.PromptForSets(agent.ValidationRules,
		"RULES YOUR ANSWER MUST SATISFY", ruleset.SeverityError)
	quality := ruleset.PromptForSets(agent.ValidationRules,
		"WHAT MAKES AN ANSWER GOOD RATHER THAN MERELY VALID", ruleset.SeverityQuality)
	if quality == "" {
		return out
	}
	if out == "" {
		return quality
	}
	return out + "\n\n" + quality
}
