package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	agents "github.com/eru-os/eru/eru-ai/agents"
	functions "github.com/eru-os/eru/eru-functions/functions"
	eru_models "github.com/eru-os/eru/eru-models"
	gotemplate "github.com/eru-os/eru/eru-templates/gotemplate"
)

type planIssue struct {
	StepPath string
	Field    string
	Template string
	Err      string
}

func (pi planIssue) String() string {
	if pi.StepPath == "" {
		return fmt.Sprintf("%s: %s", pi.Field, pi.Err)
	}
	if pi.Template == "" {
		return fmt.Sprintf("step %s -> %s: %s", pi.StepPath, pi.Field, pi.Err)
	}
	return fmt.Sprintf("step %s -> %s: %s\n    template: %s", pi.StepPath, pi.Field, pi.Err, pi.Template)
}

func formatPlanIssues(issues []planIssue) string {
	var sb strings.Builder
	for i, issue := range issues {
		sb.WriteString(fmt.Sprint(i+1, ". ", issue.String(), "\n"))
	}
	return sb.String()
}

// validatePlanTemplates parses every go template in a generated FuncGroup with the
// same function map used at execution time, so malformed templates are caught
// before any step runs. It reports structural problems too, since a plan that
// cannot be decoded into a FuncGroup can never execute.
func validatePlanTemplates(ctx context.Context, plan map[string]interface{}) []planIssue {
	return validatePlan(ctx, plan, nil, nil, codeContext{})
}

// validatePlan checks a generated FuncGroup before any step runs: every go
// template is parsed with the function map used at execution time, every
// .ResVars/.ReqVars reference is resolved against the plan's real step keys, and
// step keys, agent names and tool actions are checked against the naming rules
// and the agents/tools the orchestrator is actually allowed to use.
func validatePlan(ctx context.Context, plan map[string]interface{}, allowedAgents []agents.DiscoveredAgent, allowedTools []agents.DiscoveredTool, cc codeContext) []planIssue {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return []planIssue{{Field: "func_group", Err: err.Error()}}
	}
	var funcGroup functions.FuncGroup
	if err := json.Unmarshal(planJSON, &funcGroup); err != nil {
		return []planIssue{{Field: "func_group", Err: fmt.Sprint("plan is not a valid FuncGroup : ", err.Error())}}
	}
	if len(funcGroup.FuncSteps) == 0 {
		return []planIssue{{Field: "func_steps", Err: "plan has no func_steps - the FuncGroup must contain at least one agent or tool step at the root of func_steps"}}
	}

	issues := validateStepTemplates(ctx, "", funcGroup.FuncSteps)
	issues = append(issues, validateStepReferences(ctx, funcGroup.FuncSteps)...)
	issues = append(issues, validateUserRequestRoot(funcGroup.FuncSteps)...)
	issues = append(issues, validateStepKeyUniqueness(funcGroup.FuncSteps)...)
	issues = append(issues, validateStepIdentity(funcGroup.FuncSteps, allowedAgents, allowedTools)...)
	issues = append(issues, validateStepPayload(ctx, funcGroup.FuncSteps, allowedAgents, allowedTools)...)
	issues = append(issues, validateCodeRouting(ctx, funcGroup.FuncSteps, cc)...)
	issues = append(issues, validateParamForwarding(funcGroup.FuncSteps, allowedAgents, cc)...)
	issues = append(issues, validateClarificationForwarding(funcGroup.FuncSteps, allowedAgents)...)
	issues = append(issues, validateSqlAuthoring(funcGroup.FuncSteps, allowedAgents)...)
	issues = append(issues, validateUserMessagePassthrough(funcGroup, cc)...)
	issues = append(issues, validateAttachmentRouting(funcGroup, cc)...)
	return issues
}

