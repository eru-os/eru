package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	agents "github.com/eru-os/eru/eru-ai/agents"
	tools "github.com/eru-os/eru/eru-ai/tools"
	functions "github.com/eru-os/eru/eru-functions/functions"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// The page agent hands its pages back to the client as artifacts and never
// writes them: a person editing a page reviews the result and saves it from the
// editor. That is right when they asked for a page and are sitting in front of
// it, and wrong when a page was one step of a build - the entities that same run
// created are on the server, the pages are only in the browser, and navigating
// away loses them with nothing said. So a build that produced pages asks whether
// to keep them, and writes the ones the user accepts.

// pageSaveQuestionId is the question this asks, and the marker that tells a
// resume the answer is about pages rather than a step that paused.
const pageSaveQuestionId = "save_generated_pages"

const pageSaveYes = "Save them"
const pageSaveNo = "Leave them unsaved"

// collectGeneratedPages finds the pages a run produced. The page agent emits one
// entry per page carrying its id and its definition; anything without both is
// not a page and is left alone.
func collectGeneratedPages(resVars map[string]*functions.TemplateVars) []map[string]interface{} {
	var found []map[string]interface{}
	seen := map[string]bool{}

	var walk func(v interface{})
	walk = func(v interface{}) {
		switch node := v.(type) {
		case map[string]interface{}:
			// The id is taken from the page itself when the envelope does not
			// repeat it. Requiring a "page_id" sibling made detection depend on
			// which wrapper the answer happened to arrive in: the studio agent
			// answers {mode, page}, and a build whose envelope omitted the
			// sibling was collected as zero pages - so nothing was offered for
			// saving, nothing was said, and the page was lost on navigation.
			if def, ok := node["page"].(map[string]interface{}); ok {
				id, _ := node["page_id"].(string)
				if strings.TrimSpace(id) == "" {
					id, _ = def["id"].(string)
				}
				if id = strings.TrimSpace(id); id != "" && !seen[id] {
					seen[id] = true
					found = append(found, map[string]interface{}{"page_id": id, "page": def})
				}
			}
			for _, child := range node {
				walk(child)
			}
		case []interface{}:
			for _, child := range node {
				walk(child)
			}
		}
	}

	keys := make([]string, 0, len(resVars))
	for k := range resVars {
		keys = append(keys, k)
	}
	sort.Strings(keys) // a stable order, so the question reads the same way twice
	for _, k := range keys {
		if resVars[k] != nil {
			walk(resVars[k].Body)
		}
	}
	return found
}

