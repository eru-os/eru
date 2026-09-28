package ruleset

import (
	"fmt"
	"sort"
	"strings"
)

// Evidence is what the agent actually looked up during the run.
//
// # The check this exists for
//
// The most valuable thing a validator can ask is not "is this well formed" but
// "did you SEE this, or did you make it up". A page bound to a query nobody ran,
// a field named from memory rather than from the metadata, an entity bound to
// its physical table - all render correctly, answer 200, and show nothing. No
// amount of schema or shape checking reaches them, because the answer is
// perfectly well formed; it is simply about things that do not exist.
//
// eru_studio has had this check for weeks, as a Ledger written in Go and
// reachable only by that one agent. The idea in it is entirely general, and
// this is it with the page removed: a set of values the agent observed, and a
// rule kind that requires a binding to be one of them.
//
// # Never looked is not the same as looked and found nothing
//
// A set that was never filled does not fault anything. This is the distinction
// that took a live run and a held-out fixture to get right in the eru_studio
// version: a tool that is broken, or was never offered, must not be held against
// the agent - but a tool that RAN and answered "no such thing" is the tool
// working, and is exactly when the rule matters most. Known() is how a rule
// tells the two apart, and it is why Evidence records emptiness explicitly
// rather than inferring it from an absent key.
type Evidence struct {
	seen map[string]map[string]bool
}

func NewEvidence() *Evidence {
	return &Evidence{seen: map[string]map[string]bool{}}
}

// Observe records a value the agent saw under a named set. Calling it with no
// value still marks the set as looked-at, which is the difference between "the
// lookup returned nothing" and "the lookup never happened".
func (e *Evidence) Observe(name string, values ...string) {
	if e == nil || strings.TrimSpace(name) == "" {
		return
	}
	if e.seen == nil {
		e.seen = map[string]map[string]bool{}
	}
	set, ok := e.seen[name]
	if !ok {
		set = map[string]bool{}
		e.seen[name] = set
	}
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			set[strings.ToLower(trimmed)] = true
		}
	}
}

// Known reports whether anything was ever collected under this name - whether
// the lookup happened at all.
func (e *Evidence) Known(name string) bool {
	if e == nil {
		return false
	}
	_, ok := e.seen[name]
	return ok
}

// Has reports whether this value was observed.
func (e *Evidence) Has(name, value string) bool {
	if e == nil {
		return false
	}
	return e.seen[name][strings.ToLower(strings.TrimSpace(value))]
}

// Values lists what was observed, for a message that can say what WAS available
// instead of only what was not.
func (e *Evidence) Values(name string) []string {
	if e == nil {
		return nil
	}
	out := make([]string, 0, len(e.seen[name]))
	for value := range e.seen[name] {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// checkFromEvidence requires a property's value to be something the agent saw.
func (r Rule) checkFromEvidence(subject Subject, evidence *Evidence) []Finding {
	// Never looked: nothing to hold the agent to. Enforcing here would fault an
	// agent for a tool that was down or was never attached.
	if !evidence.Known(r.Evidence) {
		return nil
	}
	value, ok := textOf(subject.Properties[r.Property])
	if !ok || strings.TrimSpace(value) == "" || IsExpression(value) {
		return nil
	}
	if evidence.Has(r.Evidence, value) {
		return nil
	}
	available := evidence.Values(r.Evidence)
	shown := available
	if len(shown) > 12 {
		shown = shown[:12]
	}
	hint := "nothing was found"
	if len(available) > 0 {
		hint = strings.Join(shown, ", ")
		if len(available) > len(shown) {
			hint += fmt.Sprintf(" and %d more", len(available)-len(shown))
		}
	}
	return []Finding{r.finding(subject, r.Property, value, "", hint)}
}

// CheckWith evaluates rules that may consult evidence. Check is this with none,
// so a rule set that asks for evidence it was not given simply does not fire.
func CheckWith(rules []Rule, subjects []Subject, evidence *Evidence) []Finding {
	var out []Finding
	for _, subject := range subjects {
		for _, rule := range rules {
			if rule.Kind == KindFromEvidence {
				if rule.Armed(subject) {
					out = append(out, rule.checkFromEvidence(subject, evidence)...)
				}
				continue
			}
			out = append(out, rule.Check(subject)...)
		}
	}
	return out
}

// CheckSetsWith is CheckSets with evidence.
func CheckSetsWith(sets []RuleSet, answer map[string]interface{}, severity Severity, evidence *Evidence) []Finding {
	var out []Finding
	for _, set := range sets {
		out = append(out, CheckWith(Filter(set.Rules, severity), set.SubjectsIn(answer), evidence)...)
	}
	return out
}