// validateSqlAuthoring keeps invented SQL out of a plan.
//
// The planner is never shown a database schema, so SQL it writes itself is a
// guess at table and column names. The failure this was written for asked a
// non-existent "eru.entity_fields" for an entity's fields, retried the identical
// statement until the run gave up, and told the user only that something went
// wrong - while the agent it was fetching for had a purpose-built metadata
// lookup it never got the chance to use.
//
// SQL belongs to whoever knows the schema. A step may carry SQL that an earlier
// step produced (a .ResVars reference); it may not carry SQL the planner typed.
func validateSqlAuthoring(steps map[string]*functions.FuncStep, allowedAgents []agents.DiscoveredAgent) []planIssue {
	author := sqlAuthoringAgent(allowedAgents)
	if author == "" {
		// Nothing in this workspace can write SQL from the schema, so a literal
		// is the only option there is. Reporting it would be noise.
		return nil
	}

	var issues []planIssue
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		if !containsLiteralSQL(step.TransformRequest) {
			return
		}
		if strings.Contains(step.TransformRequest, ".ResVars.") {
			// The SQL came from a previous step; that is the supported shape.
			return
		}
		issues = append(issues, planIssue{
			StepPath: stepPath,
			Field:    "transform_request",
			Template: step.TransformRequest,
			Err: fmt.Sprint("this step carries SQL written into the plan, but nothing here has seen the database schema, ",
				"so the table and column names are guesses - the step fails on a relation that does not exist. ",
				"Ask the agent ", author, " for the SQL and pass its output on by reference, e.g. ",
				"(index .ResVars.", author, ".Body.actions 0).action.sql - or, when an agent in this plan already ",
				"looks the information up itself, drop the step entirely and let it."),
		})
	})
	return issues
}

// sqlAuthoringAgent names an agent that turns a request into SQL, identified by
// an "sql" field in its declared output.
func sqlAuthoringAgent(allowedAgents []agents.DiscoveredAgent) string {
	for _, agent := range allowedAgents {
		for _, field := range outputFieldNames(agent.OutputSchema) {
			if strings.EqualFold(field, "sql") {
				return agent.AgentName
			}
		}
	}
	return ""
}

