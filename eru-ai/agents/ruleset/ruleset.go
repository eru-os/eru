// Package ruleset evaluates declarative constraints over an agent's structured
// output.
//
// # Why this is not in the eru_studio catalog
//
// The rule idea started there, keyed on component types, and it earned its keep:
// a rule is declared once and both the validator and the system prompt are
// generated from it, so what the model is told and what it is judged on cannot
// disagree. But it was reachable only by an agent whose output happens to be an
// EruPage, and only by one written in Go.
//
// That is the gap this package closes. A reasoning agent's inner loop - validate,
// feed the failure back, retry - is the only place a bad answer gets corrected
// before the user sees it, and today an agent gets that loop only if someone
// writes a Go OutputValidator for it. A new agent configured through the product
// gets the SHAPE of the loop and none of its judgement: it retries RetryCount
// times and every attempt passes, because nothing is checking.
//
// Rules here are data. An agent declares them in its configuration and gets a
// working correction loop without Go.
//
// # The boundary
//
// A Kind is a closed set of compiled predicates. Configuration selects a
// predicate and supplies its arguments; it never defines one. That is deliberate
// and is the same boundary the catalog and eval package docs describe: the code
// that judges an agent's work is not authored by an agent. An expression
// language here would hand that authorship away, and the Darwin Godel Machine's
// Appendix H is what happens next - a variant scored a perfect 2.0 by deleting
// the instrumentation its grader read.
package ruleset

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Severity says what a broken rule costs.
//
// The distinction is not cosmetic. A dashboard that binds a component to a query
// it never probed is WRONG and must not be delivered. A dashboard whose grid
// shows the raw column names `an` and `os_amt` as headings is correct, renders,
// and is simply not good enough - failing the request over it would be worse for
// the user than delivering it. Both are rules worth declaring; only one may
// reject an answer.
type Severity string

const (
	// SeverityError rejects the answer and sends it back. The zero value, so a
	// rule that says nothing is a hard constraint - the safer default.
	SeverityError Severity = ""
	// SeverityQuality feeds the quality gate: worth one more attempt, never
	// worth failing the request.
	SeverityQuality Severity = "quality"
)

// Kind selects the predicate a rule applies.
type Kind string

const (
	// KindRequires: the property named by Property must carry a value.
	// The zero value, so a rule that omits Kind reads as the common case.
	KindRequires Kind = ""
	// KindForbidden: the property named by Property must not be set at all.
	KindForbidden Kind = "forbidden"
	// KindPathHead: Property holds a path whose first segment must be an array
	// index or one of AllowedHeads.
	KindPathHead Kind = "path_head"
	// KindNotRowPath: every property ending in Suffix is read against the value
	// of its matching *_field, so a path that starts at a row index or repeats
	// the field's own name is a result path copied one level too far in.
	KindNotRowPath Kind = "not_row_path"
	// KindRequiresAny: at least one of Properties must carry a value. For a
	// component that has to be attached to SOMETHING without the rule caring
	// which - the difference between a tile showing a number and a tile showing
	// nothing at all.
	KindRequiresAny Kind = "requires_any"
	// KindNotWithNameLike: Property must not be turned on when the value of
	// Field reads like one of Patterns. This is the shape of a presentation
	// choice that contradicts the data behind it - money formatting on a column
	// whose name says it is a count.
	KindNotWithNameLike Kind = "not_with_name_like"

	// The three below are deliberately domain-neutral.
	//
	// The kinds above this line were all extracted from one agent, and it
	// showed: path_head and not_row_path are page concepts wearing generic
	// clothes, and an author in another domain reaching for "this must be one of
	// these", "this number has bounds" or "these two fields must agree" would
	// have found nothing and gone back to Go. A closed predicate set only works
	// if the set covers what people actually need to say.

	// KindOneOf: Property must hold one of Allowed. The commonest constraint
	// there is, and the one a JSON schema states weakly if the value is dynamic.
	KindOneOf Kind = "one_of"
	// KindRange: Property must be a number within Min/Max, either of which may
	// be left out for an open end.
	KindRange Kind = "range"
	// KindSumOf: Property must equal the sum of Field across the objects at
	// Over. The first constraint here that reads more than one object at once -
	// a total is a claim ABOUT a list, and no per-object check can test it.
	KindSumOf Kind = "sum_of"
	// KindFromEvidence: Property must hold a value the agent actually observed
	// during the run - see evidence.go. The one check that catches an answer
	// which is well formed and about nothing.
	KindFromEvidence Kind = "from_evidence"
	// KindAgreesWith: Property must equal the value of Field. For the constraint
	// no single-field check can express - a total that must match a sum's label,
	// a currency repeated on two sides of a record, an id echoed where it is
	// referenced.
	KindAgreesWith Kind = "agrees_with"
)

