package agentspec

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/eru-os/eru/eru-ai/agents/ruleset"
)

// Issue severities. An error means the config will not work as written; a
// warning means it will run but probably not as intended; info is something the
// author may not know.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Issue is one thing wrong with a config, located by a dotted path into it.
type Issue struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// Result is the verdict on one config.
type Result struct {
	Valid          bool    `json:"valid"`
	Issues         []Issue `json:"issues"`
	CatalogVersion string  `json:"catalog_version"`
}

// Environment is what exists in the tenant the config will be saved to. A nil
// member means "not known", and the checks that need it are skipped rather than
// reporting everything as missing.
type Environment struct {
	Models []string
	// Tools maps a tool name to its action names.
	Tools        map[string][]string
	Agents       []string
	VectorStores []string
}

type validator struct {
	config    map[string]interface{}
	env       *Environment
	agentType string
	issues    []Issue
	evidence  map[string]bool
	actions   map[string]bool
}

// Validate checks a config the way the server will read it, and then some: the
// server ignores keys it does not know and never cross-checks a rule against
// the output schema, so a config can save cleanly and still be broken.
func Validate(config map[string]interface{}, env *Environment) Result {
	v := &validator{config: config, env: env, evidence: map[string]bool{}, actions: map[string]bool{}}
	v.run()
	sort.SliceStable(v.issues, func(i, j int) bool {
		return severityRank(v.issues[i].Severity) < severityRank(v.issues[j].Severity)
	})
	valid := true
	for _, issue := range v.issues {
		if issue.Severity == SeverityError {
			valid = false
			break
		}
	}
	if v.issues == nil {
		v.issues = []Issue{}
	}
	return Result{Valid: valid, Issues: v.issues, CatalogVersion: Get().Version}
}

func severityRank(severity string) int {
	switch severity {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	}
	return 2
}