// literalSQLPatterns match the shape of a statement, not just its keywords:
// "multi-select filter writes its selection ... from" is prose that happens to
// hold SELECT and FROM, and rejecting the plan over it cost a planning round.
// A select list is comma-separated expressions, never a run of bare words.
var literalSQLPatterns = func() []*regexp.Regexp {
	ident := `[\w."` + "`" + `]+`
	item := `(?:` + ident + `|\*)(?:\([^)]*\))?(?:\s+AS\s+\w+)?`
	return []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bSELECT\s+(?:DISTINCT\s+)?` + item + `(?:\s*,\s*` + item + `)*\s+FROM\s+` + ident),
		regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+` + ident + `\s*(?:\(|VALUES\b|SELECT\b)`),
		regexp.MustCompile(`(?i)\bUPDATE\s+` + ident + `\s+SET\s+` + ident + `\s*=`),
		regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+` + ident + `(?:\s+WHERE\b|\s*;|\s*["'` + "`" + `]|\s*$)`),
	}
}()

// containsLiteralSQL reports a statement typed into the template rather than
// referenced from somewhere.
func containsLiteralSQL(template string) bool {
	for _, pattern := range literalSQLPatterns {
		if pattern.MatchString(template) {
			return true
		}
	}
	return false
}

// validateClarificationForwarding keeps a sub-agent's questions answerable.
//
// When a sub-agent asks the user something, the answer comes back to the
// orchestrator and is handed to that step on resume - but only through the
// step's own request body. A step that does not forward
// params.clarification_answers re-runs with the question unanswered, and the
// agent either asks again or guesses: the observed failure was an agent reading
// a different option than the user picked. A missing key renders as null and is
// ignored, so forwarding it always is safe on the calls where nothing was asked.
func validateClarificationForwarding(steps map[string]*functions.FuncStep, allowedAgents []agents.DiscoveredAgent) []planIssue {
	if len(allowedAgents) == 0 {
		return nil
	}
	asks := map[string]bool{}
	for _, agent := range allowedAgents {
		if agent.SupportsClarification {
			asks[agent.AgentName] = true
		}
	}
	if len(asks) == 0 {
		return nil
	}

	var issues []planIssue
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		if step.AgentName == "" || !asks[step.AgentName] {
			return
		}
		if strings.Contains(step.TransformRequest, agents.ClarificationAnswersParamKey) {
			return
		}
		issues = append(issues, planIssue{
			StepPath: stepPath,
			Field:    "transform_request",
			Template: step.TransformRequest,
			Err: fmt.Sprint("agent \"", step.AgentName, "\" can ask the user a question, but this step does not forward the answer back. ",
				"Without it the agent re-runs with its question unanswered and guesses. Add it to the step's params: \"",
				agents.ClarificationAnswersParamKey, "\": {{stringify .Vars.OrgBody.params.", agents.ClarificationAnswersParamKey,
				"}} - it renders as null on the calls where nothing was asked, which the agent ignores."),
		})
	})
	return issues
}

// validateParamForwarding catches a forwarded param that would arrive in the
// wrong shape. The caller's params reach every agent through the call chain on
// their own; a step only names one to override it, and inside a dict wrapping
// it in stringify encodes it twice - the agent receives "\"auto\"", which
// matches no mode, and that explicit value beats the inherited one.
func validateParamForwarding(steps map[string]*functions.FuncStep, allowedAgents []agents.DiscoveredAgent, cc codeContext) []planIssue {
	if len(cc.ForwardParams) == 0 {
		return nil
	}
	var issues []planIssue
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		if step.AgentName == "" {
			return
		}
		template := step.TransformRequest
		if !strings.HasPrefix(strings.TrimSpace(template), "{{") {
			return
		}
		for _, name := range cc.ForwardParams {
			if strings.Contains(template, "(stringify .Vars.OrgBody.params."+name+")") {
				issues = append(issues, planIssue{
					StepPath: stepPath,
					Field:    "transform_request",
					Template: template,
					Err: fmt.Sprint("params.", name, " is wrapped in stringify inside a dict, so the agent receives it JSON-encoded twice. ",
						"The agent inherits the caller's ", name, " automatically - remove it from this step, or inside dict pass the value itself: \"", name, "\" .Vars.OrgBody.params.", name),
				})
			}
		}
	})
	return issues
}

var agentRequestKeys = []string{"content", "params", "files", "code", "conversation_id"}

// validateStepPayload checks each step's transform_request against the request
// contract of the agent or tool action it targets, so a plan that would silently
// drop its inputs is repaired before it runs rather than producing invented data.
func validateStepPayload(ctx context.Context, steps map[string]*functions.FuncStep, allowedAgents []agents.DiscoveredAgent, allowedTools []agents.DiscoveredTool) []planIssue {
	agentByName := make(map[string]agents.DiscoveredAgent, len(allowedAgents))
	for _, discovered := range allowedAgents {
		agentByName[discovered.AgentName] = discovered
	}
	toolByAction := make(map[string]agents.DiscoveredTool, len(allowedTools))
	for _, discovered := range allowedTools {
		toolByAction[fmt.Sprint(discovered.ToolName, ".", discovered.ActionName)] = discovered
	}

	var issues []planIssue
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		switch {
		case step.AgentName != "":
			agentSpec, known := agentByName[step.AgentName]
			if !known {
				return
			}
			issues = append(issues, validateAgentStepPayload(ctx, stepPath, step, agentSpec)...)
		case step.ToolName != "":
			toolSpec, known := toolByAction[fmt.Sprint(step.ToolName, ".", step.ToolAction)]
			if !known {
				return
			}
			issues = append(issues, validateToolStepPayload(ctx, stepPath, step, toolSpec)...)
		}
	})
	return issues
}

func validateAgentStepPayload(ctx context.Context, stepPath string, step *functions.FuncStep, agentSpec agents.DiscoveredAgent) []planIssue {
	template := strings.TrimSpace(step.TransformRequest)
	if template == "" {
		return []planIssue{{
			StepPath: stepPath,
			Field:    "transform_request",
			Err: fmt.Sprint("agent step has no transform_request - it is mandatory. Render the agent request body, e.g. ",
				`"{{stringify (dict \"content\" .Vars.OrgBody.content)}}"`),
		}}
	}
	if !strings.Contains(template, `"content"`) {
		return []planIssue{{
			StepPath: stepPath,
			Field:    "transform_request",
			Template: template,
			Err: fmt.Sprint("agent \"", agentSpec.AgentName, "\" is called without a \"content\" key - every agent request body must be a JSON object containing content. ",
				`Build it as {{stringify (dict \"content\" ...)}}`),
		}}
	}

	root := rootTemplateDict(ctx, stepPath, "transform_request", template)
	if root == nil || root.Dynamic || !root.HasKey("content") {
		return nil
	}

	var issues []planIssue
	for _, key := range root.Keys {
		if !containsString(agentRequestKeys, key) {
			issues = append(issues, planIssue{
				StepPath: stepPath,
				Field:    "transform_request",
				Template: template,
				Err: fmt.Sprint("\"", key, "\" is not part of the agent request body - the agent rejects unknown top-level keys. Allowed keys: ",
					strings.Join(agentRequestKeys, ", "), ". Move that value into content or into an accepted params key"),
			})
		}
	}

	paramsDict := root.Child("params")
	allowedParams := agentSpec.ParamKeys()
	if paramsDict == nil || paramsDict.Dynamic || len(allowedParams) == 0 {
		return issues
	}
	for _, key := range paramsDict.Keys {
		if containsString(allowedParams, key) {
			continue
		}
		issues = append(issues, planIssue{
			StepPath: stepPath,
			Field:    "transform_request",
			Template: template,
			Err: fmt.Sprint("agent \"", agentSpec.AgentName, "\" does not read params.", key,
				" - it is silently discarded and the agent never sees that data. Params keys it reads: ",
				strings.Join(allowedParams, ", "), ". Pass this value inside content instead"),
		})
	}
	return issues
}

