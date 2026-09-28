package agents

import (
	"context"
	"strings"

	ruleset "github.com/eru-os/eru/eru-ai/agents/ruleset"
)

// EvidenceRule says what to remember from a tool call, so a validation rule can
// later ask whether the agent made something up.
//
// # Why declared rather than recorded wholesale
//
// The obvious design is to keep every tool result and let rules query them. It
// is the wrong one twice over. Results are large - one query answer can be
// thousands of rows - and they would go into the metrics of every reply and
// every recorded trajectory. And they are the tenant's data: entity names,
// customer records, whatever the agent read. Keeping all of it, forever, on the
// chance a rule might want a field of it, is a privacy cost paid by everyone to
// serve a few.
//
// Declaring what matters keeps the values small, intentional, and inspectable -
// an owner can read their agent's configuration and see exactly what is
// retained.
type EvidenceRule struct {
	// Name is what the set is called; a rule refers to it by this.
	Name string `json:"name" eru:"required"`
	// Action is the tool action that fills it, matched on the action name the
	// model sees.
	Action string `json:"action" eru:"required"`
	// FromArg collects the value of one argument of the call - "you ran this
	// query", "you asked about this entity".
	FromArg string `json:"from_arg,omitempty"`
	// FromResult collects values from a dotted path into the result - "these are
	// the fields the metadata returned". A path segment that lands on a list is
	// walked, so "entities.fields.name" needs no bracket syntax.
	FromResult string `json:"from_result,omitempty"`
	// FailedToo collects from calls that errored as well. Off by default: a
	// lookup that failed did not show the agent anything, and treating it as
	// evidence would let a broken tool authorise any binding at all.
	FailedToo bool `json:"failed_too,omitempty"`
}

type evidenceKey struct{}
type evidenceRulesKey struct{}

// WithEvidence puts a run's collected evidence in the context.
func WithEvidence(ctx context.Context, evidence *ruleset.Evidence) context.Context {
	return context.WithValue(ctx, evidenceKey{}, evidence)
}

// EvidenceFrom returns the run's evidence, or nil.
func EvidenceFrom(ctx context.Context) *ruleset.Evidence {
	evidence, _ := ctx.Value(evidenceKey{}).(*ruleset.Evidence)
	return evidence
}

// WithEvidenceRules puts the agent's collection rules in the context, so the
// recording tool can apply them without knowing which agent it belongs to.
func WithEvidenceRules(ctx context.Context, rules []EvidenceRule) context.Context {
	return context.WithValue(ctx, evidenceRulesKey{}, rules)
}

func evidenceRulesFrom(ctx context.Context) []EvidenceRule {
	rules, _ := ctx.Value(evidenceRulesKey{}).([]EvidenceRule)
	return rules
}

// collectEvidence applies the declared rules to one completed call.
func collectEvidence(ctx context.Context, action string, args map[string]interface{}, result map[string]interface{}, ok bool) {
	evidence := EvidenceFrom(ctx)
	if evidence == nil {
		return
	}
	for _, rule := range evidenceRulesFrom(ctx) {
		if !strings.EqualFold(strings.TrimSpace(rule.Action), strings.TrimSpace(action)) {
			continue
		}
		if !ok && !rule.FailedToo {
			continue
		}
		// Observed with no values still marks the set as looked-at. That is the
		// difference between "the lookup returned nothing" and "the lookup never
		// happened", and rules must not treat them the same.
		if rule.FromArg != "" {
			evidence.Observe(rule.Name, valuesAt(args, rule.FromArg)...)
		}
		if rule.FromResult != "" {
			evidence.Observe(rule.Name, valuesAt(result, rule.FromResult)...)
		}
		if rule.FromArg == "" && rule.FromResult == "" {
			evidence.Observe(rule.Name)
		}
	}
}

// valuesAt reads every string reachable by a dotted path, walking lists.
func valuesAt(node interface{}, path string) []string {
	segments := strings.Split(strings.TrimSpace(path), ".")
	var out []string
	var walk func(node interface{}, segments []string)
	walk = func(node interface{}, segments []string) {
		if list, ok := node.([]interface{}); ok {
			for _, item := range list {
				walk(item, segments)
			}
			return
		}
		if len(segments) == 0 {
			if text := stringValue(node); text != "" {
				out = append(out, text)
			}
			return
		}
		bag, ok := node.(map[string]interface{})
		if !ok {
			return
		}
		next, present := bag[segments[0]]
		if !present {
			return
		}
		walk(next, segments[1:])
	}
	walk(node, segments)
	return out
}

func stringValue(node interface{}) string {
	if text, ok := node.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}