func (v *validator) add(path, severity, code, format string, args ...interface{}) {
	v.issues = append(v.issues, Issue{Path: path, Severity: severity, Code: code, Message: fmt.Sprintf(format, args...)})
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (v *validator) run() {
	v.agentType, _ = v.config["agent_type"].(string)
	typeSpec, known := TypeFor(v.agentType)
	switch {
	case v.agentType == "":
		v.add("agent_type", SeverityError, "type_missing", "agent_type is required")
		return
	case !known:
		v.add("agent_type", SeverityError, "type_unknown", "agent_type %q is not a known type; the server would run it as a bare agent that cannot execute", v.agentType)
		return
	}

	name, _ := v.config["agent_name"].(string)
	if strings.TrimSpace(name) == "" {
		v.add("agent_name", SeverityError, "name_missing", "agent_name is required")
	} else if !namePattern.MatchString(name) {
		v.add("agent_name", SeverityError, "name_invalid", "agent_name %q may only contain letters, digits, _ and -", name)
	}

	v.checkKeys()
	v.checkTypes()
	v.checkModel()
	v.checkTools()
	schema := v.checkOutputSchema(typeSpec)
	v.collectEvidence()
	v.checkRules(schema)
	v.checkClaims()
	v.checkNumbers()
	v.checkOrchestration(name)
	v.checkMemory()
	v.checkLongTermMemory()
	v.checkAdvice(typeSpec, schema)
}

// checkTypes holds every value to the JSON type the server decodes it as. A
// wrong type does not get a friendly message at save time: the whole agent
// fails to unmarshal and the error names nothing.
func (v *validator) checkTypes() {
	for _, field := range fields {
		if value, has := v.config[field.Key]; has {
			v.checkValueType(field.Key, field, value)
		}
	}
	for i, set := range listOfMaps(v.config["validation_rules"]) {
		for j, rule := range listOfMaps(set["rules"]) {
			for key, value := range rule {
				if want := ruleParamType(key); want != "" && !jsonTypeIs(value, want) {
					v.add(fmt.Sprintf("validation_rules[%d].rules[%d].%s", i, j, key), SeverityError, "wrong_type",
						"%s must be %s, got %s", key, describeType(want), describeValue(value))
				}
			}
		}
	}
}

func (v *validator) checkValueType(path string, field FieldSpec, value interface{}) {
	if value == nil {
		return
	}
	want := kindType(field.Kind)
	if want != "" && !jsonTypeIs(value, want) {
		v.add(path, SeverityError, "wrong_type", "%s must be %s, got %s", field.Label, describeType(want), describeValue(value))
		return
	}
	if len(field.Children) == 0 {
		return
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		for _, child := range field.Children {
			if cv, has := typed[child.Key]; has {
				v.checkValueType(path+"."+child.Key, child, cv)
			}
		}
	case []interface{}:
		for i, item := range typed {
			if m, ok := item.(map[string]interface{}); ok {
				for _, child := range field.Children {
					if cv, has := m[child.Key]; has {
						v.checkValueType(fmt.Sprintf("%s[%d].%s", path, i, child.Key), child, cv)
					}
				}
			}
		}
	}
}

// kindType maps a catalog kind to the JSON type it is decoded from.
func kindType(kind string) string {
	switch kind {
	case "int", "number":
		return "number"
	case "bool":
		return "boolean"
	case "string", "text", "enum", "duration", "ref_model", "ref_tool", "ref_action", "ref_agent", "ref_evidence", "ref_vectorstore":
		return "string"
	case "string_list", "ref_agent_list":
		return "string_list"
	case "object", "json_schema":
		return "object"
	case "array":
		return "array"
	}
	return ""
}

func ruleParamType(key string) string {
	switch key {
	case "min", "max", "tolerance":
		return "number"
	case "default_on":
		return "boolean"
	case "match":
		return "object"
	case "equals", "when_set", "except", "allowed_heads", "properties", "allowed", "patterns":
		return "string_list"
	case "kind", "property", "suffix", "evidence", "over", "field", "severity", "code", "message", "guidance", "when":
		return "string"
	}
	return ""
}

func jsonTypeIs(value interface{}, want string) bool {
	switch want {
	case "number":
		_, ok := numberOf(value)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	case "array":
		_, ok := value.([]interface{})
		return ok
	case "string_list":
		list, ok := value.([]interface{})
		if !ok {
			return false
		}
		for _, item := range list {
			if _, isText := item.(string); !isText {
				return false
			}
		}
		return true
	}
	return true
}

func describeType(want string) string {
	switch want {
	case "string_list":
		return "a list of text values"
	case "object":
		return "an object"
	case "array":
		return "a list"
	case "boolean":
		return "true or false"
	}
	return "a " + want
}

func describeValue(value interface{}) string {
	raw, _ := json.Marshal(value)
	text := string(raw)
	if len(text) > 60 {
		text = text[:60] + "…"
	}
	return text
}

// checkKeys reports keys the server will silently drop, keys that belong to
// another type, and keys this type decodes but never acts on.
func (v *validator) checkKeys() {
	for key, value := range v.config {
		field, ok := FieldFor(key)
		if !ok {
			v.add(key, SeverityWarning, "unknown_key", "%q is not an agent setting and will be ignored when saved%s", key, suggest(key, topLevelKeys()))
			continue
		}
		if !field.AppliesTo(v.agentType) {
			if isSet(value) {
				v.add(key, SeverityWarning, "not_for_type", "%s is not read by %s agents", field.Label, v.agentType)
			}
			continue
		}
		if why, ignored := field.Ignored[v.agentType]; ignored && isSet(value) {
			v.add(key, SeverityInfo, "ignored_by_type", "%s has no effect here: %s", field.Label, why)
		}
		if field.Inactive != "" && isSet(value) {
			v.add(key, SeverityInfo, "inactive", "%s: %s", field.Label, field.Inactive)
		}
	}
}

func topLevelKeys() []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.Key)
	}
	return out
}

func childKeys(field FieldSpec) []string {
	out := make([]string, 0, len(field.Children))
	for _, child := range field.Children {
		out = append(out, child.Key)
	}
	return out
}

func (v *validator) checkChildKeys(path string, item map[string]interface{}, allowed []string) {
	set := map[string]bool{}
	for _, key := range allowed {
		set[key] = true
	}
	for key := range item {
		if !set[key] {
			v.add(path+"."+key, SeverityWarning, "unknown_key", "%q is not a setting here and will be ignored%s", key, suggest(key, allowed))
		}
	}
}

func (v *validator) checkModel() {
	model, _ := v.config["model"].(string)
	if strings.TrimSpace(model) == "" {
		v.add("model", SeverityError, "model_missing", "choose a model")
		return
	}
	if v.env != nil && v.env.Models != nil && !contains(v.env.Models, model) {
		v.add("model", SeverityError, "model_unknown", "model %q is not configured in this workspace%s", model, suggest(model, v.env.Models))
	}
}