func validateToolStepPayload(ctx context.Context, stepPath string, step *functions.FuncStep, toolSpec agents.DiscoveredTool) []planIssue {
	template := strings.TrimSpace(step.TransformRequest)
	if template == "" {
		return []planIssue{{
			StepPath: stepPath,
			Field:    "transform_request",
			Err: fmt.Sprint("tool step has no transform_request - it is mandatory. Render the action input inside a root params object, e.g. ",
				`"{{stringify (dict \"params\" (dict ...))}}"`),
		}}
	}
	if !strings.Contains(template, `"params"`) {
		return []planIssue{{
			StepPath: stepPath,
			Field:    "transform_request",
			Template: template,
			Err: fmt.Sprint("tool action \"", toolSpec.ToolName, ".", toolSpec.ActionName,
				"\" is called without a root \"params\" object - a tool request body must be {\"params\": { ...action input fields... }}"),
		}}
	}

	root := rootTemplateDict(ctx, stepPath, "transform_request", template)
	if root == nil || root.Dynamic || !root.HasKey("params") {
		return nil
	}

	var issues []planIssue
	for _, key := range root.Keys {
		if key == "params" {
			continue
		}
		issues = append(issues, planIssue{
			StepPath: stepPath,
			Field:    "transform_request",
			Template: template,
			Err:      fmt.Sprint("\"", key, "\" is not accepted at the root of a tool request body - only \"params\" is. Move it inside params if the action's input schema declares it"),
		})
	}

	paramsDict := root.Child("params")
	if paramsDict == nil || paramsDict.Dynamic || len(toolSpec.InputSchema.Required) == 0 {
		return issues
	}
	var missing []string
	for _, required := range toolSpec.InputSchema.Required {
		if !paramsDict.HasKey(required) {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		issues = append(issues, planIssue{
			StepPath: stepPath,
			Field:    "transform_request",
			Template: template,
			Err: fmt.Sprint("params is missing required field(s) ", strings.Join(missing, ", "), " of tool action \"",
				toolSpec.ToolName, ".", toolSpec.ActionName, "\" - add them inside the params object"),
		})
	}
	if len(toolSpec.InputSchema.Properties) > 0 {
		var unknown []string
		for _, key := range paramsDict.Keys {
			if _, ok := toolSpec.InputSchema.Properties[key]; !ok {
				unknown = append(unknown, key)
			}
		}
		if len(unknown) > 0 {
			issues = append(issues, planIssue{
				StepPath: stepPath,
				Field:    "transform_request",
				Template: template,
				Err: fmt.Sprint("params contains field(s) ", strings.Join(unknown, ", "), " that tool action \"",
					toolSpec.ToolName, ".", toolSpec.ActionName, "\" does not accept - its input schema declares: ",
					strings.Join(schemaPropertyNames(toolSpec.InputSchema), ", ")),
			})
		}
	}
	return issues
}

// rootTemplateDict returns the outermost dict(...) the template builds, or nil
// when the template does not build one or cannot be parsed. Callers must confirm
// the returned dict really is the request envelope before acting on it.
func rootTemplateDict(ctx context.Context, stepPath string, fieldName string, template string) *gotemplate.TemplateDict {
	goTmpl := gotemplate.GoTemplate{Name: fmt.Sprint(stepPath, ".", fieldName), Template: template}
	root, err := goTmpl.RootDict(ctx)
	if err != nil {
		return nil
	}
	return root
}

func schemaPropertyNames(schema eru_models.JSONSchema) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// validateCodeRouting checks how the plan handled the caller's existing
// structured output (params.code): a step may not read an artifact that was
// never sent, and the artifact must be passed by reference rather than pasted
// into a template, which bloats the plan and breaks on quoting.
func validateCodeRouting(ctx context.Context, steps map[string]*functions.FuncStep, cc codeContext) []planIssue {
	fingerprint := cc.fingerprint()
	var issues []planIssue
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		for _, templateField := range stepTemplateFields(step) {
			if strings.TrimSpace(templateField.Template) == "" {
				continue
			}
			if !cc.Present && templateReadsCodeParam(ctx, stepPath, templateField) {
				issues = append(issues, planIssue{
					StepPath: stepPath,
					Field:    templateField.Name,
					Template: templateField.Template,
					Err:      "reads .Vars.OrgBody.params.code but the request carried no code artifact - remove the code key from this step's params",
				})
			}
			if fingerprint != "" && strings.Contains(normaliseForFingerprint(templateField.Template), fingerprint) {
				issues = append(issues, planIssue{
					StepPath: stepPath,
					Field:    templateField.Name,
					Err:      "the existing artifact from params.code is pasted into this template - pass it by reference instead, as (dict \"code\" .Vars.OrgBody.params.code), and remove the pasted copy",
				})
			}
		}
	})
	return issues
}

