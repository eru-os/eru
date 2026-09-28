package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	agents "github.com/eru-os/eru/eru-ai/agents"
	functions "github.com/eru-os/eru/eru-functions/functions"
)

const PendingResumeParamKey = "pending_resume"

// PendingResume is the checkpoint persisted in a question message so a paused
// orchestration can resume only the remaining work. ResVarsJSON holds the
// completed steps' outputs (serialized functions TemplateVars), PausedBranches
// the steps that asked, and JoinStep the convergence step (if any).
type PendingResume struct {
	RunId          string         `json:"run_id,omitempty"`
	Plan           map[string]any `json:"plan"`
	ResVarsJSON    string         `json:"res_vars_json,omitempty"`
	ReqVarsJSON    string         `json:"req_vars_json,omitempty"`
	PausedBranches []PausedBranch `json:"paused_branches"`
	JoinStep       string         `json:"join_step,omitempty"`

	// ResolvedAnswers are the questions the orchestrator answered for itself.
	// They travel with the checkpoint because a run can be partly self-resolved:
	// the user is asked only what was left over, and on resume the sub-agent
	// still needs the answers it is not going to be given twice.
	ResolvedAnswers []agents.ClarificationAnswer `json:"resolved_answers,omitempty"`

	// Assumptions are those same answers in words, to be shown with the result.
	Assumptions []string `json:"assumptions,omitempty"`

	// PagesToSave are pages a build produced that the user is being asked about.
	// They ride on the checkpoint because the answer arrives in a new request,
	// by which time the run that produced them is long gone.
	PagesToSave []map[string]interface{} `json:"pages_to_save,omitempty"`
}

// withResolvedAnswers merges the orchestrator's own answers into a message,
// letting the user's answers win where both speak to the same question.
func withResolvedAnswers(message agents.AgentMessage, resolved []agents.ClarificationAnswer) agents.AgentMessage {
	if len(resolved) == 0 {
		return message
	}
	existing, _ := message.ClarificationAnswers()
	answered := make(map[string]bool, len(existing))
	for _, a := range existing {
		answered[a.QuestionId] = true
	}

	merged := make([]agents.ClarificationAnswer, 0, len(existing)+len(resolved))
	merged = append(merged, existing...)
	for _, a := range resolved {
		if !answered[a.QuestionId] {
			merged = append(merged, a)
		}
	}

	params := make(map[string]interface{}, len(message.Params)+1)
	for key, value := range message.Params {
		params[key] = value
	}
	params[agents.ClarificationAnswersParamKey] = merged
	message.Params = params
	return message
}

type PausedBranch struct {
	StartStep   string   `json:"start_step"`
	EndStep     string   `json:"end_step"`
	QuestionIds []string `json:"question_ids,omitempty"`
}