func (v *validator) checkTools() {
	toolField, _ := FieldFor("agent_tools")
	keys := map[string]string{}
	for i, item := range listOfMaps(v.config["agent_tools"]) {
		path := fmt.Sprintf("agent_tools[%d]", i)
		v.checkChildKeys(path, item, childKeys(toolField))
		toolName, _ := item["tool_name"].(string)
		action, _ := item["action_name"].(string)
		toolKey, _ := item["tool_key"].(string)
		if toolName == "" {
			v.add(path+".tool_name", SeverityError, "tool_missing", "choose a tool")
			continue
		}
		if action != "" {
			v.actions[action] = true
		}
		if toolKey != "" {
			v.actions[toolKey] = true
		}
		key := toolKey
		if key == "" {
			key = toolName
		}
		if earlier, dup := keys[key]; dup {
			v.add(path+".tool_key", SeverityError, "tool_key_duplicate",
				"%s is attached as %q, the same name as %s. The model tells tools apart by this name, so give each attachment its own tool_key", toolName, key, earlier)
		} else {
			keys[key] = path
		}
		if v.env == nil || v.env.Tools == nil {
			continue
		}
		actions, exists := v.env.Tools[toolName]
		if !exists {
			v.add(path+".tool_name", SeverityError, "tool_unknown", "tool %q is not configured in this workspace%s", toolName, suggest(toolName, mapKeys(v.env.Tools)))
			continue
		}
		if action != "" && len(actions) > 0 && !contains(actions, action) {
			v.add(path+".action_name", SeverityError, "action_unknown", "%s has no action %q%s", toolName, action, suggest(action, actions))
		}
		if effect, _ := item["effect"].(map[string]interface{}); effect == nil || effect["read_only"] != true {
			label := action
			if label == "" {
				label = toolName
			}
			v.add(path, SeverityInfo, "tool_can_write", "%s can change data (it is not declared read only) - make sure this agent is meant to", label)
		}
		if action == "" && len(actions) > 1 {
			v.add(path+".action_name", SeverityWarning, "action_missing", "%s has %d actions; choose the one this agent may call", toolName, len(actions))
		}
	}
}

// checkOutputSchema returns the schema as a map for the rule checks, or nil
// when there is none or it does not apply.
func (v *validator) checkOutputSchema(typeSpec TypeSpec) map[string]interface{} {
	schema, _ := v.config["output_schema"].(map[string]interface{})
	if len(schema) == 0 {
		return nil
	}
	v.checkSchemaNode("output_schema", schema)
	if _, ignored := mustField("output_schema").Ignored[v.agentType]; ignored {
		return nil
	}
	return schema
}

var schemaTypes = map[string]bool{"object": true, "array": true, "string": true, "number": true, "integer": true, "boolean": true, "null": true}
var schemaKeys = []string{"type", "properties", "required", "items", "enum", "format", "description", "additionalProperties"}

func (v *validator) checkSchemaNode(path string, node map[string]interface{}) {
	v.checkChildKeys(path, node, schemaKeys)
	if raw, has := node["type"]; has && raw != nil {
		if _, isText := raw.(string); !isText {
			v.add(path+".type", SeverityError, "schema_type_not_single",
				"type must be one JSON type, not %v - the server cannot read a list of types. To let a value be missing, leave the field out of required instead of allowing null", raw)
		}
	}
	nodeType, _ := node["type"].(string)
	if nodeType != "" && !schemaTypes[nodeType] {
		v.add(path+".type", SeverityError, "schema_type_invalid", "%q is not a JSON schema type", nodeType)
	}
	properties, _ := node["properties"].(map[string]interface{})
	for _, required := range stringList(node["required"]) {
		if _, ok := properties[required]; !ok {
			v.add(path+".required", SeverityError, "schema_required_undeclared", "%q is required but not declared under properties", required)
		}
	}
	if nodeType == "array" {
		items, ok := node["items"].(map[string]interface{})
		if !ok {
			v.add(path, SeverityWarning, "schema_array_untyped", "array has no items schema, so nothing about its elements is enforced")
		} else {
			v.checkSchemaNode(path+".items", items)
		}
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if child, ok := properties[name].(map[string]interface{}); ok {
			v.checkSchemaNode(path+".properties."+name, child)
		}
	}
}

