package orchestrator

import (
	"context"
	"fmt"
	"strings"

	tools "github.com/eru-os/eru/eru-ai/tools"
	utility "github.com/eru-os/eru/eru-ai/tools/utility"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// The planner used to decide everything from the words in the prompt. It could
// not look anything up, so when a plan needed a fact about the tenant - which
// entities exist, what a field is called - it either guessed or planned a step
// to go and find out with whatever blunt instrument it had. That is how a
// hand-written SELECT against an invented table ended up standing in for a
// purpose-built metadata lookup.
//
// These are the lookups a planner may make before it commits. They are strictly
// read-only and there are deliberately few of them: research that costs a model
// call per plan has to earn its place, and a planner given every tool starts
// doing the work instead of planning it.

type DelegateToolBuilder func(ctx context.Context, delegate tools.Tooling) tools.Tooling

type researchTool struct {
	build    DelegateToolBuilder
	guidance string
}

var researchToolBuilders = map[string]researchTool{}

func RegisterResearchTool(toolName string, guidance string, build DelegateToolBuilder) {
	researchToolBuilders[toolName] = researchTool{build: build, guidance: guidance}
}

// researchTools are the read-only lookups offered during planning. It is empty
// when nothing backs them, and the planner is then told nothing about them -
// the same graceful degradation the page agent uses, because a model told about
// a tool it does not have will try it, fail, and spend an iteration finding out.
func (oa *OrchestratorAgent) researchTools(ctx context.Context) map[string]tools.Tooling {
	research := map[string]tools.Tooling{}

	if delegate := oa.eruqlDelegate(ctx); delegate != nil {
		for toolName, rt := range researchToolBuilders {
			research[toolName] = rt.build(ctx, delegate)
		}
	}

	return research
}

// eruqlDelegate finds an attached eru-ql tool to run a lookup through.
func (oa *OrchestratorAgent) eruqlDelegate(ctx context.Context) tools.Tooling {
	for _, attached := range oa.AgentTools {
		if utility.IsEruqlTool(ctx, attached.Tool) {
			return attached.Tool
		}
	}
	return nil
}

// researchGuidance is added to the planning prompt only when a lookup is
// actually available.
func researchGuidance(available map[string]tools.Tooling) string {
	var guidance strings.Builder
	for toolName := range available {
		if rt, ok := researchToolBuilders[toolName]; ok {
			guidance.WriteString(rt.guidance)
		}
	}
	return guidance.String()
}

// executeResearchTool runs one of the planner's lookups.
func executeResearchTool(ctx context.Context, research map[string]tools.Tooling, toolName string, projectId string, tenantId string, input map[string]interface{}) (map[string]interface{}, error, bool) {
	tool, ok := research[toolName]
	if !ok || tool == nil {
		return nil, nil, false
	}
	logs.WithContext(ctx).Info(fmt.Sprint("planner research lookup: ", toolName))
	result, _, err := tool.Execute(ctx, projectId, tenantId, toolName, input)
	return result, err, true
}
