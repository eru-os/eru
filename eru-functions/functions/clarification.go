package functions

// containsClarificationQuestion reports whether a decoded response body carries
// a human-in-the-loop clarification request raised by an agent step, i.e. an
// action with "action_type": "question". Used to halt a branch at the step that
// asked, without running steps that depend on its (unanswered) output.
func containsClarificationQuestion(body interface{}) bool {
	switch v := body.(type) {
	case map[string]interface{}:
		if at, ok := v["action_type"].(string); ok && at == "question" {
			return true
		}
		for _, child := range v {
			if containsClarificationQuestion(child) {
				return true
			}
		}
	case []interface{}:
		for _, child := range v {
			if containsClarificationQuestion(child) {
				return true
			}
		}
	}
	return false
}

// varsHaveQuestion reports whether a TemplateVars body holds a clarification
// question.
func varsHaveQuestion(vars *TemplateVars) bool {
	if vars == nil {
		return false
	}
	return containsClarificationQuestion(vars.Body)
}