func (v *validator) collectEvidence() {
	field := mustField("evidence")
	seen := map[string]bool{}
	for i, item := range listOfMaps(v.config["evidence"]) {
		path := fmt.Sprintf("evidence[%d]", i)
		v.checkChildKeys(path, item, childKeys(field))
		name, _ := item["name"].(string)
		action, _ := item["action"].(string)
		if name == "" {
			v.add(path+".name", SeverityError, "evidence_name_missing", "an evidence set needs a name for rules to refer to")
		} else if seen[name] {
			v.add(path+".name", SeverityError, "evidence_name_duplicate", "evidence set %q is declared twice", name)
		}
		seen[name] = true
		v.evidence[name] = true
		if action == "" {
			v.add(path+".action", SeverityError, "evidence_action_missing", "say which tool action fills %q", name)
		} else if len(v.actions) > 0 && !v.actions[action] {
			v.add(path+".action", SeverityWarning, "evidence_action_not_attached", "%q is filled by action %q, which none of this agent's tools provide, so it will stay empty", name, action)
		}
	}
}

func (v *validator) checkClaims() {
	field := mustField("claims")
	for i, item := range listOfMaps(v.config["claims"]) {
		path := fmt.Sprintf("claims[%d]", i)
		v.checkChildKeys(path, item, childKeys(field))
		if s, _ := item["claims"].(string); s == "" {
			v.add(path+".claims", SeverityError, "claim_path_missing", "say where in the answer the claimed items are")
		}
		action, _ := item["action"].(string)
		if action == "" {
			v.add(path+".action", SeverityError, "claim_action_missing", "say which action must have run for each claim")
		} else if len(v.actions) > 0 && !v.actions[action] {
			v.add(path+".action", SeverityWarning, "claim_action_not_attached", "claims are checked against action %q, which none of this agent's tools provide, so every claim will look unsupported", action)
		}
	}
}

func (v *validator) checkRules(schema map[string]interface{}) {
	setField := mustField("validation_rules")
	ruleKeys := ruleset.RuleKeys()
	allowedRuleKeys := make([]string, 0, len(ruleKeys))
	for key := range ruleKeys {
		allowedRuleKeys = append(allowedRuleKeys, key)
	}
	codes := map[string]string{}
	for i, set := range listOfMaps(v.config["validation_rules"]) {
		setPath := fmt.Sprintf("validation_rules[%d]", i)
		v.checkChildKeys(setPath, set, childKeys(setField))
		subjects, _ := set["subjects"].(string)
		var subject map[string]interface{}
		if schema != nil {
			var ok bool
			subject, ok = schemaAt(schema, subjects)
			if !ok {
				v.add(setPath+".subjects", SeverityWarning, "subjects_not_in_schema",
					"%q does not exist in the output schema, so these rules will never find anything to check", subjects)
			}
		}
		rules := listOfMaps(set["rules"])
		if len(rules) == 0 {
			v.add(setPath+".rules", SeverityWarning, "rule_set_empty", "this rule set has no rules")
		}
		for j, rule := range rules {
			path := fmt.Sprintf("%s.rules[%d]", setPath, j)
			v.checkChildKeys(path, rule, allowedRuleKeys)
			v.checkRule(path, rule, subject, codes)
		}
	}
}

