package reasoning_agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	agents "github.com/eru-os/eru/eru-ai/agents"
	ruleset "github.com/eru-os/eru/eru-ai/agents/ruleset"
	models "github.com/eru-os/eru/eru-ai/models"
	tools "github.com/eru-os/eru/eru-ai/tools"
	utility "github.com/eru-os/eru/eru-ai/tools/utility"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type ReasoningAgent struct {
	agents.Agent
	MaxIterations       int  `json:"max_iterations"`
	ThinkingBudget      int  `json:"thinking_budget"`
	EnableClarification bool `json:"enable_clarification"`
}

const clarificationGuidance = `

HUMAN-IN-THE-LOOP CLARIFICATION:
You may ask the user for clarification using the ask_user tool. Use it ONLY when the request is genuinely ambiguous or missing information you cannot infer or reasonably default — never to offload reasoning you can do yourself.
When you ask:
- Keep it to the fewest questions needed (ideally one).
- For each question provide 2-4 concrete, mutually exclusive options (value + human label).
- The user can always type their own answer instead of picking an option - that is added for you. So options are the likely answers, not the only ones: never write an option meaning "none of these", and never tell the user their situation is not covered.
- Set multi_select=true only when more than one option can legitimately be chosen.
- For a credential (API key, password, client secret, private key, token) ask with input_type "secret" and a secret_name. You get back only a $SECRET_<name> reference: put that reference in the config field. Never ask the user to type a credential as text or in chat, and never repeat one.
Calling ask_user ends your turn; the user's answers will arrive as a follow-up message in the same conversation, after which you continue.`

func (ra *ReasoningAgent) GetSpec() agents.AgentI {
	return ra
}

func (ra *ReasoningAgent) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &ra.Agent); err != nil {
		return err
	}
	type reasoningFields struct {
		MaxIterations       int  `json:"max_iterations"`
		ThinkingBudget      int  `json:"thinking_budget"`
		EnableClarification bool `json:"enable_clarification"`
	}
	var rf reasoningFields
	if err := json.Unmarshal(b, &rf); err != nil {
		return err
	}
	ra.MaxIterations = rf.MaxIterations
	ra.ThinkingBudget = rf.ThinkingBudget
	ra.EnableClarification = rf.EnableClarification
	if ra.MaxIterations <= 0 {
		ra.MaxIterations = 10
	}
	if ra.ThinkingBudget <= 0 {
		ra.ThinkingBudget = 10000
	}
	return nil
}

