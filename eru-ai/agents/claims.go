package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"

	ruleset "github.com/eru-os/eru/eru-ai/agents/ruleset"
)

// Holding an agent to what it says it did.
//
// # The failure
//
// An agent finishes and reports: "created the entity, added six fields, saved
// the page." The report is well formed, every schema check passes, and three of
// those things never happened. The model is not lying so much as summarising an
// intention - it planned six fields, wrote four, and described the plan.
//
// Nothing catches this. Validation reads the report's SHAPE; the report's
// CONTENT is a claim about the world, and the only thing that can test it is the
// record of what the agent actually did.
//
// Checking claims by hand, for one agent and one kind of claim, does not scale.
// This takes the agent out: a claim in the answer names an action, and the
// framework asks the tool record whether that action ran with those arguments.
//
// # Why it reports rather than rejects, by default
//
// A claim that cannot be matched is not proof of a lie. The tool record misses
// calls made inside a sub-agent's own loop; an action can be named differently
// in the report than in the call; work can genuinely have been done by a path
// that leaves no record. Failing a run on that evidence would make the framework
// wrong more often than the agent. Declared as an error-severity rule it will
// reject; left as quality it marks down, which is the right default for a check
// whose evidence is incomplete.

// ClaimRule says that a value in the answer must correspond to something the
// agent actually did.
type ClaimRule struct {
	// Claims is a dotted path to the claimed things - "created_fields",
	// "report.entities". Each string found there is one claim.
	Claims string `json:"claims" eru:"required"`
	// Action is the tool action that would have to have run for a claim to be
	// true.
	Action string `json:"action" eru:"required"`
	// ArgKey is the argument of that call carrying the thing's name, so a claim
	// can be matched against what was actually passed rather than merely against
	// "the action ran at some point".
	ArgKey string `json:"arg_key,omitempty"`
	// Unchanged points at the things the answer says were ALREADY correct and
	// needed no work. That is not a neutral non-statement: it claims the thing
	// exists and matches, so nothing was done.
	//
	// It is the more dangerous of the two claims. A false "I created it" leaves
	// the user looking for something that is not there, and they find out. A
	// false "it was already there" tells them their request was satisfied before
	// they asked, and they have no reason to look at all. An agent asked for
	// test_ask_a can report test_scan_a - a different, similarly named field - as
	// unchanged, and tell the user they are done.
	//
	// Verified the opposite way round from Claims: these must be PRESENT in the
	// evidence, and an absence is the fault.
	Unchanged string `json:"unchanged,omitempty"`
	// Reject makes an unsupported claim fail the answer. Off by default, and
	// stated as its own flag rather than as a ruleset.Severity on purpose:
	// SeverityError is ruleset's ZERO value, because an undeclared rule there
	// should be a hard one. Claims want the opposite default - the evidence is
	// incomplete, so silence must mean "mark it down", not "reject it" - and
	// reusing a zero value that means the opposite of what you need is how a
	// default quietly becomes the wrong one.
	Reject bool `json:"reject,omitempty"`
}

// VerifyClaims checks an answer's claims against what the run recorded.
func VerifyClaims(ctx context.Context, rules []ClaimRule, answer map[string]interface{}, severity ruleset.Severity) []string {
	if len(rules) == 0 || answer == nil {
		return nil
	}
	record := ToolRecordFrom(ctx)
	if record == nil {
		// No record means no evidence either way, and a check that cannot run
		// must never read as a check that failed.
		return nil
	}
	calls := record.Calls()

	var unsupported []string
	for _, rule := range rules {
		want := ruleset.SeverityQuality
		if rule.Reject {
			want = ruleset.SeverityError
		}
		if want != severity {
			continue
		}
		performed, ran := performedValues(calls, rule.Action, rule.ArgKey)
		if !ran && rule.Unchanged != "" && rule.Claims == "" {
			// A rule that only checks "already there" has nothing to say when
			// the action never ran: not calling a save is exactly what an agent
			// SHOULD do when the thing already exists. Reporting it would fault
			// the correct behaviour.
			continue
		}
		if !ran {
			// The action never ran at all. That is a stronger statement than a
			// mismatch and worth saying differently.
			for _, claim := range valuesAt(answer, rule.Claims) {
				unsupported = append(unsupported, fmt.Sprintf(
					"the answer says %q was done, but %s was never called", claim, rule.Action))
			}
			continue
		}
		for _, claim := range valuesAt(answer, rule.Claims) {
			if rule.ArgKey == "" {
				break // the action ran; that is all this rule asked
			}
			if performed[strings.ToLower(strings.TrimSpace(claim))] {
				continue
			}
			unsupported = append(unsupported, fmt.Sprintf(
				"the answer says %q was done, but no %s call named it (called with: %s)",
				claim, rule.Action, strings.Join(sortedKeys(performed), ", ")))
		}

		// "Already correct, nothing to do" is checked against the same evidence,
		// the other way round: the thing has to be THERE.
		for _, claim := range valuesAt(answer, rule.Unchanged) {
			if performed[strings.ToLower(strings.TrimSpace(claim))] {
				continue
			}
			unsupported = append(unsupported, fmt.Sprintf(
				"the answer says %q needed no work because it was already correct, but nothing in this run shows it exists. "+
					"A wrong \"already done\" is worse than a wrong \"created\": it tells the user to stop looking", claim))
		}
	}
	sort.Strings(unsupported)
	return unsupported
}

// performedValues collects what an action was actually called with, and whether
// it ran at all.
func performedValues(calls []ToolInvocation, action, argKey string) (map[string]bool, bool) {
	out := map[string]bool{}
	ran := false
	for _, call := range calls {
		name := call.Action
		if name == "" {
			name = call.Tool
		}
		if !strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(action)) {
			continue
		}
		if !call.Ok {
			// A failed call did not do the thing, so it cannot support a claim
			// that the thing was done.
			continue
		}
		ran = true
		if argKey == "" {
			continue
		}
		for _, value := range valuesAt(call.Args, argKey) {
			out[strings.ToLower(strings.TrimSpace(value))] = true
		}
	}
	return out, ran
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"nothing"}
	}
	return out
}