func (v *validator) checkRule(path string, rule map[string]interface{}, subject map[string]interface{}, codes map[string]string) {
	kindName, _ := rule["kind"].(string)
	spec, ok := ruleset.SpecFor(ruleset.Kind(kindName))
	if !ok {
		names := make([]string, 0, len(ruleset.KindSpecs))
		for _, s := range ruleset.KindSpecs {
			names = append(names, string(s.Kind))
		}
		v.add(path+".kind", SeverityError, "rule_kind_unknown", "%q is not a rule kind; a rule of an unknown kind passes everything%s", kindName, suggest(kindName, names))
		return
	}
	for _, param := range spec.Required {
		if !isSet(rule[param]) {
			v.add(path+"."+param, SeverityError, "rule_param_missing", "a %q rule needs %s", spec.Label, param)
		}
	}
	used := map[string]bool{"kind": true}
	for _, list := range [][]string{spec.Required, spec.Optional, ruleset.CommonRuleParams} {
		for _, param := range list {
			used[param] = true
		}
	}
	for key, value := range rule {
		if !used[key] && isSet(value) && ruleset.RuleKeys()[key] {
			v.add(path+"."+key, SeverityWarning, "rule_param_unused", "%s is not read by a %q rule", key, spec.Label)
		}
	}
	code, _ := rule["code"].(string)
	if code == "" {
		v.add(path+".code", SeverityError, "rule_code_missing", "give the rule a code, so a failure can be traced back to it")
	} else if earlier, dup := codes[code]; dup {
		v.add(path+".code", SeverityWarning, "rule_code_duplicate", "code %q is also used by %s; failures will be indistinguishable", code, earlier)
	} else {
		codes[code] = path
	}
	message, _ := rule["message"].(string)
	if strings.TrimSpace(message) == "" {
		v.add(path+".message", SeverityError, "rule_message_missing", "a rule needs a message: it is the only thing the model is told when the rule fails")
	}
	if strings.Contains(message, "{sum}") && spec.Kind != ruleset.KindSumOf {
		v.add(path+".message", SeverityWarning, "placeholder_unfilled", "{sum} is only filled by sum_of rules")
	}
	if strings.Contains(message, "{head}") && spec.Kind != ruleset.KindPathHead {
		v.add(path+".message", SeverityWarning, "placeholder_unfilled", "{head} is only filled by path_head rules")
	}
	if severity, _ := rule["severity"].(string); severity != "" && severity != string(ruleset.SeverityQuality) {
		v.add(path+".severity", SeverityError, "rule_severity_invalid", "severity must be empty (error) or %q", ruleset.SeverityQuality)
	}
	if min, okMin := numberOf(rule["min"]); okMin {
		if max, okMax := numberOf(rule["max"]); okMax && min > max {
			v.add(path, SeverityError, "range_inverted", "min %v is greater than max %v, so nothing can pass", min, max)
		}
	}
	if spec.Kind == ruleset.KindFromEvidence {
		if name, _ := rule["evidence"].(string); name != "" && !v.evidence[name] {
			v.add(path+".evidence", SeverityError, "evidence_undeclared", "evidence set %q is not declared under evidence, so this rule can never fail", name)
		}
	}
	v.checkRuleAgainstSchema(path, spec.Kind, rule, subject)
}

// checkRuleAgainstSchema catches the typo that no other check can: a rule about
// a property the answer never has passes forever.
func (v *validator) checkRuleAgainstSchema(path string, kind ruleset.Kind, rule map[string]interface{}, subject map[string]interface{}) {
	properties, _ := subject["properties"].(map[string]interface{})
	if len(properties) == 0 {
		return
	}
	checkProperty := func(param, name string) (map[string]interface{}, bool) {
		if name == "" {
			return nil, false
		}
		node, ok := properties[name].(map[string]interface{})
		if !ok {
			v.add(path+"."+param, SeverityWarning, "rule_property_not_in_schema", "%q is not a property of the answer at this point%s", name, suggest(name, mapKeys(properties)))
			return nil, false
		}
		return node, true
	}
	property, _ := rule["property"].(string)
	node, found := checkProperty("property", property)
	for _, name := range stringList(rule["properties"]) {
		checkProperty("properties", name)
	}
	switch kind {
	case ruleset.KindRange, ruleset.KindSumOf:
		if found && !isNumeric(node) {
			v.add(path+".property", SeverityWarning, "rule_property_not_numeric", "%q is typed %v in the schema, but this rule compares numbers", property, node["type"])
		}
	case ruleset.KindOneOf:
		if found {
			enum := stringList(node["enum"])
			for _, allowed := range stringList(rule["allowed"]) {
				if len(enum) > 0 && !containsFold(enum, allowed) {
					v.add(path+".allowed", SeverityWarning, "allowed_not_in_enum", "%q is allowed by the rule but not by the schema's enum", allowed)
				}
			}
		}
	case ruleset.KindAgreesWith:
		field, _ := rule["field"].(string)
		checkProperty("field", field)
	}
	if kind == ruleset.KindSumOf {
		over, _ := rule["over"].(string)
		field, _ := rule["field"].(string)
		items, ok := schemaAt(subject, over)
		if !ok {
			v.add(path+".over", SeverityWarning, "sum_over_not_in_schema", "%q does not exist in the schema here, so the sum is always 0", over)
			return
		}
		itemProps, _ := items["properties"].(map[string]interface{})
		fieldNode, ok := itemProps[field].(map[string]interface{})
		if !ok && len(itemProps) > 0 {
			v.add(path+".field", SeverityWarning, "sum_field_not_in_schema", "the items at %q have no property %q%s", over, field, suggest(field, mapKeys(itemProps)))
		} else if ok && !isNumeric(fieldNode) {
			v.add(path+".field", SeverityWarning, "sum_field_not_numeric", "%q is typed %v; only numbers can be summed", field, fieldNode["type"])
		}
	}
}

