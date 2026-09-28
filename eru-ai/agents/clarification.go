package agents

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	models "github.com/eru-os/eru/eru-ai/models"
	eru_models "github.com/eru-os/eru/eru-models"
)

const (
	ClarificationAnswersParamKey = "clarification_answers"
	ResumeContextParamKey        = "resume_context"
)

// ClarificationAnswersParamSchema describes the answers a caller sends back
// after an agent asked a question. Every clarification-capable agent reads this
// key through the message plumbing, so it belongs to the request contract of all
// of them rather than to any one agent's declared params.
func ClarificationAnswersParamSchema() eru_models.JSONSchema {
	return eru_models.JSONSchema{
		Type: "array",
		Description: "The user's answers to a question this agent asked earlier. Forward it verbatim on every call: " +
			"it is null when nothing was asked, which the agent ignores. Dropping it makes the agent re-ask or guess.",
		Items: &eru_models.JSONSchema{
			Type: "object",
			Properties: map[string]eru_models.JSONSchema{
				"question_id": {Type: "string", Description: "The id of the question being answered."},
				"selected":    {Type: "array", Description: "The option values the user picked.", Items: &eru_models.JSONSchema{Type: "string"}},
				"answer":      {Type: "string", Description: "A free-text answer, when the question was not a choice."},
			},
			Required: []string{"question_id"},
		},
	}
}

// maxResumeContextChars caps the transcript carried across a clarification. It
// is generous enough for a page's worth of lookups and small enough that a long
// tool result cannot crowd out the answer the user just gave.
const maxResumeContextChars = 6000

// BuildResumeContext summarises the work an agent had already done when it
// stopped to ask a question: what it was thinking, which tools it called and
// what they returned.
//
// Without it, answering a clarification throws that work away - the model comes
// back with only the question and the answer, and re-runs every lookup it had
// already made. The transcript is text rather than replayed tool blocks because
// it has to survive being stored in the conversation and read back by whichever
// model serves the next turn.
func BuildResumeContext(traces []models.StepTrace) string {
	if len(traces) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- WORK ALREADY DONE BEFORE YOU ASKED (do not repeat these lookups) ---\n")
	wrote := false
	for _, trace := range traces {
		if trace.ToolName == "" || trace.ToolName == models.TerminalToolAskUser {
			continue
		}
		fmt.Fprintf(&b, "\nTool: %s\n", trace.ToolName)
		if len(trace.ToolInput) > 0 {
			fmt.Fprintf(&b, "  called with: %s\n", compactJSON(trace.ToolInput))
		}
		if len(trace.ToolResult) > 0 {
			fmt.Fprintf(&b, "  returned: %s\n", compactJSON(trace.ToolResult))
		}
		wrote = true
	}
	if !wrote {
		return ""
	}
	b.WriteString("--- END WORK ALREADY DONE ---")

	text := b.String()
	if len(text) > maxResumeContextChars {
		text = text[:maxResumeContextChars] + "\n... (truncated; call a tool again only if you actually need more)\n--- END WORK ALREADY DONE ---"
	}
	return text
}

func compactJSON(value map[string]interface{}) string {
	// Sorted keys keep the transcript stable, so two identical resumes read the
	// same and prompt caching is not defeated by map iteration order.
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make([]string, 0, len(keys))
	for _, key := range keys {
		encoded, err := json.Marshal(value[key])
		if err != nil {
			continue
		}
		ordered = append(ordered, fmt.Sprintf("%q: %s", key, string(encoded)))
	}
	return "{" + strings.Join(ordered, ", ") + "}"
}

// ResumeContextFrom reads the transcript a question action carried, if any.
func ResumeContextFrom(action map[string]interface{}) string {
	if action == nil {
		return ""
	}
	text, _ := action[ResumeContextParamKey].(string)
	return text
}

// QuestionAction returns the first action on the message whose ActionType is
// "question", if any.
func (m AgentMessage) QuestionAction() (AgentOutputAction, bool) {
	for _, a := range m.Actions {
		if a.ActionType == ActionTypeQuestion {
			return a, true
		}
	}
	return AgentOutputAction{}, false
}

