package agents

import (
	"context"
	"time"
)

type contextKey string

const StreamCallbackKey contextKey = "stream_callback"
const RawOutputKey contextKey = "raw_output"
const runIdentityKey contextKey = "run_identity"

const (
	StreamEventThinking   = "thinking"
	StreamEventToolUse    = "tool_use"
	StreamEventToolResult = "tool_result"
	StreamEventTextDelta  = "text_delta"
	StreamEventDone       = "done"
	StreamEventError      = "error"
	StreamEventQuestion   = "question"
	StreamEventStatus     = "status"
	StreamEventPlan       = "plan"
	// StreamEventPageComponent carries one UI component as soon as the model has
	// finished writing it, so a renderer can draw a page while the rest of it is
	// still being generated instead of waiting for the whole answer.
	StreamEventPageComponent = "page_component"

	// Lifecycle events. These follow the shape AG-UI settled on - a run that
	// starts and ends, with named steps inside it - so a client written against
	// that protocol needs a name mapping and nothing more.
	//
	// The run is the request: one run_started per streamed execution, and either
	// the existing "done" or "error" to close it (those two are this protocol's
	// run_finished and run_error - they are terminal, and clients already stop
	// reading at them, so they were not renamed).
	StreamEventRunStarted = "run_started"
	// An agent within the run announces itself, so a client can show which agent
	// is working without inferring it from the first token it happens to emit.
	// The agent and chain fields say which one; for delegated work these are
	// AG-UI's subagent_started / subagent_finished.
	StreamEventAgentStarted  = "agent_started"
	StreamEventAgentFinished = "agent_finished"
	// A named phase inside one agent: generating, validating, retrying, applying
	// a patch. This is what lets a client say what it is waiting for.
	StreamEventStepStarted  = "step_started"
	StreamEventStepFinished = "step_finished"
)

// Named steps an agent reports. Keep these stable: clients label them.
const (
	StepGenerate = "generate"
	StepValidate = "validate"
	// StepJudge is the quality gate: asked only of an answer that already
	// passed validation, so a client can distinguish "wrong" from "not good
	// enough yet".
	StepJudge = "judge"
	// StepPlanPages and StepBuildPage are the steps of a design that takes more
	// than one page: deciding what the pages are, then building each of them.
	StepPlanPages   = "plan_pages"
	StepBuildPage   = "build_page"
	StepApplyPatch  = "apply_patch"
	StepClarify     = "clarify"
	StepOrchestrate = "orchestrate"
)

// Outcomes a step or an agent can finish with.
const (
	OutcomeSuccess = "success"
	OutcomeRetry   = "retry"
	OutcomeError   = "error"
	OutcomePaused  = "paused"
)

// StepPayload is the data of a step or agent lifecycle event.
type StepPayload struct {
	Step string `json:"step,omitempty"`
	// Attempt counts from 1 and is set only when a step can be retried.
	Attempt    int    `json:"attempt,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Detail     string `json:"detail,omitempty"`
	// Code is a stable machine-readable reason when the outcome is an error, so
	// a client can branch on it instead of matching on message text.
	Code string `json:"code,omitempty"`
}

// Error codes carried by a failing step or agent.
const (
	CodeOutputValidation = "output_validation_failed"
	CodeQualityBelowBar  = "quality_below_bar"
	CodePatchUnresolved  = "patch_could_not_be_applied"
	CodeModelError       = "model_error"
	CodeToolError        = "tool_error"
)

type StreamEvent struct {
	Event     string      `json:"event"`
	Data      interface{} `json:"data,omitempty"`
	Iteration int         `json:"iteration,omitempty"`
	Agent     string      `json:"agent,omitempty"`
	Chain     string      `json:"chain,omitempty"`
	Seq       int64       `json:"seq,omitempty"`
	// RunId identifies this execution and ThreadId the conversation it belongs
	// to, so a client that multiplexes several runs over one connection - or
	// reconnects to one - can tell which events are whose.
	RunId    string `json:"run_id,omitempty"`
	ThreadId string `json:"thread_id,omitempty"`
}

// Attribute stamps an event with the agent that produced it and that agent's
// delegation chain, so a client receiving interleaved events from parallel
// sub-agents can tell them apart. Set only when empty: the deepest agent to touch
// an event owns it, and relaying hops must not relabel it.
func (e StreamEvent) Attribute(agentName string, chain []string) StreamEvent {
	if e.Agent == "" {
		e.Agent = agentName
	}
	if e.Chain == "" {
		e.Chain = FormatAgentChain(ChainWith(chain, agentName))
	}
	return e
}

// RunIdentity names one execution and the conversation it continues.
type RunIdentity struct {
	RunId    string `json:"run_id"`
	ThreadId string `json:"thread_id"`
}

// Identify stamps an event with the run it belongs to. Set only when empty, for
// the same reason as Attribute: a relaying hop must not relabel what it carries.
func (e StreamEvent) Identify(run RunIdentity) StreamEvent {
	if e.RunId == "" {
		e.RunId = run.RunId
	}
	if e.ThreadId == "" {
		e.ThreadId = run.ThreadId
	}
	return e
}

// WithRunIdentity records the run this context belongs to.
func WithRunIdentity(ctx context.Context, run RunIdentity) context.Context {
	return context.WithValue(ctx, runIdentityKey, run)
}

// RunIdentityFrom returns the run this context belongs to, zero-valued outside a
// streamed execution.
func RunIdentityFrom(ctx context.Context) RunIdentity {
	if ctx == nil {
		return RunIdentity{}
	}
	run, _ := ctx.Value(runIdentityKey).(RunIdentity)
	return run
}

type StreamCallback func(event StreamEvent)

// Emit sends one event if this request is being streamed, and does nothing if it
// is not. Every lifecycle emitter goes through it so no caller has to nil-check.
func Emit(ctx context.Context, event StreamEvent) {
	cb := GetStreamCallback(ctx)
	if cb == nil {
		return
	}
	cb(event.Identify(RunIdentityFrom(ctx)))
}

// EmitStepStarted announces a named phase.
func EmitStepStarted(ctx context.Context, step string, attempt int) {
	Emit(ctx, StreamEvent{Event: StreamEventStepStarted, Data: StepPayload{Step: step, Attempt: attempt}})
}

// EmitStepFinished closes a named phase with what came of it.
func EmitStepFinished(ctx context.Context, step string, attempt int, outcome string, started time.Time, detail string, code string) {
	Emit(ctx, StreamEvent{Event: StreamEventStepFinished, Data: StepPayload{
		Step:       step,
		Attempt:    attempt,
		Outcome:    outcome,
		DurationMs: time.Since(started).Milliseconds(),
		Detail:     detail,
		Code:       code,
	}})
}

func WithStreamCallback(ctx context.Context, cb StreamCallback) context.Context {
	return context.WithValue(ctx, StreamCallbackKey, cb)
}

func GetStreamCallback(ctx context.Context) StreamCallback {
	if cb, ok := ctx.Value(StreamCallbackKey).(StreamCallback); ok {
		return cb
	}
	return nil
}

// WithRawOutput marks the request as wanting internal artifacts (orchestration
// plans, structured_output tool inputs) in the response. Set from the ?raw=true
// query param and meant for development only.
func WithRawOutput(ctx context.Context, raw bool) context.Context {
	return context.WithValue(ctx, RawOutputKey, raw)
}

func RawOutputEnabled(ctx context.Context) bool {
	if raw, ok := ctx.Value(RawOutputKey).(bool); ok {
		return raw
	}
	return false
}