func (v *validator) checkNumbers() {
	nonNegative := []string{"retry_count", "max_iterations", "thinking_budget", "max_delegation_depth", "max_replans"}
	for _, key := range nonNegative {
		if n, ok := numberOf(v.config[key]); ok && n < 0 {
			v.add(key, SeverityError, "negative", "%s cannot be negative", key)
		}
	}
	budget, _ := v.config["budget"].(map[string]interface{})
	if budget != nil {
		v.checkChildKeys("budget", budget, childKeys(mustField("budget")))
		for key, value := range budget {
			if n, ok := numberOf(value); ok && n < 0 {
				v.add("budget."+key, SeverityError, "negative", "%s cannot be negative", key)
			}
		}
	}
}

func (v *validator) checkOrchestration(self string) {
	if v.agentType != TypeOrchestrator {
		return
	}
	available := stringList(v.config["available_agents"])
	for i, name := range available {
		path := fmt.Sprintf("available_agents[%d]", i)
		if name == self {
			v.add(path, SeverityError, "agent_calls_itself", "an orchestrator cannot delegate to itself")
		} else if v.env != nil && v.env.Agents != nil && !contains(v.env.Agents, name) {
			v.add(path, SeverityError, "agent_unknown", "agent %q does not exist in this workspace%s", name, suggest(name, v.env.Agents))
		}
	}
	for i, name := range stringList(v.config["client_output_agents"]) {
		path := fmt.Sprintf("client_output_agents[%d]", i)
		if len(available) > 0 && !contains(available, name) {
			v.add(path, SeverityWarning, "client_output_not_available", "%q passes results to the app but is not among the agents it may call", name)
		}
	}
	toolsField := mustField("available_tools")
	for i, item := range listOfMaps(v.config["available_tools"]) {
		path := fmt.Sprintf("available_tools[%d]", i)
		v.checkChildKeys(path, item, childKeys(toolsField))
		toolName, _ := item["tool_name"].(string)
		if v.env == nil || v.env.Tools == nil || toolName == "" {
			continue
		}
		actions, ok := v.env.Tools[toolName]
		if !ok {
			v.add(path+".tool_name", SeverityError, "tool_unknown", "tool %q is not configured in this workspace", toolName)
			continue
		}
		for _, action := range stringList(item["actions"]) {
			if len(actions) > 0 && !contains(actions, action) {
				v.add(path+".actions", SeverityError, "action_unknown", "%s has no action %q", toolName, action)
			}
		}
	}
}

func (v *validator) checkMemory() {
	memory, _ := v.config["chat_memory"].(map[string]interface{})
	if memory == nil {
		return
	}
	field := mustField("chat_memory")
	v.checkChildKeys("chat_memory", memory, append(childKeys(field), "cache_values"))
	storeType, _ := memory["cache_store_type"].(string)
	storeField, _ := field.Child("cache_store_type")
	if storeType == "" {
		v.add("chat_memory.cache_store_type", SeverityError, "memory_type_missing", "choose where conversations are kept, or remove chat memory")
		return
	}
	if !contains(storeField.Enum, storeType) && storeType != "ELASTICACHE" {
		v.add("chat_memory.cache_store_type", SeverityError, "memory_type_unknown", "%q is not a memory store", storeType)
		return
	}
	for _, child := range field.Children {
		if !child.Required {
			continue
		}
		if values, conditional := child.ShowWhen["cache_store_type"]; conditional && !contains(values, storeType) {
			continue
		}
		if !isSet(memory[child.Key]) {
			v.add("chat_memory."+child.Key, SeverityError, "memory_setting_missing", "%s is required for %s", child.Label, storeType)
		}
	}
}

