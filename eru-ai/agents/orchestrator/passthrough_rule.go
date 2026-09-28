package orchestrator

import (
	"strings"

	functions "github.com/eru-os/eru/eru-functions/functions"
)

// When the whole plan is one agent step that reads nothing from any other step,
// the orchestrator has nothing to add and everything to lose by rewriting the
// user's message.
//
// What happened without this: the user said "change the bg color of this comp
// with id tb-title to blue", and then "blue is not looking gd, lets change it to
// yellow". The planner turned the second message into its own self-contained
// instruction - "replace all blue colors with yellow throughout the page" - and
// the page agent, which had the earlier turn in its own conversation and would
// have resolved "blue" to tb-title, instead obeyed the instruction it was given,
// scanned the whole page and changed three components.
//
// The orchestrator sees one message. The agent sees the conversation and the
// page. So the message goes through as the user wrote it.

// singleAgentStep returns the only agent step in a plan, when there is exactly
// one and nothing else runs alongside it.
func singleAgentStep(funcGroup functions.FuncGroup) (string, *functions.FuncStep, bool) {
	if len(funcGroup.FuncSteps) != 1 {
		return "", nil, false
	}
	for key, step := range funcGroup.FuncSteps {
		if step == nil || step.AgentName == "" {
			return "", nil, false
		}
		if len(step.FuncSteps) > 0 {
			return "", nil, false
		}
		return key, step, true
	}
	return "", nil, false
}

// validateUserMessagePassthrough reports a lone agent step whose request rewrites
// what the user said.
//
// It only fires when there is genuinely nothing to combine: one step, no nested
// steps, and a template that reads no other step's output. A plan that gathers
// data first and then asks an agent to render it is untouched - there the
// instruction really is the orchestrator's to write.
func validateUserMessagePassthrough(funcGroup functions.FuncGroup, cc codeContext) []planIssue {
	userMessage := strings.TrimSpace(cc.UserMessage)
	if userMessage == "" {
		return nil
	}
	stepKey, step, ok := singleAgentStep(funcGroup)
	if !ok {
		return nil
	}
	template := step.TransformRequest
	if strings.TrimSpace(template) == "" {
		return nil
	}
	// A step that consumes another step's output is not a passthrough candidate.
	if strings.Contains(template, ".ResVars.") || strings.Contains(template, ".ReqVars.") {
		return nil
	}
	// Already forwarding the message is exactly what we want.
	if strings.Contains(template, userRequestRoot+".content") || strings.Contains(template, staleRequestRoot+".content") {
		return nil
	}

	return []planIssue{{
		StepPath: stepKey,
		Field:    "request",
		Template: firstLine(template, 160),
		Err: "this plan is a single agent step, so the user's message must be forwarded as they wrote it rather than rewritten. " +
			"Use \"request\": {\"content\": {\"from\": \"user.content\"}} (keep any params you are already passing). " +
			"A rewritten instruction loses what the message was leaning on - \"it\", \"that one\", a colour or an id named in an earlier turn - " +
			"and the agent, which has the conversation and the artifact, is left following your paraphrase instead of the user",
	}}
}

// firstLine trims a template down to something readable in an error.
func firstLine(template string, max int) string {
	template = strings.TrimSpace(template)
	if index := strings.IndexAny(template, "\r\n"); index >= 0 {
		template = template[:index]
	}
	if len(template) > max {
		return template[:max] + "..."
	}
	return template
}