func (ra *ReasoningAgent) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("ReasoningAgent MakeFromJson - Start")
	err := json.Unmarshal(*rj, ra)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (ra *ReasoningAgent) Execute(ctx context.Context, agentMessage agents.AgentMessage, conversationId string, projectId string, tenantId string) (agents.AgentMessage, error) {
	logs.WithContext(ctx).Debug("ReasoningAgent Execute - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "ReasoningAgent.Execute",
		oteltrace.WithAttributes(attribute.String("agent_name", ra.AgentName), attribute.String("conversation_id", conversationId)),
	)
	defer span.End()
	startTime := time.Now()

	// One record per run, unless a caller already started one - a sub-agent runs
	// inside its parent's context and should add to the same record rather than
	// keep a private one nobody reads.
	// Refuse before doing any work if this call is too deep or revisits an agent
	// already running. An agent calling itself is not a slow run, it is a
	// service that keeps calling models until something else runs out.
	enteredCtx, depthErr := agents.EnterAgent(ctx, ra.AgentName, ra.MaxDelegationDepth)
	if depthErr != nil {
		logs.WithContext(ctx).Error(depthErr.Error())
		return agents.AgentMessage{}, depthErr
	}
	ctx = enteredCtx

	if agents.ToolRecordFrom(ctx) == nil {
		ctx = agents.WithToolRecord(ctx, agents.NewToolRecord())
	}

	// An agent that declares evidence gets a collector for this run, and the
	// rules that say what to put in it. Both go in the context because the
	// collecting happens at the tool boundary, which has no idea which agent it
	// is serving.
	// Tools that write get a per-run guard, so a retry cannot repeat a write the
	// first attempt already made. Installed only when some tool actually says it
	// writes: an agent that declares nothing behaves exactly as before.
	if effects := agents.EffectsOf(ra.AgentTools); len(effects) > 0 {
		ctx = agents.WithToolEffects(ctx, effects)
		if agents.WriteOnceFrom(ctx) == nil {
			ctx = agents.WithWriteOnce(ctx, agents.NewWriteOnce())
		}
	}

	if len(ra.Evidence) > 0 && agents.EvidenceFrom(ctx) == nil {
		ctx = agents.WithEvidence(ctx, ruleset.NewEvidence())
		ctx = agents.WithEvidenceRules(ctx, ra.Evidence)
	}

	if answers, ok := agentMessage.ClarificationAnswers(); ok {
		var req agents.ClarificationRequest
		resumeContext := ""
		if priorConv, lerr := ra.LoadConversationHistory(ctx, conversationId, projectId, tenantId); lerr == nil && priorConv != nil {
			if _, qa, found := agents.PendingQuestion(priorConv); found {
				req, _ = agents.ParseClarificationRequest(qa.Action)
				resumeContext = agents.ResumeContextFrom(qa.Action)
			}
		}
		agents.ScrubSecretAnswers(&agentMessage, req)
		answers, _ = agentMessage.ClarificationAnswers()
		answerText := agents.FormatAnswersForModel(req, answers)
		// Hand back the lookups the agent had already made before it asked, so
		// answering a question resumes the work instead of restarting it.
		if resumeContext != "" {
			answerText = resumeContext + "\n\n" + answerText
		}
		if strings.TrimSpace(agentMessage.Content) != "" {
			agentMessage.Content = agentMessage.Content + "\n\n" + answerText
		} else {
			agentMessage.Content = answerText
		}
	}

	if augment := buildCodeAugmentation(agentMessage.Params); augment != "" {
		if strings.TrimSpace(agentMessage.Content) == "" {
			agentMessage.Content = augment
		} else {
			agentMessage.Content = fmt.Sprintf("%s\n\n--- USER PROMPT ---\n%s", augment, agentMessage.Content)
		}
	}

	chatRequest, conversation, err := ra.LoadConversations(ctx, conversationId, agentMessage, projectId, tenantId)
	if err != nil {
		return agents.AgentMessage{}, err
	}

	toolsMap := make(map[string]tools.Tooling)
	for _, at := range ra.AgentTools {
		if at.Tool != nil {
			key := at.ToolKey
			if key == "" {
				key = at.ToolName
			}
			toolsMap[key] = at.Tool
		}
	}

	outputSchema := ra.getOutputSchema(ctx)
	// Held onto so a repair turn can narrow the schema mid-loop: an agent that
	// answers a rejection with a diff needs the diff's schema, not the one that
	// described the whole answer.
	var outputTool *utility.StructuredOutputTool
	if outputSchema.Type != "" {
		outputTool = &utility.StructuredOutputTool{}
		outputTool.SetAttribute(ctx, "output_schema", outputSchema)
		outputTool.SetAttribute(ctx, "parameters", outputSchema)
		outputTool.SetAttribute(ctx, "description", "Output the final result as structured JSON. Call this tool when you have your final answer ready.")
		outputTool.SetAttribute(ctx, "tool_name", "structured_output")
		outputTool.SetAttribute(ctx, "tool_type", "STRUCTURED_OUTPUT")
		outputTool.SetToolAction("structured_output")
		toolsMap["structured_output"] = outputTool
	}

	// An agent type may carry built-in reference tools of its own (the Eru Studio
	// agent's component-library lookup, say). Configured tools win on a name
	// clash, so an owner can always override one.
	if provider, ok := ra.GetProvider().(agents.ExtraToolProvider); ok && provider != nil {
		for name, tool := range provider.ExtraTools(ctx) {
			if tool == nil {
				continue
			}
			if _, configured := toolsMap[name]; configured {
				continue
			}
			toolsMap[name] = tool
		}
	}

	if ra.EnableClarification {
		askTool := &utility.AskUserTool{}
		askTool.SetAttribute(ctx, "parameters", utility.AskUserToolSchema())
		askTool.SetAttribute(ctx, "description", "Ask the user clarifying questions when the request is ambiguous or missing information needed to proceed. Provide 2-4 concrete, mutually exclusive options per question and allow free text when the options may not be exhaustive.")
		askTool.SetAttribute(ctx, "system_prompt", "")
		askTool.SetAttribute(ctx, "tool_name", utility.AskUserToolName)
		askTool.SetAttribute(ctx, "tool_type", "ASK_USER")
		askTool.SetToolAction(utility.AskUserToolName)
		toolsMap[utility.AskUserToolName] = askTool
	}

	if ra.HasMemory() {
		for name, tool := range ra.memoryTools(ctx) {
			if _, configured := toolsMap[name]; !configured {
				toolsMap[name] = tool
			}
		}
	}

	// Every tool this agent can reach is wrapped here - the configured ones, the
	// provider's internal and extra tools, ask_user and structured_output - so
	// every call is recorded whichever loop makes it.
	//
	// AgentTools must be wrapped SEPARATELY even though toolsMap holds the same
	// tools, because toolExecutor resolves configured tools straight out of
	// ra.AgentTools and only falls back to toolsMap for the built-ins. Wrapping
	// the map alone left every configured tool unrecorded: a builder agent's
	// run showed save_field called twice in the metrics tally and absent from
	// the record, so Called("save_field") passed while CalledWith(...) reported
	// it had never been called. An assertion about arguments can only be as
	// complete as the record, and a record with a hole in it is worse than none
	// - it reads as evidence of absence.
	for key, tool := range toolsMap {
		toolsMap[key] = agents.Recording(tool)
	}
	for i := range ra.AgentTools {
		ra.AgentTools[i].Tool = agents.Recording(ra.AgentTools[i].Tool)
	}

	toolExecutor := func(ctx context.Context, toolName string, input map[string]interface{}) (map[string]interface{}, error) {
		// Resolve by the key the model was actually shown, which is the entry's
		// tool_key. Matching on the tool's own name instead would run the first
		// entry that shares it - so an agent holding one tool under several
		// actions would call save_entity no matter which action the model picked.
		for _, at := range ra.AgentTools {
			if at.Tool == nil {
				continue
			}
			key := at.ToolKey
			if key == "" {
				key = at.ToolName
			}
			if key == toolName {
				result, _, execErr := at.Tool.Execute(ctx, projectId, tenantId, at.ActionName, input)
				return result, execErr
			}
		}
		// An agent configured before tool_key was meaningful still names its tools
		// by the tool's own name.
		for _, at := range ra.AgentTools {
			if at.Tool == nil {
				continue
			}
			tnI, _ := at.Tool.GetAttribute(ctx, "tool_name")
			if tn, ok := tnI.(string); ok && tn == toolName {
				result, _, execErr := at.Tool.Execute(ctx, projectId, tenantId, at.ActionName, input)
				return result, execErr
			}
		}
		// Built-in tools contributed by the agent type are not in AgentTools.
		if tool, ok := toolsMap[toolName]; ok && tool != nil {
			result, _, execErr := tool.Execute(ctx, projectId, tenantId, toolName, input)
			return result, execErr
		}
		return nil, fmt.Errorf("tool %s not found", toolName)
	}

	sp := ra.SystemPrompt
	if ra.GetProvider() != nil {
		sp = ra.GetProvider().GetSystemPrompt() + "\n" + sp
	}
	// What the model will be judged on, in the same words it will be judged by -
	// generated from the declarations, so the prompt and the check cannot drift.
	if configured := ra.ConfiguredRulesPrompt(); configured != "" {
		sp = sp + "\n\n" + configured
	}
	if ra.EnableClarification {
		sp = sp + clarificationGuidance
	}
	if ra.HasMemory() {
		sp = sp + agents.MemoryGuidance
	}
	sp = sp + ra.GuardrailSection()
	agentPrompt := models.AgentPrompt{
		Static:  sp,
		Dynamic: ra.ExecutionContextSection(projectId, tenantId),
	}

	// When the agent produces structured output, the final answer is the
	// structured_output tool payload, delivered to the client in the terminal
	// `done` event — NOT as streamed text. If the model instead emits the answer
	// as plain text (e.g. a markdown ```json block), those text_delta events must
	// NOT be forwarded, otherwise the whole answer leaks into the stream. Only
	// thinking is streamed for structured-output agents.
	suppressTextStream := outputSchema.Type != ""
	streamCb := agents.GetStreamCallback(ctx)
	enricher, _ := ra.GetProvider().(agents.StreamEnricher)
	runModel := func() (models.Message, []models.StepTrace, error) {
		if streamCb != nil {
			if streamingModel, ok := ra.Model.(models.StreamingModelI); ok {
				modelCb := func(me models.ModelStreamEvent) {
					// An agent type may derive something useful from the partial
					// answer - a page component the client can render now.
					if enricher != nil {
						for _, derived := range enricher.EnrichStream(ctx, me) {
							streamCb(derived)
						}
					}
					if suppressTextStream && me.Type == models.StreamTextDelta {
						return
					}
					// Raw argument deltas are the answer in fragments: megabytes of
					// half-written JSON that no client can use. Whatever they are
					// worth reaches the client through enrichment instead.
					if me.Type == models.StreamToolInputDelta {
						return
					}
					streamCb(agents.StreamEvent{
						Event:     string(me.Type),
						Data:      me,
						Iteration: me.Iteration,
					})
				}
				return streamingModel.RunToolLoopStreaming(ctx, chatRequest, toolsMap, agentPrompt, ra.MaxIterations, ra.ThinkingBudget, toolExecutor, modelCb)
			}
		}
		return ra.Model.RunToolLoop(ctx, chatRequest, toolsMap, agentPrompt, ra.MaxIterations, ra.ThinkingBudget, toolExecutor)
	}

	agents.Emit(ctx, agents.StreamEvent{
		Event: agents.StreamEventAgentStarted,
		Data:  agents.StepPayload{Detail: ra.AgentName},
	})

	var response models.Message
	var traces []models.StepTrace
	var agentResponse map[string]interface{}
	// attempt counts every trip round the loop, which is what the client sees
	// numbered. conformanceRetries counts only the trips a VALIDATION failure
	// caused, and it alone is measured against RetryCount: a retry spent on
	// taste must not consume the budget a real defect needs.
	attempt := 0
	conformanceRetries := 0
	qualitySpent := 0
	var qualityVerdicts []agents.QualityVerdict
	// shipped is the last answer that passed every conformance check, kept only
	// when the quality gate chose to send it back. See the fallback below: it is
	// the difference between the gate costing a little polish and the gate
	// costing the whole request.
	var shipped *acceptedAnswer
	// Why this run ended, decided at whichever exit is taken, and what it has
	// spent getting there.
	stopReason := agents.StopEndTurn
	spend := agents.Spend{Started: startTime}
	// Every rejection's shape, so a fault the agent has already been shown and
	// already "fixed" is recognised when it comes back.
	faults := newFaultTrail()
	reviewRounds := 0
	for {
		// Before the attempt, not after it: checking afterwards means always
		// paying for the call that crosses the line, and on tokens and minutes
		// that is the call worth not making. A cancelled caller is checked here
		// too - the loop used to keep calling the model for a request nobody was
		// waiting for any more.
		if reason, why := ra.Budget.Exceeded(ctx, spend); reason != "" {
			logs.WithContext(ctx).Error(fmt.Sprintf("agent %s stopped: %s", ra.AgentName, why))
			// Only a VALIDATED answer may be delivered here. agentResponse holds
			// whatever the last attempt produced, and if the loop is still going
			// that attempt was rejected - handing it over because a budget
			// expired would ship an artifact no check ever passed, which is the
			// failure the delivery seal exists to stop. A budget is a reason to
			// stop working, never a reason to lower the bar.
			if shipped == nil {
				agents.Emit(ctx, agents.StreamEvent{Event: agents.StreamEventAgentFinished,
					Data: agents.StepPayload{Detail: ra.AgentName, Outcome: agents.OutcomeError, Code: string(reason)}})
				return agents.AgentMessage{}, fmt.Errorf("agent %s stopped before producing a usable answer: %s", ra.AgentName, why)
			}
			// An answer the quality gate had sent back is still an answer that
			// passed every check. Deliver it, and say plainly that the run was
			// cut short rather than finished.
			response, traces, agentResponse = shipped.response, shipped.traces, shipped.output
			unenforced := shipped.verdict
			unenforced.Enforced = false
			qualityVerdicts = append(qualityVerdicts, unenforced)
			stopReason = reason
			break
		}

		stepStarted := time.Now()
		agents.EmitStepStarted(ctx, agents.StepGenerate, attempt+1)
		response, traces, err = runModel()
		if err != nil {
			logs.WithContext(ctx).Error(err.Error())
			agents.EmitStepFinished(ctx, agents.StepGenerate, attempt+1, agents.OutcomeError, stepStarted, err.Error(), agents.CodeModelError)
			stopReason = agents.StopModelError
			if ctx.Err() != nil {
				stopReason = agents.StopCancelled
			}
			agents.Emit(ctx, agents.StreamEvent{Event: agents.StreamEventAgentFinished,
				Data: agents.StepPayload{Detail: ra.AgentName, Outcome: agents.OutcomeError, Code: string(stopReason)}})
			return agents.AgentMessage{}, err
		}
		agents.EmitStepFinished(ctx, agents.StepGenerate, attempt+1, agents.OutcomeSuccess, stepStarted, "", "")
		spend.Attempts++
		spend.Add(response.Usage)

		agentResponse = parseAgentResponse(response.Content)
		if normalized, ok := deepUnstringifyJSON(agentResponse, "").(map[string]interface{}); ok {
			agentResponse = normalized
		}
		agentResponse = unwrapOutputEnvelope(agentResponse, outputSchema)

		if response.TerminalTool == models.TerminalToolAskUser {
			stopReason = agents.StopAskedUser
			break
		}

		// Whatever the agent type can put right by itself, it puts right here -
		// before the output is judged, so a deterministic fix never costs a
		// retry and never fails a run.
		if normalizer, ok := ra.GetProvider().(agents.OutputNormalizer); ok && normalizer != nil {
			normalizer.NormalizeOutput(ctx, agentResponse)
		}

		validationStarted := time.Now()
		agents.EmitStepStarted(ctx, agents.StepValidate, attempt+1)
		valErr := validateRootKeys(agentResponse, outputSchema)
		if valErr == nil {
			valErr = validateAgainstSchema(agentResponse, outputSchema, "")
		}
		if valErr == nil {
			if validator, ok := ra.GetProvider().(agents.OutputValidator); ok && validator != nil {
				valErr = validator.ValidateOutput(ctx, agentResponse)
			}
		}
		// The rules this agent declares in its configuration, enforced in the
		// same breath as the ones written in Go. An agent may have both; an
		// agent built through the product has only these, and without them its
		// correction loop would retry RetryCount times with nothing checking.
		if valErr == nil {
			valErr = ra.CheckConfiguredRules(ctx, agentResponse)
		}
		if valErr == nil {
			agents.EmitStepFinished(ctx, agents.StepValidate, attempt+1, agents.OutcomeSuccess, validationStarted, "", "")

			// The answer passed every rule. If the agent type can look at it -
			// have the client render it - it does so now, and a defect it sees
			// buys one more attempt. The reviewed answer is held first, so a
			// revision that does worse never costs the user the answer they had.
			if reviewer, ok := ra.GetProvider().(agents.OutputReviewer); ok && reviewer != nil && reviewRounds < agents.MaxReviewRounds {
				review, reviewed := reviewer.ReviewOutput(ctx, agentResponse, reviewRounds)
				if reviewed && (review.Findings != nil || (reviewRounds == 0 && len(review.Files) > 0)) {
					shipped = &acceptedAnswer{response: response, traces: traces, output: agentResponse}
					prompt := review.Prompt
					if review.Findings != nil {
						if repairer, ok := ra.GetProvider().(agents.OutputRepairer); ok && repairer != nil {
							if turn, repairable := repairer.RepairTurn(ctx, agentResponse, review.Findings); repairable {
								if turn.Schema.Type != "" && outputTool != nil {
									outputSchema = turn.Schema
									outputTool.SetAttribute(ctx, "output_schema", outputSchema)
									outputTool.SetAttribute(ctx, "parameters", outputSchema)
								}
								if strings.TrimSpace(turn.Prompt) != "" {
									prompt = strings.TrimSpace(prompt + "\n\n" + turn.Prompt)
								}
							}
						}
						if !strings.Contains(prompt, review.Findings.Error()) {
							prompt = strings.TrimSpace(prompt + "\n\n" + review.Findings.Error())
						}
					}
					logs.WithContext(ctx).Info(fmt.Sprintf("agent %s review round %d: findings=%v screenshots=%d", ra.AgentName, reviewRounds+1, review.Findings != nil, len(review.Files)))
					chatRequest.Messages = append(chatRequest.Messages, models.Message{
						Role:    "user",
						Content: prompt,
						Name:    ra.AgentName,
						Files:   review.Files,
					})
					reviewRounds++
					attempt++
					continue
				}
			}

			// The answer is well formed. The remaining question is whether it is
			// any good, and no rule in this package can answer that.
			verdict, judged := ra.judgeQuality(ctx, agentResponse, attempt, qualitySpent < agents.QualityGateBudget)
			if judged {
				qualityVerdicts = append(qualityVerdicts, verdict)
			}
			if !verdict.Enforced {
				break
			}
			// Hold on to it before gambling it for polish. The gate is about to
			// discard a DELIVERABLE answer in the hope of a better one, and the
			// model may return something worse or nothing usable at all.
			shipped = &acceptedAnswer{response: response, traces: traces, output: agentResponse, verdict: verdict}
			qualitySpent++

			// Ask for a diff if the agent type can answer one. Improving a
			// correct answer by rewriting it wholesale is a large bet on a small
			// change, and twice in a row it lost.
			qualityPrompt := fmt.Sprintf(agents.QualityRetryPrompt, verdict.Reason)
			if repairer, ok := ra.GetProvider().(agents.QualityRepairer); ok && repairer != nil {
				if turn, repairable := repairer.QualityRepairTurn(ctx, agentResponse, verdict); repairable {
					if turn.Schema.Type != "" && outputTool != nil {
						outputSchema = turn.Schema
						outputTool.SetAttribute(ctx, "output_schema", outputSchema)
						outputTool.SetAttribute(ctx, "parameters", outputSchema)
					}
					if strings.TrimSpace(turn.Prompt) != "" {
						qualityPrompt = turn.Prompt
					}
					logs.WithContext(ctx).Info(fmt.Sprintf("agent %s is improving attempt %d as a patch rather than regenerating it", ra.AgentName, attempt+1))
				}
			}
			chatRequest.Messages = append(chatRequest.Messages, models.Message{
				Role:    "user",
				Content: qualityPrompt,
				Name:    ra.AgentName,
			})
			attempt++
			continue
		}
		logs.WithContext(ctx).Error(fmt.Sprintf("agent %s output validation failed (conformance attempt %d of %d): %v", ra.AgentName, conformanceRetries+1, ra.RetryCount+1, valErr))
		if conformanceRetries >= ra.RetryCount {
			// The conformance budget is gone. If the quality gate is the reason
			// we are here at all, take back the answer it rejected.
			//
			// This is the constraint the gate was designed around - "it cannot
			// veto, only annotate" - and without this it is quietly violated.
			// The first live run made that concrete: attempt 1 produced a valid
			// dashboard, the gate sent it back over one missing chart title, and
			// the regenerated answer dropped required keys, invented properties
			// and finally unbound every query. Four conformance attempts later
			// the request failed outright. The user asked for a dashboard and
			// got an error, because we had one with an untitled chart and threw
			// it away.
			//
			// A gate for taste must never be able to do that. The remembered
			// answer is delivered, its poor verdict recorded and marked
			// unenforced, which is exactly what it now is.
			if shipped != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("agent %s could not improve on the answer the quality gate rejected and has run out of conformance attempts; delivering that answer as it stood: %v", ra.AgentName, valErr))
				agents.EmitStepFinished(ctx, agents.StepValidate, attempt+1, agents.OutcomeSuccess, validationStarted,
					"delivered the earlier valid answer: "+shipped.verdict.Reason, "")
				response, traces, agentResponse = shipped.response, shipped.traces, shipped.output
				unenforced := shipped.verdict
				unenforced.Enforced = false
				qualityVerdicts = append(qualityVerdicts, unenforced)
				stopReason = agents.StopRecoveredEarlierAnswer
				break
			}
			agents.EmitStepFinished(ctx, agents.StepValidate, attempt+1, agents.OutcomeError, validationStarted, valErr.Error(), agents.CodeOutputValidation)
			agents.Emit(ctx, agents.StreamEvent{Event: agents.StreamEventAgentFinished,
				Data: agents.StepPayload{Detail: ra.AgentName, Outcome: agents.OutcomeError, Code: string(agents.StopValidationExhausted)}})
			return agents.AgentMessage{}, fmt.Errorf("agent output failed JSON validation after %d attempt(s): %w", attempt+1, valErr)
		}
		agents.EmitStepFinished(ctx, agents.StepValidate, attempt+1, agents.OutcomeRetry, validationStarted, valErr.Error(), agents.CodeOutputValidation)

		// Has this exact rejection been given before? If so the agent is trading
		// one fault for another, and the feedback has to say so - a fourth
		// "this is wrong" is the one thing that demonstrably does not work.
		repeats, earlier := faults.note(attempt+1, valErr.Error())
		if repeats > 0 && faults.distinct() > 1 {
			logs.WithContext(ctx).Error(fmt.Sprintf(
				"agent %s is cycling: the rejection on attempt %d was already given on attempt(s) %s, across %d distinct faults",
				ra.AgentName, attempt+1, describeAttempts(earlier), faults.distinct()))
		}
		// Twice round the same circle is enough to know. Spending the rest of the
		// budget rediscovering it costs minutes and tells nobody anything.
		//
		// Both conditions matter. ONE fault repeated is not a cycle - it is an
		// agent failing to fix something, which the retry budget already exists
		// for, and cutting its attempts short would take away the tries it might
		// have succeeded on. A cycle is two or more faults being traded.
		if repeats >= 2 && faults.distinct() > 1 {
			if shipped != nil {
				response, traces, agentResponse = shipped.response, shipped.traces, shipped.output
				unenforced := shipped.verdict
				unenforced.Enforced = false
				qualityVerdicts = append(qualityVerdicts, unenforced)
				stopReason = agents.StopCycling
				break
			}
			agents.EmitStepFinished(ctx, agents.StepValidate, attempt+1, agents.OutcomeError, validationStarted, valErr.Error(), agents.CodeOutputValidation)
			agents.Emit(ctx, agents.StreamEvent{Event: agents.StreamEventAgentFinished,
				Data: agents.StepPayload{Detail: ra.AgentName, Outcome: agents.OutcomeError, Code: string(agents.StopCycling)}})
			return agents.AgentMessage{}, fmt.Errorf(
				"agent %s is alternating between %d faults and cannot satisfy them at once - the same rejection was given on attempts %s and again on %d: %w",
				ra.AgentName, faults.distinct(), describeAttempts(earlier), attempt+1, valErr)
		}

		// Ask the agent type whether it can answer this rejection with a diff.
		// Nothing here knows what a diff means - that is entirely the agent's
		// business; this only carries the schema and the wording it asks for.
		retryPrompt := fmt.Sprintf(agentValidationRetryPrompt, valErr.Error())
		if repairer, ok := ra.GetProvider().(agents.OutputRepairer); ok && repairer != nil {
			if turn, repairable := repairer.RepairTurn(ctx, agentResponse, valErr); repairable {
				if turn.Schema.Type != "" && outputTool != nil {
					outputSchema = turn.Schema
					outputTool.SetAttribute(ctx, "output_schema", outputSchema)
					outputTool.SetAttribute(ctx, "parameters", outputSchema)
				}
				if strings.TrimSpace(turn.Prompt) != "" {
					retryPrompt = turn.Prompt
				}
				logs.WithContext(ctx).Info(fmt.Sprintf("agent %s is repairing attempt %d rather than regenerating it", ra.AgentName, attempt+1))
			}
		}
		if repeats > 0 && faults.distinct() > 1 {
			retryPrompt = fmt.Sprintf(cycleNote, describeAttempts(earlier)) + "\n" + retryPrompt
		}
		chatRequest.Messages = append(chatRequest.Messages, models.Message{
			Role:    "user",
			Content: retryPrompt,
			Name:    ra.AgentName,
		})
		conformanceRetries++
		attempt++
	}

	metrics := agents.BuildMetrics(ctx, traces, startTime, response.Usage)
	if metrics != nil {
		metrics.Quality = qualityVerdicts
	}

	actionType := agents.ActionTypeAnswer
	if response.TerminalTool == models.TerminalToolAskUser {
		actionType = agents.ActionTypeQuestion
		agents.AttachResumeContext(agentResponse, agents.BuildResumeContext(traces))
	}

	agentOutput := agents.AgentMessage{
		Role: "assistant",
		Actions: []agents.AgentOutputAction{{
			ActionType: actionType,
			ActionName: ra.AgentName,
			Action:     agentResponse,
		}},
		Traces:           traces,
		Metrics:          metrics,
		MessageId:        agentMessage.MessageId,
		MessageTimestamp: time.Now(),
		RetryCount:       attempt,
		StopReason:       stopReason,
	}

	// Everything above has judged this artifact. Seal it, so anything that
	// changes it on the way to the caller is visible rather than silent.
	agents.SealAnswer(&agentOutput)

	conversation.Messages = append(conversation.Messages, agentOutput)
	conversation.NewMessages = append(conversation.NewMessages, agentOutput)
	err = ra.SaveConversation(ctx, conversation, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to save conversation: %v", err))
		return agents.AgentMessage{}, err
	}

	outcome := agents.OutcomeSuccess
	if actionType == agents.ActionTypeQuestion {
		// The agent has not finished: it is waiting for the user to answer.
		outcome = agents.OutcomePaused
	}
	agents.Emit(ctx, agents.StreamEvent{
		Event: agents.StreamEventAgentFinished,
		Data: agents.StepPayload{
			Detail:     ra.AgentName,
			Outcome:    outcome,
			DurationMs: time.Since(startTime).Milliseconds(),
			// The reason travels with the event, so a client can tell a run that
			// finished from one that was cut short without reading the answer.
			Code: string(stopReason),
		},
	})

	return agentOutput, nil
}