func (v *validator) checkLongTermMemory() {
	store, _ := v.config["memory_store"].(string)
	namespace, _ := v.config["memory_namespace"].(string)
	if store == "" {
		if strings.TrimSpace(namespace) != "" {
			v.add("memory_namespace", SeverityWarning, "namespace_without_store", "a memory namespace does nothing without a memory store")
		}
		return
	}
	if v.env != nil && v.env.VectorStores != nil && !contains(v.env.VectorStores, store) {
		v.add("memory_store", SeverityError, "vectorstore_unknown", "vector store %q is not configured in this workspace%s", store, suggest(store, v.env.VectorStores))
	}
}

// checkAdvice is for configs that are valid and probably not what was meant.
func (v *validator) checkAdvice(typeSpec TypeSpec, schema map[string]interface{}) {
	prompt, _ := v.config["system_prompt"].(string)
	if !typeSpec.Specialist && v.agentType != TypeOrchestrator && strings.TrimSpace(prompt) == "" {
		v.add("system_prompt", SeverityWarning, "no_instructions", "the agent has no instructions; it will only know its tools and output schema")
	}
	description, _ := v.config["description"].(string)
	if strings.TrimSpace(description) == "" {
		v.add("description", SeverityInfo, "no_description", "without a description, orchestrators and other agents cannot tell when to use this one")
	}
	rules := listOfMaps(v.config["validation_rules"])
	if _, ignored := mustField("validation_rules").Ignored[v.agentType]; ignored || len(rules) == 0 {
		return
	}
	if schema == nil && !typeSpec.Specialist {
		v.add("validation_rules", SeverityWarning, "rules_without_schema",
			"rules judge the fields of a structured answer, but there is no output schema - the answer will be {\"output\": \"<text>\"} and most rules will find nothing")
	}
	hasErrorRule := false
	for _, set := range rules {
		for _, rule := range listOfMaps(set["rules"]) {
			if severity, _ := rule["severity"].(string); severity == "" {
				hasErrorRule = true
			}
		}
	}
	if retries, _ := numberOf(v.config["retry_count"]); hasErrorRule && retries == 0 {
		v.add("retry_count", SeverityWarning, "rules_without_retries",
			"retry_count is 0, so an answer that breaks a rule fails the request instead of being sent back to be fixed")
	}
}

// schemaAt walks a dotted path through a schema the way RuleSet.Subjects walks
// an answer: arrays are stepped into transparently.
func schemaAt(schema map[string]interface{}, path string) (map[string]interface{}, bool) {
	node := unwrapArrays(schema)
	path = strings.TrimSpace(path)
	if path == "" {
		return node, true
	}
	for _, segment := range strings.Split(path, ".") {
		properties, _ := node["properties"].(map[string]interface{})
		next, ok := properties[segment].(map[string]interface{})
		if !ok {
			return nil, false
		}
		node = unwrapArrays(next)
	}
	return node, true
}

func unwrapArrays(node map[string]interface{}) map[string]interface{} {
	for {
		if t, _ := node["type"].(string); t != "array" {
			return node
		}
		items, ok := node["items"].(map[string]interface{})
		if !ok {
			return node
		}
		node = items
	}
}

func mustField(key string) FieldSpec {
	field, _ := FieldFor(key)
	return field
}

func isNumeric(node map[string]interface{}) bool {
	t, _ := node["type"].(string)
	return t == "" || t == "number" || t == "integer"
}

func isSet(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case bool:
		return typed
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case []interface{}:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	}
	return true
}

func numberOf(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	}
	return 0, false
}

func listOfMaps(value interface{}) []map[string]interface{} {
	list, _ := value.([]interface{})
	out := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringList(value interface{}) []string {
	list, _ := value.([]interface{})
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func containsFold(list []string, want string) bool {
	for _, item := range list {
		if strings.EqualFold(item, want) {
			return true
		}
	}
	return false
}

// suggest names the closest candidate when it is plausibly a typo.
func suggest(got string, candidates []string) string {
	best, bestDistance := "", 1<<30
	for _, candidate := range candidates {
		if d := distance(strings.ToLower(got), strings.ToLower(candidate)); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	if best == "" || bestDistance > 2+len(got)/4 {
		return ""
	}
	return fmt.Sprintf(" - did you mean %q?", best)
}

func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
