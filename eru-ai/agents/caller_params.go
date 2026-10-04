package agents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
)

// HeaderCallerParams carries the caller's response-shaping params down every
// delegation hop, beside the agent chain and the stream target. eru-functions
// passes unknown headers through untouched, so a sub-agent receives what the
// original caller asked for without the plan having to spell it out.
const HeaderCallerParams = "Eru-Caller-Params"

// ForwardableParams are the params whose whole purpose is to change the
// response contract. A caller that sets one has already written the code to
// read the result it produces, so every agent on the way down that reads one
// must get the caller's value - which is exactly how an "output_mode: auto"
// request came back as a single full page when a plan left it out.
var ForwardableParams = []string{"output_mode", "inline_nested_pages", "base_revision", "scope", "page_id", "client_capabilities"}

func IsForwardableParam(name string) bool {
	for _, candidate := range ForwardableParams {
		if candidate == name {
			return true
		}
	}
	return false
}

func emptyParam(value interface{}) bool {
	return value == nil || value == ""
}

// CallerParams picks the forwardable params present on a request.
func CallerParams(params map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, name := range ForwardableParams {
		if value, ok := params[name]; ok && !emptyParam(value) {
			out[name] = value
		}
	}
	return out
}

type callerParamsKey struct{}

// WithCallerParams records the params inherited from further up the chain, so
// an agent that does not read them itself - an orchestrator in the middle -
// still hands them on to the agents it calls.
func WithCallerParams(ctx context.Context, params map[string]interface{}) context.Context {
	if len(params) == 0 {
		return ctx
	}
	return context.WithValue(ctx, callerParamsKey{}, params)
}

// OutgoingCallerParams is what a delegation from this request passes down:
// the inherited params, overridden by any this request set itself.
func OutgoingCallerParams(ctx context.Context, own map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	if inherited, ok := ctx.Value(callerParamsKey{}).(map[string]interface{}); ok {
		for name, value := range inherited {
			out[name] = value
		}
	}
	for name, value := range CallerParams(own) {
		out[name] = value
	}
	return out
}

// EncodeCallerParams renders forwardable params for the HeaderCallerParams
// header, or "" when there are none.
func EncodeCallerParams(params map[string]interface{}) string {
	picked := CallerParams(params)
	if len(picked) == 0 {
		return ""
	}
	encoded, err := json.Marshal(picked)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// DecodeCallerParams reads a HeaderCallerParams value. Anything that is not a
// forwardable param is dropped, so the header can only ever carry what a caller
// could have put in the request body itself.
func DecodeCallerParams(header string) map[string]interface{} {
	if header == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		return nil
	}
	decoded := map[string]interface{}{}
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	return CallerParams(decoded)
}

// InheritCallerParams fills, on a sub-agent's request, each inherited param the
// agent declares and the step did not set itself. A value the plan wrote wins:
// the planner may have a reason to ask this one step for something else.
// Returns the names it filled.
func InheritCallerParams(message *AgentMessage, inherited map[string]interface{}, declared []string) []string {
	if message == nil || len(inherited) == 0 || len(declared) == 0 {
		return nil
	}
	var filled []string
	for _, name := range declared {
		value, ok := inherited[name]
		if !ok {
			continue
		}
		if current, set := message.Params[name]; set && !emptyParam(current) {
			continue
		}
		if message.Params == nil {
			message.Params = map[string]interface{}{}
		}
		message.Params[name] = value
		filled = append(filled, name)
	}
	sort.Strings(filled)
	return filled
}
