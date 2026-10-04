package agents

import (
	"encoding/json"
	"fmt"
	"regexp"
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
				"free_text":   {Type: "string", Description: "A free-text answer, when the question was not a choice."},
				"secret_ref":  {Type: "string", Description: "For a secret question: the $SECRET_<name> reference of the secret the user saved."},
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
		if q.IsSecret() {
			NormalizeSecretQuestion(q)
			continue
		}
		if q.InputType == InputTypeOAuth {
			NormalizeOAuthQuestion(q)
			continue
		}
		if q.FreeTextLabel == "" {
			q.FreeTextLabel = DefaultFreeTextLabel
		}
	}
}

const (
	InputTypeSecret = "secret"
	// InputTypeOAuth asks the user to authorize a saved OAuth connection. The
	// client shows an Authorize button for tool_name; the answer is only the
	// user's confirmation that they finished.
	InputTypeOAuth       = "oauth"
	OAuthAuthorizedValue = "authorized"
	// SecretFreeTextLabel names the box of a secret question: the user should
	// know the value is not going to the assistant.
	SecretFreeTextLabel = "Saved as a secret - the assistant never sees it"
)

var secretNameChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func (q ClarificationQuestion) IsSecret() bool {
	return q.InputType == InputTypeSecret
}

// NormalizeSecretQuestion turns a question into a plain secret prompt: no
// options to pick (a credential is never one of a list) and a secret name the
// client can save under, made safe for a $SECRET_ reference.
func NormalizeSecretQuestion(q *ClarificationQuestion) {
	q.Options = nil
	q.MultiSelect = false
	q.AllowFreeText = true
	q.FreeTextLabel = SecretFreeTextLabel
	name := strings.Trim(secretNameChars.ReplaceAllString(q.SecretName, "_"), "_")
	if name == "" {
		name = strings.Trim(secretNameChars.ReplaceAllString(q.Id, "_"), "_")
	}
	q.SecretName = strings.ToLower(name)
}

// NormalizeOAuthQuestion leaves one answer - the user confirming they
// authorized - since there is nothing to type.
func NormalizeOAuthQuestion(q *ClarificationQuestion) {
	q.Options = []QuestionOption{{Value: OAuthAuthorizedValue, Label: "I've authorized it"}}
	q.MultiSelect = false
	q.AllowFreeText = false
	q.FreeTextLabel = ""
	q.Required = true
}

func lookupQuestion(byId map[string]ClarificationQuestion, id string) (ClarificationQuestion, bool) {
	if q, ok := byId[id]; ok {
		return q, true
	}
	if idx := strings.LastIndex(id, "::"); idx >= 0 {
		q, ok := byId[id[idx+2:]]
		return q, ok
	}
	return ClarificationQuestion{}, false
}

// ScrubSecretAnswers removes any typed value from the answers to secret
// questions, and from the message text, before the message is formatted for
// the model or stored. A well-behaved client never sends the value - it saves
// the secret and answers with secret_ref - so this is the guard for one that
// does: the value must not reach the model, the transcript or the database.
// req may be empty; an answer carrying secret_ref is treated as secret anyway.
func ScrubSecretAnswers(msg *AgentMessage, req ClarificationRequest) {
	answers, ok := msg.ClarificationAnswers()
	if !ok {
		return
	}
	byId := make(map[string]ClarificationQuestion, len(req.Questions))
	for _, q := range req.Questions {
		byId[q.Id] = q
	}
	changed := false
	for i, ans := range answers {
		q, known := lookupQuestion(byId, ans.QuestionId)
		if ans.SecretRef == "" && !(known && q.IsSecret()) {
			continue
		}
		if ans.FreeText == "" {
			continue
		}
		if len(strings.TrimSpace(ans.FreeText)) >= 4 {
			msg.Content = strings.ReplaceAll(msg.Content, ans.FreeText, "[secret]")
		}
		answers[i].FreeText = ""
		changed = true
	}
	if changed {
		var raw []interface{}
		b, _ := json.Marshal(answers)
		_ = json.Unmarshal(b, &raw)
		msg.Params[ClarificationAnswersParamKey] = raw
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
		return lookupQuestion(questionById, id)
	}

	var sb strings.Builder
	sb.WriteString("The user answered the clarification questions:\n")
	for _, ans := range answers {
		if q, ok := lookup(ans.QuestionId); ok {
			sb.WriteString(fmt.Sprintf("- Q: %s\n", q.Question))
		} else {
			sb.WriteString(fmt.Sprintf("- Q (%s):\n", ans.QuestionId))
		}
		if q, ok := lookup(ans.QuestionId); (ok && q.IsSecret()) || ans.SecretRef != "" {
			if ans.SecretRef != "" {
				sb.WriteString(fmt.Sprintf("  A: saved as a secret - refer to it as %s\n", ans.SecretRef))
			} else {
				sb.WriteString("  A: (not saved - ask again)\n")
			}
			continue
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