// templateReadsCodeParam reports whether a template feeds the caller's
// params.code artifact into its step.
func validateUserRequestRoot(steps map[string]*functions.FuncStep) []planIssue {
	var issues []planIssue
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		if !readsStaleRequestRoot(step.TransformRequest) {
			return
		}
		issues = append(issues, planIssue{
			StepPath: stepPath,
			Field:    "transform_request",
			Template: step.TransformRequest,
			Err: fmt.Sprint(staleRequestRoot, " is the body of whichever step ran before this one, not the caller's request - below or after the first step it resolves to that step's own body and the caller's params are silently gone. ",
				"Read the caller's request as ", userRequestRoot, " instead: replace ", staleRequestRoot, " with ", userRequestRoot, " in this template."),
		})
	})
	return issues
}

const staleRequestRoot = ".Vars.Body"

func readsStaleRequestRoot(template string) bool {
	for _, field := range []string{".content", ".params", ".files"} {
		if strings.Contains(template, staleRequestRoot+field) {
			return true
		}
	}
	return false
}

func templateReadsCodeParam(ctx context.Context, stepPath string, templateField stepTemplateField) bool {
	goTmpl := gotemplate.GoTemplate{Name: fmt.Sprint(stepPath, ".", templateField.Name), Template: templateField.Template}
	refs, err := goTmpl.FieldReferences(ctx)
	if err != nil {
		return strings.Contains(templateField.Template, fmt.Sprint("params.", codeParamKey))
	}
	for _, ref := range refs {
		if len(ref) >= 4 && ref[0] == "Vars" && (ref[1] == "OrgBody" || ref[1] == "Body") && ref[2] == "params" && ref[3] == codeParamKey {
			return true
		}
	}
	return false
}

// codeRoutedSteps lists the steps that receive the params.code artifact, so the
// routing decision the planner made is visible in the logs.
func codeRoutedSteps(ctx context.Context, plan map[string]interface{}) []string {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil
	}
	var funcGroup functions.FuncGroup
	if err := json.Unmarshal(planJSON, &funcGroup); err != nil {
		return nil
	}
	var routed []string
	walkSteps(funcGroup.FuncSteps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		for _, templateField := range stepTemplateFields(step) {
			if strings.TrimSpace(templateField.Template) == "" {
				continue
			}
			if templateReadsCodeParam(ctx, stepPath, templateField) {
				routed = append(routed, stepPath)
				return
			}
		}
	})
	return routed
}