// Rule is one constraint, stated as data.
type Rule struct {
	// Match selects the objects this rule applies to: every key here must equal
	// the object's value for that key. An empty Match applies to all of them.
	// {"type": "grid"} is how a studio rule names its component.
	Match map[string]string `json:"match,omitempty"`
	// When and Equals arm the rule: the property named by When must hold one of
	// Equals. An empty When arms it unconditionally.
	When   string   `json:"when,omitempty"`
	Equals []string `json:"equals,omitempty"`
	// WhenSet arms the rule only if every property named here carries a value.
	// It is how a rule says "this only matters when the component is doing the
	// thing at all" without enumerating the values that count as doing it.
	WhenSet []string `json:"when_set,omitempty"`

	Kind Kind `json:"kind,omitempty"`
	// Property is the property the rule is about.
	Property string `json:"property,omitempty"`
	// Suffix selects a family of properties by name instead of Property, for a
	// rule that is about every *_path rather than one of them.
	Suffix string `json:"suffix,omitempty"`
	// Except names properties a Suffix match skips.
	Except []string `json:"except,omitempty"`
	// AllowedHeads are the leading path segments KindPathHead accepts, besides
	// a bare array index.
	AllowedHeads []string `json:"allowed_heads,omitempty"`
	// Properties is the set KindRequiresAny is satisfied by any one of.
	Properties []string `json:"properties,omitempty"`
	// Evidence names the observed-value set KindFromEvidence checks against.
	Evidence string `json:"evidence,omitempty"`
	// Allowed are the values KindOneOf accepts.
	Allowed []string `json:"allowed,omitempty"`
	// Min and Max bound KindRange. Either may be nil for an open end.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Over is a dotted path from this subject to the objects KindSumOf adds up,
	// walking lists the same way RuleSet.Subjects does.
	Over string `json:"over,omitempty"`
	// Tolerance is how far the stated total may be from the computed one before
	// it counts as wrong. Unset means 0.01, because this kind exists for money
	// and comparing currency with exact float equality accuses honest answers:
	// 371070.00 + 139052.00 does not reliably equal 510122.00 in binary floating
	// point. Set it explicitly to 0 to demand exactness.
	Tolerance *float64 `json:"tolerance,omitempty"`
	// Field names the property whose VALUE KindNotWithNameLike reads, and
	// Patterns are the substrings it looks for in it, matched case-insensitively.
	Field    string   `json:"field,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
	// DefaultOn says the property KindNotWithNameLike reads is on when absent.
	// Without it a rule about a default-true setting is silent in exactly the
	// case it exists for: the page that never mentions the property, which is
	// most of them.
	DefaultOn bool `json:"default_on,omitempty"`

	// Severity says whether breaking this rule rejects the answer or only
	// marks it down. Empty means SeverityError.
	Severity Severity `json:"severity,omitempty"`

	Code string `json:"code"`
	// Message is what the model is told. It has to read as an instruction: it is
	// the only thing the model gets to act on.
	//
	// Placeholders: {subject} {property} {value} {head} {field}.
	Message string `json:"message"`
	// Guidance states the same rule before the fact, for the system prompt.
	Guidance string `json:"guidance,omitempty"`
}

// Finding is one broken rule.
type Finding struct {
	Path     string
	Code     string
	Message  string
	Property string
	Severity Severity
}

// Subject is one object a rule is evaluated against: its identity for messages
// and error paths, and the property bag to read.
type Subject struct {
	// Path locates it for the report, e.g. `components[2] (id "kpi")`.
	Path string
	// Name is what {subject} renders as - a component type, an entity name.
	Name string
	// Properties is the bag to check.
	Properties map[string]interface{}
}

// IsExpression spots a value resolved at runtime, which no static check can
// constrain. A rule is satisfied by one: refusing it would ban a legitimate
// answer for being dynamic.
func IsExpression(value string) bool {
	return strings.HasPrefix(value, "@") || strings.Contains(value, "{{")
}

// Armed reports whether this rule applies to this subject.
func (r Rule) Armed(subject Subject) bool {
	for key, want := range r.Match {
		got, _ := subject.Properties[key].(string)
		if !strings.EqualFold(strings.TrimSpace(got), want) {
			return false
		}
	}
	for _, property := range r.WhenSet {
		if !isSet(subject.Properties[property]) {
			return false
		}
	}
	if r.When == "" {
		return true
	}
	value, _ := subject.Properties[r.When].(string)
	value = strings.TrimSpace(value)
	for _, candidate := range r.Equals {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

// render fills the message placeholders.
func (r Rule) render(subject, property, value, head, field, sum string) string {
	return strings.NewReplacer(
		"{subject}", subject,
		"{property}", property,
		"{value}", strconv.Quote(value),
		"{head}", strconv.Quote(head),
		"{field}", strconv.Quote(field),
		// {sum} is rendered bare rather than quoted: it is a number the reader
		// will compare against {value}, and quotation marks around one of two
		// figures being compared just make them harder to read together.
		"{sum}", sum,
	).Replace(r.Message)
}

// Check evaluates one rule against one subject.
func (r Rule) Check(subject Subject) []Finding {
	if !r.Armed(subject) {
		return nil
	}
	switch r.Kind {
	case KindRequires:
		return r.checkRequires(subject)
	case KindForbidden:
		return r.checkForbidden(subject)
	case KindPathHead:
		return r.checkPathHead(subject)
	case KindNotRowPath:
		return r.checkNotRowPath(subject)
	case KindRequiresAny:
		return r.checkRequiresAny(subject)
	case KindNotWithNameLike:
		return r.checkNotWithNameLike(subject)
	case KindOneOf:
		return r.checkOneOf(subject)
	case KindRange:
		return r.checkRange(subject)
	case KindAgreesWith:
		return r.checkAgreesWith(subject)
	case KindSumOf:
		return r.checkSumOf(subject)
	case KindFromEvidence:
		// Dispatched here so the kind is not silently inert, and answered with
		// no evidence: Check has none to give. checkFromEvidence treats an
		// unknown set as "never looked", which faults nothing - the correct
		// reading, and the same one it gives when a lookup did not run.
		//
		// A caller that wants evidence rules ENFORCED must use CheckWith. That
		// is stated on CheckSets, and a guard test asserts every declared kind
		// reaches this switch, because a kind that falls through passes
		// everything it is asked to judge.
		return r.checkFromEvidence(subject, nil)
	}
	return nil
}

func (r Rule) finding(subject Subject, property, value, head, field string) Finding {
	return r.findingWith(subject, property, value, head, field, "")
}

func (r Rule) findingWith(subject Subject, property, value, head, field, sum string) Finding {
	return Finding{
		Path:     subject.Path,
		Code:     r.Code,
		Property: property,
		Message:  r.render(subject.Name, property, value, head, field, sum),
		Severity: r.Severity,
	}
}

// isSet reports whether a property carries anything.
//
// An empty list or object counts as unset. A grid with `column_overrides: []` has
// exactly the problem a rule requiring column_overrides is about, and reading the
// bare presence of the key as satisfaction would let it through.
func isSet(raw interface{}) bool {
	switch value := raw.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(value) != ""
	case []interface{}:
		return len(value) > 0
	case map[string]interface{}:
		return len(value) > 0
	default:
		return true
	}
}

// isActive reports whether a property is switched on or carries a value that
// turns something on. A boolean written as the string "true" and a currency
// symbol written as "GBP" both count; false, "false" and empty do not.
func isActive(raw interface{}) bool {
	switch value := raw.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		trimmed := strings.TrimSpace(value)
		return trimmed != "" && !strings.EqualFold(trimmed, "false")
	case []interface{}:
		return len(value) > 0
	case map[string]interface{}:
		return len(value) > 0
	default:
		return true
	}
}

func (r Rule) checkRequires(subject Subject) []Finding {
	raw := subject.Properties[r.Property]
	if isSet(raw) {
		return nil
	}
	value, _ := raw.(string)
	return []Finding{r.finding(subject, r.Property, value, "", "")}
}

func (r Rule) checkRequiresAny(subject Subject) []Finding {
	for _, property := range r.Properties {
		if isSet(subject.Properties[property]) {
			return nil
		}
	}
	return []Finding{r.finding(subject, strings.Join(r.Properties, ", "), "", "", "")}
}

func (r Rule) checkNotWithNameLike(subject Subject) []Finding {
	raw, present := subject.Properties[r.Property]
	on := isActive(raw)
	if (!present || raw == nil) && r.DefaultOn {
		on = true
	}
	if !on {
		return nil
	}
	name, _ := subject.Properties[r.Field].(string)
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || IsExpression(name) {
		return nil
	}
	for _, pattern := range r.Patterns {
		if strings.Contains(name, strings.ToLower(pattern)) {
			return []Finding{r.finding(subject, r.Property, name, "", r.Field)}
		}
	}
	return nil
}

func (r Rule) checkForbidden(subject Subject) []Finding {
	raw, present := subject.Properties[r.Property]
	if !present || raw == nil {
		return nil
	}
	value, _ := raw.(string)
	return []Finding{r.finding(subject, r.Property, value, "", "")}
}

func (r Rule) checkPathHead(subject Subject) []Finding {
	text, ok := subject.Properties[r.Property].(string)
	if !ok || strings.TrimSpace(text) == "" || IsExpression(text) {
		return nil
	}
	head := strings.TrimSpace(strings.Split(strings.TrimSpace(text), ".")[0])
	if head == "" {
		return nil
	}
	// A leading index is the array shape; anything else is a key the response
	// would have to carry at its top level.
	if _, err := strconv.Atoi(head); err == nil {
		return nil
	}
	for _, allowed := range r.AllowedHeads {
		if head == allowed {
			return nil
		}
	}
	return []Finding{r.finding(subject, r.Property, text, head, "")}
}

func (r Rule) checkNotRowPath(subject Subject) []Finding {
	skip := map[string]bool{}
	for _, name := range r.Except {
		skip[name] = true
	}
	// Sorted, so a subject breaking the rule twice reports in a stable order.
	keys := make([]string, 0, len(subject.Properties))
	for key := range subject.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var out []Finding
	for _, key := range keys {
		if !strings.HasSuffix(key, r.Suffix) || skip[key] {
			continue
		}
		path, _ := subject.Properties[key].(string)
		path = strings.TrimSpace(path)
		if path == "" || IsExpression(path) {
			continue
		}
		field, _ := subject.Properties[strings.TrimSuffix(key, r.Suffix)+"_field"].(string)
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		segments := strings.Split(path, ".")
		_, notAnIndex := strconv.Atoi(strings.TrimSpace(segments[0]))
		startsAtARow := notAnIndex == nil
		repeatsTheField := strings.TrimSpace(segments[len(segments)-1]) == field
		if !startsAtARow && !repeatsTheField {
			continue
		}
		out = append(out, r.finding(subject, key, path, "", field))
	}
	return out
}

// Filter returns the rules of one severity. A caller takes the table it is
// entitled to enforce: the validator may reject only on SeverityError, and the
// quality gate reads only SeverityQuality.
func Filter(rules []Rule, severity Severity) []Rule {
	out := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if rule.Severity == severity {
			out = append(out, rule)
		}
	}
	return out
}

// Check evaluates every rule against every subject.
func Check(rules []Rule, subjects []Subject) []Finding {
	var out []Finding
	for _, subject := range subjects {
		for _, rule := range rules {
			out = append(out, rule.Check(subject)...)
		}
	}
	return out
}

// Prompt renders the guidance, so the model is told what it will be judged on.
// Grouped by the Match that selects each rule, which is how an author thinks
// about them.
func Prompt(rules []Rule, heading string) string {
	byGroup := map[string][]Rule{}
	order := []string{}
	for _, rule := range rules {
		if strings.TrimSpace(rule.Guidance) == "" {
			continue
		}
		group := describeMatch(rule.Match)
		if _, seen := byGroup[group]; !seen {
			order = append(order, group)
		}
		byGroup[group] = append(byGroup[group], rule)
	}
	if len(order) == 0 {
		return ""
	}
	sort.Strings(order)

	var b strings.Builder
	b.WriteString(heading + "\n")
	b.WriteString("These are checked. An answer that breaks one is rejected and sent back to you.\n\n")
	for _, group := range order {
		fmt.Fprintf(&b, "%s:\n", group)
		for _, rule := range byGroup[group] {
			fmt.Fprintf(&b, "- %s\n", rule.Guidance)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func describeMatch(match map[string]string) string {
	if len(match) == 0 {
		return "every component"
	}
	parts := make([]string, 0, len(match))
	for key, value := range match {
		if key == "type" {
			parts = append(parts, value)
			continue
		}
		parts = append(parts, key+" "+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// An absent property is not a wrong one. These three judge a value that is
// there; requiring it at all is KindRequires' job, and saying so twice lets an
// author state the two halves independently.

func (r Rule) checkOneOf(subject Subject) []Finding {
	value, ok := textOf(subject.Properties[r.Property])
	if !ok || strings.TrimSpace(value) == "" || IsExpression(value) {
		return nil
	}
	for _, allowed := range r.Allowed {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(allowed)) {
			return nil
		}
	}
	return []Finding{r.finding(subject, r.Property, value, "", strings.Join(r.Allowed, ", "))}
}

func (r Rule) checkRange(subject Subject) []Finding {
	raw, present := subject.Properties[r.Property]
	if !present || raw == nil {
		return nil
	}
	if text, isText := raw.(string); isText && (strings.TrimSpace(text) == "" || IsExpression(text)) {
		return nil
	}
	value, ok := numberOf(raw)
	if !ok {
		// Not a number at all is a type complaint, and the schema makes it
		// better than this rule could.
		return nil
	}
	if r.Min != nil && value < *r.Min {
		return []Finding{r.finding(subject, r.Property, fmt.Sprint(raw), "", fmt.Sprintf("at least %v", *r.Min))}
	}
	if r.Max != nil && value > *r.Max {
		return []Finding{r.finding(subject, r.Property, fmt.Sprint(raw), "", fmt.Sprintf("at most %v", *r.Max))}
	}
	return nil
}

func (r Rule) checkAgreesWith(subject Subject) []Finding {
	mine, ok := textOf(subject.Properties[r.Property])
	if !ok || strings.TrimSpace(mine) == "" || IsExpression(mine) {
		return nil
	}
	theirs, ok := textOf(subject.Properties[r.Field])
	if !ok || strings.TrimSpace(theirs) == "" || IsExpression(theirs) {
		// Nothing to disagree with.
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(mine), strings.TrimSpace(theirs)) {
		return nil
	}
	return []Finding{r.finding(subject, r.Property, mine, "", theirs)}
}

// defaultSumTolerance is the slack allowed between a stated total and the
// computed one. See Rule.Tolerance for why it is not zero.
const defaultSumTolerance = 0.01

// checkSumOf verifies a total against the things it totals.
//
// The first predicate here that reads more than one object. Every other kind
// judges a subject on its own properties, which is enough for "is this field
// set" and "is this value allowed" - and cannot express the most ordinary
// arithmetic claim a document makes: that a total is the sum of its lines.
//
// It matters more than it looks. A wrong total is the one error in a financial
// answer that is never obviously wrong: every invoice is right, every amount is
// right, and the figure the downstream system reads is not. Nothing else in this
// package could have caught it, and a human reading the output would have to add
// the numbers up to notice.
func (r Rule) checkSumOf(subject Subject) []Finding {
	stated, ok := numberOf(subject.Properties[r.Property])
	if !ok {
		// Absent or not a number. Requiring it at all is KindRequires' job, and
		// saying so twice would report one fault as two.
		return nil
	}

	amounts, found := sumOver(subject.Properties, r.Over, r.Field)
	if !found {
		// The path resolves to nothing at all - no list, not even an empty one.
		// There is nothing to add up, so there is nothing to disagree with.
		return nil
	}

	tolerance := defaultSumTolerance
	if r.Tolerance != nil {
		tolerance = *r.Tolerance
	}
	// Inclusive, and via Abs: with a tolerance of 0 a strict comparison rejects
	// an exact match, because a difference of 0 is not less than 0. Anyone who
	// sets 0 means "these must be equal", and that is the one case a strict
	// test gets wrong.
	if math.Abs(stated-amounts) <= tolerance {
		return nil
	}
	return []Finding{r.findingWith(subject, r.Property,
		strconv.FormatFloat(stated, 'f', -1, 64), "", r.Field,
		strconv.FormatFloat(amounts, 'f', -1, 64))}
}

// sumOver adds up field across every object reachable at path, and reports
// whether the path resolved to anything.
//
// An EMPTY list resolves and sums to zero, which is a real claim - "there are no
// invoices, so the total is nothing" can be checked. A path that resolves to
// nothing at all is different and is not judged.
func sumOver(bag map[string]interface{}, path, field string) (float64, bool) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(field) == "" {
		return 0, false
	}
	total := 0.0
	resolved := false

	var walk func(node interface{}, segments []string)
	walk = func(node interface{}, segments []string) {
		if list, ok := node.([]interface{}); ok {
			// An empty list still counts as resolved: zero is an answer.
			resolved = true
			for _, item := range list {
				walk(item, segments)
			}
			return
		}
		item, ok := node.(map[string]interface{})
		if !ok {
			return
		}
		if len(segments) == 0 {
			resolved = true
			if value, isNumber := numberOf(item[field]); isNumber {
				total += value
			}
			return
		}
		next, present := item[segments[0]]
		if !present {
			return
		}
		walk(next, segments[1:])
	}
	walk(bag, strings.Split(path, "."))
	return total, resolved
}