// pageName is what the page calls itself, falling back to its id.
func pageName(page map[string]interface{}) string {
	def, _ := page["page"].(map[string]interface{})
	for _, key := range []string{"title", "name", "page_name"} {
		if v, ok := def[key].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	id, _ := page["page_id"].(string)
	return id
}

// pageSaveRequest is the one question asked at the end of a build that made pages.
func pageSaveRequest(pages []map[string]interface{}) agents.ClarificationRequest {
	names := make([]string, 0, len(pages))
	for _, p := range pages {
		names = append(names, pageName(p))
	}
	subject, pronoun := "1 page was", "it"
	if len(pages) != 1 {
		subject = fmt.Sprintf("%d pages were", len(pages))
		pronoun = "them"
	}
	return agents.ClarificationRequest{
		Questions: []agents.ClarificationQuestion{{
			Id: pageSaveQuestionId,
			Question: fmt.Sprintf("%s built: %s. Save %s to this workspace? An unsaved page is lost when you navigate away.",
				subject, strings.Join(names, ", "), pronoun),
			Options: []agents.QuestionOption{
				{Value: pageSaveYes, Label: pageSaveYes},
				{Value: pageSaveNo, Label: pageSaveNo},
			},
			Required: true,
		}},
	}
}

// answeredYesToPageSave reports whether the user accepted the save.
func answeredYesToPageSave(answers []agents.ClarificationAnswer) bool {
	for _, a := range answers {
		if !strings.HasSuffix(a.QuestionId, pageSaveQuestionId) {
			continue
		}
		for _, chosen := range a.Selected {
			if strings.EqualFold(strings.TrimSpace(chosen), pageSaveYes) {
				return true
			}
		}
		return strings.EqualFold(strings.TrimSpace(a.FreeText), pageSaveYes)
	}
	return false
}

// pageSaveKey is where the page-save report lands in the execution result, so
// the synthesis has it whether the save happened, was declined, or was never
// possible.
const pageSaveKey = "page_save"

// attachedToolNames lists what this orchestrator actually holds, so a failure to
// find a delegate names the gap instead of only asserting one. "No tool offering
// save_page is attached" sent me looking for a bug in the save; the tool was
// simply not in this agent's configuration, and one line would have said so.
func (oa *OrchestratorAgent) attachedToolNames() string {
	var names []string
	for _, attached := range oa.AgentTools {
		name := attached.ToolName
		if attached.Tool == nil {
			name += " (not loaded)"
		}
		names = append(names, name)
	}
	for action := range oa.internalTools {
		names = append(names, action+" (internal)")
	}
	if len(names) == 0 {
		return "no tools at all"
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// queryDelegate is the tool the context lookup runs its query through.
//
// The workspace context lookup is a utility tool rather than a tenant action, so
// no tenant offers it and the lookup is always built over a query tool. The
// internal one comes first for the same reason the save does: an orchestrator
// attaches none.
func (oa *OrchestratorAgent) queryDelegate(ctx context.Context) tools.Tooling {
	if tool := oa.internalTool("execute_query"); tool != nil {
		return tool
	}
	return oa.eruqlDelegate(ctx)
}

// savePageDelegate finds a tool that can write a page.
//
// The internal tool comes first and is normally the only one: an orchestrator
// carries no attached tools, so the AgentTools scan below found nothing on a
// real tenant and the save was skipped in silence. It is kept for an agent type
// that does attach its own.
func (oa *OrchestratorAgent) savePageDelegate(ctx context.Context) tools.Tooling {
	if tool := oa.internalTool("save_page"); tool != nil {
		return tool
	}
	for _, attached := range oa.AgentTools {
		if attached.Tool == nil {
			continue
		}
		for _, action := range attached.Tool.GetActionsList() {
			if action.Name == "save_page" {
				return attached.Tool
			}
		}
	}
	return nil
}

var (
	workspaceContextToolName string
	workspaceContextBuilder  DelegateToolBuilder
)

func RegisterWorkspaceContextTool(toolName string, build DelegateToolBuilder) {
	workspaceContextToolName = toolName
	workspaceContextBuilder = build
}

func (oa *OrchestratorAgent) workspaceContextTool() tools.Tooling {
	if workspaceContextToolName == "" {
		return nil
	}
	return oa.internalTool(workspaceContextToolName)
}

// workspaceIds resolves the org and process this request is running against.
// save_page is addressed by org and process, not by the tenant the request
// carries, so they have to be looked up the same way a sub-agent looks them up.
func (oa *OrchestratorAgent) workspaceIds(ctx context.Context, projectId string, tenantId string) (orgId string, processId string, err error) {
	// The tenant's own context action when it has one, since it answers this
	// question directly. Building the tool over an eru-ql delegate is the
	// fallback, and on an orchestrator - which attaches no tools - it never
	// resolved at all.
	var (
		result map[string]interface{}
		action string
	)
	if tool := oa.workspaceContextTool(); tool != nil {
		action = workspaceContextToolName
		result, _, err = tool.Execute(ctx, projectId, tenantId, action, map[string]interface{}{})
	} else if delegate := oa.queryDelegate(ctx); delegate != nil && workspaceContextBuilder != nil {
		action = workspaceContextToolName
		result, _, err = workspaceContextBuilder(ctx, delegate).Execute(ctx, projectId, tenantId, action, map[string]interface{}{})
	} else {
		return "", "", fmt.Errorf("neither a workspace context action nor an eru-ql tool is available, so the workspace ids cannot be resolved")
	}
	if err != nil {
		return "", "", err
	}
	orgId, _ = result["org_id"].(string)
	processId, _ = result["process_id"].(string)
	if orgId == "" || processId == "" {
		return "", "", fmt.Errorf("%s did not return both org_id and process_id", action)
	}
	return orgId, processId, nil
}

// savePages writes the accepted pages and reports what happened, one line per
// page. A page that fails to save is named rather than swallowed: the rest are
// still attempted, because a partial save the user can see beats an all-or
// -nothing failure they cannot.
func (oa *OrchestratorAgent) savePages(ctx context.Context, pages []map[string]interface{}, projectId string, tenantId string) []string {
	if len(pages) == 0 {
		return nil
	}
	delegate := oa.savePageDelegate(ctx)
	if delegate == nil {
		return []string{fmt.Sprintf(
			"The pages could not be saved: no tool offering save_page is attached to this orchestrator (it has: %s).",
			oa.attachedToolNames())}
	}
	orgId, processId, err := oa.workspaceIds(ctx, projectId, tenantId)
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("page save could not resolve the workspace: ", err))
		return []string{fmt.Sprintf("The pages could not be saved: %v", err)}
	}

	var report []string
	for _, page := range pages {
		id, _ := page["page_id"].(string)
		def, _ := page["page"].(map[string]interface{})
		name := pageName(page)
		// page_def travels as an object. Sent as a string it persists as {}.
		_, _, saveErr := delegate.Execute(ctx, projectId, tenantId, "save_page", map[string]interface{}{
			"org_id":     orgId,
			"process_id": processId,
			"page_id":    id,
			"page_name":  name,
			"page_def":   def,
		})
		if saveErr != nil {
			logs.WithContext(ctx).Error(fmt.Sprintf("saving page %s failed: %v", id, saveErr))
			report = append(report, fmt.Sprintf("Page %q (%s) was NOT saved: %v", name, id, saveErr))
			continue
		}
		logs.WithContext(ctx).Info(fmt.Sprintf("saved page %s (%s)", name, id))
		report = append(report, fmt.Sprintf("Saved page %q (%s).", name, id))
	}
	return report
}

// describeResVars reports the shape of each step result the page search walked:
// the Go type of the body, its top-level keys, and for an agent envelope the
// keys of each action. A page that is present but not recognised and a page
// that is absent produce the same "0 collected", and only the shape tells them
// apart.
func describeResVars(funcVars map[string]functions.FuncTemplateVars) []string {
	resVars := extractResVars(funcVars)
	names := make([]string, 0, len(resVars))
	for name := range resVars {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, name := range names {
		vars := resVars[name]
		if vars == nil || vars.Body == nil {
			out = append(out, fmt.Sprintf("%s=<nil>", name))
			continue
		}
		out = append(out, fmt.Sprintf("%s=%s", name, describeNode(vars.Body, 0)))
	}
	return out
}

// describeNode renders a value's shape - never its content, which can be a
// whole page and carries user data into the log.
func describeNode(node interface{}, depth int) string {
	switch typed := node.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if depth >= 2 {
			return fmt.Sprintf("map{%s}", strings.Join(keys, ","))
		}
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			if k == "actions" || k == "action" || k == "page" || k == "pages" || k == "mode" {
				parts = append(parts, k+":"+describeNode(typed[k], depth+1))
				continue
			}
			parts = append(parts, k)
		}
		return fmt.Sprintf("map{%s}", strings.Join(parts, ","))
	case []interface{}:
		if len(typed) == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d x %s]", len(typed), describeNode(typed[0], depth+1))
	case string:
		return fmt.Sprintf("string(len=%d)", len(typed))
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("%T", node)
	}
}

// resVarStepNames lists the steps whose results were searched for pages, so a
// "0 page(s) collected" line says where it looked. Without it, a build that
// produced a page and offered no save is indistinguishable from one that
// produced none.
func resVarStepNames(funcVars map[string]functions.FuncTemplateVars) []string {
	resVars := extractResVars(funcVars)
	names := make([]string, 0, len(resVars))
	for name, vars := range resVars {
		if vars == nil || vars.Body == nil {
			name += " (empty)"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
