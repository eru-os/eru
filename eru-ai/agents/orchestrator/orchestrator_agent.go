package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	agents "github.com/eru-os/eru/eru-ai/agents"
	"github.com/eru-os/eru/eru-ai/agents/reasoning_agents"
	models "github.com/eru-os/eru/eru-ai/models"
	tools "github.com/eru-os/eru/eru-ai/tools"
	utility "github.com/eru-os/eru/eru-ai/tools/utility"
	functions "github.com/eru-os/eru/eru-functions/functions"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const orchestratorClarificationGuidance = `

HUMAN-IN-THE-LOOP CLARIFICATION:
If the task is too ambiguous or missing information you need to build a correct orchestration plan, call the ask_user tool instead of outputting a FuncGroup. Ask the fewest questions needed, give 2-4 concrete options per question, and allow free text when options may not be exhaustive. Calling ask_user ends your turn; the user's answers arrive as a follow-up message and you then produce the plan.`

// orchestratorGuardrailNote tells the planner how to refuse: the base prompt demands
// structured_output only, so an out-of-scope request needs an explicit text path.
const orchestratorGuardrailNote = `
For a request that falls outside the guardrails above, do NOT build a FuncGroup and
do NOT call structured_output. Reply with plain text saying the request is outside
this agent's scope, and state what it can help with instead. Selecting agents, tools
or steps whose purpose lies outside the guardrails is a violation of them.
`

type AvailableTool struct {
	ToolName string   `json:"tool_name"`
	Actions  []string `json:"actions"`
}

type OrchestratorAgent struct {
	reasoning_agents.ReasoningAgent
	AllowedAgents      []string        `json:"available_agents"`
	AvailableTools     []AvailableTool `json:"available_tools"`
	ClientOutputAgents []string        `json:"client_output_agents"`
	DelegationStrategy string          `json:"delegation_strategy"`
	MaxReplans         int             `json:"max_replans"`
	SynthesisPrompt    string          `json:"synthesis_prompt"`
	discoveredAgents   []agents.DiscoveredAgent
	discoveredTools    []agents.DiscoveredTool
	internalTools      map[string]tools.Tooling
}

// InternalToolRequests names the tools the orchestrator needs for itself rather
// than for a plan.
//
// An orchestrator holds no attached tools: it plans, and eru-functions resolves
// and executes each tool step. That is right for planned work and wrong for the
// page save, which happens after execution, outside any plan - so it looked for
// an attached tool, found "no tools at all", and silently saved nothing while
// telling the user the page was handled.
//
// available_tools cannot serve here: it is the planner's allow-list and carries
// no executable tool. These are resolved from the tenant's configured tools, by
// action, so an owner does not have to know that keeping a generated page needs
// the workspace ids first.
func (oa *OrchestratorAgent) InternalToolRequests() []agents.InternalToolRequest {
	requests := []agents.InternalToolRequest{
		{Action: "save_page", Why: "persisting a page a build produced, which otherwise exists only in the browser"},
	}
	if workspaceContextToolName != "" {
		requests = append(requests, agents.InternalToolRequest{Action: workspaceContextToolName, Why: "the org and process a page is saved against"})
	}
	return append(requests, agents.InternalToolRequest{Action: "execute_query", Why: "resolving the workspace ids when the context action is unavailable"})
}

// SetInternalTools receives whatever the tenant actually has. A missing action
// removes that capability, and the run says so rather than going quiet.
func (oa *OrchestratorAgent) SetInternalTools(resolved map[string]tools.Tooling) {
	oa.internalTools = resolved
}

func (oa *OrchestratorAgent) internalTool(action string) tools.Tooling {
	if oa.internalTools == nil {
		return nil
	}
	return oa.internalTools[action]
}

func (oa *OrchestratorAgent) AllowedAgentNames() []string {
	return oa.AllowedAgents
}

func (oa *OrchestratorAgent) SetDiscoveredAgents(discovered []agents.DiscoveredAgent) {
	oa.discoveredAgents = discovered
}

func (oa *OrchestratorAgent) AllowedToolActions() map[string][]string {
	allowed := make(map[string][]string)
	for _, t := range oa.AvailableTools {
		allowed[t.ToolName] = t.Actions
	}
	return allowed
}

func (oa *OrchestratorAgent) SetDiscoveredTools(discovered []agents.DiscoveredTool) {
	oa.discoveredTools = nil
	for _, dt := range discovered {
		if dt.ActionName == "" {
			continue
		}
		oa.discoveredTools = append(oa.discoveredTools, dt)
	}
}

func (oa *OrchestratorAgent) GetSpec() agents.AgentI {
	return oa
}

func (oa *OrchestratorAgent) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &oa.ReasoningAgent); err != nil {
		return err
	}
	type orchestratorFields struct {
		AllowedAgents      []string        `json:"available_agents"`
		AvailableTools     []AvailableTool `json:"available_tools"`
		ClientOutputAgents []string        `json:"client_output_agents"`
		DelegationStrategy string          `json:"delegation_strategy"`
		MaxReplans         int             `json:"max_replans"`
		SynthesisPrompt    string          `json:"synthesis_prompt"`
	}
	var of orchestratorFields
	if err := json.Unmarshal(b, &of); err != nil {
		return err
	}
	oa.AllowedAgents = of.AllowedAgents
	oa.AvailableTools = of.AvailableTools
	oa.ClientOutputAgents = of.ClientOutputAgents
	oa.DelegationStrategy = of.DelegationStrategy
	oa.MaxReplans = of.MaxReplans
	oa.SynthesisPrompt = of.SynthesisPrompt
	if oa.MaxReplans <= 0 {
		oa.MaxReplans = 2
	}
	if oa.DelegationStrategy == "" {
		oa.DelegationStrategy = "adaptive"
	}
	return nil
}

func (oa *OrchestratorAgent) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("OrchestratorAgent MakeFromJson - Start")
	if err := json.Unmarshal(*rj, oa); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	oa.ReasoningAgent.Agent.Provider = oa
	return nil
}

