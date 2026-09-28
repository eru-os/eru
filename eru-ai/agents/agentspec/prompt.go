package agentspec

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eru-os/eru/eru-ai/agents/ruleset"
)

// Contract renders the configuration language as prompt text, for the agent
// that writes agent configs. It is generated from the same catalog the builder
// UI and the validator read, so the agent is never told about a field the
// server does not have.
func Contract() string {
	c := Get()
	var b strings.Builder

	b.WriteString("AGENT TYPES you may create (agent_type):\n")
	for _, t := range c.Types {
		if !t.Creatable {
			continue
		}
		fmt.Fprintf(&b, "- %s: %s\n", t.Type, t.Summary)
	}

	b.WriteString("\nSETTINGS (top-level keys). Keys not listed here are silently dropped by the server.\n")
	for _, group := range c.Groups {
		var lines []string
		for _, f := range c.Fields {
			if f.Group != group.Group || f.Inactive != "" || !appliesToCreatable(f) {
				continue
			}
			lines = append(lines, renderField(f, "  "))
		}
		if len(lines) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n[%s] %s\n", group.Label, group.Summary)
		b.WriteString(strings.Join(lines, ""))
	}

	b.WriteString("\nRULE KINDS for validation_rules[].rules[].kind (omit kind for \"requires\"):\n")
	for _, k := range c.RuleKinds {
		if k.Domain != "general" {
			continue
		}
		name := string(k.Kind)
		if name == "" {
			name = "(omitted)"
		}
		fmt.Fprintf(&b, "- %s - %s. %s Needs: %s.", name, k.Sentence, k.Summary, strings.Join(k.Required, ", "))
		if len(k.Optional) > 0 {
			fmt.Fprintf(&b, " Optional: %s.", strings.Join(k.Optional, ", "))
		}
		if len(k.Placeholders) > 0 {
			fmt.Fprintf(&b, " Useful placeholders: %s.", strings.Join(k.Placeholders, " "))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Every rule also takes: %s.\n", strings.Join(ruleset.CommonRuleParams, ", "))
	b.WriteString("Severity: omit for an error rule (the answer is rejected and sent back with the message); \"quality\" for a rule worth one more attempt that never fails the request.\n")
	b.WriteString("Message placeholders: ")
	for i, p := range c.Placeholders {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s = %s", p.Value, strings.TrimSuffix(p.Help, "."))
	}
	b.WriteString(".\n")
	return b.String()
}

func appliesToCreatable(f FieldSpec) bool {
	if len(f.Types) == 0 {
		return true
	}
	for _, t := range f.Types {
		if spec, ok := TypeFor(t); ok && spec.Creatable {
			return true
		}
	}
	return false
}

func renderField(f FieldSpec, indent string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s- %s (%s", indent, f.Key, f.Kind)
	if len(f.Enum) > 0 {
		fmt.Fprintf(&b, ": %s", strings.Join(f.Enum, "|"))
	}
	if f.Default != nil {
		fmt.Fprintf(&b, ", default %v", f.Default)
	}
	if f.Required {
		b.WriteString(", required")
	}
	b.WriteString(")")
	if len(f.Types) > 0 {
		var creatable []string
		for _, t := range f.Types {
			if spec, ok := TypeFor(t); ok && spec.Creatable {
				creatable = append(creatable, t)
			}
		}
		fmt.Fprintf(&b, " [only %s]", strings.Join(creatable, ", "))
	}
	fmt.Fprintf(&b, ": %s", f.Help)
	if len(f.Ignored) > 0 {
		var notes []string
		for t, why := range f.Ignored {
			if spec, ok := TypeFor(t); ok && spec.Creatable {
				notes = append(notes, fmt.Sprintf("%s ignores it (%s)", t, strings.TrimSuffix(why, ".")))
			}
		}
		sort.Strings(notes)
		if len(notes) > 0 {
			fmt.Fprintf(&b, " NOTE: %s.", strings.Join(notes, "; "))
		}
	}
	b.WriteString("\n")
	for _, child := range f.Children {
		if child.Inactive != "" {
			continue
		}
		if len(child.ShowWhen) > 0 && child.Level == "advanced" && !child.Required {
			continue
		}
		b.WriteString(renderField(child, indent+"    "))
	}
	return b.String()
}