// answersForBranch returns the clarification answers that belong to a paused
// branch, with the step prefix stripped.
//
// The orchestrator prefixes every sub-agent question with the step that asked
// it, so the client answers "eru_studio::q1". The sub-agent only knows "q1", and
// it must not see another step's answers at all - so the routing happens here,
// where the mapping is known, rather than being left to whatever the client sent.
func answersForBranch(answers []agents.ClarificationAnswer, branch PausedBranch) []agents.ClarificationAnswer {
	if len(answers) == 0 {
		return nil
	}
	owned := make(map[string]bool, len(branch.QuestionIds))
	for _, id := range branch.QuestionIds {
		owned[id] = true
	}

	prefix := branch.StartStep + "::"
	out := make([]agents.ClarificationAnswer, 0, len(answers))
	for _, answer := range answers {
		id := answer.QuestionId
		switch {
		case owned[id], strings.HasPrefix(id, prefix):
			answer.QuestionId = strings.TrimPrefix(id, prefix)
		case len(branch.QuestionIds) == 0 && !strings.Contains(id, "::"):
			// A branch that recorded no question ids, answered without a prefix:
			// there is nothing to route by, so take it as this branch's.
		default:
			continue
		}
		out = append(out, answer)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// withBranchAnswers returns a copy of the message carrying only the answers that
// belong to this branch. The params map is copied because the same message is
// reused for every branch of a resume.
func withBranchAnswers(message agents.AgentMessage, branch PausedBranch) agents.AgentMessage {
	answers, ok := message.ClarificationAnswers()
	if !ok {
		return message
	}
	routed := answersForBranch(answers, branch)

	params := make(map[string]interface{}, len(message.Params)+1)
	for key, value := range message.Params {
		params[key] = value
	}
	if len(routed) == 0 {
		delete(params, agents.ClarificationAnswersParamKey)
	} else {
		params[agents.ClarificationAnswersParamKey] = routed
	}
	message.Params = params
	return message
}

// collectQuestions walks an executed FuncGroup result and returns every
// clarification request raised by a sub-agent, tagged with the step that asked.
func collectQuestions(node interface{}, path string, out *[]taggedQuestion) {
	switch v := node.(type) {
	case map[string]interface{}:
		if at, ok := v["action_type"].(string); ok && at == agents.ActionTypeQuestion {
			if action, ok := v["action"].(map[string]interface{}); ok {
				if req, err := agents.ParseClarificationRequest(action); err == nil && len(req.Questions) > 0 {
					*out = append(*out, taggedQuestion{Step: path, Request: req})
				}
			}
		}
		for k, child := range v {
			childPath := k
			if path != "" {
				childPath = fmt.Sprintf("%s.%s", path, k)
			}
			collectQuestions(child, childPath, out)
		}
	case []interface{}:
		for _, child := range v {
			collectQuestions(child, path, out)
		}
	}
}

type taggedQuestion struct {
	Step    string
	Request agents.ClarificationRequest
}

// mergeQuestions flattens questions from one or more sub-agents into a single
// ClarificationRequest, prefixing each question id with its originating step so
// answers can be routed back unambiguously.
func mergeQuestions(tagged []taggedQuestion) agents.ClarificationRequest {
	merged := agents.ClarificationRequest{}
	for _, tq := range tagged {
		for _, q := range tq.Request.Questions {
			if tq.Step != "" {
				q.Id = fmt.Sprintf("%s::%s", tq.Step, q.Id)
			}
			merged.Questions = append(merged.Questions, q)
		}
	}
	if len(tagged) == 1 {
		merged.Prompt = tagged[0].Request.Prompt
	} else if len(tagged) > 1 {
		merged.Prompt = "Some steps need clarification before I can continue."
	}
	return merged
}

// assignStableConversationIds walks the func_steps tree of a decomposed plan
// and assigns each agent step a deterministic conversation id derived from the
// orchestrator conversation id and the step key. This keeps each sub-agent's
// state stable across a pause/resume cycle so a sub-agent that asked a question
// continues its own thread on resume.
func assignStableConversationIds(plan map[string]interface{}, parentConversationId string) {
	if parentConversationId == "" {
		return
	}
	steps, ok := plan["func_steps"].(map[string]interface{})
	if !ok {
		return
	}
	assignStepConversationIds(steps, parentConversationId)
}

// extractResVars merges the per-step ResVars from a funcVarsMap into a single
// map keyed by step name.
func extractResVars(funcVarsMap map[string]functions.FuncTemplateVars) map[string]*functions.TemplateVars {
	merged := map[string]*functions.TemplateVars{}
	for _, fv := range funcVarsMap {
		for k, v := range fv.ResVars {
			if v != nil {
				merged[k] = v
			}
		}
	}
	return merged
}

func marshalVars(vars map[string]*functions.TemplateVars) string {
	if len(vars) == 0 {
		return ""
	}
	b, err := json.Marshal(vars)
	if err != nil {
		return ""
	}
	return string(b)
}

func unmarshalVars(s string) map[string]*functions.TemplateVars {
	vars := map[string]*functions.TemplateVars{}
	if s == "" {
		return vars
	}
	_ = json.Unmarshal([]byte(s), &vars)
	return vars
}

// questionInBody finds a clarification request embedded in a decoded response
// body (an agent step's output), if present.
func questionInBody(body interface{}) (agents.ClarificationRequest, bool) {
	switch v := body.(type) {
	case map[string]interface{}:
		if at, ok := v["action_type"].(string); ok && at == agents.ActionTypeQuestion {
			if action, ok := v["action"].(map[string]interface{}); ok {
				if req, err := agents.ParseClarificationRequest(action); err == nil && len(req.Questions) > 0 {
					return req, true
				}
			}
		}
		for _, child := range v {
			if req, ok := questionInBody(child); ok {
				return req, true
			}
		}
	case []interface{}:
		for _, child := range v {
			if req, ok := questionInBody(child); ok {
				return req, true
			}
		}
	}
	return agents.ClarificationRequest{}, false
}

// deriveJoinStep returns the key of the convergence step — a step carrying a
// non-empty wait_for. Returns "" when the plan has no parallel join.
func deriveJoinStep(plan map[string]interface{}) string {
	steps, ok := plan["func_steps"].(map[string]interface{})
	if !ok {
		return ""
	}
	return findWaitForStep(steps)
}

func findWaitForStep(steps map[string]interface{}) string {
	for key, raw := range steps {
		step, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if wf, _ := step["wait_for"].(string); wf != "" {
			return key
		}
		if nested, ok := step["func_steps"].(map[string]interface{}); ok {
			if found := findWaitForStep(nested); found != "" {
				return found
			}
		}
	}
	return ""
}

// buildPendingResume inspects the completed run's per-step variables, collects
// every step that paused with a question, and assembles the checkpoint plus the
// merged (UI-facing) clarification request.
func buildPendingResume(plan map[string]interface{}, funcVarsMap map[string]functions.FuncTemplateVars, runId string) (PendingResume, agents.ClarificationRequest, bool) {
	return buildPendingResumeFromVars(plan, extractResVars(funcVarsMap), runId)
}

// resVarsToResult projects each step's response body into a result map keyed by
// step name (used when a resumed plan has no join step).
func resVarsToResult(resVars map[string]*functions.TemplateVars) map[string]interface{} {
	result := map[string]interface{}{}
	for k, v := range resVars {
		if v != nil {
			result[k] = v.Body
		}
	}
	return result
}

func buildPendingResumeFromVars(plan map[string]interface{}, resVars map[string]*functions.TemplateVars, runId string) (PendingResume, agents.ClarificationRequest, bool) {
	planSteps, _ := plan["func_steps"].(map[string]interface{})

	var tagged []taggedQuestion
	pr := PendingResume{RunId: runId, Plan: plan}

	// A branch is bounded to the step that asked ONLY when the plan has a join
	// step. That bound exists so a join nested under several paused siblings is
	// not run once per sibling - the join is run once, afterwards.
	//
	// A plan with no join is a chain, and there the step that asked carries its
	// continuation as its own nested children. Bounding it would run that step
	// and stop: the question gets answered and the rest of the plan is dropped
	// without a word. Observed on a four-step chain where the first step asked
	// for a storage name - the entity it was building was created and the three
	// steps after it never ran. Leaving EndStep empty runs the branch to the end
	// of the plan, and RunFuncStepInner only skips the children of a step that
	// still carries a question, which a resumed step no longer does.
	joinStep := deriveJoinStep(plan)

	for stepKey, vars := range resVars {
		if vars == nil {
			continue
		}
		if _, isPlanStep := planStep(planSteps, stepKey); !isPlanStep {
			continue
		}
		req, ok := questionInBody(vars.Body)
		if !ok {
			continue
		}
		var qids []string
		for _, q := range req.Questions {
			qids = append(qids, fmt.Sprintf("%s::%s", stepKey, q.Id))
		}
		endStep := ""
		if joinStep != "" {
			endStep = stepKey
		}
		pr.PausedBranches = append(pr.PausedBranches, PausedBranch{StartStep: stepKey, EndStep: endStep, QuestionIds: qids})
		tagged = append(tagged, taggedQuestion{Step: stepKey, Request: req})
	}

	if len(pr.PausedBranches) == 0 {
		return PendingResume{}, agents.ClarificationRequest{}, false
	}

	pr.JoinStep = joinStep
	pr.ResVarsJSON = marshalVars(resVars)
	return pr, mergeQuestions(tagged), true
}

// planStep reports whether stepKey is a step in the (possibly nested) plan.
func planStep(steps map[string]interface{}, stepKey string) (map[string]interface{}, bool) {
	for key, raw := range steps {
		step, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if key == stepKey {
			return step, true
		}
		if nested, ok := step["func_steps"].(map[string]interface{}); ok {
			if s, found := planStep(nested, stepKey); found {
				return s, true
			}
		}
	}
	return nil, false
}

func assignStepConversationIds(steps map[string]interface{}, parentConversationId string) {
	for key, raw := range steps {
		step, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if _, isAgent := step["agent_name"]; isAgent {
			if existing, _ := step["conversation_id"].(string); existing == "" {
				step["conversation_id"] = fmt.Sprintf("%s::%s", parentConversationId, key)
			}
		}
		if nested, ok := step["func_steps"].(map[string]interface{}); ok {
			assignStepConversationIds(nested, parentConversationId)
		}
	}
}