func (oa *OrchestratorAgent) Execute(ctx context.Context, agentMessage agents.AgentMessage, conversationId string, projectId string, tenantId string) (agents.AgentMessage, error) {
	logs.WithContext(ctx).Debug("OrchestratorAgent Execute - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "OrchestratorAgent.Execute",
		oteltrace.WithAttributes(attribute.String("agent_name", oa.AgentName), attribute.String("conversation_id", conversationId)),
	)
	defer span.End()

	// Enter the delegation chain before doing anything. The orchestrator is the
	// agent this guard was written for - it is the only one that calls others -
	// and leaving it out would have made the limit count only the layers BELOW
	// it, which is the half that was never going to loop.
	enteredCtx, depthErr := agents.EnterAgent(ctx, oa.AgentName, oa.MaxDelegationDepth)
	if depthErr != nil {
		logs.WithContext(ctx).Error(depthErr.Error())
		return agents.AgentMessage{}, depthErr
	}
	ctx = enteredCtx

	// Attachments go to the file store before anything else sees them, so what
	// is remembered, planned with, and forwarded to a step is an id rather than
	// a payload. Nothing downstream should ever carry the bytes.
	agentMessage.Files = oa.offloadAttachments(ctx, agentMessage.Files, projectId, tenantId)

	_, conversation, err := oa.LoadConversations(ctx, conversationId, agentMessage, projectId, tenantId)
	if err != nil {
		return agents.AgentMessage{}, err
	}

	if oa.EnableClarification {
		if answers, ok := agentMessage.ClarificationAnswers(); ok {
			pendingMsg, qa, found := agents.PendingQuestion(conversation)
			if found {
				if pending, perr := agents.ParseClarificationRequest(qa.Action); perr == nil {
					agents.ScrubSecretAnswers(&agentMessage, pending)
					answers, _ = agentMessage.ClarificationAnswers()
				}
			}
			// A checkpoint is worth resuming when it has a step waiting to run OR
			// pages waiting on a yes/no. Requiring a paused branch dropped the
			// page question on the floor: the answer was appended to the message
			// as text and the whole request was planned again from scratch.
			if pr := loadPendingResume(pendingMsg); found && pr != nil && (len(pr.PausedBranches) > 0 || len(pr.PagesToSave) > 0) {
				return oa.resumeOrchestration(ctx, pr, agentMessage, conversation, conversationId, projectId, tenantId)
			}
			var req agents.ClarificationRequest
			if found {
				req, _ = agents.ParseClarificationRequest(qa.Action)
			}
			answerText := agents.FormatAnswersForModel(req, answers)
			if strings.TrimSpace(agentMessage.Content) != "" {
				agentMessage.Content = agentMessage.Content + "\n\n" + answerText
			} else {
				agentMessage.Content = answerText
			}
		}
	}

	bb := NewBlackboard()
	ctx = WithBlackboard(ctx, bb)

	metrics := newRunMetrics()
	ctx = withRunMetrics(ctx, metrics)
	runOutcome := "error"
	defer func() { metrics.report(ctx, oa.AgentName, runOutcome) }()

	streamCb := agents.GetStreamCallback(ctx)
	emitStatus := func(stage string) {
		if streamCb != nil {
			streamCb(agents.StreamEvent{Event: agents.StreamEventStatus, Data: stage})
		}
	}

	codeCtx := describeCodeParam(agentMessage.Params)
	codeCtx.UserMessage = agentMessage.Content
	codeCtx.Attachments = attachmentNames(agentMessage.Files)
	if codeCtx.Present {
		logs.WithContext(ctx).Info(fmt.Sprint("orchestrator params.code received - agent=", oa.AgentName, " conversation_id=", conversationId,
			" kind=", codeCtx.Kind, " size=", codeCtx.Size, " top_keys=", strings.Join(codeCtx.TopKeys, ",")))
	}

	emitStatus("planning")
	decompositionResult, decompositionQuestion, directAnswer, traces, err := oa.decompose(ctx, agentMessage, conversation, codeCtx, projectId, tenantId)
	if err != nil {
		return agents.AgentMessage{}, err
	}

	var allTraces []models.StepTrace
	allTraces = append(allTraces, traces...)

	if decompositionQuestion != nil {
		return oa.emitClarification(ctx, *decompositionQuestion, nil, allTraces, agentMessage, conversation, projectId, tenantId)
	}

	if directAnswer != "" {
		return oa.emitDirectAnswer(ctx, directAnswer, allTraces, agentMessage, conversation, projectId, tenantId)
	}

	decompositionResult, repairTraces, err := oa.repairPlan(ctx, agentMessage, decompositionResult, codeCtx, projectId, tenantId)
	allTraces = append(allTraces, repairTraces...)
	if err != nil {
		return agents.AgentMessage{}, err
	}

	assignStableConversationIds(decompositionResult, conversationId)

	oa.logPlan(ctx, decompositionResult, conversationId)
	oa.logCodeRouting(ctx, decompositionResult, codeCtx)
	if streamCb != nil {
		streamCb(agents.StreamEvent{Event: agents.StreamEventPlan, Data: oa.planEventData(ctx, decompositionResult)})
	}

	var executionResult map[string]interface{}
	var funcVarsMap map[string]functions.FuncTemplateVars
	var execErr error
	// Results of steps that already succeeded on an earlier attempt.
	var carriedResVars map[string]*functions.TemplateVars

	for attempt := 0; attempt <= oa.MaxReplans; attempt++ {
		emitStatus("executing")
		metrics.countExecAttempt(countPlanSteps(decompositionResult))
		endExecuting := metrics.stage("executing")
		executionResult, funcVarsMap, execErr = oa.executeFuncGroup(ctx, decompositionResult, agentMessage, projectId, tenantId, "", "", nil, carriedResVars)
		endExecuting()
		if execErr == nil {
			break
		}

		logs.WithContext(ctx).Info(fmt.Sprintf("Execution failed (attempt %d/%d): %v", attempt+1, oa.MaxReplans+1, execErr))

		if attempt < oa.MaxReplans {
			metrics.countReplan()
			// Whatever finished is kept: the replan is told not to redo it, and
			// its results travel into the next execution so references resolve.
			done := collectCompletedWork(decompositionResult, funcVarsMap)
			if done.any() {
				logs.WithContext(ctx).Info(fmt.Sprint("replanning around ", len(done.Steps), " completed step(s): ", strings.Join(done.Steps, ", ")))
				if carriedResVars == nil {
					carriedResVars = map[string]*functions.TemplateVars{}
				}
				for step, vars := range done.ResVars {
					carriedResVars[step] = vars
				}
			}
			replanResult, replanQuestion, replanTraces, replanErr := oa.replan(ctx, agentMessage, decompositionResult, execErr, done, codeCtx, projectId, tenantId)
			allTraces = append(allTraces, replanTraces...)
			if replanErr != nil {
				logs.WithContext(ctx).Error(fmt.Sprintf("Re-planning failed: %v", replanErr))
				break
			}
			if replanQuestion != nil {
				return oa.emitClarification(ctx, *replanQuestion, nil, allTraces, agentMessage, conversation, projectId, tenantId)
			}
			replanResult, replanRepairTraces, replanRepairErr := oa.repairPlan(ctx, agentMessage, replanResult, codeCtx, projectId, tenantId)
			allTraces = append(allTraces, replanRepairTraces...)
			if replanRepairErr != nil {
				logs.WithContext(ctx).Error(fmt.Sprint("re-planned plan failed template validation: ", replanRepairErr.Error()))
				break
			}
			assignStableConversationIds(replanResult, conversationId)
			oa.logCodeRouting(ctx, replanResult, codeCtx)
			decompositionResult = replanResult
		}
	}

	if execErr != nil {
		return agents.AgentMessage{}, fmt.Errorf("orchestration failed after %d attempts: %w", oa.MaxReplans+1, execErr)
	}

	runOutcome = "success"
	allTraces = append(allTraces, collectSubAgentTraces(funcVarsMap)...)

	if oa.EnableClarification {
		if pr, merged, paused := buildPendingResume(decompositionResult, funcVarsMap, agentMessage.MessageId); paused {
			// A sub-agent's question is not automatically a question for the
			// user: the answer is often in this run's own results, in the
			// conversation, or one read-only lookup away.
			resolution := oa.resolveClarifications(ctx, merged, extractResVars(funcVarsMap), conversation, projectId, tenantId)
			allTraces = append(allTraces, resolution.Traces...)
			pr.ResolvedAnswers = resolution.Answers
			pr.Assumptions = resolution.Assumptions

			if len(resolution.Remaining.Questions) == 0 && len(resolution.Answers) > 0 {
				logs.WithContext(ctx).Info("every question was answerable without the user - resuming")
				return oa.resumeOrchestration(ctx, &pr, withResolvedAnswers(agentMessage, resolution.Answers),
					conversation, conversationId, projectId, tenantId)
			}
			return oa.emitClarification(ctx, resolution.Remaining, &pr, allTraces, agentMessage, conversation, projectId, tenantId)
		}
	}

	// A build that produced pages asks whether to keep them before reporting.
	// The page agent hands pages to the client and never writes them, so without
	// this the entities of a build are on the server and its pages are only in
	// the browser - lost on the next navigation, with nothing said.
	generatedPages := collectGeneratedPages(extractResVars(funcVarsMap))
	logs.WithContext(ctx).Info(fmt.Sprintf("page save: %d page(s) collected from steps [%s]; save_page delegate present=%t, clarification=%t",
		len(generatedPages), strings.Join(resVarStepNames(funcVarsMap), ", "), oa.savePageDelegate(ctx) != nil, oa.EnableClarification))
	logs.WithContext(ctx).Info(fmt.Sprintf("page save: attached tools [%s]", oa.attachedToolNames()))
	if len(generatedPages) == 0 {
		logs.WithContext(ctx).Info(fmt.Sprintf("page save: step shapes %s",
			strings.Join(describeResVars(funcVarsMap), " | ")))
	}
	if pages := generatedPages; len(pages) > 0 {
		switch {
		// Only worth asking if the answer can be acted on. Asked without a
		// save_page delegate, the question costs the user a decision, reports
		// the pages as handled and saves nothing - which is how a dashboard was
		// checked on screen, navigated away from, and found empty.
		case oa.savePageDelegate(ctx) == nil:
			executionResult[pageSaveKey] = []string{fmt.Sprintf(
				"%d page(s) were built and CANNOT be saved: no tool offering save_page is attached to this orchestrator (it has: %s). "+
					"They exist only in this browser session and are lost on the next navigation. Attach a tool exposing save_page to persist them.",
				len(pages), oa.attachedToolNames())}
		case oa.EnableClarification:
			pending := PendingResume{
				RunId:       agentMessage.MessageId,
				Plan:        decompositionResult,
				ResVarsJSON: marshalVars(extractResVars(funcVarsMap)),
				PagesToSave: pages,
			}
			// The pages travel with the question. Asked alone, "save these?" left
			// the client with nothing to draw: the page appeared while it streamed
			// and vanished when the answer - a bare question - arrived, so there
			// was nothing to review before choosing.
			return oa.emitClarification(ctx, pageSaveRequest(pages), &pending, allTraces, agentMessage, conversation, projectId, tenantId,
				oa.collectClientOutputs(ctx, decompositionResult, funcVarsMap, executionResult)...)
		}
	}

	emitStatus("synthesizing")
	synthesisResult, synthesisTraces, err := oa.synthesize(ctx, agentMessage, executionResult, projectId, tenantId)
	allTraces = append(allTraces, synthesisTraces...)
	if err != nil {
		return agents.AgentMessage{}, err
	}

	agentActions := []agents.AgentOutputAction{{
		ActionType: agents.ActionTypeAnswer,
		ActionName: oa.AgentName,
		Action:     synthesisResult,
	}}
	agentActions = append(agentActions, oa.collectClientOutputs(ctx, decompositionResult, funcVarsMap, executionResult)...)

	agentOutput := agents.AgentMessage{
		Role:             "assistant",
		Actions:          agentActions,
		Traces:           labelOwnTraces(allTraces, oa.AgentName),
		MessageId:        agentMessage.MessageId,
		MessageTimestamp: time.Now(),
	}

	conversation.Messages = append(conversation.Messages, agentOutput)
	conversation.NewMessages = append(conversation.NewMessages, agentOutput)
	err = oa.SaveConversation(ctx, conversation, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to save conversation: %v", err))
		return agents.AgentMessage{}, err
	}

	agentOutput.Traces = clientTraces(ctx, agentOutput.Traces)
	return agentOutput, nil
}

// logPlan records the full FuncGroup server side. The plan is internal
// orchestration detail, so this log — not the response — is where it belongs.
func (oa *OrchestratorAgent) logPlan(ctx context.Context, plan map[string]interface{}, conversationId string) {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("failed to marshal plan for logging : ", err.Error()))
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint("orchestrator plan - agent=", oa.AgentName, " conversation_id=", conversationId, " plan=", string(planJSON)))
}

// logCodeRouting records which steps the planner decided need the caller's
// existing structured output, so an unexpected routing decision is diagnosable
// without re-reading the whole plan.
func (oa *OrchestratorAgent) logCodeRouting(ctx context.Context, plan map[string]interface{}, cc codeContext) {
	if !cc.Present {
		return
	}
	routed := codeRoutedSteps(ctx, plan)
	if len(routed) == 0 {
		logs.WithContext(ctx).Info(fmt.Sprint("orchestrator params.code routing - agent=", oa.AgentName, " kind=", cc.Kind, " routed_to=none (planner judged it irrelevant to every step)"))
		return
	}
	logs.WithContext(ctx).Info(fmt.Sprint("orchestrator params.code routing - agent=", oa.AgentName, " kind=", cc.Kind, " routed_to=", strings.Join(routed, ",")))
}

