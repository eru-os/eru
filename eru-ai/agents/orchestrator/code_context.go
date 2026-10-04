package orchestrator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	agents "github.com/eru-os/eru/eru-ai/agents"
)

const (
	codeParamKey       = "code"
	codePreviewLimit   = 800
	codeFingerprintLen = 48
)

var sqlPrefixPattern = regexp.MustCompile(`(?is)^\s*(with|select|insert\s+into|update|delete\s+from|create\s+(table|view|index|function)|alter\s+table|drop\s+(table|view))\b`)

// codeContext describes the existing structured output the caller sent in
// params.code. The orchestrator plans with this description only - never with
// the artifact itself - so the planner can decide which sub-agents need it
// without the whole blob entering the planning prompt.
type codeContext struct {
	Present   bool
	Kind      string
	Size      int
	TopKeys   []string
	Preview   string
	Truncated bool
	code      string
	// ForwardParams are the request-shaping params the caller sent alongside the
	// artifact. They change the shape of the answer rather than its content, so a
	// plan that drops one silently gives the caller a different protocol than it
	// asked for.
	ForwardParams []string
	// UserMessage is what the user actually typed. A plan that rewrites it into
	// its own words for a single agent step throws away everything the message
	// was leaning on - "it", "that one", "blue" meaning the thing named a moment
	// ago - and the agent, which does have the conversation, is handed a
	// self-contained instruction instead and follows it literally.
	UserMessage string
	// Attachments are the files the user sent with this message, by name. A
	// plan that forwards none of them to any step has quietly decided the user
	// attached something for no reason.
	Attachments []string
}

// callerForwardParams lists the request-shaping params present on the incoming
// request, in a stable order.
func callerForwardParams(params map[string]interface{}) []string {
	out := []string{}
	for _, name := range agents.ForwardableParams {
		if value, ok := params[name]; ok && value != nil && value != "" {
			out = append(out, name)
		}
	}
	return out
}

// describeCodeParam characterises params.code: what kind of artifact it is, how
// big it is, its top-level keys and a short preview. A blank, empty or null
// artifact is treated as absent, matching how the sub-agents themselves ignore it.
func describeCodeParam(params map[string]interface{}) codeContext {
	if params == nil {
		return codeContext{}
	}
	forward := callerForwardParams(params)
	raw, found := params[codeParamKey]
	if !found {
		if len(forward) > 0 {
			// No artifact, but the caller still shaped the response it expects.
			return codeContext{ForwardParams: forward}
		}
		return codeContext{}
	}
	code := strings.TrimSpace(stringifyCodeParam(raw))
	switch code {
	case "", "{}", "[]", "null", `""`:
		return codeContext{ForwardParams: forward}
	}
	kind, topKeys := classifyCode(code)
	preview, truncated := previewCode(code)
	return codeContext{
		Present:       true,
		Kind:          kind,
		Size:          len(code),
		TopKeys:       topKeys,
		Preview:       preview,
		Truncated:     truncated,
		code:          code,
		ForwardParams: forward,
	}
}

func stringifyCodeParam(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// classifyCode labels the artifact and, for JSON objects, returns its top-level
// keys - the strongest cheap signal of which agent produced it.
func classifyCode(code string) (string, []string) {
	if strings.HasPrefix(code, "{") {
		var obj map[string]interface{}
		if json.Unmarshal([]byte(code), &obj) == nil {
			keys := sortedMapKeys(obj)
			switch {
			case hasAnyKey(obj, "components", "eru_page", "page"):
				return "eru_page_json", keys
			case hasAnyKey(obj, "func_steps", "func_group_name"), isFuncGroupWrapper(obj):
				return "func_group_json", keys
			}
			return "json_object", keys
		}
	}
	if strings.HasPrefix(code, "[") {
		var arr []interface{}
		if json.Unmarshal([]byte(code), &arr) == nil {
			return "json_array", nil
		}
	}
	if sqlPrefixPattern.MatchString(code) {
		return "sql", nil
	}
	if strings.Contains(code, "{{") && strings.Contains(code, "}}") {
		return "go_template", nil
	}
	if strings.HasPrefix(code, "<") {
		return "markup", nil
	}
	return "text", nil
}

func previewCode(code string) (string, bool) {
	if len(code) <= codePreviewLimit {
		return code, false
	}
	return code[:codePreviewLimit], true
}

func sortedMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// isFuncGroupWrapper reports whether obj is the eru_func agent's wrapper output
// - {"function": <func group>, "trigger": {...}} - instead of a bare func group.
func isFuncGroupWrapper(m map[string]interface{}) bool {
	fn, ok := m["function"].(map[string]interface{})
	if !ok {
		return false
	}
	return hasAnyKey(fn, "func_steps", "func_group_name")
}

func hasAnyKey(m map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if _, found := m[key]; found {
			return true
		}
	}
	return false
}

