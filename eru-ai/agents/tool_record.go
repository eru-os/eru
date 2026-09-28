package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	tools "github.com/eru-os/eru/eru-ai/tools"
)

// ToolInvocation is one tool call as it happened: which tool, which action, the
// arguments it was given, and how it ended.
//
// Recorded at the tool boundary rather than by the loop that made the call.
// Loops differ - the shared reasoning loop traces every call, the page agent
// runs its own plan/build loop and traces some of them - so anything derived
// from traces inherits whichever loop happened to run. A tool cannot avoid
// recording itself, so this sees every call however it was made.
type ToolInvocation struct {
	Seq        int                    `json:"seq"`
	Tool       string                 `json:"tool"`
	Action     string                 `json:"action,omitempty"`
	Args       map[string]interface{} `json:"args,omitempty"`
	Ok         bool                   `json:"ok"`
	Error      string                 `json:"error,omitempty"`
	DurationMs int64                  `json:"duration_ms,omitempty"`
}

// ToolRecord is every invocation of one run, in order.
type ToolRecord struct {
	mu    sync.Mutex
	calls []ToolInvocation
}

func NewToolRecord() *ToolRecord { return &ToolRecord{} }

// Add appends an invocation. Safe on a nil record so a caller never has to ask.
func (r *ToolRecord) Add(call ToolInvocation) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	call.Seq = len(r.calls) + 1
	r.calls = append(r.calls, call)
}

// Calls is a copy of what ran.
func (r *ToolRecord) Calls() []ToolInvocation {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ToolInvocation(nil), r.calls...)
}

// Tally counts calls per tool, for the run metrics.
func (r *ToolRecord) Tally() map[string]int {
	out := map[string]int{}
	for _, call := range r.Calls() {
		out[call.Tool]++
	}
	return out
}

type toolRecordKey struct{}

// WithToolRecord puts a fresh record on the context for one run.
func WithToolRecord(ctx context.Context, record *ToolRecord) context.Context {
	return context.WithValue(ctx, toolRecordKey{}, record)
}

// ToolRecordFrom returns this run's record, or nil outside a run. Every method
// on ToolRecord tolerates nil, so callers do not branch.
func ToolRecordFrom(ctx context.Context) *ToolRecord {
	if ctx == nil {
		return nil
	}
	record, _ := ctx.Value(toolRecordKey{}).(*ToolRecord)
	return record
}

// Recording wraps a tool so that every call it receives is recorded.
//
// The interface is embedded, so only Execute is overridden and the wrapper
// cannot fall behind as the interface grows.
func Recording(tool tools.Tooling) tools.Tooling {
	if tool == nil {
		return nil
	}
	// Idempotent, so a tool reachable through two registries is wrapped once and
	// its calls are counted once.
	if _, already := tool.(*recordingTool); already {
		return tool
	}
	return &recordingTool{Tooling: tool}
}

type recordingTool struct {
	tools.Tooling
}

func (t *recordingTool) Execute(ctx context.Context, projectId string, tenantId string, actionName string, params map[string]interface{}) (map[string]interface{}, bool, error) {
	// A repeat of a write already performed in this run is answered with the
	// first call's result rather than performed again. The loop retries; the
	// tools it calls have side effects; without this the two combine into an
	// entity saved once per attempt.
	if earlier, suppressed := guardWrite(ctx, actionName, params); suppressed {
		return earlier, false, nil
	}

	started := time.Now()
	result, persist, err := t.Tooling.Execute(ctx, projectId, tenantId, actionName, params)

	// Evidence is collected whether or not the run keeps a tool record: the two
	// answer different questions, and a rule that asks "did you see this" must
	// not depend on metrics being switched on.
	collectEvidence(ctx, actionName, params, result, err == nil)

	record := ToolRecordFrom(ctx)
	if record == nil {
		return result, persist, err
	}
	name := actionName
	if nameI, gErr := t.Tooling.GetAttribute(ctx, "tool_name"); gErr == nil {
		if toolName, ok := nameI.(string); ok && toolName != "" {
			name = toolName
		}
	}
	call := ToolInvocation{
		Tool:       name,
		Action:     actionName,
		Args:       safeArgs(params),
		Ok:         err == nil,
		DurationMs: time.Since(started).Milliseconds(),
	}
	if err != nil {
		call.Error = err.Error()
	}
	record.Add(call)
	if err == nil {
		if writes := WriteOnceFrom(ctx); writes != nil {
			writes.Record(actionName, params, result)
		}
	}
	return result, persist, err
}

// maxRecordedValue caps one argument's rendered size. A saved page's definition
// runs to tens of kilobytes and would otherwise be carried back in full on every
// reply, for no benefit: what an assertion needs from save_page is that it was
// called and for which page, not the design.
const maxRecordedValue = 512

// safeArgs is the arguments as they are worth keeping: secrets removed, large
// values summarised.
//
// Arguments travel back to the client in the reply, so a token handed to a tool
// would travel with them. Nothing here is a secret by design, but a tool that
// takes one should not be the reason it leaks.
func safeArgs(params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(params))
	for key, value := range params {
		if isSecretKey(key) {
			out[key] = "[redacted]"
			continue
		}
		out[key] = summarise(value)
	}
	return out
}

var secretHints = []string{"password", "secret", "token", "authorization", "api_key", "apikey", "credential", "claims"}

func isSecretKey(key string) bool {
	lower := strings.ToLower(key)
	for _, hint := range secretHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// summarise keeps a value whole when it is small, and describes it when it is
// not, so the record stays readable and the reply stays a sensible size.
func summarise(value interface{}) interface{} {
	switch typed := value.(type) {
	case nil, bool, float64, int, int64, string:
		if text, ok := typed.(string); ok && len(text) > maxRecordedValue {
			return fmt.Sprintf("%s… [%d chars]", text[:maxRecordedValue], len(text))
		}
		return value
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("[unencodable %T]", value)
	}
	if len(encoded) <= maxRecordedValue {
		// Round-tripped so the record holds data rather than a quoted blob.
		var plain interface{}
		if json.Unmarshal(encoded, &plain) == nil {
			return plain
		}
	}
	return fmt.Sprintf("[%T, %d bytes]", value, len(encoded))
}
