package ruleset

import (
	"fmt"
	"strconv"
	"strings"
)

// A RuleSet is what an agent declares in configuration: some rules, and which
// part of its answer they judge.
//
// The rules alone were not enough to be usable from config. Check takes
// []Subject, and eru_studio built those in Go by walking a page for components.
// An agent configured through the product has no such code and no way to write
// any, so the engine was generic and unreachable - the whole point of the
// package, missing by one step.
//
// Subjects closes it: a path into the answer, resolved the same way for
// everyone.
type RuleSet struct {
	// Subjects is a dotted path to what the rules judge. Empty means the answer
	// itself is the single subject.
	//
	// Any segment that resolves to a list is walked, so "line_items" and
	// "invoice.sections.fields" both work without bracket syntax. A path that
	// resolves to nothing yields no subjects, and no subjects means no findings
	// - an agent that did not produce the section a rule is about has not broken
	// that rule. Say that with a KindRequires on the parent instead.
	Subjects string `json:"subjects,omitempty"`
	// NameKey is the property to render as {subject} in messages. Falls back to
	// "name", then "id", then "type", then the path.
	NameKey string `json:"name_key,omitempty"`
	Rules   []Rule `json:"rules"`
}

// SubjectsIn resolves the objects this set applies to.
func (s RuleSet) SubjectsIn(answer map[string]interface{}) []Subject {
	if answer == nil {
		return nil
	}
	path := strings.TrimSpace(s.Subjects)
	if path == "" {
		return []Subject{{Path: "answer", Name: s.nameOf(answer, "answer"), Properties: answer}}
	}

	var out []Subject
	var descend func(node interface{}, segments []string, where string)
	descend = func(node interface{}, segments []string, where string) {
		// A list at any depth is walked, so a path never has to say so.
		if list, ok := node.([]interface{}); ok {
			for i, item := range list {
				descend(item, segments, fmt.Sprintf("%s[%d]", where, i))
			}
			return
		}
		bag, ok := node.(map[string]interface{})
		if !ok {
			return
		}
		if len(segments) == 0 {
			out = append(out, Subject{Path: where, Name: s.nameOf(bag, where), Properties: bag})
			return
		}
		next, present := bag[segments[0]]
		if !present {
			return
		}
		descend(next, segments[1:], where+"."+segments[0])
	}
	descend(answer, strings.Split(path, "."), "answer")
	return out
}

func (s RuleSet) nameOf(bag map[string]interface{}, fallback string) string {
	keys := []string{s.NameKey, "name", "id", "type"}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if value, ok := bag[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return fallback
}

// Check evaluates the set against an answer.
//
// Without evidence, so a KindFromEvidence rule faults nothing here. Use
// CheckSetsWith to enforce those.
func (s RuleSet) Check(answer map[string]interface{}) []Finding {
	return Check(s.Rules, s.SubjectsIn(answer))
}

// CheckSets evaluates several sets, keeping only one severity.
//
// Evidence rules are inert through this path - they have nothing to check
// against. CheckSetsWith is the one that enforces them, and it is what the
// reasoning loop calls.
func CheckSets(sets []RuleSet, answer map[string]interface{}, severity Severity) []Finding {
	var out []Finding
	for _, set := range sets {
		out = append(out, Check(Filter(set.Rules, severity), set.SubjectsIn(answer))...)
	}
	return out
}

// PromptForSets renders the guidance of every set, so the model is told what it
// will be judged on.
func PromptForSets(sets []RuleSet, heading string, severity Severity) string {
	var rules []Rule
	for _, set := range sets {
		rules = append(rules, Filter(set.Rules, severity)...)
	}
	return Prompt(rules, heading)
}

// Describe renders one finding for a human or a model.
func Describe(findings []Finding, limit int) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	for i, finding := range findings {
		if limit > 0 && i >= limit {
			fmt.Fprintf(&b, "- ...and %d more\n", len(findings)-i)
			break
		}
		fmt.Fprintf(&b, "- %s: %s\n", finding.Path, finding.Message)
	}
	return strings.TrimRight(b.String(), "\n")
}

// numberOf reads a numeric property however JSON decoded it.
func numberOf(raw interface{}) (float64, bool) {
	switch typed := raw.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		value, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return value, err == nil
	default:
		return 0, false
	}
}

// textOf reads a property as text, so a rule about a value does not have to care
// whether the model quoted it.
func textOf(raw interface{}) (string, bool) {
	switch typed := raw.(type) {
	case string:
		return typed, true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(typed), true
	case nil:
		return "", false
	default:
		return fmt.Sprint(typed), true
	}
}