func buildCodeAugmentation(params map[string]interface{}) string {
	if params == nil {
		return ""
	}
	codeRaw, ok := params["code"]
	if !ok {
		return ""
	}
	codeStr := StringifyParam(codeRaw)
	if strings.TrimSpace(codeStr) == "" || strings.TrimSpace(codeStr) == "{}" || strings.TrimSpace(codeStr) == "null" {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- EXISTING STRUCTURED OUTPUT FROM PREVIOUS ATTEMPT ---\n")
	b.WriteString("This is the structured output (e.g. JSON or SQL) produced in a previous attempt. Build on top of it and improvize incorporating the user's new instructions: preserve what still applies unless the user prompt clearly requires changing it. If this is blank, generate a fresh output.\n\n")
	b.WriteString(codeStr)
	b.WriteString("\n--- END EXISTING STRUCTURED OUTPUT ---\n\n")
	return b.String()
}

func (ra *ReasoningAgent) ClarificationEnabled() bool {
	return ra.EnableClarification
}

func (ra *ReasoningAgent) GetInputSchema(ctx context.Context) eru_models.JSONSchema {
	if ra.getOutputSchema(ctx).Type == "" {
		return agents.AgentInputSchema(nil, nil)
	}
	return agents.AgentInputSchema(map[string]eru_models.JSONSchema{
		"code": agents.CodeParamSchema("structured output of this agent"),
	}, nil)
}

func (ra *ReasoningAgent) getOutputSchema(ctx context.Context) eru_models.JSONSchema {
	outputSchema := ra.OutputSchema
	if ra.GetProvider() != nil {
		providerSchema := ra.GetProvider().GetOutputSchema(ctx)
		if providerSchema.Type != "" {
			outputSchema = providerSchema
		}
	}
	return outputSchema
}

// parseAgentResponse turns the model's final Message.Content into a structured
// map suitable for AgentOutputAction.Action. It handles three cases:
//  1. Content is already valid JSON object → return parsed map.
//  2. Content is a JSON object wrapped in markdown fences (```json … ``` or
//     ``` … ```) → strip fences, parse, return parsed map. This is what
//     happens when the model returns its answer as plain text instead of
//     calling structured_output.
//  3. Anything else → return {"output": <raw content>}.
func parseAgentResponse(content string) map[string]interface{} {
	trimmed := strings.TrimSpace(content)

	if parsed, ok := tryUnmarshalObject(trimmed); ok {
		return parsed
	}

	if stripped, ok := stripMarkdownFences(trimmed); ok {
		if parsed, ok := tryUnmarshalObject(stripped); ok {
			return parsed
		}
	}

	if start, end, ok := findOuterJSONObject(trimmed); ok {
		if parsed, parsedOk := tryUnmarshalObject(trimmed[start : end+1]); parsedOk {
			return parsed
		}
	}

	return map[string]interface{}{"output": content}
}

// preserveStringJSONKeys lists object keys whose value is intentionally a
// stringified-JSON string that downstream consumers expect to stay a string.
// eru_studio component `data` properties are stringified JSON by design (see the
// eru_studio system prompt), so they must NOT be expanded into objects/arrays.
var preserveStringJSONKeys = map[string]bool{
	"data": true,
}

// deepUnstringifyJSON walks the entire parsed action and repairs values that a
// structured-output model emitted as stringified JSON instead of real JSON.
// Any string whose content is a JSON object/array is parsed back into that type
// and recursed into, so that a stringified `components` array, and deeply nested
// double/triple-encoded arrays inside free-form event payloads / entity_data
// (e.g. "fns":"[\"SEC-FIN-2\"]"), all become proper JSON. Strings that are not
// valid JSON are left untouched, as are keys in preserveStringJSONKeys. The key
// argument is the object key the value was found under ("" for array elements /
// the root).
func deepUnstringifyJSON(value interface{}, key string) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		for k, child := range v {
			v[k] = deepUnstringifyJSON(child, k)
		}
		return v
	case []interface{}:
		for i, item := range v {
			v[i] = deepUnstringifyJSON(item, "")
		}
		return v
	case string:
		if preserveStringJSONKeys[key] {
			return v
		}
		if parsed, ok := parseJSONStringFlexible(v); ok {
			return deepUnstringifyJSON(parsed, key)
		}
		return v
	default:
		return value
	}
}

