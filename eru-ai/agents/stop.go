package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/eru-os/eru/eru-ai/models"
)

// Why a run ended, and what it was allowed to spend getting there.
//
// # The problem with inferring it
//
// The loop had five ways out and no way to tell them apart. "The agent
// finished", "it asked the user something", "it ran out of attempts", "the model
// errored" and "it fell back to an earlier answer" arrived as either a message
// or an error, and anything downstream that cared had to read prose to find out
// which. An eval could assert what the agent DID and never assert that it got to
// the end; a client could not distinguish a paused run from a failed one without
// matching on text; and "it hung" and "it gave up" looked identical from outside.
//
// A closed set fixes that, and it is the prerequisite for anything that wants to
// react to how a run ended rather than only to what it produced.
//
// # And the limit that was not there
//
// RetryCount is an allowance for a specific thing - how many times a REJECTED
// answer may be sent back - and it was doing duty as the loop's only bound. It
// cannot do that job. It says nothing about wall-clock time, nothing about
// tokens, and since the quality gate got its own budget it does not even bound
// the number of trips round the loop. A run could spend an hour and a million
// tokens without crossing a single declared limit.
//
// Budget is the ceiling: separate from RetryCount, checked before each attempt
// rather than after the damage, and reported as a stop reason of its own so
// "stopped because it was told to" is never mistaken for "stopped because it
// finished".

// StopReason is the closed set of ways a run can end.
type StopReason string

const (
	// StopEndTurn - the agent produced an answer that passed everything.
	StopEndTurn StopReason = "end_turn"
	// StopAskedUser - the agent paused to ask a question. Not a failure and not
	// a completion; a client that conflates it with either gets this wrong.
	StopAskedUser StopReason = "asked_user"
	// StopValidationExhausted - no attempt satisfied the checks and nothing
	// could be delivered. The only reason here that is always an error.
	StopValidationExhausted StopReason = "validation_exhausted"
	// StopRecoveredEarlierAnswer - conformance attempts ran out, but an earlier
	// answer the quality gate had sent back was still good enough to deliver.
	// Distinct from StopEndTurn on purpose: what shipped is not what the run was
	// trying to produce, and a report that hides that is flattering.
	StopRecoveredEarlierAnswer StopReason = "recovered_earlier_answer"
	// StopLimitAttempts / StopLimitTokens / StopLimitDuration - a declared
	// ceiling was reached. The agent might have succeeded with more; it was not
	// given more.
	StopLimitAttempts StopReason = "limit_attempts"
	StopLimitTokens   StopReason = "limit_total_tokens"
	StopLimitDuration StopReason = "limit_duration"
	// StopModelError - the model call itself failed.
	StopModelError StopReason = "model_error"
	// StopCycling - the agent is alternating between two faults, fixing each by
	// reintroducing the other. More attempts will not help, and the honest thing
	// is to say which two rather than spend the budget discovering it again.
	StopCycling StopReason = "cycling"
	// StopCancelled - the caller went away. Worth its own name: a cancelled run
	// is not a broken one, and counting it as a failure makes a deploy look like
	// a regression.
	StopCancelled StopReason = "cancelled"
)

// Terminal reports whether this reason means the run produced no answer.
func (r StopReason) Terminal() bool {
	switch r {
	case StopValidationExhausted, StopModelError, StopCancelled, StopCycling:
		return true
	default:
		return false
	}
}

// Budget is the ceiling on one run, declared in configuration.
//
// Every field is optional and zero means no limit, so an agent that says nothing
// behaves exactly as it did before this existed. That is deliberate: a framework
// that silently acquires limits is one that starts failing work nobody changed.
type Budget struct {
	// MaxAttempts bounds trips round the loop, whatever caused them - a
	// validation retry, a repair, a quality improvement. This is the one that
	// RetryCount stopped being able to do once the quality gate got its own
	// allowance.
	MaxAttempts int `json:"max_attempts,omitempty"`
	// MaxTotalTokens bounds cumulative usage across attempts. Checked before an
	// attempt, against what has been spent: the point is to avoid starting work
	// that cannot be afforded, not to interrupt it halfway.
	MaxTotalTokens int64 `json:"max_total_tokens,omitempty"`
	// MaxDurationSeconds bounds wall clock, which is the limit a waiting user
	// actually feels.
	MaxDurationSeconds int `json:"max_duration_seconds,omitempty"`
}

// Spend is what a run has used so far.
type Spend struct {
	Attempts int
	Tokens   int64
	Started  time.Time
}

// Add accumulates one model call's usage.
func (s *Spend) Add(usage *models.TokenUsage) {
	if s == nil || usage == nil {
		return
	}
	if usage.TotalTokens > 0 {
		s.Tokens += usage.TotalTokens
		return
	}
	s.Tokens += usage.InputTokens + usage.OutputTokens + usage.ReasoningTokens
}

// Exceeded reports which limit stops the next attempt, and why in words.
//
// Checked BEFORE an attempt rather than after one. Checking afterwards means
// always paying for the call that crosses the line, and on the expensive limits
// - tokens, minutes - that call is the one worth not making.
func (b Budget) Exceeded(ctx context.Context, spend Spend) (StopReason, string) {
	if err := ctx.Err(); err != nil {
		return StopCancelled, fmt.Sprintf("the caller went away after %d attempt(s): %v", spend.Attempts, err)
	}
	if b.MaxAttempts > 0 && spend.Attempts >= b.MaxAttempts {
		return StopLimitAttempts, fmt.Sprintf("the attempt limit of %d was reached", b.MaxAttempts)
	}
	if b.MaxTotalTokens > 0 && spend.Tokens >= b.MaxTotalTokens {
		return StopLimitTokens, fmt.Sprintf("the token budget of %d was spent (%d used)", b.MaxTotalTokens, spend.Tokens)
	}
	if b.MaxDurationSeconds > 0 && !spend.Started.IsZero() {
		if elapsed := time.Since(spend.Started); elapsed >= time.Duration(b.MaxDurationSeconds)*time.Second {
			return StopLimitDuration, fmt.Sprintf("the time budget of %ds was reached after %s", b.MaxDurationSeconds, elapsed.Round(time.Second))
		}
	}
	return "", ""
}

// Set records the reason on a message, and never overwrites one already there:
// the first reason to be decided is the true one, and a later default would
// paper over it.
func (r StopReason) Set(message *AgentMessage) {
	if message == nil || message.StopReason != "" {
		return
	}
	message.StopReason = r
}