// validateStepReferences rejects a template that reads .ResVars/.ReqVars of a
// step that does not exist in the plan. A missing key renders as nil rather than
// failing, so without this check the step silently receives null data.
func validateStepReferences(ctx context.Context, steps map[string]*functions.FuncStep) []planIssue {
	stepKeys := make(map[string]bool)
	collectStepKeys(steps, stepKeys)

	var issues []planIssue
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		for _, templateField := range stepTemplateFields(step) {
			if strings.TrimSpace(templateField.Template) == "" {
				continue
			}
			goTmpl := gotemplate.GoTemplate{Name: fmt.Sprint(stepPath, ".", templateField.Name), Template: templateField.Template}
			refs, err := goTmpl.FieldReferences(ctx)
			if err != nil {
				continue
			}
			reported := make(map[string]bool)
			for _, ref := range refs {
				if len(ref) < 2 {
					continue
				}
				if ref[0] != "ResVars" && ref[0] != "ReqVars" {
					continue
				}
				if stepKeys[ref[1]] || reported[ref[1]] {
					continue
				}
				reported[ref[1]] = true
				issues = append(issues, planIssue{
					StepPath: stepPath,
					Field:    templateField.Name,
					Template: templateField.Template,
					Err: fmt.Sprint(".", ref[0], ".", ref[1], " does not exist - there is no step named \"", ref[1],
						"\" in this plan. Valid step keys are: ", strings.Join(sortedKeys(stepKeys), ", "),
						". Reference the step by its exact func_steps key"),
				})
			}
		}
	})
	return issues
}

// validateStepKeyUniqueness rejects a step key used more than once anywhere in
// the plan. Step results live in one flat namespace keyed by the step key
// (eru-functions merges every branch's vars back by bare key), so duplicates
// overwrite each other and downstream .ResVars references resolve to whichever
// branch finished last.
func validateStepKeyUniqueness(steps map[string]*functions.FuncStep) []planIssue {
	paths := make(map[string][]string)
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		paths[stepKey] = append(paths[stepKey], stepPath)
	})
	var issues []planIssue
	for _, stepKey := range sortedKeys(duplicateKeySet(paths)) {
		issues = append(issues, planIssue{
			StepPath: strings.Join(paths[stepKey], ", "),
			Field:    "func_steps",
			Err: fmt.Sprint("step key \"", stepKey, "\" is used ", len(paths[stepKey]),
				" times in this plan - step keys share one flat namespace, so these steps overwrite each other. Give each occurrence a unique key by appending a numeric suffix (\"",
				stepKey, "2\", \"", stepKey, "3\") and update every .ResVars/.ReqVars/wait_for reference to match"),
		})
	}
	return issues
}

func duplicateKeySet(paths map[string][]string) map[string]bool {
	duplicates := make(map[string]bool)
	for stepKey, stepPaths := range paths {
		if len(stepPaths) > 1 {
			duplicates[stepKey] = true
		}
	}
	return duplicates
}