// promptSection is appended to the planning system prompt only when
// params.code is present. It tells the planner what the artifact is and that it
// must be routed - by reference, to the steps that actually revise it - rather
// than broadcast to every step.
func (cc codeContext) promptSection(discovered []agents.DiscoveredAgent) string {
	if !cc.Present {
		return cc.forwardParamsSection(discovered)
	}
	var sb strings.Builder
	sb.WriteString(`

============================================================
RULE #2c - INCOMING params.code : ROUTE IT, DO NOT BROADCAST IT
============================================================

The caller sent an EXISTING structured output (produced by an earlier run) in the
request payload. It is available to every step as .Vars.OrgBody.params.code, but NO
step receives it unless your transform_request passes it explicitly.

What was sent:
`)
	sb.WriteString(fmt.Sprint("  Detected kind : ", cc.Kind, "\n"))
	sb.WriteString(fmt.Sprint("  Size          : ", cc.Size, " characters\n"))
	if len(cc.TopKeys) > 0 {
		sb.WriteString(fmt.Sprint("  Top-level keys: ", strings.Join(cc.TopKeys, ", "), "\n"))
	}
	if candidates := cc.candidateAgents(discovered); len(candidates) > 0 {
		sb.WriteString(fmt.Sprint("  Agents that produce this shape of output: ", strings.Join(candidates, ", "),
			" (a hint, not an instruction - confirm against the user's request)\n"))
	}
	if revisers := codeCapableAgents(discovered); len(revisers) > 0 {
		sb.WriteString(fmt.Sprint("  Agents that declare a \"code\" params key (the only ones that can revise an artifact): ",
			strings.Join(revisers, ", "), "\n"))
	} else {
		sb.WriteString("  No available agent declares a \"code\" params key, so no step in this plan can revise the artifact - do not route it anywhere.\n")
	}
	if cc.Truncated {
		sb.WriteString(fmt.Sprint("  Preview (first ", codePreviewLimit, " characters, TRUNCATED - the step you route it to receives the whole artifact):\n"))
	} else {
		sb.WriteString("  Full value:\n")
	}
	sb.WriteString(indentBlock(cc.Preview, "    "))
	sb.WriteString(`

HOW TO DECIDE, per step:
- Route params.code ONLY to a step whose job is to produce a NEW VERSION of THIS
  SAME artifact - i.e. the step's output is the same kind of thing as above (page
  JSON to the page/widget agent, SQL to the SQL-generating agent, a go template to
  the template agent, and so on). Such a step must build on the existing artifact
  instead of starting from scratch.
- Usually that is EXACTLY ONE step. Do NOT pass it to steps that fetch or compute
  data, classify, route, validate, summarise, or produce a different kind of
  output: it is irrelevant noise to them, costs tokens, and misleads them.
- If NO step in your plan revises this artifact (the user's request is about
  something else entirely), do not reference params.code anywhere. That is a valid
  and expected outcome.
- If the artifact was clearly produced by an agent you are NOT using in this plan,
  do not route it at all.

Only an agent whose "Params keys this agent READS" line includes "code" can receive
it. Passing params.code to any other agent silently discards it.

HOW TO PASS IT (by reference - never paste the artifact into the template):
  "transform_request": "{{stringify (dict \"content\" .Vars.OrgBody.content \"params\" (dict \"code\" .Vars.OrgBody.params.code))}}"

Combined with fetched data for the same step (Rule #2b), both keys go in one params dict:
  "transform_request": "{{stringify (dict \"content\" .Vars.OrgBody.content \"params\" (dict \"code\" .Vars.OrgBody.params.code \"context\" (stringify .ResVars.<data_step>.Body)))}}"

WRONG:
  pasting the artifact's text/JSON literally into transform_request  -> bloats the plan, breaks the template on quotes
  adding params.code to every step                                  -> information overload, wasted tokens
  "params" (dict "code" .Vars.OrgBody.content)                          -> that is the user's instruction, not the artifact

CHECKLIST ADDITION:
[ ] params.code is passed - by .Vars.OrgBody.params.code reference - only to the step(s) that revise the artifact described above, and to no other step`)
	return sb.String()
}

