package orchestrator

import (
	"strings"

	functions "github.com/eru-os/eru/eru-functions/functions"
)

// Attaching a file is an instruction.
//
// Nobody attaches a screenshot in passing. It is there because the answer
// depends on it, and a plan that does not hand it to any step has decided,
// silently, that it does not matter - which is not a decision the user asked for
// and not one they can see being made. What they see is an assistant that
// looked straight past the thing they pointed at.
//
// Forwarding costs a step the tokens to carry the file, so the rule is not that
// every step should get it. The rule is that SOME step should: whichever one has
// to look at the thing. Deciding which is the planner's judgement about the
// task, and this only insists that the judgement is made rather than skipped.
func validateAttachmentRouting(funcGroup functions.FuncGroup, cc codeContext) []planIssue {
	if len(cc.Attachments) == 0 {
		return nil
	}
	if len(funcGroup.FuncSteps) == 0 {
		return nil
	}

	forwarded := false
	walkSteps(funcGroup.FuncSteps, "", func(_ string, _ string, step *functions.FuncStep) {
		if forwarded {
			return
		}
		for _, templateField := range stepTemplateFields(step) {
			if strings.Contains(templateField.Template, userRequestRoot+".files") || strings.Contains(templateField.Template, staleRequestRoot+".files") {
				forwarded = true
				return
			}
		}
	})
	if forwarded {
		return nil
	}

	return []planIssue{{
		Field: "transform_request",
		Err: "the user attached " + describeAttachments(cc.Attachments) +
			" and no step receives " + them(len(cc.Attachments)) +
			". Add \"files\": {\"from\": \"user.files\"} to the request of the step that has to look at " +
			them(len(cc.Attachments)) +
			" - \"files\" is a top-level key of the request body, beside \"content\", and every agent accepts it. " +
			"If no step genuinely needs to see " + them(len(cc.Attachments)) +
			", say what was attached in that step's \"content\" so the agent at least knows it exists",
	}}
}

func describeAttachments(names []string) string {
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) != "" {
			cleaned = append(cleaned, name)
		}
	}
	if len(cleaned) == 0 {
		if len(names) == 1 {
			return "a file"
		}
		return "files"
	}
	return strings.Join(cleaned, ", ")
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}
