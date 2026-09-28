package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agents "github.com/eru-os/eru/eru-ai/agents"
	models "github.com/eru-os/eru/eru-ai/models"
	"github.com/eru-os/eru/eru-ai/tools"
	utility "github.com/eru-os/eru/eru-ai/tools/utility"
	functions "github.com/eru-os/eru/eru-functions/functions"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
)

// A sub-agent's question is not automatically a question for the user.
//
// When a step pauses to ask something, the orchestrator used to hand it straight
// on. Much of the time that is the orchestrator making its own homework the
// user's problem: "which entity holds transactions", "what is the field code for
// rating", "what was that label before I changed it" all have answers, and they
// are in the run's own results, in the conversation, or one read-only lookup
// away. Asking for them is slower than finding them and teaches the user that
// the assistant does not pay attention.
//
// What must still be asked is anything the system cannot know. "Chart or table",
// "include archived rows", "which of these three layouts" are not facts to be
// looked up; they are the user's intent, and a confident guess at intent is
// worse than a question, because a question is visible and a wrong assumption
// usually is not. The costs are not symmetric: an unnecessary question costs a
// few seconds, a silent wrong assumption costs work that has to be found and
// undone later.
//
// So the resolver answers questions of fact, from evidence it can cite, and
// refers everything else - including anything it is merely unsure about - to the
// user. It runs once. A resolution that would itself need a question is a
// question for the user.

// resolvedAnswer is one question the orchestrator answered for itself.
type resolvedAnswer struct {
	QuestionId string `json:"question_id"`
	Answer     string `json:"answer"`
	Source     string `json:"source"`
}

// assumptionLine is how a self-answered question is reported back.
func (r resolvedAnswer) assumptionLine(question string) string {
	q := strings.TrimSpace(question)
	if q == "" {
		q = r.QuestionId
	}
	if r.Source == "" {
		return fmt.Sprintf("- %s — assumed **%s**", q, r.Answer)
	}
	return fmt.Sprintf("- %s — assumed **%s** (%s)", q, r.Answer, r.Source)
}

// resolutionOutcome is what one pass of the resolver produced.
type resolutionOutcome struct {
	Answers     []agents.ClarificationAnswer
	Assumptions []string
	Remaining   agents.ClarificationRequest
	Traces      []models.StepTrace
}

func resolverSchema() eru_models.JSONSchema {
	resolution := eru_models.JSONSchema{
		Type: "object",
		Properties: map[string]eru_models.JSONSchema{
			"question_id": {Type: "string", Description: "The id of the question being resolved, exactly as given."},
			"answered": {
				Type: "boolean",
				Description: "true only if this is a question of fact AND you found the answer in the evidence or a lookup. " +
					"false for anything about the user's intent, preference or taste, and for anything you are not certain of.",
			},
			"answer": {Type: "string", Description: "The answer, phrased as the sub-agent expects it. Empty when answered is false."},
			"source": {
				Type: "string",
				Description: "Where the answer came from, in a few words a person can check - the step result, the earlier turn, " +
					"or the lookup you ran. Empty when answered is false.",
			},
			"reason": {Type: "string", Description: "When answered is false, one short line on why this has to go to the user."},
		},
		Required: []string{"question_id", "answered"},
	}
	return eru_models.JSONSchema{
		Type: "object",
		Properties: map[string]eru_models.JSONSchema{
			"resolutions": {
				Type:        "array",
				Description: "One entry per question, in the order given.",
				Items:       &resolution,
			},
		},
		Required: []string{"resolutions"},
	}
}

const resolverSystemPrompt = `You are deciding which of a set of questions can be answered from what is already known, and which genuinely need the user.

Answer a question yourself ONLY when both hold:
  1. It is a question of FACT about this system or this conversation - the name of an entity or field, a value that was used earlier, whether something exists, what a page currently contains.
  2. You can point at where the answer came from: a step result below, something earlier in this conversation, or a lookup you ran with the tools available.

Send a question to the user when ANY of these hold:
  - It asks what the user WANTS: a choice of layout, chart type, wording, styling, which records to include, whether to proceed.
  - It is a matter of taste or judgement rather than fact.
  - You cannot find the answer, or you found something that only resembles an answer.
  - You are not certain.

Being unsure is a perfectly good outcome; say so and let the user answer. A question costs them a few seconds. A confident wrong answer costs them work they have to find and undo, and they may not notice it happened at all. Never invent a plausible value to avoid asking, and never answer from what would be a reasonable default - a default is a preference, not a fact.

Use the lookup tools when they would settle a factual question. Then call structured_output with one resolution per question.`