// planningSystemPrompt builds the system prompt used for every planning call.
// When the caller sent an existing structured output in params.code, a
// description of that artifact - never the artifact itself - is appended so the
// planner can route it to the sub-agents it is actually relevant to.
func (oa *OrchestratorAgent) planningSystemPrompt(cc codeContext, includeClarification bool, projectId string, tenantId string) models.AgentPrompt {
	sp := oa.GetSystemPrompt()
	if oa.GetProvider() != nil {
		providerPrompt := oa.GetProvider().GetSystemPrompt()
		if providerPrompt != "" {
			sp = providerPrompt
		}
	}
	if includeClarification && oa.EnableClarification {
		sp = sp + orchestratorClarificationGuidance
	}
	sp = sp + strategyGuidance(oa.DelegationStrategy)
	// Only mentioned when a lookup is actually wired up - a model told about a
	// tool it does not have will try it and spend an iteration finding out.
	sp = sp + researchGuidance(oa.researchTools(context.Background()))

	dynamic := oa.ExecutionContextSection(projectId, tenantId)
	dynamic = dynamic + cc.promptSection(oa.discoveredAgents)
	if guardrail := oa.GuardrailSection(); guardrail != "" {
		dynamic = dynamic + guardrail + orchestratorGuardrailNote
	}
	reportUnsubstitutedPlaceholders(context.Background(), "orchestrator planning prompt", sp)
	return models.AgentPrompt{Static: sp, Dynamic: dynamic}
}

// placeholderPattern matches a template marker that should have been replaced
// before the prompt reached a model.
var placeholderPattern = regexp.MustCompile(`\{\{[A-Z0-9_]+\}\}`)

// reportUnsubstitutedPlaceholders says so when a prompt still carries one.
//
// The orchestrator prompt ended with "{{GUIDELINES_PLACEHOLDER}}" and
// "{{EXAMPLES_PLACEHOLDER}}", neither of which anything ever filled in. Usually
// the model ignores such a marker; asked a short enough question it answered
// with one, verbatim. A prompt is not finished while it still names a hole.
func reportUnsubstitutedPlaceholders(ctx context.Context, what string, prompt string) {
	if found := placeholderPattern.FindAllString(prompt, 3); len(found) > 0 {
		logs.WithContext(ctx).Error(fmt.Sprint("the ", what, " still contains unsubstituted placeholder(s): ", strings.Join(found, ", ")))
	}
}

// planEventData returns what the client receives on the plan event: the step
// graph only, or the whole FuncGroup when raw output was requested.
func (oa *OrchestratorAgent) planEventData(ctx context.Context, plan map[string]interface{}) interface{} {
	if agents.RawOutputEnabled(ctx) {
		return plan
	}
	summary, err := summarizePlan(plan)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("failed to summarize plan : ", err.Error()))
		return planSummary{}
	}
	return summary
}

// emitClarification persists and returns a question action, pausing the
// orchestration until the user answers in the same conversation. When pending
// is non-nil the resume checkpoint is stored on the message so the next turn
// can resume only the remaining steps.
func (oa *OrchestratorAgent) emitClarification(ctx context.Context, req agents.ClarificationRequest, pending *PendingResume, traces []models.StepTrace, agentMessage agents.AgentMessage, conversation *agents.Conversation, projectId string, tenantId string, alongside ...agents.AgentOutputAction) (agents.AgentMessage, error) {
	streamCb := agents.GetStreamCallback(ctx)
	if streamCb != nil {
		action := req.ToAction(oa.AgentName)
		streamCb(agents.StreamEvent{Event: agents.StreamEventQuestion, Data: action.Action})
	}

	agentOutput := agents.AgentMessage{
		Role:             "assistant",
		Actions:          append([]agents.AgentOutputAction{req.ToAction(oa.AgentName)}, alongside...),
		Traces:           traces,
		MessageId:        agentMessage.MessageId,
		MessageTimestamp: time.Now(),
	}
	if pending != nil {
		prBytes, _ := json.Marshal(pending)
		var prMap map[string]interface{}
		if json.Unmarshal(prBytes, &prMap) == nil {
			agentOutput.Params = map[string]interface{}{PendingResumeParamKey: prMap}
		}
	}

	conversation.Messages = append(conversation.Messages, agentOutput)
	conversation.NewMessages = append(conversation.NewMessages, agentOutput)
	if err := oa.SaveConversation(ctx, conversation, projectId, tenantId); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to save conversation: %v", err))
		return agents.AgentMessage{}, err
	}
	agentOutput.Traces = clientTraces(ctx, agentOutput.Traces)
	return agentOutput, nil
}

// emitDirectAnswer persists and returns a plain answer produced by the planner
// itself when the task needs no sub-agents or tools. The text has already been
// streamed to the client via text_delta events during decomposition, so this
// only records the final answer action and saves the conversation.
func (oa *OrchestratorAgent) emitDirectAnswer(ctx context.Context, answer string, traces []models.StepTrace, agentMessage agents.AgentMessage, conversation *agents.Conversation, projectId string, tenantId string) (agents.AgentMessage, error) {
	agentOutput := agents.AgentMessage{
		Role: "assistant",
		Actions: []agents.AgentOutputAction{{
			ActionType: agents.ActionTypeAnswer,
			ActionName: oa.AgentName,
			Action:     map[string]interface{}{"response": answer},
		}},
		Traces:           traces,
		MessageId:        agentMessage.MessageId,
		MessageTimestamp: time.Now(),
	}

	conversation.Messages = append(conversation.Messages, agentOutput)
	conversation.NewMessages = append(conversation.NewMessages, agentOutput)
	if err := oa.SaveConversation(ctx, conversation, projectId, tenantId); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to save conversation: %v", err))
		return agents.AgentMessage{}, err
	}
	agentOutput.Traces = clientTraces(ctx, agentOutput.Traces)
	return agentOutput, nil
}

