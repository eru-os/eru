package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// What calling a tool does, and what the framework does about it.
//
// # The bug class this closes
//
// The inner loop retries. That is the whole point of it: an answer is rejected,
// the reason goes back, the agent tries again. But the agent does not only
// produce an answer - it calls tools, and some of those tools WRITE. An agent
// that saved an entity, then had its report rejected for a missing key, will
// very often save the entity again on the retry. Nothing stopped it. The user
// asked for one entity and got three, one per attempt, and every check passed
// because the final report was perfectly well formed.
//
// This is not hypothetical and it is not rare. It is the direct consequence of
// combining a retry loop with tools that have side effects, which is what every
// useful agent is.
//
// # Why the framework has to know, rather than the agent being careful
//
// "The agent should not call it twice" is a prompt, and prompts are advice. The
// framework is the only thing that sees both attempts, so it is the only thing
// that can tell the second call from the first. An annotation is how a tool says
// what it is, and the guard below is the framework acting on it.
//
// Declaring nothing means nothing changes: an unannotated tool is called exactly
// as before, because a framework that silently starts suppressing calls is worse
// than one that duplicates them.

// ToolEffect declares what calling a tool does.
type ToolEffect struct {
	// ReadOnly says the call changes nothing. A lookup, a probe, a list.
	ReadOnly bool `json:"read_only,omitempty"`
	// Destructive says the call removes or overwrites something that was there.
	// Nothing in the framework acts on this yet; it is declared because the
	// question "which of these tools can lose data" should be answerable from
	// configuration rather than by reading Go.
	Destructive bool `json:"destructive,omitempty"`
	// Idempotent says calling twice with the same arguments has the same effect
	// as calling once - a save keyed by id, a set rather than an append. Such a
	// tool is safe to repeat and is never suppressed.
	Idempotent bool `json:"idempotent,omitempty"`
}

// Whether an effect was DECLARED is a separate question from what it says, and
// the answer is the presence of the tool in the effects map rather than a flag
// on the struct. A zero ToolEffect means "nothing was said" and must never read
// as "this writes": the guard below looks the action up, and a miss leaves the
// call alone.

// WriteOnce is the record of side effects already performed in one run.
//
// Keyed on the action and its arguments, because that is what makes two calls
// "the same call" from the outside. Two saves of different entities are two
// pieces of work; two identical saves are one piece of work attempted twice.
type WriteOnce struct {
	mu   sync.Mutex
	done map[string]map[string]interface{}
}

func NewWriteOnce() *WriteOnce {
	return &WriteOnce{done: map[string]map[string]interface{}{}}
}

type writeOnceKey struct{}

// WithWriteOnce installs the record for a run.
func WithWriteOnce(ctx context.Context, record *WriteOnce) context.Context {
	return context.WithValue(ctx, writeOnceKey{}, record)
}

// WriteOnceFrom returns the run's record, or nil.
func WriteOnceFrom(ctx context.Context) *WriteOnce {
	record, _ := ctx.Value(writeOnceKey{}).(*WriteOnce)
	return record
}

// Seen returns the earlier result of an identical call, if there was one.
func (w *WriteOnce) Seen(action string, args map[string]interface{}) (map[string]interface{}, bool) {
	if w == nil {
		return nil, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	result, ok := w.done[callKey(action, args)]
	return result, ok
}

// Record remembers a completed call and what it returned, so a repeat can be
// answered with the same thing rather than performed again.
func (w *WriteOnce) Record(action string, args map[string]interface{}, result map[string]interface{}) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done == nil {
		w.done = map[string]map[string]interface{}{}
	}
	w.done[callKey(action, args)] = result
}

// callKey is the idempotency key: what makes two calls the same call.
func callKey(action string, args map[string]interface{}) string {
	canonical, err := json.Marshal(canonicalValue(argsForKey(args)))
	if err != nil {
		// Unhashable arguments mean the call cannot be recognised as a repeat,
		// so it is allowed through. Suppressing a call we cannot identify would
		// be guessing with someone's data.
		return ""
	}
	sum := sha256.Sum256(append([]byte(action+"|"), canonical...))
	return hex.EncodeToString(sum[:])[:20]
}

