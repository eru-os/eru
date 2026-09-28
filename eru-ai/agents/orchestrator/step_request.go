package orchestrator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// A step's request body reaches its agent through `transform_request`, a Go
// template the planner used to write by hand. That was the single most
// failure-prone thing it produced: the body is usually a long instruction with
// quotes and newlines, wrapped in nested `stringify (dict ...)` calls, and the
// parentheses stop balancing. Three plan-repair rounds in a row have died on
// "unexpected right paren" with the model mis-balancing somewhere new each time,
// even though the rejection message told it exactly how to avoid the problem.
//
// So the planner no longer writes the template. It describes the request, and
// this compiles it - correctly quoted and balanced by construction, because
// nothing here is typed by a model.
//
//	"request": {
//	  "content": "Build a page that ...",
//	  "params":  {"code": {"from": "user.params.code"}},
//	  "files":   {"from": "user.files"}
//	}
//
// A value is either a literal (any JSON value), or a reference:
//
//	{"from": "user.content"}            the user's message
//	{"from": "user.params.<key>"}       a param the caller sent
//	{"from": "<step>.<field>"}          an earlier step's output field
//	{"join": [ ... ]}                   several of the above, concatenated
//
// `transform_request` is still accepted for anything this cannot express, so
// nothing that worked before stops working.

const stepRequestKey = "request"

const userRequestRoot = ".Vars.OrgBody"

// compileStepRequests rewrites every `request` in a plan into the
// `transform_request` the executor runs. It reports what it could not compile in
// the same form as plan validation, so a bad reference comes back to the model
// as an ordinary plan rejection.
func compileStepRequests(plan map[string]interface{}) []planIssue {
	steps, ok := plan["func_steps"].(map[string]interface{})
	if !ok {
		return nil
	}
	return compileStepMap(steps, "")
}

func compileStepMap(steps map[string]interface{}, prefix string) []planIssue {
	issues := []planIssue{}
	names := make([]string, 0, len(steps))
	for name := range steps {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		step, ok := steps[name].(map[string]interface{})
		if !ok {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}

		if raw, present := step[stepRequestKey]; present {
			// A hand-written template wins, so a plan that sets both is not
			// silently rewritten under the author.
			if existing, _ := step["transform_request"].(string); strings.TrimSpace(existing) == "" {
				template, err := buildTransformRequest(raw)
				if err != nil {
					issues = append(issues, planIssue{StepPath: path, Field: stepRequestKey, Err: err.Error()})
				} else {
					step["transform_request"] = template
				}
			}
			delete(step, stepRequestKey)
		}

		if nested, ok := step["func_steps"].(map[string]interface{}); ok {
			issues = append(issues, compileStepMap(nested, path)...)
		}
	}
	return issues
}

// buildTransformRequest renders one request description as a template.
//
// The output is the JSON form: a literal JSON body with {{...}} only where a
// value has to be interpolated. There is no dict call and nothing to balance.
func buildTransformRequest(raw interface{}) (string, error) {
	request, ok := raw.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("\"request\" must be an object with \"content\" and optionally \"params\"")
	}
	_, hasContent := request["content"]
	_, hasParams := request["params"]
	_, hasFiles := request["files"]
	if !hasContent && !hasParams && !hasFiles {
		// An agent step needs "content"; a tool step is all params. Requiring
		// neither would compile an empty body nobody asked for.
		return "", fmt.Errorf("\"request\" needs \"content\" (an agent's input message) or \"params\" (a tool's inputs)")
	}

	var b strings.Builder
	b.WriteString("{")
	if hasContent {
		b.WriteString("\"content\": ")
		content, err := renderValue(request["content"])
		if err != nil {
			return "", fmt.Errorf("content: %w", err)
		}
		b.WriteString(content)
	}

	if rawParams, present := request["params"]; present {
		params, ok := rawParams.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("\"params\" must be an object")
		}
		keys := make([]string, 0, len(params))
		for key := range params {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		if hasContent {
			b.WriteString(", ")
		}
		b.WriteString("\"params\": {")
		for i, key := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			encodedKey, err := json.Marshal(key)
			if err != nil {
				return "", err
			}
			value, err := renderValue(params[key])
			if err != nil {
				return "", fmt.Errorf("params.%s: %w", key, err)
			}
			b.Write(encodedKey)
			b.WriteString(": ")
			b.WriteString(value)
		}
		b.WriteString("}")
	}

	// files sits beside content at the top of the body, not inside params.
	// Every agent decodes the request into the same message shape, so this is
	// not something an agent opts into - it is how an attachment reaches any of
	// them, exactly as it reaches this orchestrator from the browser.
	if hasFiles {
		files, err := renderValue(request["files"])
		if err != nil {
			return "", fmt.Errorf("files: %w", err)
		}
		if hasContent || hasParams {
			b.WriteString(", ")
		}
		b.WriteString("\"files\": ")
		b.WriteString(files)
	}

	b.WriteString("}")
	return b.String(), nil
}

