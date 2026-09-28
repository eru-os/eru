package ruleset

import (
	"reflect"
	"strings"
)

// KindSpec describes one predicate as data: what it is called, what it needs,
// and how a person would say it.
//
// It exists so that everything outside Go - the agent builder UI, the agent
// that writes agent configs, the config validator - learns the rule language
// from one table instead of each keeping a copy. A copy is how a builder ends
// up offering a kind the engine does not have, or missing the one it just got.
type KindSpec struct {
	Kind Kind `json:"kind"`
	// Label is the short name a picker shows.
	Label string `json:"label"`
	// Sentence is the rule read aloud, with the parameters in braces, for a
	// builder that edits rules as sentences.
	Sentence string `json:"sentence"`
	// Summary says when to reach for it.
	Summary string `json:"summary"`
	// Required are the parameters a rule of this kind is meaningless without.
	Required []string `json:"required"`
	// Optional are the parameters it also reads.
	Optional []string `json:"optional,omitempty"`
	// Placeholders are the message placeholders that carry something useful for
	// this kind, beyond {subject} and {property} which every kind fills.
	Placeholders []string `json:"placeholders,omitempty"`
	// Domain is "general" for kinds any agent can use and "page" for the ones
	// extracted from eru_studio, which only make sense for page JSON.
	Domain string `json:"domain"`
}

// Parameters every rule reads whatever its kind: the arming conditions and the
// outcome.
var CommonRuleParams = []string{"match", "when", "equals", "when_set", "severity", "code", "message", "guidance"}

// KindSpecs is every predicate the engine dispatches, in the order a builder
// should offer them. TestEveryKindHasASpec parses ruleset.go and fails if a
// declared Kind is missing here.
var KindSpecs = []KindSpec{
	{
		Kind: KindRequires, Label: "Is required",
		Sentence: "{property} must have a value",
		Summary:  "The field must be present and non-empty. The commonest rule there is.",
		Required: []string{"property"}, Domain: "general",
	},
	{
		Kind: KindForbidden, Label: "Must be absent",
		Sentence: "{property} must not be set",
		Summary:  "The field must be absent or null - for a field that only belongs in some answers.",
		Required: []string{"property"}, Domain: "general",
	},
	{
		Kind: KindRequiresAny, Label: "At least one of",
		Sentence: "at least one of {properties} must have a value",
		Summary:  "Any one of several fields satisfies it, without caring which.",
		Required: []string{"properties"}, Domain: "general",
	},
	{
		Kind: KindOneOf, Label: "One of",
		Sentence: "{property} must be one of {allowed}",
		Summary:  "The value must come from a fixed list, compared case-insensitively. An absent value passes.",
		Required: []string{"property", "allowed"}, Placeholders: []string{"{value}"}, Domain: "general",
	},
	{
		Kind: KindRange, Label: "Within range",
		Sentence: "{property} must be between {min} and {max}",
		Summary:  "A number with a lower bound, an upper bound, or both. Absent or non-numeric values pass.",
		Required: []string{"property"}, Optional: []string{"min", "max"},
		Placeholders: []string{"{value}", "{field}"}, Domain: "general",
	},
	{
		Kind: KindSumOf, Label: "Equals the sum of",
		Sentence: "{property} must equal the sum of {field} across {over} (within {tolerance})",
		Summary:  "A total must add up to the items it summarises. Tolerance defaults to 0.01; set 0 for exact.",
		Required: []string{"property", "over", "field"}, Optional: []string{"tolerance"},
		Placeholders: []string{"{value}", "{sum}"}, Domain: "general",
	},
	{
		Kind: KindAgreesWith, Label: "Agrees with",
		Sentence: "{property} must equal {field}",
		Summary:  "Two fields on the same object must hold the same value.",
		Required: []string{"property", "field"}, Placeholders: []string{"{value}", "{field}"}, Domain: "general",
	},
	{
		Kind: KindFromEvidence, Label: "Was actually seen",
		Sentence: "{property} must be a value the agent saw in {evidence}",
		Summary:  "The value must have been observed in a tool call recorded by an evidence set. Catches answers that are well formed and about nothing.",
		Required: []string{"property", "evidence"}, Placeholders: []string{"{value}", "{field}"}, Domain: "general",
	},
	{
		Kind: KindNotWithNameLike, Label: "Not on when name looks like",
		Sentence: "{property} must not be on when {field} looks like {patterns}",
		Summary:  "A setting must stay off when another field's value reads like one of the patterns.",
		Required: []string{"property", "field", "patterns"}, Optional: []string{"default_on"}, Domain: "page",
	},
	{
		Kind: KindPathHead, Label: "Path starts with",
		Sentence: "{property} must start at an array index or one of {allowed_heads}",
		Summary:  "A data path's first segment must be a row index or an allowed head.",
		Required: []string{"property"}, Optional: []string{"allowed_heads"},
		Placeholders: []string{"{value}", "{head}"}, Domain: "page",
	},
	{
		Kind: KindNotRowPath, Label: "Not a row path",
		Sentence: "every *{suffix} property must not be a row path",
		Summary:  "Catches result paths copied one level too deep.",
		Required: []string{"suffix"}, Optional: []string{"except"},
		Placeholders: []string{"{value}", "{field}"}, Domain: "page",
	},
}

// SpecFor returns the spec for a kind, and whether it exists.
func SpecFor(kind Kind) (KindSpec, bool) {
	for _, spec := range KindSpecs {
		if spec.Kind == kind {
			return spec, true
		}
	}
	return KindSpec{}, false
}

// RuleKeys is the set of json keys a Rule reads, for spotting a key in a config
// that the engine will silently ignore.
func RuleKeys() map[string]bool {
	keys := map[string]bool{}
	ruleType := reflect.TypeOf(Rule{})
	for i := 0; i < ruleType.NumField(); i++ {
		name := strings.Split(ruleType.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			keys[name] = true
		}
	}
	return keys
}