// validateStepIdentity enforces the step-key naming rule and that every step
// uses an agent or tool the orchestrator is allowed to delegate to.
func validateStepIdentity(steps map[string]*functions.FuncStep, allowedAgents []agents.DiscoveredAgent, allowedTools []agents.DiscoveredTool) []planIssue {
	agentNames := make(map[string]bool)
	for _, discovered := range allowedAgents {
		agentNames[discovered.AgentName] = true
	}
	toolActions := make(map[string][]string)
	for _, discovered := range allowedTools {
		toolActions[discovered.ToolName] = append(toolActions[discovered.ToolName], discovered.ActionName)
	}

	var issues []planIssue
	walkSteps(steps, "", func(stepPath string, stepKey string, step *functions.FuncStep) {
		switch {
		case step.AgentName != "":
			if step.ToolName != "" {
				issues = append(issues, planIssue{StepPath: stepPath, Field: "agent_name", Err: fmt.Sprint("step sets both agent_name (", step.AgentName, ") and tool_name (", step.ToolName, ") - a step is either an agent step or a tool step")})
			}
			if !stepKeyMatches(stepKey, step.AgentName) {
				issues = append(issues, planIssue{StepPath: stepPath, Field: "agent_name", Err: fmt.Sprint("func_step key \"", stepKey, "\" must equal agent_name \"", step.AgentName, "\" (a numeric suffix is allowed for duplicates, e.g. \"", step.AgentName, "2\")")})
			}
			if len(agentNames) > 0 && !agentNames[step.AgentName] {
				issues = append(issues, planIssue{StepPath: stepPath, Field: "agent_name", Err: fmt.Sprint("agent \"", step.AgentName, "\" is not available - use one of: ", strings.Join(sortedKeys(agentNames), ", "))})
			}
		case step.ToolName != "":
			if step.ToolAction == "" {
				issues = append(issues, planIssue{StepPath: stepPath, Field: "tool_action", Err: fmt.Sprint("tool step \"", stepKey, "\" is missing tool_action")})
			}
			if !stepKeyMatches(stepKey, step.ToolName) && !stepKeyMatches(stepKey, fmt.Sprint(step.ToolName, "_", step.ToolAction)) {
				issues = append(issues, planIssue{StepPath: stepPath, Field: "tool_name", Err: fmt.Sprint("func_step key \"", stepKey, "\" must equal \"", step.ToolName, "\" or \"", step.ToolName, "_", step.ToolAction, "\"")})
			}
			if len(toolActions) > 0 {
				actions, found := toolActions[step.ToolName]
				if !found {
					issues = append(issues, planIssue{StepPath: stepPath, Field: "tool_name", Err: fmt.Sprint("tool \"", step.ToolName, "\" is not available - use one of: ", strings.Join(sortedKeys(toolActionNames(toolActions)), ", "))})
				} else if step.ToolAction != "" && !containsString(actions, step.ToolAction) {
					issues = append(issues, planIssue{StepPath: stepPath, Field: "tool_action", Err: fmt.Sprint("action \"", step.ToolAction, "\" is not allowed on tool \"", step.ToolName, "\" - allowed actions: ", strings.Join(actions, ", "))})
				}
			}
		default:
			issues = append(issues, planIssue{StepPath: stepPath, Field: "step", Err: "step has neither agent_name nor tool_name - the orchestrator may only delegate to agents and tools"})
		}
	})
	return issues
}

// stepKeyMatches allows the documented numeric suffix for duplicate steps at the
// same level ("classifier", "classifier2").
func stepKeyMatches(stepKey string, name string) bool {
	if name == "" {
		return false
	}
	if stepKey == name {
		return true
	}
	if !strings.HasPrefix(stepKey, name) {
		return false
	}
	suffix := strings.TrimPrefix(stepKey, name)
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return suffix != ""
}

type stepTemplateField struct {
	Name     string
	Template string
}

func stepTemplateFields(step *functions.FuncStep) []stepTemplateField {
	fields := []stepTemplateField{
		{"transform_request", step.TransformRequest},
		{"transform_response", step.TransformResponse},
		{"condition", step.Condition},
		{"condition_fail_message", step.ConditionFailMessage},
		{"loop_variable", step.LoopVariable},
		{"async_message", step.AsyncMessage},
	}
	headerFields := []struct {
		Name    string
		Headers []functions.Headers
	}{
		{"request_headers", step.RequestHeaders},
		{"query_params", step.QueryParams},
		{"form_data", step.FormData},
		{"response_headers", step.ResponseHeaders},
	}
	for _, headerField := range headerFields {
		for i, header := range headerField.Headers {
			if !header.IsTemplate {
				continue
			}
			fields = append(fields, stepTemplateField{fmt.Sprint(headerField.Name, "[", i, "].value"), header.Value})
		}
	}
	return fields
}

func collectStepKeys(steps map[string]*functions.FuncStep, keys map[string]bool) {
	for stepKey, step := range steps {
		keys[stepKey] = true
		if step != nil {
			collectStepKeys(step.FuncSteps, keys)
		}
	}
}

func walkSteps(steps map[string]*functions.FuncStep, parentPath string, visit func(stepPath string, stepKey string, step *functions.FuncStep)) {
	for _, stepKey := range sortedStepKeys(steps) {
		step := steps[stepKey]
		if step == nil {
			continue
		}
		stepPath := stepKey
		if parentPath != "" {
			stepPath = fmt.Sprint(parentPath, ".", stepKey)
		}
		visit(stepPath, stepKey, step)
		walkSteps(step.FuncSteps, stepPath, visit)
	}
}