// argsForKey drops the keys that vary between two attempts at the same work -
// a regenerated request id would otherwise make every repeat look new.
func argsForKey(args map[string]interface{}) map[string]interface{} {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(args))
	for key, value := range args {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "request_id") || strings.Contains(lower, "correlation") ||
			strings.Contains(lower, "timestamp") || strings.Contains(lower, "idempotency") {
			continue
		}
		out[key] = value
	}
	return out
}

// suppressedResult is what a caller gets instead of a second write.
//
// The earlier result is returned verbatim with a note added, rather than an
// error: the agent asked for a thing to be true, and it IS true. Telling it the
// call failed would send it round the loop trying to fix something that already
// worked.
func suppressedResult(action string, earlier map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range earlier {
		out[key] = value
	}
	out["note"] = fmt.Sprintf(
		"%s was already called with exactly these arguments during this run, and this is that call's result. "+
			"It was not performed again, because repeating it would have made a second copy of the same change. "+
			"Nothing is wrong: what you asked for is already done.", action)
	return out
}

// effectKey carries the per-run map of action name to declared effect.
type effectKey struct{}

// WithToolEffects installs the declared effects for a run.
func WithToolEffects(ctx context.Context, effects map[string]ToolEffect) context.Context {
	if len(effects) == 0 {
		return ctx
	}
	return context.WithValue(ctx, effectKey{}, effects)
}

func toolEffectsFrom(ctx context.Context) map[string]ToolEffect {
	effects, _ := ctx.Value(effectKey{}).(map[string]ToolEffect)
	return effects
}

// guardWrite decides whether this call should run, and if not, what to answer.
func guardWrite(ctx context.Context, action string, args map[string]interface{}) (map[string]interface{}, bool) {
	effects := toolEffectsFrom(ctx)
	if len(effects) == 0 {
		return nil, false
	}
	effect, declared := effects[action]
	if !declared || effect.ReadOnly || effect.Idempotent {
		return nil, false
	}
	record := WriteOnceFrom(ctx)
	if record == nil {
		return nil, false
	}
	earlier, repeated := record.Seen(action, args)
	if !repeated {
		return nil, false
	}
	logs.WithContext(ctx).Error(fmt.Sprintf(
		"%s was called a second time with identical arguments and is not idempotent; the repeat was not performed", action))
	return suppressedResult(action, earlier), true
}

// EffectsOf collects the declared effects of an agent's tools, keyed by the
// action name the model sees - which is what a call is identified by.
func EffectsOf(agentTools []AgentTools) map[string]ToolEffect {
	out := map[string]ToolEffect{}
	var walk func(list []AgentTools)
	walk = func(list []AgentTools) {
		for _, attached := range list {
			if attached.Effect != nil {
				key := attached.ActionName
				if key == "" {
					key = attached.ToolKey
				}
				if key == "" {
					key = attached.ToolName
				}
				if key != "" {
					out[key] = *attached.Effect
				}
			}
			walk(attached.DependentTools)
		}
	}
	walk(agentTools)
	if len(out) == 0 {
		return nil
	}
	return out
}

// DeclaredEffects renders the effects for a report, sorted so two runs compare.
func DeclaredEffects(effects map[string]ToolEffect) []string {
	out := make([]string, 0, len(effects))
	for action, effect := range effects {
		var flags []string
		if effect.ReadOnly {
			flags = append(flags, "read-only")
		}
		if effect.Destructive {
			flags = append(flags, "destructive")
		}
		if effect.Idempotent {
			flags = append(flags, "idempotent")
		}
		if len(flags) == 0 {
			flags = append(flags, "writes")
		}
		out = append(out, action+": "+strings.Join(flags, ", "))
	}
	sort.Strings(out)
	return out
}
