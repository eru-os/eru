package reasoning_agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eru-os/eru/eru-ai/agents"
	"github.com/eru-os/eru/eru-ai/models"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// judgeQuality runs the gate for one attempt.
//
// It is called only on an answer that has already passed every conformance
// check, so a verdict here is about taste, not correctness. The return says
// whether a verdict was reached at all; whether it costs an attempt is carried
// on the verdict itself as Enforced.
//
// Three refusals are deliberate, and each one is a way this gate could have
// become worse than no gate:
//
//   - An agent type with no judge is left entirely alone. The gate is opt-in.
//   - A judge that errors leaves the answer alone. A check that could not run
//     must never read as a check that failed; that is the same reason the eval
//     has a BLOCKED state distinct from FAIL.
//   - A poor verdict with nothing to say is recorded and not enforced. Feeding
//     "not good enough" back with no named fault spends an attempt to ask the
//     model to guess what it did wrong.
func (ra *ReasoningAgent) judgeQuality(ctx context.Context, output map[string]interface{}, attempt int, mayRetry bool) (agents.QualityVerdict, bool) {
	judge, hasJudge := ra.GetProvider().(agents.QualityJudge)
	if !hasJudge || judge == nil {
		// No Go judge. An agent that declares quality rules in configuration
		// still gets a gate - that is the point of the rules being data.
		return ra.judgeConfiguredQuality(ctx, output, attempt, mayRetry)
	}

	started := time.Now()
	agents.EmitStepStarted(ctx, agents.StepJudge, attempt+1)

	verdict, err := judge.JudgeQuality(ctx, output)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("agent %s quality gate could not run on attempt %d, answer accepted unjudged: %v", ra.AgentName, attempt+1, err))
		agents.EmitStepFinished(ctx, agents.StepJudge, attempt+1, agents.OutcomeSuccess, started, "quality gate skipped: "+err.Error(), "")
		return agents.QualityVerdict{}, false
	}

	verdict.Attempt = attempt + 1
	verdict.Reason = strings.TrimSpace(verdict.Reason)
	verdict.Enforced = verdict.Rating.RetryWorthy() && mayRetry && verdict.Reason != ""

	if verdict.Enforced {
		logs.WithContext(ctx).Info(fmt.Sprintf("agent %s passed validation on attempt %d but the quality gate is sending it back: %s", ra.AgentName, attempt+1, verdict.Reason))
		agents.EmitStepFinished(ctx, agents.StepJudge, attempt+1, agents.OutcomeRetry, started, verdict.Reason, agents.CodeQualityBelowBar)
		return verdict, true
	}

	detail := string(verdict.Rating)
	if verdict.Rating.RetryWorthy() {
		// Shipping something imperfect beats failing the request, so the gate
		// annotates instead of vetoing once its budget is gone. The verdict is
		// still recorded, and an answer that shipped below the bar is precisely
		// what anyone reading the metrics later wants to find.
		logs.WithContext(ctx).Info(fmt.Sprintf("agent %s is delivering an answer the quality gate rated poor, with no attempt left to improve it: %s", ra.AgentName, verdict.Reason))
		detail = "delivered below bar: " + verdict.Reason
	} else if verdict.Reason != "" {
		detail = detail + ": " + verdict.Reason
	}
	agents.EmitStepFinished(ctx, agents.StepJudge, attempt+1, agents.OutcomeSuccess, started, detail, "")
	return verdict, verdict.Rating != agents.QualityUnrated
}

// acceptedAnswer is an answer that passed every conformance check and was then
// sent back by the quality gate anyway.
//
// It is kept for one reason: so the gate can never turn a deliverable answer
// into a failed request. Everything the run would have returned is held
// together, because delivering the output without its traces would report work
// that the answer did not do.
type acceptedAnswer struct {
	response models.Message
	traces   []models.StepTrace
	output   map[string]interface{}
	verdict  agents.QualityVerdict
}

// judgeConfiguredQuality is the gate for an agent whose rubric is declared
// rather than written.
//
// Deliberately the same shape as the Go path, including the refusals: a poor
// verdict with no budget left is recorded and not enforced, and a rubric that
// says nothing produces no verdict rather than a cheerful one.
func (ra *ReasoningAgent) judgeConfiguredQuality(ctx context.Context, output map[string]interface{}, attempt int, mayRetry bool) (agents.QualityVerdict, bool) {
	verdict, judged := ra.JudgeConfiguredQuality(ctx, output)
	if !judged {
		return agents.QualityVerdict{}, false
	}

	started := time.Now()
	agents.EmitStepStarted(ctx, agents.StepJudge, attempt+1)

	verdict.Attempt = attempt + 1
	verdict.Reason = strings.TrimSpace(verdict.Reason)
	verdict.Enforced = verdict.Rating.RetryWorthy() && mayRetry && verdict.Reason != ""

	if verdict.Enforced {
		logs.WithContext(ctx).Info(fmt.Sprintf("agent %s passed validation on attempt %d but its declared quality rules are sending it back", ra.AgentName, attempt+1))
		agents.EmitStepFinished(ctx, agents.StepJudge, attempt+1, agents.OutcomeRetry, started, verdict.Reason, agents.CodeQualityBelowBar)
		return verdict, true
	}
	detail := string(verdict.Rating)
	if verdict.Rating.RetryWorthy() {
		detail = "delivered below bar: " + verdict.Reason
	}
	agents.EmitStepFinished(ctx, agents.StepJudge, attempt+1, agents.OutcomeSuccess, started, detail, "")
	return verdict, true
}
