// Package agentspec describes the agent configuration language as data.
//
// An agent config is JSON that Go structs decode, and until this package the
// only description of that JSON was the structs themselves. The agent builder
// UI, the agent that writes agent configs, and the config validator all need to
// know the same things - which keys exist, which types read them, which are
// silently ignored by which type, what a rule kind needs - and three copies of
// that knowledge would drift the day a field is added.
//
// The catalog here is that one copy. Its STRUCTURE is not trusted to this file:
// TestCatalogMatchesTheStructs in agents_factory reflects over every agent type
// and fails if a json key exists without an entry, or an entry names a key no
// type has. Only the words - labels, help, grouping - are written by hand.
package agentspec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/eru-os/eru/eru-ai/agents/ruleset"
)

// TypeSpec is one agent_type.
type TypeSpec struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Summary string `json:"summary"`
	// Family groups types that share an execution model: "reasoning" runs a
	// tool loop checked by rules, "reflex" runs its tools once and answers,
	// "orchestrator" plans across other agents.
	Family string `json:"family"`
	// Iterative types read max_iterations, thinking_budget and
	// enable_clarification.
	Iterative bool `json:"iterative"`
	// Specialist types bring their own system prompt and output schema from Go;
	// what a config says is appended to, or replaced by, the built-in one.
	Specialist bool `json:"specialist"`
	// Creatable is whether a builder should offer this type for a new agent.
	// The specialists are configured once per platform, not per use case.
	Creatable bool   `json:"creatable"`
	Icon      string `json:"icon"`
}

// GroupSpec is a section of the builder.
type GroupSpec struct {
	Group   string `json:"group"`
	Label   string `json:"label"`
	Summary string `json:"summary"`
	Icon    string `json:"icon"`
	// Level "basic" groups are shown to everyone; "advanced" ones behind the
	// advanced switch.
	Level string `json:"level"`
}

// FieldSpec is one json key, possibly with children.
type FieldSpec struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Help  string `json:"help"`
	// Kind is how to edit it: string, text, int, number, bool, enum,
	// string_list, object, array, json_schema, json, duration, ref_model,
	// ref_tool, ref_action, ref_agent, ref_agent_list, ref_evidence,
	// ref_vectorstore.
	Kind     string      `json:"kind"`
	Enum     []string    `json:"enum,omitempty"`
	Default  interface{} `json:"default,omitempty"`
	Required bool        `json:"required,omitempty"`
	Pattern  string      `json:"pattern,omitempty"`
	Group    string      `json:"group,omitempty"`
	Level    string      `json:"level"`
	// Types lists the agent types whose structs carry this key. Empty means
	// every type.
	Types []string `json:"types,omitempty"`
	// Ignored names types that decode the key but never act on it, with why.
	// A builder should say so rather than let someone tune a dead setting.
	Ignored map[string]string `json:"ignored,omitempty"`
	// ShowWhen limits the field to parents whose sibling keys hold one of the
	// listed values, e.g. redis_addr only when cache_store_type is REDIS.
	ShowWhen map[string][]string `json:"show_when,omitempty"`
	// Inactive marks a key that decodes but is not wired to anything yet.
	Inactive string      `json:"inactive,omitempty"`
	Children []FieldSpec `json:"children,omitempty"`
}

// Catalog is everything a builder needs to know about agent configuration.
type Catalog struct {
	// Version changes whenever anything in the catalog does, so a client can
	// cache it and a generated config can say which language it was written in.
	Version    string             `json:"version"`
	Types      []TypeSpec         `json:"types"`
	Groups     []GroupSpec        `json:"groups"`
	Fields     []FieldSpec        `json:"fields"`
	RuleKinds  []ruleset.KindSpec `json:"rule_kinds"`
	RuleCommon []string           `json:"rule_common"`
	Severities []EnumValue        `json:"severities"`
	// Placeholders are the message placeholders every rule kind fills.
	Placeholders []EnumValue `json:"placeholders"`
}