// forwardParamsSection tells the planner which request-shaping params the caller
// set and that it does not need to forward them: every agent it calls inherits
// them through the call chain (agents.HeaderCallerParams).
func (cc codeContext) forwardParamsSection(discovered []agents.DiscoveredAgent) string {
	if len(cc.ForwardParams) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprint("\n\nNOTE - the caller set response-shaping params: ", strings.Join(cc.ForwardParams, ", "), ".\n"))
	sb.WriteString("Every agent you call that reads one of them receives the caller's value automatically. Do NOT add\n")
	sb.WriteString("them to a step's params; only set one on a step when that step deliberately needs a DIFFERENT value.\n")
	if capable := paramCapableAgents(discovered, cc.ForwardParams); len(capable) > 0 {
		sb.WriteString(fmt.Sprint("Agents here that read at least one of them: ", strings.Join(capable, ", "), "\n"))
	}
	return sb.String()
}

// paramCapableAgents lists the discovered agents that read at least one of the
// named params.
func paramCapableAgents(discovered []agents.DiscoveredAgent, names []string) []string {
	out := []string{}
	for _, agent := range discovered {
		for _, key := range agent.ParamKeys() {
			if containsString(names, key) {
				out = append(out, agent.AgentName)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func codeCapableAgents(discovered []agents.DiscoveredAgent) []string {
	var names []string
	for _, ad := range discovered {
		for _, key := range ad.ParamKeys() {
			if key == codeParamKey {
				names = append(names, ad.AgentName)
				break
			}
		}
	}
	return names
}

// candidateAgents names the discovered agents whose declared output looks like
// the incoming artifact, either by output-field overlap with its top-level keys
// or by a keyword match on the detected kind.
func (cc codeContext) candidateAgents(discovered []agents.DiscoveredAgent) []string {
	keySet := make(map[string]bool)
	for _, key := range cc.TopKeys {
		keySet[strings.ToLower(key)] = true
	}
	var candidates []string
	for _, ad := range discovered {
		fields := outputFieldNames(ad.OutputSchema)
		overlap := 0
		kindMatch := false
		for _, field := range fields {
			lower := strings.ToLower(field)
			if keySet[lower] {
				overlap++
			}
			if cc.Kind == "sql" && lower == "sql" {
				kindMatch = true
			}
			if cc.Kind == "go_template" && (lower == "template" || lower == "code") {
				kindMatch = true
			}
		}
		if overlap >= 2 || kindMatch {
			candidates = append(candidates, ad.AgentName)
		}
	}
	sort.Strings(candidates)
	return candidates
}

func indentBlock(text string, indent string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// fingerprint returns a normalised slice of the artifact used to detect a plan
// that pasted the artifact into a template instead of referencing it. Quotes,
// escapes and whitespace are stripped so the match survives JSON escaping and
// reformatting.
func (cc codeContext) fingerprint() string {
	if !cc.Present {
		return ""
	}
	normalised := normaliseForFingerprint(cc.code)
	if len(normalised) < codeFingerprintLen {
		return ""
	}
	start := len(normalised) / 4
	if start+codeFingerprintLen > len(normalised) {
		start = len(normalised) - codeFingerprintLen
	}
	return normalised[start : start+codeFingerprintLen]
}

func normaliseForFingerprint(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, c := range s {
		switch c {
		case ' ', '\t', '\n', '\r', '"', '\'', '\\':
			continue
		}
		sb.WriteRune(c)
	}
	return sb.String()
}