// parseJSONStringFlexible parses a string that looks like a JSON object/array.
// It returns false for anything that is not JSON so plain strings are preserved.
// When a first parse fails because the model left raw control characters inside
// a string literal (a common truncation/formatting artifact that makes the whole
// payload unparseable), it escapes those control chars and retries once.
func parseJSONStringFlexible(s string) (interface{}, bool) {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 {
		return nil, false
	}
	if c := trimmed[0]; c != '{' && c != '[' {
		return nil, false
	}
	var out interface{}
	if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
		return out, true
	}
	sanitized := escapeControlCharsInStrings(trimmed)
	if sanitized == trimmed {
		return nil, false
	}
	if err := json.Unmarshal([]byte(sanitized), &out); err == nil {
		return out, true
	}
	return nil, false
}

// escapeControlCharsInStrings escapes raw control characters (<0x20) that appear
// INSIDE JSON string literals, leaving structural whitespace between tokens
// untouched so the result stays valid JSON.
func escapeControlCharsInStrings(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				b.WriteByte(c)
				escaped = false
				continue
			}
			switch {
			case c == '\\':
				b.WriteByte(c)
				escaped = true
			case c == '"':
				b.WriteByte(c)
				inString = false
			case c < 0x20:
				switch c {
				case '\n':
					b.WriteString(`\n`)
				case '\t':
					b.WriteString(`\t`)
				case '\r':
					b.WriteString(`\r`)
				default:
					fmt.Fprintf(&b, `\u%04x`, c)
				}
			default:
				b.WriteByte(c)
			}
			continue
		}
		if c == '"' {
			inString = true
		}
		b.WriteByte(c)
	}
	return b.String()
}