func sortedStepKeys(steps map[string]*functions.FuncStep) []string {
	stepKeys := make([]string, 0, len(steps))
	for stepKey := range steps {
		stepKeys = append(stepKeys, stepKey)
	}
	sort.Strings(stepKeys)
	return stepKeys
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func toolActionNames(toolActions map[string][]string) map[string]bool {
	names := make(map[string]bool)
	for toolName := range toolActions {
		names[toolName] = true
	}
	return names
}

func containsString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func validateStepTemplates(ctx context.Context, parentPath string, steps map[string]*functions.FuncStep) []planIssue {
	var issues []planIssue
	walkSteps(steps, parentPath, func(stepPath string, stepKey string, step *functions.FuncStep) {
		for _, templateField := range stepTemplateFields(step) {
			if issue, ok := validateStepTemplate(ctx, stepPath, templateField.Name, templateField.Template); !ok {
				issues = append(issues, issue)
			}
		}
	})
	return issues
}

func validateStepTemplate(ctx context.Context, stepPath string, fieldName string, templateString string) (planIssue, bool) {
	if strings.TrimSpace(templateString) == "" {
		return planIssue{}, true
	}
	goTmpl := gotemplate.GoTemplate{Name: fmt.Sprint(stepPath, ".", fieldName), Template: templateString}
	if err := goTmpl.Validate(ctx); err != nil {
		return planIssue{
			StepPath: stepPath,
			Field:    fieldName,
			Template: templateString,
			Err:      err.Error() + templateParseRemedy(templateString),
		}, false
	}
	return planIssue{}, true
}

// templateParseRemedy turns a Go template parse error into something the planner
// can act on.
//
// A bare parse error makes the model rewrite the same nested "stringify (dict
// ...)" expression and mis-balance it somewhere else - observed three times in a
// row on one request, each attempt failing at a different paren. Naming the
// actual cause, and pointing at the JSON body form that avoids the nesting
// altogether, is what breaks that loop.
func templateParseRemedy(templateString string) string {
	var remedies []string

	if start := strings.Index(templateString, "{{"); start >= 0 {
		if action, ok := firstTemplateAction(templateString); ok {
			if literalNewlineInQuotes(action) {
				remedies = append(remedies, "The template has a real line break inside a quoted string. A Go template string literal cannot span lines: write \\n instead of an actual newline.")
			}
			if strings.Contains(action, "}") {
				remedies = append(remedies, "There is a \"}\" inside the {{...}} action: a dict( was closed with } instead of ). Braces are JSON; a template action closes with ).")
			}
			if opens, closes := strings.Count(action, "("), strings.Count(action, ")"); opens != closes {
				remedies = append(remedies, fmt.Sprint("The {{...}} action has ", opens, " \"(\" and ", closes, " \")\" - the parentheses do not balance. Every dict( must be closed before the action ends."))
			}
		}
	}

	remedies = append(remedies, "Do NOT retry the same nested \"{{stringify (dict ...)}}\" expression - rewriting it tends to mis-balance somewhere else. "+
		"Write the request body as JSON instead, with {{...}} only where a value has to be interpolated. That is a valid template and there is nothing to balance:\n"+
		`  "transform_request": "{\"content\": \"your instruction, with \\n for line breaks\", \"params\": {\"code\": {{stringify .Vars.OrgBody.params.code}}}}"`+"\n"+
		"Use the JSON form whenever the content is long, spans lines, or contains quotes.")

	return "\n    " + strings.Join(remedies, "\n    ")
}

// firstTemplateAction returns the text inside the first {{ }} of a template.
func firstTemplateAction(templateString string) (string, bool) {
	start := strings.Index(templateString, "{{")
	if start < 0 {
		return "", false
	}
	end := strings.Index(templateString[start:], "}}")
	if end < 0 {
		// An unterminated action: everything after {{ is the action.
		return templateString[start+2:], true
	}
	return templateString[start+2 : start+end], true
}

// literalNewlineInQuotes reports a real line break inside a quoted string, which
// Go's template lexer rejects.
func literalNewlineInQuotes(action string) bool {
	inQuotes := false
	escaped := false
	for _, r := range action {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
		case (r == '\n' || r == '\r') && inQuotes:
			return true
		}
	}
	return false
}