// EnumValue is a value with the words to show for it.
type EnumValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Help  string `json:"help"`
}

var (
	catalogOnce sync.Once
	catalog     Catalog
)

// Get returns the catalog.
func Get() Catalog {
	catalogOnce.Do(func() {
		catalog = Catalog{
			Types:      types,
			Groups:     groups,
			Fields:     fields,
			RuleKinds:  ruleset.KindSpecs,
			RuleCommon: ruleset.CommonRuleParams,
			Severities: []EnumValue{
				{Value: "", Label: "Error", Help: "Rejects the answer and sends it back to the model. Counts against retry_count."},
				{Value: string(ruleset.SeverityQuality), Label: "Quality", Help: "Worth at most one more attempt. Never fails the request."},
			},
			Placeholders: []EnumValue{
				{Value: "{subject}", Label: "Subject", Help: "The object being judged, named by the rule set's name_key."},
				{Value: "{property}", Label: "Property", Help: "The field the rule is about."},
				{Value: "{value}", Label: "Value", Help: "What the answer actually holds."},
				{Value: "{field}", Label: "Field / bound", Help: "The other field, the range bound, or the values seen - depends on the kind."},
				{Value: "{sum}", Label: "Sum", Help: "sum_of only: the computed total."},
				{Value: "{head}", Label: "Head", Help: "path_head only: the path's first segment."},
			},
		}
		raw, _ := json.Marshal(catalog)
		sum := sha256.Sum256(raw)
		catalog.Version = hex.EncodeToString(sum[:])[:12]
	})
	return catalog
}

func RegisterType(spec TypeSpec, extraFields ...FieldSpec) {
	if catalog.Version != "" {
		panic(fmt.Sprintf("agent type %s registered after the catalog was built", spec.Type))
	}
	if _, known := TypeFor(spec.Type); known {
		panic(fmt.Sprintf("agent type %s registered twice", spec.Type))
	}
	types = append(types, spec)
	if like, ok := familyType[spec.Family]; ok {
		for i := range fields {
			fields[i] = withType(fields[i], like, spec.Type)
		}
	}
	if spec.Specialist && spec.Family == "reasoning" {
		for i := range fields {
			if fields[i].Key == "output_schema" {
				fields[i].Ignored = merge(fields[i].Ignored, map[string]string{spec.Type: whySpecialistSchema})
			}
		}
	}
	fields = append(fields, extraFields...)
}

var familyType = map[string]string{"reasoning": TypeReasoning, "reflex": TypeReflex}

func withType(field FieldSpec, like string, agentType string) FieldSpec {
	if contains(field.Types, like) {
		field.Types = append(append([]string{}, field.Types...), agentType)
	}
	if why, ok := field.Ignored[like]; ok {
		field.Ignored = merge(field.Ignored, map[string]string{agentType: why})
	}
	if len(field.Children) > 0 {
		children := make([]FieldSpec, len(field.Children))
		for i, child := range field.Children {
			children[i] = withType(child, like, agentType)
		}
		field.Children = children
	}
	return field
}

// TypeFor returns the spec for an agent_type, and whether it is known.
func TypeFor(agentType string) (TypeSpec, bool) {
	for _, spec := range types {
		if spec.Type == agentType {
			return spec, true
		}
	}
	return TypeSpec{}, false
}

// FieldFor returns the top-level field spec for a key.
func FieldFor(key string) (FieldSpec, bool) {
	for _, field := range fields {
		if field.Key == key {
			return field, true
		}
	}
	return FieldSpec{}, false
}

// AppliesTo reports whether a type's struct carries this field.
func (f FieldSpec) AppliesTo(agentType string) bool {
	if len(f.Types) == 0 {
		return true
	}
	for _, t := range f.Types {
		if t == agentType {
			return true
		}
	}
	return false
}

// Child returns a child field spec by key.
func (f FieldSpec) Child(key string) (FieldSpec, bool) {
	for _, child := range f.Children {
		if child.Key == key {
			return child, true
		}
	}
	return FieldSpec{}, false
}