// agentValidationRetryPrompt is fed back to the model when its output fails
// JSON validation, so it can self-correct on the next attempt.
//
// It is explicit that the rejection is about the shape of the report because
// models read "your previous output was rejected, send it again" as evidence
// that the work behind the report already happened. One then reported a field
// as created on a retry without ever having called the save tool: the earlier
// attempt had been rejected for a missing key, and it took "call it again" to
// mean the save was done and only the reporting had to be fixed.
const agentValidationRetryPrompt = `Your previous structured_output was NOT valid and was rejected: %s

This rejection is about the SHAPE of your report, nothing else. It is not a signal that your
work succeeded, and it is not a signal that it failed - it says nothing at all about what you
did or did not do. Do NOT assume any tool call has already happened. Judge that only from the
tool results you can actually see in this conversation: if the work has not been done yet, do
it now before reporting, and report only outcomes your tool results support.

Call structured_output again with a corrected result. Requirements:
- The ENTIRE output must be a single valid, parseable JSON object.
- Every field must match its declared type EXACTLY. In particular, array/object fields (e.g. ` + "`components`" + `) MUST be real JSON arrays/objects, NEVER a stringified JSON string.
- All control characters inside string values (newlines, tabs, quotes, backslashes) MUST be properly escaped.
Do not repeat the previous mistake.`