// loadPendingResume reads a resume checkpoint persisted on a question message.
func loadPendingResume(msg agents.AgentMessage) *PendingResume {
	if msg.Params == nil {
		return nil
	}
	raw, ok := msg.Params[PendingResumeParamKey]
	if !ok {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var pr PendingResume
	if err := json.Unmarshal(b, &pr); err != nil {
		return nil
	}
	return &pr
}

// resumeOrchestration resumes a paused plan: it re-runs each paused branch from
// its own step (bounded so the join child does not run), seeded with the
// completed steps' outputs, then runs the join step with the merged results.
// Parallel siblings already finished on the original run, so branch-resumes are
// independent; they are run sequentially here and their disjoint outputs merged. Re-pause is handled by the same path.
func (oa *OrchestratorAgent) resumeOrchestration(ctx context.Context, pr *PendingResume, agentMessage agents.AgentMessage, conversation *agents.Conversation, conversationId string, projectId string, tenantId string) (agents.AgentMessage, error) {
	logs.WithContext(ctx).Debug("OrchestratorAgent resumeOrchestration - Start")
	bb := NewBlackboard()
	ctx = WithBlackboard(ctx, bb)

	reqVars := unmarshalVars(pr.ReqVarsJSON)
	merged := unmarshalVars(pr.ResVarsJSON)
	var allTraces []models.StepTrace

	// The pages question has no step behind it - the plan already finished - so it
	// is answered here. There are no paused branches and no join step on such a
	// checkpoint, so the rest of this function falls through to synthesis with
	// the report carried alongside the results.
	var pageSaveReport []string
	if len(pr.PagesToSave) > 0 {
		answers, _ := agentMessage.ClarificationAnswers()
		if answeredYesToPageSave(answers) {
			pageSaveReport = oa.savePages(ctx, pr.PagesToSave, projectId, tenantId)
		} else {
			pageSaveReport = []string{fmt.Sprintf(
				"%d page(s) were built but left unsaved at your request; they are gone once this screen is left.",
				len(pr.PagesToSave))}
		}
	}

	// Questions the orchestrator answered for itself are not re-asked, so they
	// only reach the sub-agent if they are put back here alongside the user's.
	agentMessage = withResolvedAnswers(agentMessage, pr.ResolvedAnswers)

	for _, branch := range pr.PausedBranches {
		// Each branch gets only its own answers, with the step prefix stripped, so
		// the sub-agent recognises the question ids it asked with.
		branchMessage := withBranchAnswers(agentMessage, branch)
		if answers, ok := branchMessage.ClarificationAnswers(); ok {
			logs.WithContext(ctx).Info(fmt.Sprint("resumeOrchestration - forwarding ", len(answers), " answer(s) to step ", branch.StartStep))
		} else {
			logs.WithContext(ctx).Info(fmt.Sprint("resumeOrchestration - step ", branch.StartStep,
				" is resuming with no answers of its own; it will re-ask unless the plan forwards params.clarification_answers"))
		}
		_, fvm, err := oa.executeFuncGroup(ctx, pr.Plan, branchMessage, projectId, tenantId, branch.StartStep, branch.EndStep, reqVars, merged)
		if err != nil {
			return agents.AgentMessage{}, fmt.Errorf("branch resume %s failed: %w", branch.StartStep, err)
		}
		allTraces = append(allTraces, collectSubAgentTraces(fvm)...)
		for k, v := range extractResVars(fvm) {
			merged[k] = v
		}
	}

	var executionResult map[string]interface{}
	if pr.JoinStep != "" {
		res, fvm, err := oa.executeFuncGroup(ctx, pr.Plan, agentMessage, projectId, tenantId, pr.JoinStep, "", reqVars, merged)
		if err != nil {
			return agents.AgentMessage{}, fmt.Errorf("join resume %s failed: %w", pr.JoinStep, err)
		}
		allTraces = append(allTraces, collectSubAgentTraces(fvm)...)
		for k, v := range extractResVars(fvm) {
			merged[k] = v
		}
		executionResult = res
	} else {
		executionResult = resVarsToResult(merged)
	}
	if len(pageSaveReport) > 0 {
		if executionResult == nil {
			executionResult = map[string]interface{}{}
		}
		executionResult[pageSaveKey] = pageSaveReport
	}

	if newPr, newMerged, paused := buildPendingResumeFromVars(pr.Plan, merged, agentMessage.MessageId); paused {
		return oa.emitClarification(ctx, newMerged, &newPr, allTraces, agentMessage, conversation, projectId, tenantId)
	}

	synthesisResult, synthesisTraces, err := oa.synthesize(ctx, agentMessage, executionResult, projectId, tenantId)
	allTraces = append(allTraces, synthesisTraces...)
	if err != nil {
		return agents.AgentMessage{}, err
	}
	// Anything answered on the user's behalf is said out loud. An assumption
	// nobody sees is how an assistant stops being worth trusting.
	noteAssumptions(synthesisResult, pr.Assumptions)

	resumeActions := []agents.AgentOutputAction{{
		ActionType: agents.ActionTypeAnswer,
		ActionName: oa.AgentName,
		Action:     synthesisResult,
	}}
	resumeActions = append(resumeActions, oa.collectClientOutputs(ctx, pr.Plan, map[string]functions.FuncTemplateVars{pr.JoinStep: {ResVars: merged}}, executionResult)...)

	agentOutput := agents.AgentMessage{
		Role:             "assistant",
		Actions:          resumeActions,
		Traces:           labelOwnTraces(allTraces, oa.AgentName),
		MessageId:        agentMessage.MessageId,
		MessageTimestamp: time.Now(),
	}
	conversation.Messages = append(conversation.Messages, agentOutput)
	conversation.NewMessages = append(conversation.NewMessages, agentOutput)
	if err := oa.SaveConversation(ctx, conversation, projectId, tenantId); err != nil {
		logs.WithContext(ctx).Error(fmt.Sprintf("Failed to save conversation: %v", err))
		return agents.AgentMessage{}, err
	}
	agentOutput.Traces = clientTraces(ctx, agentOutput.Traces)
	return agentOutput, nil
}

func (oa *OrchestratorAgent) decompose(ctx context.Context, agentMessage agents.AgentMessage, conversation *agents.Conversation, cc codeContext, projectId string, tenantId string) (map[string]interface{}, *agents.ClarificationRequest, string, []models.StepTrace, error) {
	defer runMetricsFrom(ctx).stage("planning")()
	runMetricsFrom(ctx).countPlanAttempt()
	logs.WithContext(ctx).Debug("OrchestratorAgent decompose - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "OrchestratorAgent.Decompose")
	defer span.End()

	// The planner is shown a small copy of an image and the name of everything
	// else. It is deciding who should look at a file, not reading it, and the
	// original can be megabytes.
	planningFiles := planningAttachments(ctx, agentMessage.Files)
	content := agentMessage.Content
	if summary := attachmentSummary(agentMessage.Files); summary != "" {
		content = content + "\n\n" + summary
	}
	current := models.Message{
		Role:    "user",
		Content: content,
		Files:   planningFiles,
	}
	chatRequest := models.ChatRequest{Messages: []models.Message{current}}
	// Plan with what has already been said. The conversation was loaded and then
	// discarded here, so every turn was planned as though it were the first: the
	// user could tell the assistant something and it had no idea a moment later.
	// The conversation manager owns how much of the history fits.
	if oa.ConversationManager != nil && conversation != nil && len(conversation.Messages) > 0 {
		if built, buildErr := oa.ConversationManager.BuildChatRequest(ctx, conversation, current, oa.AgentName); buildErr == nil && built != nil {
			chatRequest = *built
		} else if buildErr != nil {
			logs.WithContext(ctx).Error(fmt.Sprint("could not build the planning request from history, planning from this message alone: ", buildErr.Error()))
		}
	}
	logs.WithContext(ctx).Info(fmt.Sprint("planning with ", len(chatRequest.Messages), " message(s) of context"))

	toolsMap := oa.buildDecompositionTools(ctx)

	sp := oa.planningSystemPrompt(cc, true, projectId, tenantId)

	research := oa.researchTools(ctx)
	// Whether the planner COULD look anything up is otherwise invisible: a run
	// with no lookups looks the same whether it declined to research or was
	// never offered the chance.
	if len(research) == 0 {
		logs.WithContext(ctx).Info("planner research: none available (no eru-ql tool is attached to this orchestrator)")
	} else {
		names := make([]string, 0, len(research))
		for name := range research {
			names = append(names, name)
		}
		sort.Strings(names)
		logs.WithContext(ctx).Info(fmt.Sprint("planner research: offered ", strings.Join(names, ", ")))
	}
	toolExecutor := func(ctx context.Context, toolName string, input map[string]interface{}) (map[string]interface{}, error) {
		if result, err, handled := executeResearchTool(ctx, research, toolName, projectId, tenantId, input); handled {
			return result, err
		}
		return nil, fmt.Errorf("tool %s not expected during decomposition", toolName)
	}

	var response models.Message
	var traces []models.StepTrace
	var err error
	streamCb := agents.GetStreamCallback(ctx)
	if streamingModel, ok := oa.Model.(models.StreamingModelI); ok && streamCb != nil {
		modelCb := func(me models.ModelStreamEvent) {
			streamCb(agents.StreamEvent{Event: string(me.Type), Data: me, Iteration: me.Iteration})
		}
		response, traces, err = streamingModel.RunToolLoopStreaming(ctx, chatRequest, toolsMap, sp, oa.MaxIterations, oa.ThinkingBudget, toolExecutor, modelCb)
	} else {
		response, traces, err = oa.Model.RunToolLoop(ctx, chatRequest, toolsMap, sp, oa.MaxIterations, oa.ThinkingBudget, toolExecutor)
	}
	if err != nil {
		return nil, nil, "", traces, err
	}

	if response.TerminalTool == models.TerminalToolAskUser {
		var action map[string]interface{}
		if err := json.Unmarshal([]byte(response.Content), &action); err != nil {
			return nil, nil, "", traces, fmt.Errorf("failed to parse clarification request: %w", err)
		}
		req, err := agents.ParseClarificationRequest(action)
		if err != nil {
			return nil, nil, "", traces, err
		}
		return nil, &req, "", traces, nil
	}

	if response.TerminalTool != models.TerminalToolStructuredOutput {
		return nil, nil, response.Content, traces, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		return nil, nil, "", traces, fmt.Errorf("failed to parse decomposition result: %w", err)
	}

	return result, nil, "", traces, nil
}

func (oa *OrchestratorAgent) executeFuncGroup(ctx context.Context, funcGroupMap map[string]interface{}, agentMessage agents.AgentMessage, projectId string, tenantId string, startStep string, endStep string, reqVars map[string]*functions.TemplateVars, resVars map[string]*functions.TemplateVars) (map[string]interface{}, map[string]functions.FuncTemplateVars, error) {
	logs.WithContext(ctx).Debug("OrchestratorAgent executeFuncGroup - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "OrchestratorAgent.ExecuteFuncGroup")
	defer span.End()

	funcGroupJSON, err := json.Marshal(funcGroupMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal FuncGroup: %w", err)
	}

	var funcGroup functions.FuncGroup
	if err := json.Unmarshal(funcGroupJSON, &funcGroup); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal FuncGroup: %w", err)
	}

	result, funcVarsMap, err := oa.ExecuteFuncGroup(ctx, funcGroup, agentMessage, projectId, tenantId, startStep, endStep, reqVars, resVars)
	if err != nil {
		return nil, nil, err
	}

	return result, funcVarsMap, nil
}

// repairPlan parses every go template in the plan and, when any is invalid, feeds
// the parse errors back to the model for correction. Templates are validated
// before execution so a malformed one cannot half-run the plan and leave side
// effects behind.
func (oa *OrchestratorAgent) repairPlan(ctx context.Context, agentMessage agents.AgentMessage, plan map[string]interface{}, cc codeContext, projectId string, tenantId string) (map[string]interface{}, []models.StepTrace, error) {
	logs.WithContext(ctx).Debug("OrchestratorAgent repairPlan - Start")
	maxAttempts := oa.RetryCount
	if maxAttempts < 2 {
		maxAttempts = 2
	}
	var traces []models.StepTrace
	for attempt := 0; ; attempt++ {
		// A step that described its request rather than writing the template is
		// compiled here, before anything is validated, so the rest of the
		// pipeline only ever sees a plan with real templates in it.
		issues := compileStepRequests(plan)
		issues = append(issues, validatePlan(ctx, plan, oa.discoveredAgents, oa.discoveredTools, cc)...)
		if strings.EqualFold(oa.DelegationStrategy, StrategySequential) {
			issues = append(issues, validateSequentialPlan(plan)...)
		}
		if len(issues) == 0 {
			return plan, traces, nil
		}
		runMetricsFrom(ctx).recordPlanInvalid(issues)
		issueText := formatPlanIssues(issues)
		logs.WithContext(ctx).Error(fmt.Sprint("invalid plan (attempt ", attempt+1, "/", maxAttempts+1, "):\n", issueText))
		if attempt >= maxAttempts {
			return nil, traces, fmt.Errorf("plan is still invalid after %d repair attempt(s):\n%s", maxAttempts, issueText)
		}
		if streamCb := agents.GetStreamCallback(ctx); streamCb != nil {
			streamCb(agents.StreamEvent{Event: agents.StreamEventStatus, Data: "repairing_plan"})
		}
		repairedPlan, repairTraces, err := oa.repairPlanOnce(ctx, agentMessage, plan, issueText, cc, projectId, tenantId)
		traces = append(traces, repairTraces...)
		if err != nil {
			return nil, traces, err
		}
		plan = repairedPlan
	}
}

func (oa *OrchestratorAgent) repairPlanOnce(ctx context.Context, agentMessage agents.AgentMessage, plan map[string]interface{}, issueText string, cc codeContext, projectId string, tenantId string) (map[string]interface{}, []models.StepTrace, error) {
	defer runMetricsFrom(ctx).stage("plan_repair")()
	runMetricsFrom(ctx).countPlanAttempt()
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, nil, err
	}

	repairContent := fmt.Sprintf(
		"The FuncGroup you produced is invalid. Fix ONLY the problems listed below and return the corrected FuncGroup via structured_output.\n\nProblems found:\n%s\nPrevious plan:\n%s\n\nOriginal request: %s\n\nRules:\n- Change only what is needed to clear the problems listed above; keep the rest of the plan (steps, order, agents, tools, logic) exactly as it is.\n- Every '{{' must have a matching '}}' and every '(' a matching ')'; do not leave stray braces or parentheses at the end of an action.\n- Any template that builds an object must still be wrapped in / piped through stringify.\n- Reference a previous step only by its exact func_steps key, and only a step that actually runs before this one.\n- Use only the agents and tool actions listed in the AVAILABLE AGENTS / AVAILABLE TOOLS sections.",
		issueText,
		string(planJSON),
		agentMessage.Content,
	)

	chatRequest := models.ChatRequest{
		Messages: []models.Message{{
			Role:    "user",
			Content: repairContent,
		}},
	}

	toolsMap := oa.buildDecompositionTools(ctx)
	delete(toolsMap, utility.AskUserToolName)

	sp := oa.planningSystemPrompt(cc, false, projectId, tenantId)

	toolExecutor := func(ctx context.Context, toolName string, input map[string]interface{}) (map[string]interface{}, error) {
		return nil, fmt.Errorf("tool %s not expected during plan repair", toolName)
	}

	response, traces, err := oa.Model.RunToolLoop(ctx, chatRequest, toolsMap, sp, oa.MaxIterations, oa.ThinkingBudget, toolExecutor)
	if err != nil {
		return nil, traces, err
	}
	if response.TerminalTool != models.TerminalToolStructuredOutput {
		return nil, traces, fmt.Errorf("plan repair did not return a FuncGroup")
	}

	var repairedPlan map[string]interface{}
	if err := json.Unmarshal([]byte(response.Content), &repairedPlan); err != nil {
		return nil, traces, fmt.Errorf("failed to parse repaired plan: %w", err)
	}
	return repairedPlan, traces, nil
}

func (oa *OrchestratorAgent) replan(ctx context.Context, agentMessage agents.AgentMessage, previousPlan map[string]interface{}, previousErr error, done completedWork, cc codeContext, projectId string, tenantId string) (map[string]interface{}, *agents.ClarificationRequest, []models.StepTrace, error) {
	defer runMetricsFrom(ctx).stage("replanning")()
	runMetricsFrom(ctx).countPlanAttempt()
	logs.WithContext(ctx).Debug("OrchestratorAgent replan - Start")

	previousPlanJSON, _ := json.Marshal(previousPlan)

	replanContent := fmt.Sprintf(
		"The previous plan failed with error: %s\n\nPrevious plan:\n%s\n\nOriginal request: %s\n\nPlease generate a corrected FuncGroup that avoids this error.%s",
		previousErr.Error(),
		string(previousPlanJSON),
		agentMessage.Content,
		done.briefing(),
	)

	chatRequest := models.ChatRequest{
		Messages: []models.Message{{
			Role:    "user",
			Content: replanContent,
		}},
	}

	toolsMap := oa.buildDecompositionTools(ctx)

	sp := oa.planningSystemPrompt(cc, false, projectId, tenantId)

	toolExecutor := func(ctx context.Context, toolName string, input map[string]interface{}) (map[string]interface{}, error) {
		return nil, fmt.Errorf("tool %s not expected during replanning", toolName)
	}

	response, traces, err := oa.Model.RunToolLoop(ctx, chatRequest, toolsMap, sp, oa.MaxIterations, oa.ThinkingBudget, toolExecutor)
	if err != nil {
		return nil, nil, traces, err
	}

	if response.TerminalTool == models.TerminalToolAskUser {
		var action map[string]interface{}
		if err := json.Unmarshal([]byte(response.Content), &action); err != nil {
			return nil, nil, traces, fmt.Errorf("failed to parse clarification request: %w", err)
		}
		req, err := agents.ParseClarificationRequest(action)
		if err != nil {
			return nil, nil, traces, err
		}
		return nil, &req, traces, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		return nil, nil, traces, fmt.Errorf("failed to parse replan result: %w", err)
	}

	return result, nil, traces, nil
}

func (oa *OrchestratorAgent) synthesize(ctx context.Context, agentMessage agents.AgentMessage, executionResult map[string]interface{}, projectId string, tenantId string) (map[string]interface{}, []models.StepTrace, error) {
	defer runMetricsFrom(ctx).stage("synthesizing")()
	logs.WithContext(ctx).Debug("OrchestratorAgent synthesize - Start")
	ctx, span := otel.Tracer("eru-ai").Start(ctx, "OrchestratorAgent.Synthesize")
	defer span.End()

	resultJSON, _ := json.Marshal(executionResult)

	synthesisPrompt := oa.SynthesisPrompt
	if synthesisPrompt == "" {
		synthesisPrompt = "Synthesize the results from the sub-agents into a coherent, unified response that directly addresses the user's request."
	}

	var bbSection string
	if bb := GetBlackboard(ctx); bb != nil {
		bbState := bb.GetAll()
		if len(bbState) > 0 {
			bbJSON, _ := json.Marshal(bbState)
			bbSection = fmt.Sprintf("\n\nShared state (blackboard):\n%s", string(bbJSON))
		}
	}

	synthesisContent := fmt.Sprintf(
		"Original request: %s\n\nSub-agent results:\n%s%s\n\n%s\n\n%s\n\n%s",
		agentMessage.Content,
		string(resultJSON),
		bbSection,
		synthesisPrompt,
		"IMPORTANT: Respond with human-readable prose only. Do NOT embed raw JSON, code fences, or structured/widget payloads in your response — those are returned to the client separately as distinct actions. Summarize the result in words.",
		"GROUNDING (ABSOLUTE): every figure, name, date and total you state MUST appear in the sub-agent results above. Never invent, estimate, extrapolate or illustrate values, and never restate numbers that a sub-agent may itself have fabricated in place of missing data. If the results are empty, null, an error, or contain no rows for what the user asked, say plainly that no data was returned and what failed — do not produce a summary, table or percentages. Do not compute shares or totals unless the underlying values are present.",
	)

	chatRequest := models.ChatRequest{
		Messages: []models.Message{{
			Role:    "user",
			Content: synthesisContent,
		}},
	}

	var response models.Message
	var err error
	streamCb := agents.GetStreamCallback(ctx)
	if streamingModel, ok := oa.Model.(models.StreamingModelI); ok && streamCb != nil {
		response, err = streamingModel.QueryModelStreaming(ctx, chatRequest, func(chunk string) {
			streamCb(agents.StreamEvent{Event: agents.StreamEventTextDelta, Data: chunk})
		})
	} else {
		response, err = oa.Model.QueryModel(ctx, chatRequest)
	}
	if err != nil {
		return nil, nil, err
	}

	trace := models.StepTrace{
		Iteration: 1,
		Content:   response.Content,
		Timestamp: time.Now(),
	}

	result := map[string]interface{}{
		"response": response.Content,
	}
	responseMap := map[string]interface{}{}
	if jsonErr := json.Unmarshal([]byte(response.Content), &responseMap); jsonErr == nil {
		result = responseMap
	}

	return result, []models.StepTrace{trace}, nil
}

func (oa *OrchestratorAgent) buildDecompositionTools(ctx context.Context) map[string]tools.Tooling {
	toolsMap := make(map[string]tools.Tooling)
	outputSchema := oa.GetOutputSchema(ctx)
	if outputSchema.Type != "" {
		outputTool := &utility.StructuredOutputTool{}
		outputTool.SetAttribute(ctx, "output_schema", outputSchema)
		outputTool.SetAttribute(ctx, "parameters", outputSchema)
		outputTool.SetAttribute(ctx, "description", "Output the final FuncGroup JSON. Call this tool when you have the complete orchestration plan ready.")
		outputTool.SetAttribute(ctx, "tool_name", "structured_output")
		outputTool.SetAttribute(ctx, "tool_type", "STRUCTURED_OUTPUT")
		outputTool.SetToolAction("structured_output")
		toolsMap["structured_output"] = outputTool
	}
	if oa.EnableClarification {
		askTool := &utility.AskUserTool{}
		askTool.SetAttribute(ctx, "parameters", utility.AskUserToolSchema())
		askTool.SetAttribute(ctx, "description", "Ask the user clarifying questions when the task is ambiguous or missing information needed to plan the orchestration. Provide 2-4 concrete options per question and allow free text when options may not be exhaustive.")
		askTool.SetAttribute(ctx, "system_prompt", "")
		askTool.SetAttribute(ctx, "tool_name", utility.AskUserToolName)
		askTool.SetAttribute(ctx, "tool_type", "ASK_USER")
		askTool.SetToolAction(utility.AskUserToolName)
		toolsMap[utility.AskUserToolName] = askTool
	}
	// Read-only lookups the planner may make before committing to a plan.
	for name, tool := range oa.researchTools(ctx) {
		if _, taken := toolsMap[name]; !taken {
			toolsMap[name] = tool
		}
	}
	return toolsMap
}

func (oa *OrchestratorAgent) GetResponseSchema(_ context.Context) eru_models.JSONSchema {
	return oa.OutputSchema
}

func (oa *OrchestratorAgent) GetOutputSchema(ctx context.Context) eru_models.JSONSchema {
	sampleFuncGroup := functions.FuncGroup{
		FuncCategoryName:        "sample",
		FuncGroupName:           "sample",
		ResponseStatusCode:      200,
		ResponseStatusCondition: "ERROR",
		ResponseContentType:     "application/json",
		FuncSteps: map[string]*functions.FuncStep{
			"sample_agent": {
				AgentName: "sample",
				FuncSteps: map[string]*functions.FuncStep{},
			},
		},
	}
	return eru_utils.StructToJSONSchema(reflect.TypeOf(sampleFuncGroup), []string{})
}

func (oa *OrchestratorAgent) GetSystemPrompt() string {
	agentDescriptions := oa.buildAgentDescriptions()
	toolDescriptions := oa.buildToolDescriptions()

	systemPrompt := `You are an expert orchestration engineer for the Eru platform.
Your job is to decompose complex user tasks into a FuncGroup that coordinates sub-agents and tools.
Output ONLY the FuncGroup JSON via the structured_output tool. No markdown, no explanations.

============================================================
AVAILABLE AGENTS
============================================================

` + agentDescriptions + `

============================================================
AVAILABLE TOOLS
============================================================

` + toolDescriptions + `

============================================================
SELECTION — USE ONLY WHAT THE TASK NEEDS
============================================================

The lists above are what you MAY use, not what you MUST use. Do NOT use every
available agent/tool. Choose only the minimal set of agents and tools that best
accomplish the user's request — often that is a single agent or a short chain.
Omit anything not required.

============================================================
RULE #1 — STEP KEY = AGENT NAME / TOOL+ACTION NAME (most common mistake)
============================================================

Every func_step key MUST exactly equal the agent_name value of that step.

CORRECT:
  {"classifier": {"agent_name": "classifier"}}
  {"summarizer": {"agent_name": "summarizer"}}

WRONG:
  {"step1": {"agent_name": "classifier"}}
  {"classify_data": {"agent_name": "classifier"}}

STEP KEYS MUST BE UNIQUE ACROSS THE WHOLE PLAN — not just within one func_steps
map. Step results are stored in ONE FLAT namespace keyed by the step key, so two
steps sharing a key (even in different branches, at different nesting depths)
OVERWRITE each other and every downstream .ResVars reference to that key reads
whichever branch happened to finish last.

If the same agent or tool action is needed more than once ANYWHERE in the plan,
give each occurrence a numeric suffix — "classifier", "classifier2",
"classifier3" — and reference each one by its own exact key.

============================================================
FUNCGROUP STRUCTURE
============================================================

{
  "func_category_name": "<snake_case, MANDATORY>",
  "func_group_name":    "<snake_case, MANDATORY>",
  "response_status_code": 200,
  "response_status_condition": "ERROR",
  "response_content_type": "application/json",
  "func_steps": { ... }
}

============================================================
STEP TYPE — AGENT or TOOL
============================================================

A step is EITHER an agent step OR a tool step. Use ONLY agents listed in
AVAILABLE AGENTS and tools listed in AVAILABLE TOOLS.

Agent step:
  "agent_name": "<name from available agents>"

Tool step:
  "tool_name":   "<tool from available tools>"
  "tool_action": "<one of that tool's allowed actions>"

Steps carry no tenant. Every agent and tool runs for the tenant this plan is
executed for, falling back to that tenant's default tenant and then to the
project. Never emit a "tenant_id" field on a step.

Do NOT use query_name, function_name, or api steps.

============================================================
RULE #2 — EVERY STEP MUST DESCRIBE ITS REQUEST (use "request")
============================================================

Every agent receives its request body decoded into this exact JSON shape:
  {"content": "<string>"}      ← "content" is the agent's input message (REQUIRED)
You may also include "params" (object) and "files" (array) ONLY if needed.
ANY other/unknown top-level key is REJECTED by the agent (unknown field error),
and a bare string or number is REJECTED (it must be a JSON object).

The exact contract for EACH agent is printed in the AVAILABLE AGENTS section above:
  - "Params keys this agent READS" is the CLOSED list of params keys it consumes,
    and "Params schema" gives their exact types and meaning.
    A params key that is not on that agent's list is silently DISCARDED - the step
    still succeeds, the data just never reaches the agent, and the agent then makes
    something up. If the data you need to pass has no matching params key on that
    agent, put it in "content" instead. Never invent a params key.
  - "Response" says whether it answers with named output fields or free text, and
    "Output schema" gives the exact fields. Read the response only by those paths -
    do not guess a field name that is not in that agent's schema.

ATTACHMENTS
"files" is a top-level key of the request, beside "content" - NOT a params key.
Every agent takes it, whatever its type; it is the same field this orchestrator
was itself given the user's attachments in. To pass them on:
  "request": {"content": "...", "files": {"from": "user.files"}}
Pass them to the step that needs to look at the file, and only that step - an
agent that has no use for it should not be carrying it. Never write the file's
contents into the plan: pass the reference, never the data.

Decide from what the file IS and what each step DOES. A screenshot of a broken
page goes to the agent that edits pages; a photographed invoice goes to the step
that extracts its figures; a logo goes to the step that will place it. You are
shown a small copy of each image for exactly this decision - look at it.

  User: "make the dashboard look like this" + mockup.png
    "build_page": {
      "agent_name": "eru_studio",
      "request": {
        "content": {"from": "user.content"},
        "params":  {"code": {"from": "user.params.code"}},
        "files":   {"from": "user.files"}
      }
    }

If the user attached something, some step must receive it. A plan that forwards
it nowhere has decided the attachment was pointless, which is not your decision
to make silently - if you truly believe no step needs to see it, say what was
attached in that step's "content" instead.

THE USER'S MESSAGE IS NOT YOURS TO REWRITE
When the whole plan is ONE agent step that reads nothing from another step, forward the
user's message exactly as they wrote it:

  "request": {"content": {"from": "user.content"}}

Do NOT restate it in your own words. You see one message; the agent sees the whole
conversation and the artifact it is working on. A message leans on what came before -
"it", "that one", "blue" meaning the thing named a moment ago - and a rewritten,
self-contained instruction throws that away. The agent then follows your paraphrase
instead of the user, and does far more (or far less) than was asked. This is checked.

You may still add "params" - those carry context, not intent.

PREFERRED — DESCRIBE THE REQUEST, DO NOT WRITE THE TEMPLATE
Set "request" on the step and the template is generated for you, correctly quoted
and balanced. Use this for every step unless you genuinely cannot express it:

  "request": {
    "content": "Build a page that lists financiers ...",        <- literal text
    "params":  {"code": {"from": "user.params.code"}}           <- optional
  }

A value is a literal (any JSON value) or one of:
  {"from": "user.content"}          the user's message
  {"from": "user.params.<key>"}     a param the caller sent
  {"from": "user.files"}            every file the user attached to this message
  {"from": "user.files.<n>"}        one of them, counting from 0
  {"from": "<step>.<field>"}        an earlier step's output field, by the
                                    "Output fields" name for that agent
  {"join": ["text ", {"from": "gen_sql.sql"}]}   several of those, concatenated

Examples:
  First step, passing the user's message straight through:
    "request": {"content": {"from": "user.content"}}
  A long instruction you are writing yourself - quotes and newlines are fine,
  nothing needs escaping:
    "request": {"content": "Build a dashboard.\n\nLAYOUT:\n- a chart on top ..."}
  Feeding a previous agent's output field in:
    "request": {"content": {"from": "generate_sql.sql"}}

Use "request" OR "transform_request", never both - a step that sets both keeps
the hand-written template.

FALLBACK — "transform_request" (only when "request" cannot express it)
It is a Go template that renders a JSON object of the form {"content":"..."}.
Build it with the dict function and ALWAYS pipe the result through stringify so
it renders as a JSON string (a bare dict renders as Go's map[...] and is NOT
valid JSON). Long content is exactly where this goes wrong - the parentheses stop
balancing - so prefer "request" whenever the body is more than a short phrase:

AGENT OUTPUT SHAPE — how to read a previous step's result:
Every agent RESPONDS with this envelope:
  {"actions":[{"action_name":"<that_agent_name>","action":{ ...output fields... }}]}
The useful values live in actions[0].action.<field>, where <field> is one of the
"Output fields" listed for that agent in the AVAILABLE AGENTS section above. So to
read a prior step's output, use:
  (index .ResVars.<prev_step>.Body.actions 0).action.<field>
NEVER use .ResVars.<prev_step>.Body.content for an agent step — the text is NOT there.

  First step (from the user's message — .Vars.OrgBody IS already {"content":...}):
    "request": {"content": {"from": "user.content"}}

  Chained step (feed prior agent's output field as the next input). Example: a
  generate_sql agent whose Output fields are "sql":
    "request": {"content": {"from": "generate_sql.sql"}}

RULE: any template that builds an object/dict (in transform_request OR
transform_response) MUST end with " | stringify" (or wrap in stringify) so the
final output is a JSON string. A bare dict renders as Go's map[...] and is invalid.

WRONG (these all break the agent):
  "{{dict \"content\" .Vars.OrgBody.content}}"                  → renders map[...], invalid JSON (not stringified)
  "{{.Vars.OrgBody.content}}"                                     → bare unquoted string, not an object
  "{{stringify (dict \"content\" .ResVars.generate_sql.Body.content)}}" → wrong path; agent output is in actions[0].action.<field>, not .content
  passing the whole .ResVars.prev.Body                         → carries unknown AgentMessage fields → rejected

To combine multiple agents' outputs into one input, concatenate the fields into a
single content string, e.g.:
  "request": {"content": {"join": ["sql: ", {"from": "generate_sql.sql"}, "\\nrows: ", {"from": "execute_sql.result"}]}}

============================================================
RULE #2a — FORWARD THE USER'S ANSWER BACK TO THE AGENT THAT ASKED
============================================================

An agent may stop and ask the user a question. The answer comes back to you, and
you hand it to that step when the plan resumes - but ONLY through the step's own
request body. A step that does not forward it re-runs with its question still
unanswered, and the agent asks again or guesses at what the user chose.

So for EVERY agent step whose agent can ask a question, forward the answer:

  "params": {"clarification_answers": {{stringify .Vars.OrgBody.params.clarification_answers}}}

Forward it on every call, not only after a question: the key renders as null when
nothing was asked, and the agent ignores that. You cannot know in advance whether
a question will be asked, so it goes on every step that could ask one.

============================================================
RULE #2b — PASS FETCHED DATA IN params.context, NOT PROSE
============================================================

When a step's job is to RENDER or ANALYSE data produced by an earlier step, check
that agent's "Params keys this agent READS" line in AVAILABLE AGENTS. If it lists
a "context" key, the rows MUST be passed as "params" -> "context", with "content"
carrying only the instruction - that key IS the agent's data channel:

  "request": {"content": {"from": "user.content"}, "params": {"context": {"from": "<data_step>.<output field>"}}}

Do NOT bury the rows inside the content string for these agents, and NEVER
paraphrase, sample, round or retype the data into the template — always pass the
upstream value itself, so the downstream agent renders real values instead of
inventing plausible ones.

If the plan produces data and then displays it, the displaying step MUST reference
the data step through .ResVars. A display step whose transform_request contains no
.ResVars reference to the data step is WRONG — it will render fabricated data.

If the receiving agent does NOT declare a "context" params key, pass the rows inside
"content" instead (e.g. via printf) - do not add a params key it does not read.
Some agents declare further data channels of their own ("entities", "apis", ...);
use exactly the keys listed for that agent, and no others.

============================================================
RULE #3 — TOOL STEP INPUT / OUTPUT (different from agents)
============================================================

A tool step's request body MUST be {"params": { ...action input fields... }} —
the action's "Input schema" fields go INSIDE a root "params" object (NOT the
{"content":...} agent envelope, and NOT at the root). Any other root key is
rejected. Build params with dict, wrap in another dict under "params", stringify.
Example — an execute_sql action whose Input schema needs {"query","project_id",
"vars"}, fed from a prior agent's sql output:
  "request": {"params": {"query": {"from": "generate_sql.sql"}, "project_id": "my_project", "vars": {}}}

Reading a tool's OUTPUT to chain forward:
  - If the tool shows an "Output schema": read .ResVars.<tool_step>.Body.<field> per that schema.
  - If the tool's Output is "dynamic": do NOT try to pick fields — pass the whole
    result as content to a downstream AGENT or to synthesis:
      "request": {"content": {"from_body": "<tool_step>"}}

NOTE: agent output lives in actions[0].action.<field>; tool output lives directly
in .ResVars.<tool_step>.Body (no actions envelope). Don't mix them up.

============================================================
EXECUTION MODEL
============================================================

Two rules:
  - SIBLING steps (same func_steps map) → run in PARALLEL
  - NESTED steps (func_steps inside a parent) → run SEQUENTIALLY after parent

Sequential (A then B): nest B inside A's func_steps.
Parallel (A and B): place both as siblings.

PARALLEL IS THE DEFAULT — MAXIMISE IT
The ONLY valid reason to nest a step is a real dependency. Nest B inside A ONLY if
at least one is true:
  - B's transform_request references .ResVars.A / .ReqVars.A (it consumes A's output)
  - B must not run unless A succeeded (A's side effect is a precondition of B)
  - B's loop_variable is derived from A's output
If none of those hold, B MUST be a sibling of A and run in parallel.

Do NOT nest for any of these reasons — they are all WRONG:
  - "the user didn't ask for parallel / didn't ask to optimise performance"
  - "sequential is simpler / easier to read / safer / more predictable"
  - "the steps are conceptually ordered" or "it reads naturally as step 1, step 2"
  - "parallelism is not needed for this small task"
Parallel execution is the platform default and needs NO user request to justify it.
Only an EXPLICIT user instruction (e.g. "run these one at a time", "do X only after
Y", "execute sequentially") may override a dependency-free parallel layout.

Method — derive the layout from data flow, not from narrative order:
  1. List the steps the task needs.
  2. For each step, list which other steps' outputs it actually reads.
  3. Steps with no unmet dependency → siblings at the same level (parallel).
  4. A step that depends on exactly one step → nest it inside that step.
  5. A step that depends on SEVERAL steps → nest it inside one of them and add
     "wait_for" naming a sibling it also needs (see the merge example below).
  6. Prefer the WIDEST layout that respects the dependencies — never serialise
     independent work into one chain.

Loops: when the iterations of a loop_variable are independent of each other, set
"loop_in_parallel": true. Use false only when iteration N depends on iteration N-1
or the target must be hit one call at a time.

If parent fails, nested children do NOT execute — no success-check conditions needed.

Example — WRONG (independent steps needlessly serialised):
{
  "sentiment_analyzer": {
    "agent_name": "sentiment_analyzer",
    "request": {"content": {"from": "user.content"}},
    "func_steps": {
      "topic_classifier": {
        "agent_name": "topic_classifier",
        "request": {"content": {"from": "user.content"}}
      }
    }
  }
}
topic_classifier never reads .ResVars.sentiment_analyzer — both read only
.Vars.OrgBody.content, so they MUST be siblings (see the parallel example below).

Example — sequential: extract data, then summarize it:
{
  "extractor": {
    "agent_name": "extractor",
    "request": {"content": {"from": "user.content"}},
    "func_steps": {
      "summarizer": {
        "agent_name": "summarizer",
        "request": {"content": {"from": "extractor.<extractor_output_field>"}}
      }
    }
  }
}

Example — parallel: two independent agents, then merge:
{
  "sentiment_analyzer": {
    "agent_name": "sentiment_analyzer",
    "request": {"content": {"from": "user.content"}}
  },
  "topic_classifier": {
    "agent_name": "topic_classifier",
    "request": {"content": {"from": "user.content"}}
  }
}

Example — parallel then sequential merge:
{
  "sentiment_analyzer": {
    "agent_name": "sentiment_analyzer",
    "request": {"content": {"from": "user.content"}}
  },
  "topic_classifier": {
    "agent_name": "topic_classifier",
    "request": {"content": {"from": "user.content"}},
    "func_steps": {
      "report_generator": {
        "agent_name": "report_generator",
        "wait_for": "sentiment_analyzer",
        "request": {"content": {"join": ["sentiment: ", {"from": "sentiment_analyzer.<field>"}, "\\ntopics: ", {"from": "topic_classifier.<field>"}]}}
      }
    }
  }
}

============================================================
TEMPLATE VARIABLES
============================================================

.Vars.OrgBody              — original user input (request body)
.Vars.Headers           — original request headers
.Vars.Params            — original query params
.Vars.Token             — auth token
.Vars.LoopVar           — current loop item
.Vars.LoopVars          — full loop array
.ResVars.<step_key>.Body — response FROM a completed step (an agent envelope: {"actions":[{"action_name":...,"action":{...}}]})
.ReqVars.<step_key>.Body — request sent TO a step
(index .ResVars.<step_key>.Body.actions 0).action.<field> — a prior agent's output value

Syntax:
  {{.Vars.OrgBody.content}}                                   — the user's input string
  {{(index .ResVars.<step>.Body.actions 0).action.<field>}} — a prior agent's output value
  {{stringify (dict "content" .Vars.OrgBody.content)}}        — wrap into the agent input object (JSON string)
  {{printf "%s / %s" .X .Y}}                                — combine strings before wrapping
  {{index .Vars.OrgBody "field-with-dash"}}

stringify (= JSON-encode) is MANDATORY whenever the template builds a dict/object,
in BOTH transform_request and transform_response. Output must be a JSON string.

Conditions:
  {{if eq .Vars.OrgBody.status "active"}}true{{else}}false{{end}}

============================================================
OPTIONAL STEP FIELDS
============================================================

Transforms (any object output MUST end with " | stringify"):
  "transform_request":  "<Go template → JSON string, e.g. {{dict ... | stringify}}>"
  "transform_response": "<Go template → JSON string, e.g. {{dict ... | stringify}}>"

Conditional:
  "condition": "<Go template → 'true' or 'false'>"
  "condition_fail_action": "ERROR | STOP | IGNORE"
  "condition_fail_message": "<Go template>"

Looping:
  "loop_variable": "<Go template → JSON array>"
  "loop_in_parallel": <bool>

Synchronization:
  "wait_for": "<sibling step key>" (ONLY for parallel siblings)

============================================================
CHECKLIST (verify before outputting)
============================================================

[ ] Every func_step key exactly matches agent_name, or tool_name_action for tool steps (Rule #1)
[ ] Agent steps: transform_request renders {"content":"..."} (Rule #2)
[ ] Tool steps: transform_request renders {"params": {<Input schema fields>}} (Rule #3)
[ ] EVERY dict/object in transform_request AND transform_response ends with " | stringify"
[ ] EVERY template parses: each "{{" has a matching "}}", every "(" a matching ")", and no stray brace or parenthesis is left at the end of an action
[ ] No step passes a bare string or the raw .Vars.OrgBody / whole AgentMessage
[ ] func_category_name and func_group_name are set (snake_case)
[ ] Each step uses ONLY (agent_name) OR (tool_name+tool_action), with no tenant_id (no query/function/api)
[ ] Sequential steps are NESTED, parallel steps are SIBLINGS
[ ] EVERY nested step is justified by a real dependency on its parent (reads its
    .ResVars, needs its success, or loops over its output) — otherwise it is moved
    up to be a sibling and run in parallel
[ ] Independent steps are NOT serialised for simplicity/readability; parallel is
    the default and only an explicit user instruction may force sequential
[ ] loop_in_parallel is true wherever the loop iterations are independent
[ ] transform_request passes data correctly between steps
[ ] Only agents/tools from the AVAILABLE lists are used
[ ] wait_for only references sibling step keys, not nested ones
[ ] EVERY .ResVars/.ReqVars reference names an EXACT func_steps key of an earlier step (not an agent name, tool name or invented short form)
[ ] A step that renders or analyses earlier data receives it via params.context and references that step through .ResVars (Rule #2b)
[ ] EVERY func_step key is unique across the ENTIRE plan (all branches, all depths)
[ ] EVERY step has a non-empty transform_request
[ ] EVERY agent step's transform_request renders a "content" key
[ ] EVERY params key used on an agent step appears in that agent's "Params keys this agent READS" list
[ ] EVERY tool step's transform_request renders a root "params" object containing all of that action's Required params
[ ] EVERY field read off an agent response exists in that agent's Output schema

`
	return systemPrompt
}

func (oa *OrchestratorAgent) buildAgentDescriptions() string {
	if len(oa.discoveredAgents) == 0 {
		return "No agents configured."
	}

	var sb strings.Builder
	for _, ad := range oa.discoveredAgents {
		sb.WriteString(fmt.Sprintf("Agent: %s\n", ad.AgentName))
		sb.WriteString(fmt.Sprintf("  Type: %s   Tenant: %s\n", ad.AgentType, ad.TenantId))
		sb.WriteString(fmt.Sprintf("  Description: %s\n", ad.Description))
		if len(ad.Tools) > 0 {
			sb.WriteString(fmt.Sprintf("  Can call these tools itself: %s (do NOT duplicate them as separate steps)\n", strings.Join(ad.Tools, ", ")))
		}
		if len(ad.InternalCapabilities) > 0 {
			sb.WriteString(fmt.Sprintf("  Looks these up ITSELF - do not add a step to fetch them, and never hand-write SQL for them: %s\n",
				strings.Join(ad.InternalCapabilities, "; ")))
		}
		if strings.TrimSpace(ad.PlanningNote) != "" {
			sb.WriteString(fmt.Sprintf("  HOW TO PLAN WITH IT: %s\n", ad.PlanningNote))
		}
		if guardrail := summariseGuardrail(ad.Guardrail); guardrail != "" {
			sb.WriteString(fmt.Sprintf("  Scope limits: %s\n", guardrail))
		}
		if ad.IsOrchestrator {
			sb.WriteString("  This agent is itself an orchestrator - it plans and runs its own sub-steps.\n")
		}
		if ad.SupportsClarification {
			sb.WriteString("  May ask the user a clarifying question, which pauses the plan until answered.\n")
		}
		if keys := ad.ParamKeys(); len(keys) > 0 {
			sb.WriteString(fmt.Sprintf("  Params keys this agent READS: %s - any other params key is silently discarded, so put that information in content instead\n", strings.Join(keys, ", ")))
			sb.WriteString(fmt.Sprintf("  Params schema: %s\n", renderSchemaJSON(ad.ParamsSchema(), agentSchemaRenderLimit)))
		} else {
			sb.WriteString("  Params keys this agent READS: none - everything it needs must be in content\n")
		}
		if ad.HasStructuredOutput() {
			fields := outputFieldNames(ad.OutputSchema)
			sb.WriteString(fmt.Sprintf("  Response: structured - read it as (index .ResVars.%s.Body.actions 0).action.<field>\n", ad.AgentName))
			if len(fields) > 0 {
				sb.WriteString(fmt.Sprintf("  Output fields (in actions[0].action): %s\n", strings.Join(fields, ", ")))
			}
			sb.WriteString(fmt.Sprintf("  Output schema: %s\n", renderSchemaJSON(ad.OutputSchema, agentSchemaRenderLimit)))
		} else if ad.IsOrchestrator {
			sb.WriteString(fmt.Sprintf("  Response: one or more actions produced by its own sub-steps - the field names depend on the plan it builds, so do NOT index into them. To chain it, pass {{stringify .ResVars.%s.Body.actions}}\n", ad.AgentName))
		} else {
			sb.WriteString(fmt.Sprintf("  Response: free text - it declares NO output fields. Plain text lands at (index .ResVars.%s.Body.actions 0).action.output ; to chain it safely pass {{stringify (index .ResVars.%s.Body.actions 0).action}}\n", ad.AgentName, ad.AgentName))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

const (
	agentSchemaRenderLimit = 3000
	toolSchemaRenderLimit  = 3000
	guardrailSummaryLimit  = 240
)

func summariseGuardrail(guardrail string) string {
	guardrail = strings.Join(strings.Fields(guardrail), " ")
	if guardrail == "" {
		return ""
	}
	if len(guardrail) <= guardrailSummaryLimit {
		return guardrail
	}
	return guardrail[:guardrailSummaryLimit] + " ..."
}

// renderSchemaJSON inlines a schema into the planning prompt, shrinking it by
// dropping nesting levels when the full schema would swamp the prompt (page and
// FuncGroup schemas run to tens of kilobytes).
func renderSchemaJSON(schema eru_models.JSONSchema, limit int) string {
	if schema.Type == "" && len(schema.Properties) == 0 {
		return "{}"
	}
	full, err := json.Marshal(schema)
	if err != nil {
		return "{}"
	}
	if len(full) <= limit {
		return string(full)
	}
	for depth := 3; depth >= 1; depth-- {
		trimmed, terr := json.Marshal(trimSchemaDepth(schema, depth))
		if terr == nil && len(trimmed) <= limit {
			return string(trimmed) + "   (nested detail omitted - follow the field descriptions)"
		}
	}
	return fmt.Sprint("{\"type\":\"", schema.Type, "\",\"top_level_fields\":[", strings.Join(outputFieldNames(schema), ", "), "]}   (schema too large to inline)")
}

func trimSchemaDepth(schema eru_models.JSONSchema, depth int) eru_models.JSONSchema {
	trimmed := eru_models.JSONSchema{
		Type:        schema.Type,
		Description: schema.Description,
		Format:      schema.Format,
		Enum:        schema.Enum,
		Required:    schema.Required,
	}
	if depth <= 0 {
		return trimmed
	}
	if len(schema.Properties) > 0 {
		trimmed.Properties = make(map[string]eru_models.JSONSchema, len(schema.Properties))
		for name, prop := range schema.Properties {
			trimmed.Properties[name] = trimSchemaDepth(prop, depth-1)
		}
	}
	if schema.Items != nil {
		items := trimSchemaDepth(*schema.Items, depth-1)
		trimmed.Items = &items
	}
	return trimmed
}

func outputFieldNames(schema eru_models.JSONSchema) []string {
	fields := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	return fields
}

// collectClientOutputs returns the structured outputs to forward to the client
// as separate actions. If ClientOutputAgents is set, it forwards each named
// step's output; otherwise it falls back to the terminal step's output.
func (oa *OrchestratorAgent) collectClientOutputs(ctx context.Context, plan map[string]interface{}, funcVarsMap map[string]functions.FuncTemplateVars, executionResult map[string]interface{}) []agents.AgentOutputAction {
	var out []agents.AgentOutputAction
	if len(oa.ClientOutputAgents) > 0 {
		seen := make(map[string]bool)
		for _, name := range oa.ClientOutputAgents {
			if seen[name] {
				continue
			}
			body, found := stepOutputBody(funcVarsMap, name)
			if !found {
				logs.WithContext(ctx).Info(fmt.Sprint("collectClientOutputs - no output found for step ", name))
				continue
			}
			outputs := extractStructuredOutputs(body)
			if len(outputs) > 1 {
				logs.WithContext(ctx).Info(fmt.Sprint("collectClientOutputs - step ", name, " returned ", len(outputs), " outputs, forwarding all"))
			}
			for _, action := range outputs {
				out = append(out, agents.AgentOutputAction{
					ActionType: agents.ActionTypeData,
					ActionName: name,
					Action:     action,
				})
			}
			seen[name] = true
		}
		return out
	}
	if len(executionResult) > 0 {
		actionName := terminalStepName(ctx, plan)
		if actionName == "" {
			actionName = oa.AgentName
		}
		outputs := extractStructuredOutputs(executionResult)
		if len(outputs) > 1 {
			logs.WithContext(ctx).Info(fmt.Sprint("collectClientOutputs - terminal step ", actionName, " returned ", len(outputs), " outputs, forwarding all"))
		}
		for _, action := range outputs {
			out = append(out, agents.AgentOutputAction{
				ActionType: agents.ActionTypeData,
				ActionName: actionName,
				Action:     action,
			})
		}
	}
	return out
}

// stepOutputBody finds a completed step's response body in the per-step vars.
func stepOutputBody(funcVarsMap map[string]functions.FuncTemplateVars, stepName string) (interface{}, bool) {
	for _, fv := range funcVarsMap {
		if fv.ResVars == nil {
			continue
		}
		if tv, ok := fv.ResVars[stepName]; ok && tv != nil && tv.Body != nil {
			return tv.Body, true
		}
	}
	return nil, false
}

// extractStructuredOutputs unwraps an agent envelope
// ({"actions":[{"action":{...}}, ...]}) to the inner action objects; for tool
// results (no actions envelope) it returns the body as the only output.
//
// Every action is returned, not just the first. A step can legitimately answer
// with several: the Eru Studio agent returns the page it was asked about plus one
// action per nested page it had to create - a row template, a side panel, a board
// card - and each of those is a separate unit the user saves. Keeping only
// actions[0] built and validated those pages and then dropped them one hop
// before the client.
func extractStructuredOutputs(body interface{}) []map[string]interface{} {
	m, ok := body.(map[string]interface{})
	if !ok {
		return []map[string]interface{}{{"output": body}}
	}
	actionsI, hasActions := m["actions"]
	if !hasActions {
		return []map[string]interface{}{m}
	}
	actions, ok := actionsI.([]interface{})
	if !ok {
		return []map[string]interface{}{m}
	}

	out := make([]map[string]interface{}, 0, len(actions))
	for _, raw := range actions {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if action, ok := entry["action"].(map[string]interface{}); ok {
			out = append(out, action)
		}
	}
	if len(out) == 0 {
		// An envelope whose actions carry nothing usable: forward the body rather
		// than forwarding nothing, so the failure is visible to the client.
		return []map[string]interface{}{m}
	}
	return out
}

func (oa *OrchestratorAgent) buildToolDescriptions() string {
	if len(oa.discoveredTools) == 0 {
		return "No tools configured."
	}

	var sb strings.Builder
	for _, dt := range oa.discoveredTools {
		sb.WriteString(fmt.Sprintf("Tool: %s  Action: %s\n", dt.ToolName, dt.ActionName))
		sb.WriteString(fmt.Sprintf("  Tenant: %s\n", dt.TenantId))
		sb.WriteString(fmt.Sprintf("  Description: %s\n", dt.Description))
		sb.WriteString(fmt.Sprintf("  Input schema (goes inside the root \"params\" object of transform_request): %s\n", renderSchemaJSON(dt.InputSchema, toolSchemaRenderLimit)))
		if len(dt.InputSchema.Required) > 0 {
			sb.WriteString(fmt.Sprintf("  Required params: %s\n", strings.Join(dt.InputSchema.Required, ", ")))
		}
		if dt.OutputSchema.Type != "" {
			sb.WriteString(fmt.Sprintf("  Output schema (result at .ResVars.<step>.Body): %s\n", renderSchemaJSON(dt.OutputSchema, toolSchemaRenderLimit)))
		} else {
			sb.WriteString("  Output: dynamic - to chain, pass {{stringify .ResVars.<step>.Body}} to a downstream agent or synthesis\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func init() {
	agents.RegisterAgentType("ORCHESTRATOR", func() agents.AgentI { return new(OrchestratorAgent) })
}
