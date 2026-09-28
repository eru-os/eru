package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// The delivery seal: what the loop judged is what the caller receives.
//
// # The invariant, and why it is the one worth enforcing
//
// Every guard in this framework - the schema, OutputValidator, the quality gate,
// and whatever rules an agent declares in configuration - runs against the
// artifact the loop holds when it decides to stop. None of them means anything
// unless that artifact is the one the caller actually gets.
//
// It was not. An agent type overrides Execute, calls the loop, and is then free
// to rewrite the answer on the way out; the framework has already finished and
// never looks again. In a live run this produced two log lines five seconds
// apart - the validator clearing a page that bound no queries, and the delivery
// path handing over a page bound to a query that does not exist. A green
// validation meant the agent had written something acceptable at some point, not
// that anyone received it.
//
// # Why a seal rather than more hooks
//
// The obvious fix is a "finalizer" capability: a declared place for an agent
// type to shape its answer, running before validation. The framework already has
// one - OutputNormalizer - and eru_studio already uses it. A second hook with
// the same semantics would add surface without adding a guarantee, because
// nothing would stop the next agent type doing its shaping somewhere else again.
//
// Three things have to hold for a verdict to be worth anything:
//
//	a. what is judged is what is delivered
//	b. everything that shapes the artifact happens before judgement
//	c. content produced by sub-agents is itself judged
//
// They look like three jobs and they are one. Enforce (a) and the other two stop
// needing to be policed: shaping that happens late breaks the seal, and a
// sub-agent's page merged in after the fact breaks the seal. Both become
// failures that report themselves, at the moment they happen, naming the agent
// that did it. That is the whole argument for doing this instead of adding
// capabilities - an invariant catches the cases nobody thought of, and a hook
// only catches the ones someone remembered to route through it.
//
// # What an agent type is still allowed to do
//
// Plenty. Transforming the answer after the loop is legitimate - resolving an
// envelope, stamping an identity, merging pages built elsewhere. It just has to
// go through DeliverTransformed, which re-runs the agent's own validator on the
// result. That costs no model call and refuses exactly one thing: a transform
// that introduces a fault the loop would have rejected.

// Seal is a fingerprint of the artifact the loop validated.
//
// It is carried on the message rather than in the context because the context
// belongs to the caller: an agent type builds its own ctx before calling the
// loop, so the loop cannot write a value back into it. The message is the one
// thing that definitely travels from the loop to the caller.
type Seal string

// SealOf fingerprints an answer's action payloads.
//
// Only the actions are hashed. Traces, metrics and timestamps change for reasons
// that have nothing to do with the artifact, and a seal that broke on a duration
// would be noise within a day.
func SealOf(actions []AgentOutputAction) Seal {
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		canonical, err := json.Marshal(canonicalValue(action.Action))
		if err != nil {
			// Unhashable means unsealable. Returning an empty seal is the honest
			// answer: verification will report "not sealed" rather than claim a
			// match it cannot support.
			return ""
		}
		parts = append(parts, action.ActionType+"|"+action.ActionName+"|"+string(canonical))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return Seal("s" + hex.EncodeToString(sum[:])[:16])
}

// canonicalValue makes a decoded JSON value hash stably. Go sorts map keys on
// marshal already; this exists so nested values decoded from different sources
// compare equal.
func canonicalValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]interface{}, len(typed))
		for _, key := range keys {
			out[key] = canonicalValue(typed[key])
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, canonicalValue(item))
		}
		return out
	default:
		return value
	}
}

// SealAnswer stamps a message with the fingerprint of what it carries. The loop
// calls this once, at the point the artifact has passed every check.
func SealAnswer(message *AgentMessage) {
	if message == nil {
		return
	}
	message.Seal = SealOf(message.Actions)
}

// DeliveryBroken is returned when the delivered artifact is not the judged one
// and the difference could not be cleared.
type DeliveryBroken struct {
	Agent  string
	Reason string
}

func (e *DeliveryBroken) Error() string {
	return fmt.Sprintf("agent %s delivered an answer that was never validated: %s", e.Agent, e.Reason)
}