// validateRootKeys checks that the model's output actually has the top-level
// keys the output schema declares as required. Without this a completely
// off-shape answer (e.g. a hand-rolled step DSL instead of the declared
// wrapper) passes validation untouched, because validateAgainstSchema only
// inspects the keys that ARE present. Only the root object is checked - nested
// required lists are left alone so a deep optional omission never fails a
// whole generation.
func validateRootKeys(value interface{}, schema eru_models.JSONSchema) error {
	if schema.Type != "object" || len(schema.Required) == 0 {
		return nil
	}
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	var missing []string
	for _, key := range schema.Required {
		if _, found := m[key]; !found {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	present := make([]string, 0, len(m))
	for key := range m {
		present = append(present, key)
	}
	sort.Strings(present)
	return fmt.Errorf("output is missing required top-level key(s) %s - it must be a single JSON object with exactly the declared top-level keys (got %s); do not invent your own structure", strings.Join(missing, ", "), strings.Join(present, ", "))
}

// validateAgainstSchema checks that value conforms to the type declared by
// schema wherever the schema is strict about arrays/objects. It is the
// validation step for reasoning agents (analogous to go-template execution for
// the gotemplate agent): if a field the schema declares as an array/object is
// still a string after normalization, the model emitted a stringified or
// malformed value that could not be repaired, and we return a descriptive error
// so the model can fix it on retry. Agents without an output schema
// (schema.Type == "") are not validated.
func validateAgainstSchema(value interface{}, schema eru_models.JSONSchema, path string) error {
	switch schema.Type {
	case "object":
		if s, isStr := value.(string); isStr {
			return jsonFieldError(path, "object", s)
		}
		m, ok := value.(map[string]interface{})
		if !ok {
			return nil
		}
		for key, propSchema := range schema.Properties {
			child, exists := m[key]
			if !exists {
				continue
			}
			if err := validateAgainstSchema(child, propSchema, joinSchemaPath(path, key)); err != nil {
				return err
			}
		}
	case "array":
		if s, isStr := value.(string); isStr {
			return jsonFieldError(path, "array", s)
		}
		arr, ok := value.([]interface{})
		if !ok {
			return nil
		}
		if schema.Items != nil {
			for i, item := range arr {
				if err := validateAgainstSchema(item, *schema.Items, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// jsonFieldError builds a descriptive validation error for a field that should
// be a JSON array/object but arrived as a string, including the underlying JSON
// parse error and a snippet around the offending byte to guide the model's fix.
func jsonFieldError(path, want, s string) error {
	field := path
	if field == "" {
		field = "<root>"
	}
	trimmed := strings.TrimSpace(s)
	var probe interface{}
	perr := json.Unmarshal([]byte(trimmed), &probe)
	if perr == nil {
		return fmt.Errorf("field %q must be a JSON %s, but it was sent as a stringified JSON string; emit it as a real %s, not a string", field, want, want)
	}
	snippet := ""
	if se, ok := perr.(*json.SyntaxError); ok {
		off := int(se.Offset)
		start := off - 60
		if start < 0 {
			start = 0
		}
		end := off + 60
		if end > len(trimmed) {
			end = len(trimmed)
		}
		snippet = fmt.Sprintf(" near byte %d: ...%s...", off, trimmed[start:end])
	}
	return fmt.Errorf("field %q must be a valid JSON %s but its value could not be parsed: %v%s", field, want, perr, snippet)
}

func joinSchemaPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func tryUnmarshalObject(s string) (map[string]interface{}, bool) {
	if s == "" {
		return nil, false
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, false
	}
	return out, true
}

// stripMarkdownFences removes a single ```lang … ``` wrapper if present.
// Returns (stripped, true) when fences were found and removed.
func stripMarkdownFences(s string) (string, bool) {
	if !strings.HasPrefix(s, "```") {
		return s, false
	}
	rest := strings.TrimPrefix(s, "```")
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	}
	rest = strings.TrimSpace(rest)
	if idx := strings.LastIndex(rest, "```"); idx >= 0 {
		rest = strings.TrimSpace(rest[:idx])
	}
	return rest, true
}

// findOuterJSONObject locates the first balanced { … } object in the string,
// ignoring braces that appear inside JSON strings. This recovers JSON when
// the model precedes/follows it with prose.
func findOuterJSONObject(s string) (int, int, bool) {
	start := -1
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				return start, i, true
			}
		}
	}
	return 0, 0, false
}

// unwrapOutputEnvelope undoes a wrapper the model sometimes puts around an
// otherwise correct answer: {"output": {...the real answer...}}.
//
// The tool's input schema is flat and says so, but models still wrap, and every
// wrap costs a full retry - and worse, the retry tells the model its output was
// rejected, which it can read as "the work is done, just re-report" and answer
// with fabricated results. Undoing it here is deterministic and costs nothing.
//
// It only unwraps when the answer cannot be anything else: exactly one top-level
// key, a key the schema does not declare, and an inner object that carries every
// key the schema requires. Anything less is left alone for validation to judge.
func unwrapOutputEnvelope(response map[string]interface{}, schema eru_models.JSONSchema) map[string]interface{} {
	if len(response) != 1 || len(schema.Required) == 0 {
		return response
	}
	for key, value := range response {
		if _, declared := schema.Properties[key]; declared {
			return response
		}
		inner, ok := value.(map[string]interface{})
		if !ok {
			return response
		}
		for _, required := range schema.Required {
			if _, found := inner[required]; !found {
				return response
			}
		}
		return inner
	}
	return response
}

func init() {
	agents.RegisterAgentType("REASONING", func() agents.AgentI { return new(ReasoningAgent) })
}

func StringifyParam(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		bytes, err := json.Marshal(val)
		if err != nil {
			return ""
		}
		return string(bytes)
	}
}