// renderValue turns one described value into a JSON fragment: a literal encoded
// here, or a template action that interpolates one.
func renderValue(value interface{}) (string, error) {
	switch typed := value.(type) {
	case map[string]interface{}:
		if from, present := typed["from"]; present {
			reference, ok := from.(string)
			if !ok {
				return "", fmt.Errorf("\"from\" must be a string")
			}
			expression, err := referenceExpression(reference)
			if err != nil {
				return "", err
			}
			return "{{stringify " + expression + "}}", nil
		}
		// A tool answers with a plain body rather than the agent action
		// envelope, so there is no field to name - the whole body is the value.
		if fromBody, present := typed["from_body"]; present {
			step, ok := fromBody.(string)
			if !ok || strings.TrimSpace(step) == "" || strings.ContainsAny(step, " \"'()") {
				return "", fmt.Errorf("\"from_body\" must name a step")
			}
			return "{{stringify .ResVars." + step + ".Body}}", nil
		}
		if join, present := typed["join"]; present {
			return renderJoin(join)
		}
		// Any other object is a literal object - a params value can be one.
		return encodeLiteral(typed)
	default:
		return encodeLiteral(value)
	}
}

// renderJoin concatenates parts into one string. The format string is built
// here, so the only thing that could be mis-typed is a reference name.
func renderJoin(raw interface{}) (string, error) {
	parts, ok := raw.([]interface{})
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("\"join\" must be a non-empty array")
	}
	format := strings.Repeat("%s", len(parts))
	expressions := make([]string, 0, len(parts))
	for _, part := range parts {
		if object, ok := part.(map[string]interface{}); ok {
			if from, present := object["from"]; present {
				reference, ok := from.(string)
				if !ok {
					return "", fmt.Errorf("\"from\" must be a string")
				}
				expression, err := referenceExpression(reference)
				if err != nil {
					return "", err
				}
				expressions = append(expressions, expression)
				continue
			}
			return "", fmt.Errorf("every entry of \"join\" must be a string or {\"from\": ...}")
		}
		text, ok := part.(string)
		if !ok {
			return "", fmt.Errorf("every entry of \"join\" must be a string or {\"from\": ...}")
		}
		encoded, err := json.Marshal(text)
		if err != nil {
			return "", err
		}
		// A literal inside printf must not be read as a format verb.
		expressions = append(expressions, string(encoded))
	}
	encodedFormat, err := json.Marshal(format)
	if err != nil {
		return "", err
	}
	return "{{stringify (printf " + string(encodedFormat) + " " + strings.Join(expressions, " ") + ")}}", nil
}

// referenceExpression turns a reference into the template expression that reads
// it. This is the only place those paths are written, so the shapes the planner
// used to get wrong - .Body.content on an agent step, a missing index call - are
// not expressible.
func referenceExpression(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	switch {
	case reference == "user.content":
		return userRequestRoot + ".content", nil
	case strings.HasPrefix(reference, "user.params."):
		key := strings.TrimPrefix(reference, "user.params.")
		if key == "" || strings.Contains(key, " ") {
			return "", fmt.Errorf("%q does not name a param", reference)
		}
		return userRequestRoot + ".params." + key, nil
	case reference == "user.params":
		return userRequestRoot + ".params", nil
	case reference == "user.files":
		// The attachments the user sent with this message. They already sit in
		// the request body alongside content and params; what was missing was
		// any way for a plan to name them, which left the orchestrator able to
		// see an attached image and unable to hand it on.
		return userRequestRoot + ".files", nil
	case strings.HasPrefix(reference, "user.files."):
		index := strings.TrimPrefix(reference, "user.files.")
		if !isIndex(index) {
			return "", fmt.Errorf("%q does not name an attachment - use \"user.files\" for all of them, or \"user.files.0\" for the first", reference)
		}
		return "(index " + userRequestRoot + ".files " + index + ")", nil
	default:
		step, field, found := strings.Cut(reference, ".")
		if !found || step == "" || field == "" {
			return "", fmt.Errorf("%q is not a reference - use \"user.content\", \"user.params.<key>\", \"user.files\" or \"<step>.<output field>\"", reference)
		}
		if strings.ContainsAny(step+field, " \"'()") {
			return "", fmt.Errorf("%q is not a reference", reference)
		}
		return "(index .ResVars." + step + ".Body.actions 0).action." + field, nil
	}
}

func encodeLiteral(value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("value could not be encoded: %w", err)
	}
	return string(encoded), nil
}

// isIndex reports a non-negative integer written in decimal.
func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