// resolveClarifications tries to answer a sub-agent's questions from the run's
// own results, the conversation, and read-only lookups, and reports what is left
// for the user.
func (oa *OrchestratorAgent) resolveClarifications(ctx context.Context, req agents.ClarificationRequest, resVars map[string]*functions.TemplateVars, conversation *agents.Conversation, projectId string, tenantId string) resolutionOutcome {
	outcome := resolutionOutcome{Remaining: req}
	if len(req.Questions) == 0 {
		return outcome
	}
	if oa.Model == nil {
		return outcome
	}

	research := oa.researchTools(ctx)
	evidence := oa.resolutionEvidence(ctx, resVars, conversation)

	toolsMap := map[string]tools.Tooling{}
	schema := resolverSchema()
	outputTool := &utility.StructuredOutputTool{}
	outputTool.SetAttribute(ctx, "output_schema", schema)
	outputTool.SetAttribute(ctx, "parameters", schema)
	outputTool.SetAttribute(ctx, "description", "Report, for each question, whether you answered it and what the answer is.")
	outputTool.SetAttribute(ctx, "tool_name", "structured_output")
	outputTool.SetAttribute(ctx, "tool_type", "STRUCTURED_OUTPUT")
	outputTool.SetToolAction("structured_output")
	toolsMap["structured_output"] = outputTool
	for name, tool := range research {
		if _, taken := toolsMap[name]; !taken {
			toolsMap[name] = tool
		}
	}

	prompt := resolverSystemPrompt
	if guidance := researchGuidance(research); guidance != "" {
		prompt = prompt + "\n\n" + guidance
	}

	chatRequest := models.ChatRequest{Messages: []models.Message{{
		Role:    "user",
		Content: evidence + "\n\nQuestions raised:\n" + renderQuestions(req.Questions),
	}}}

	toolExecutor := func(ctx context.Context, toolName string, input map[string]interface{}) (map[string]interface{}, error) {
		if result, err, handled := executeResearchTool(ctx, research, toolName, projectId, tenantId, input); handled {
			return result, err
		}
		return nil, fmt.Errorf("tool %s is not available while resolving a question", toolName)
	}

	// Deliberately not streamed: this is the orchestrator checking its own
	// homework, not work the user asked to watch.
	response, traces, err := oa.Model.RunToolLoop(ctx, chatRequest, toolsMap, models.StaticAgentPrompt(prompt), resolverMaxIterations, oa.ThinkingBudget, toolExecutor)
	outcome.Traces = traces
	if err != nil {
		logs.WithContext(ctx).Info(fmt.Sprintf("clarification resolver failed, asking the user instead: %v", err))
		return outcome
	}
	if response.TerminalTool != models.TerminalToolStructuredOutput {
		logs.WithContext(ctx).Info("clarification resolver produced no resolutions, asking the user instead")
		return outcome
	}

	var parsed struct {
		Resolutions []struct {
			QuestionId string `json:"question_id"`
			Answered   bool   `json:"answered"`
			Answer     string `json:"answer"`
			Source     string `json:"source"`
			Reason     string `json:"reason"`
		} `json:"resolutions"`
	}
	if err := json.Unmarshal([]byte(response.Content), &parsed); err != nil {
		logs.WithContext(ctx).Info(fmt.Sprintf("clarification resolver output was unreadable, asking the user instead: %v", err))
		return outcome
	}

	byId := map[string]string{}
	for _, q := range req.Questions {
		byId[q.Id] = q.Question
	}

	answered := map[string]resolvedAnswer{}
	for _, r := range parsed.Resolutions {
		if !r.Answered {
			continue
		}
		// An answer with nothing behind it is a guess wearing a fact's clothes.
		if strings.TrimSpace(r.Answer) == "" || strings.TrimSpace(r.Source) == "" {
			logs.WithContext(ctx).Info(fmt.Sprintf("clarification %s claimed an answer with no value or no source - asking the user", r.QuestionId))
			continue
		}
		if _, known := byId[r.QuestionId]; !known {
			continue
		}
		answered[r.QuestionId] = resolvedAnswer{QuestionId: r.QuestionId, Answer: r.Answer, Source: r.Source}
	}

	var remaining []agents.ClarificationQuestion
	for _, q := range req.Questions {
		if _, ok := answered[q.Id]; !ok {
			remaining = append(remaining, q)
		}
	}

	ids := make([]string, 0, len(answered))
	for id := range answered {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := answered[id]
		outcome.Answers = append(outcome.Answers, agents.ClarificationAnswer{
			QuestionId: r.QuestionId,
			FreeText:   r.Answer,
		})
		outcome.Assumptions = append(outcome.Assumptions, r.assumptionLine(byId[id]))
	}

	outcome.Remaining = agents.ClarificationRequest{Prompt: req.Prompt, Questions: remaining}
	logs.WithContext(ctx).Info(fmt.Sprintf("clarification resolver answered %d of %d question(s); %d left for the user",
		len(outcome.Answers), len(req.Questions), len(remaining)))
	return outcome
}