// ClarificationAnswers extracts structured answers sent back by the UI via
// AgentMessage.Params[ClarificationAnswersParamKey].
func (m AgentMessage) ClarificationAnswers() ([]ClarificationAnswer, bool) {
	if m.Params == nil {
		return nil, false
	}
	raw, ok := m.Params[ClarificationAnswersParamKey]
	if !ok {
		return nil, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var answers []ClarificationAnswer
	if err := json.Unmarshal(b, &answers); err != nil {
		return nil, false
	}
	if len(answers) == 0 {
		return nil, false
	}
	return answers, true
}

// PendingQuestion scans loaded history (most recent first) and returns the last
// assistant message that asked a clarification question. Callers use this to
// detect that an incoming message is an answer to a pending question.
func PendingQuestion(conversation *Conversation) (AgentMessage, AgentOutputAction, bool) {
	for i := len(conversation.Messages) - 1; i >= 0; i-- {
		msg := conversation.Messages[i]
		if action, ok := msg.QuestionAction(); ok {
			return msg, action, true
		}
		if msg.Role == "user" {
			break
		}
	}
	return AgentMessage{}, AgentOutputAction{}, false
}

// DefaultFreeTextLabel names the free-text box when the asker did not, so a
// client always has something to render it with.
const DefaultFreeTextLabel = "Something else - let me describe it"

// Normalize makes a request answerable.
//
// A question with no options and no free text is a dead end: the client renders
// no choices and no box, so there is nothing for the user to click or type and
// the wizard can never be submitted. The asker cannot judge whether its options
// cover the situation - the case it did not think of is exactly the one the user
// needs to state - so free text is always on, and a question that offered no
// options at all becomes a plain text prompt.
//
// The ask_user tool applies the same rule to the questions an agent asks through
// it. This is the other door: a clarification the orchestrator raises itself
// comes back through ParseClarificationRequest without ever passing that tool.
func (req *ClarificationRequest) Normalize() {
	if req == nil {
		return
	}
	for i := range req.Questions {
		q := &req.Questions[i]
		if q.Id == "" {
			q.Id = fmt.Sprintf("q%d", i+1)
		}
		q.AllowFreeText = true
		if q.FreeTextLabel == "" {
			q.FreeTextLabel = DefaultFreeTextLabel
		}
	}
}

// ParseClarificationRequest converts the action payload of a question action
// back into a typed ClarificationRequest.
func ParseClarificationRequest(action map[string]interface{}) (ClarificationRequest, error) {
	var req ClarificationRequest
	b, err := json.Marshal(action)
	if err != nil {
		return req, err
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return req, err
	}
	req.Normalize()
	return req, nil
}

// ToAction builds the question AgentOutputAction carrying this request.
func (req ClarificationRequest) ToAction(agentName string) AgentOutputAction {
	action := map[string]interface{}{}
	b, _ := json.Marshal(req)
	_ = json.Unmarshal(b, &action)
	return AgentOutputAction{
		ActionType: ActionTypeQuestion,
		ActionName: agentName,
		Action:     action,
	}
}

// AttachResumeContext stores the work-so-far transcript on a question action, so
// the turn that answers the question can hand it back to the model.
func AttachResumeContext(action map[string]interface{}, resumeContext string) {
	if action == nil || strings.TrimSpace(resumeContext) == "" {
		return
	}
	action[ResumeContextParamKey] = resumeContext
}

// FormatAnswersForModel renders the asked questions and the user's answers into
// a single text block so the model sees, on resume, what it asked and what was
// chosen. When the original request is empty (questions not available) it falls
// back to rendering the answers alone.

func FormatAnswersForModel(req ClarificationRequest, answers []ClarificationAnswer) string {
	questionById := make(map[string]ClarificationQuestion, len(req.Questions))
	for _, q := range req.Questions {
		questionById[q.Id] = q
	}
	// An answer may come back with the id namespaced by whoever relayed the
	// question - an orchestrator prefixes it with the step that asked. Matching
	// on the last segment keeps the question text attached to its answer, so the
	// model reads "Q: ... A: ..." instead of a bare id it has to guess about.
	lookup := func(id string) (ClarificationQuestion, bool) {
		if q, ok := questionById[id]; ok {
			return q, true
		}
		if idx := strings.LastIndex(id, "::"); idx >= 0 {
			q, ok := questionById[id[idx+2:]]
			return q, ok
		}
		return ClarificationQuestion{}, false
	}

	var sb strings.Builder
	sb.WriteString("The user answered the clarification questions:\n")
	for _, ans := range answers {
		if q, ok := lookup(ans.QuestionId); ok {
			sb.WriteString(fmt.Sprintf("- Q: %s\n", q.Question))
		} else {
			sb.WriteString(fmt.Sprintf("- Q (%s):\n", ans.QuestionId))
		}
		var parts []string
		if len(ans.Selected) > 0 {
			parts = append(parts, strings.Join(ans.Selected, ", "))
		}
		if strings.TrimSpace(ans.FreeText) != "" {
			parts = append(parts, ans.FreeText)
		}
		answerText := strings.Join(parts, " | ")
		if answerText == "" {
			answerText = "(no answer)"
		}
		sb.WriteString(fmt.Sprintf("  A: %s\n", answerText))
	}
	return strings.TrimRight(sb.String(), "\n")
}