// DeliverTransformed is how an agent type changes its answer after the loop.
//
// The transform is applied, the agent's own validator is re-run on the result,
// and the message is resealed. Nothing here knows what the transform does; it
// only insists that whatever comes out would still have been accepted.
//
// An agent type with no OutputValidator gets the reseal and nothing else, which
// is the correct amount of checking for an agent that declares no rules.
//
// # Why this refuses and VerifyDelivery only reports
//
// The two are at different moments and the difference is real. Here the agent
// type is ASKING the framework to sanction a change, before anything has been
// handed over: refusing costs the request, and the alternative is knowingly
// shipping an artifact that fails the agent's own rules. A user who asked for a
// dashboard is better served by an error they can act on than by a page that
// renders nothing behind a 200 - that silent blank is the failure this whole
// harness was started over.
//
// VerifyDelivery runs at the boundary, where the answer is already out. There,
// refusing would turn a reporting problem into an outage, so it says so loudly
// and lets it through.
// OnFault says what a transform's author wants done if the result no longer
// passes.
//
// The distinction is whether the user will be told. The seal exists to stop
// work being handed over that nothing checked; a fault the answer itself
// discloses has been checked and reported, and the decision to ship it anyway
// belongs to the agent type that made it.
//
// eru_studio's page composer is the case that forced this. It builds a nested
// page, checks it, retries, and if it still has a flaw attaches it WITH a
// warning - on the stated grounds that a flawed card page beats a mount
// pointing at nothing. Refusing that would override a deliberate and visible
// product decision, and turn a page with one imperfection into an error.
type OnFault int

const (
	// FaultRefuses: do not hand over an answer the transform broke. The default
	// and the right one when nothing else will tell the user.
	FaultRefuses OnFault = iota
	// FaultReports: hand it over and say so loudly. Only for a transform whose
	// faults the answer already discloses to the user.
	FaultReports
)

func DeliverTransformed(ctx context.Context, provider SystemPromptProvider, message AgentMessage, onFault OnFault, transform func(AgentMessage) (AgentMessage, error)) (AgentMessage, error) {
	name := agentNameOf(message)
	transformed, err := transform(message)
	if err != nil {
		return AgentMessage{}, err
	}
	if SealOf(transformed.Actions) == message.Seal {
		// Nothing changed; the original judgement still covers it.
		transformed.Seal = message.Seal
		return transformed, nil
	}

	validator, ok := provider.(OutputValidator)
	if !ok || validator == nil {
		SealAnswer(&transformed)
		return transformed, nil
	}
	for _, action := range transformed.Actions {
		if action.ActionType != ActionTypeAnswer || action.Action == nil {
			continue
		}
		if err := validator.ValidateOutput(ctx, action.Action); err != nil {
			if onFault == FaultReports {
				// Disclosed, not hidden: the answer tells the user about this.
				logs.WithContext(ctx).Error(fmt.Sprintf(
					"agent %s transformed its answer after the loop and the result no longer passes validation; delivering it because the answer discloses the fault: %v", name, err))
				continue
			}
			// The loop accepted an artifact and the transform broke it. Handing
			// it over anyway is the exact failure this file exists to stop.
			return AgentMessage{}, &DeliveryBroken{
				Agent:  name,
				Reason: fmt.Sprintf("the answer passed validation, was then transformed on the way out, and the result no longer passes: %v", err),
			}
		}
	}
	SealAnswer(&transformed)
	logs.WithContext(ctx).Info(fmt.Sprintf("agent %s transformed its answer after the loop; the result was re-validated and resealed", name))
	return transformed, nil
}

// VerifyDelivery is the tripwire, called by the framework where an agent is
// invoked - never by an agent type, which is the point. It catches a transform
// that did not go through DeliverTransformed, including one nobody knew was
// happening.
//
// It does not fail the request. An answer that reaches here has already been
// produced, and refusing to deliver it would turn a reporting problem into an
// outage. It re-validates, and says loudly which agent broke its seal.
func VerifyDelivery(ctx context.Context, provider SystemPromptProvider, message AgentMessage) error {
	if message.Seal == "" {
		// Not every message is sealed: a clarification question, an agent type
		// that does not use the reasoning loop. Unsealed is "no claim made",
		// which must not read as "claim broken".
		return nil
	}
	if SealOf(message.Actions) == message.Seal {
		return nil
	}

	name := agentNameOf(message)
	broken := &DeliveryBroken{Agent: name, Reason: "the artifact changed between validation and delivery"}
	validator, ok := provider.(OutputValidator)
	if !ok || validator == nil {
		logs.WithContext(ctx).Error(broken.Error() + " (no validator to re-check it with)")
		return broken
	}
	for _, action := range message.Actions {
		if action.ActionType != ActionTypeAnswer || action.Action == nil {
			continue
		}
		if err := validator.ValidateOutput(ctx, action.Action); err != nil {
			broken.Reason = fmt.Sprintf("the artifact changed between validation and delivery, and the delivered one does not pass: %v", err)
			logs.WithContext(ctx).Error(broken.Error())
			return broken
		}
	}
	// Changed, but still sound. Worth saying: shaping an answer outside the loop
	// is how the two drift apart, and the next change may not be harmless.
	logs.WithContext(ctx).Error(fmt.Sprintf(
		"agent %s changed its answer after validation without going through DeliverTransformed; the delivered artifact still passes, but it was not the one judged", name))
	return nil
}

func agentNameOf(message AgentMessage) string {
	for _, action := range message.Actions {
		if action.ActionName != "" {
			return action.ActionName
		}
	}
	return "unknown"
}