// resolverMaxIterations bounds the lookups one resolution pass may make. This is
// a side quest, not the work: a resolver that goes hunting costs more than the
// question it is trying to avoid.
const resolverMaxIterations = 4

func renderQuestions(questions []agents.ClarificationQuestion) string {
	var b strings.Builder
	for _, q := range questions {
		fmt.Fprintf(&b, "- id: %s\n  question: %s\n", q.Id, q.Question)
		if len(q.Options) > 0 {
			labels := make([]string, 0, len(q.Options))
			for _, o := range q.Options {
				labels = append(labels, o.Label)
			}
			fmt.Fprintf(&b, "  offered options: %s\n", strings.Join(labels, " | "))
		}
	}
	return b.String()
}

// resolutionEvidence is what the run already knows, in the order it is cheapest
// to consult: this run's own step results first, then the conversation. Both are
// free - they are already in hand - and between them they answer most questions
// that are worth answering at all.
func (oa *OrchestratorAgent) resolutionEvidence(ctx context.Context, resVars map[string]*functions.TemplateVars, conversation *agents.Conversation) string {
	var b strings.Builder
	b.WriteString("Evidence already available.\n\n")

	b.WriteString("Results of the steps that have run so far in this request:\n")
	if len(resVars) == 0 {
		b.WriteString("(none)\n")
	} else {
		names := make([]string, 0, len(resVars))
		for name := range resVars {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			body, err := json.Marshal(resVars[name])
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "- %s: %s\n", name, truncate(string(body), resolverEvidenceBudget))
		}
	}

	if note := agents.ConversationJournal(ctx, oa.ConversationManager, conversation); note != "" {
		b.WriteString("\nWhat has already been changed in this conversation:\n")
		b.WriteString(note)
		b.WriteString("\n")
	}

	if conversation != nil && len(conversation.Messages) > 0 {
		b.WriteString("\nWhat has been said in this conversation, oldest first:\n")
		for _, m := range conversation.Messages {
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s: %s\n", m.Role, truncate(m.Content, resolverMessageBudget))
		}
	}

	return b.String()
}

const (
	resolverEvidenceBudget = 4000
	resolverMessageBudget  = 600
)

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + " ...(truncated)"
}

// assumptionsNote is the line added to the answer so a self-resolved question is
// visible. A question answered on the user's behalf that the user never sees is
// how an assistant stops being trustworthy.
func assumptionsNote(assumptions []string) string {
	if len(assumptions) == 0 {
		return ""
	}
	heading := "I answered this without asking you:"
	if len(assumptions) > 1 {
		heading = "I answered these without asking you:"
	}
	return heading + "\n" + strings.Join(assumptions, "\n") + "\n\nTell me if any of that is wrong and I will redo it."
}

// noteAssumptions appends the assumptions to the answer the user reads.
func noteAssumptions(synthesisResult map[string]interface{}, assumptions []string) {
	note := assumptionsNote(assumptions)
	if note == "" || synthesisResult == nil {
		return
	}
	existing, _ := synthesisResult["response"].(string)
	if strings.TrimSpace(existing) == "" {
		synthesisResult["response"] = note
		return
	}
	synthesisResult["response"] = existing + "\n\n" + note
}
