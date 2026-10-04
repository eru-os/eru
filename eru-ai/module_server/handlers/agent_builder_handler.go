package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	agents "github.com/eru-os/eru/eru-ai/agents"
	"github.com/eru-os/eru/eru-ai/agents/agents_factory"
	"github.com/eru-os/eru/eru-ai/agents/agentspec"
	"github.com/eru-os/eru/eru-ai/agents/ruleset"
	"github.com/eru-os/eru/eru-ai/module_store"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/gorilla/mux"
)

// BuilderModel is a model as the agent builder lists it.
type BuilderModel struct {
	ModelName string `json:"model_name"`
	Provider  string `json:"provider"`
	LLMName   string `json:"llm_name"`
}

// BuilderAction is one action a tool offers.
type BuilderAction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// BuilderTool is a tool as the agent builder lists it: enough to pick it and
// one of its actions.
type BuilderTool struct {
	ToolName    string          `json:"tool_name"`
	ToolType    string          `json:"tool_type"`
	Description string          `json:"description"`
	Actions     []BuilderAction `json:"actions"`
}

// BuilderAgent is another agent, for orchestrator pickers.
type BuilderAgent struct {
	AgentName   string `json:"agent_name"`
	AgentType   string `json:"agent_type"`
	Description string `json:"description"`
}

// BuilderContext is everything in a tenant an agent config can refer to.
type BuilderContext struct {
	Models       []BuilderModel `json:"models"`
	Tools        []BuilderTool  `json:"tools"`
	Agents       []BuilderAgent `json:"agents"`
	VectorStores []string       `json:"vector_stores"`
}

// AgentCatalogHandler serves the agent configuration language.
func AgentCatalogHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(agentspec.Get())
	}
}

// ModelListHandler lists the models configured for a tenant, including those
// it inherits from the project.
func ModelListHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		prj, err := sh.Store.GetProjectConfig(r.Context(), vars["project"])
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		seen := map[string]bool{}
		models := make([]interface{}, 0)
		for _, tid := range eru_utils.TenantLookupOrder(r.Context(), vars["tenant"], vars["project"]) {
			tenant, ok := prj.Tenants[tid]
			if !ok {
				continue
			}
			names := make([]string, 0, len(tenant.Models))
			for name := range tenant.Models {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if seen[name] {
					continue
				}
				seen[name] = true
				models = append(models, tenant.Models[name])
			}
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"models": models})
	}
}

// AgentBuilderContextHandler lists what an agent in this tenant can refer to.
func AgentBuilderContextHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		builderContext, err := LoadBuilderContext(r.Context(), sh, vars["project"], vars["tenant"])
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(builderContext)
	}
}

// LoadBuilderContext gathers models, tools and agents visible to a tenant.
func LoadBuilderContext(ctx context.Context, sh *module_store.StoreHolder, projectId string, tenantId string) (BuilderContext, error) {
	out := BuilderContext{Models: []BuilderModel{}, Tools: []BuilderTool{}, Agents: []BuilderAgent{}, VectorStores: []string{}}
	if names, vErr := sh.Store.GetVectorStoreNames(ctx, projectId, tenantId); vErr == nil {
		sort.Strings(names)
		out.VectorStores = append(out.VectorStores, names...)
	}
	prj, err := sh.Store.GetProjectConfig(ctx, projectId)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, tid := range eru_utils.TenantLookupOrder(ctx, tenantId, projectId) {
		tenant, ok := prj.Tenants[tid]
		if !ok {
			continue
		}
		for name, model := range tenant.Models {
			if seen[name] {
				continue
			}
			seen[name] = true
			entry := BuilderModel{ModelName: name}
			if v, e := model.GetAttribute(ctx, "provider"); e == nil {
				entry.Provider = fmt.Sprint(v)
			}
			if v, e := model.GetAttribute(ctx, "llm_name"); e == nil {
				entry.LLMName = fmt.Sprint(v)
			}
			out.Models = append(out.Models, entry)
		}
	}
	sort.Slice(out.Models, func(i, j int) bool { return out.Models[i].ModelName < out.Models[j].ModelName })

	toolNames, err := sh.Store.GetToolNames(ctx, projectId, tenantId)
	if err != nil {
		return out, err
	}
	sort.Strings(toolNames)
	for _, name := range toolNames {
		tool, tErr := sh.Store.GetTool(ctx, projectId, tenantId, name, "", sh.Store)
		if tErr != nil {
			logs.WithContext(ctx).Info(fmt.Sprintf("builder context: skipping tool %s: %v", name, tErr))
			continue
		}
		entry := BuilderTool{ToolName: name, Actions: []BuilderAction{}}
		if spec, mErr := json.Marshal(tool.GetSpec()); mErr == nil {
			var head struct {
				ToolType    string `json:"tool_type"`
				Description string `json:"description"`
			}
			_ = json.Unmarshal(spec, &head)
			entry.ToolType, entry.Description = head.ToolType, head.Description
		}
		for _, action := range tool.GetActionsList() {
			entry.Actions = append(entry.Actions, BuilderAction{Name: action.Name, Description: action.Description})
		}
		out.Tools = append(out.Tools, entry)
	}

	// Read from the loaded config rather than through GetAgent, which also
	// resolves the agent's tools and model and fails for an agent whose model
	// has been deleted - exactly the agent a builder needs to be able to list.
	stored, err := StoredAgents(ctx, sh, projectId, tenantId)
	if err != nil {
		return out, err
	}
	for _, agent := range stored {
		entry := BuilderAgent{}
		if v, e := agent.GetAttribute(ctx, "agent_name"); e == nil {
			entry.AgentName = fmt.Sprint(v)
		}
		if v, e := agent.GetAttribute(ctx, "agent_type"); e == nil {
			entry.AgentType = fmt.Sprint(v)
		}
		if v, e := agent.GetAttribute(ctx, "description"); e == nil {
			entry.Description = fmt.Sprint(v)
		}
		out.Agents = append(out.Agents, entry)
	}
	sort.Slice(out.Agents, func(i, j int) bool { return out.Agents[i].AgentName < out.Agents[j].AgentName })
	return out, nil
}

// StoredAgents returns the agents visible to a tenant as they are held in the
// loaded config, nearest tenant first, sorted by name.
func StoredAgents(ctx context.Context, sh *module_store.StoreHolder, projectId string, tenantId string) ([]agents.AgentI, error) {
	prj, err := sh.Store.GetProjectConfig(ctx, projectId)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var names []string
	byName := map[string]agents.AgentI{}
	for _, tid := range eru_utils.TenantLookupOrder(ctx, tenantId, projectId) {
		tenant, ok := prj.Tenants[tid]
		if !ok {
			continue
		}
		for name, agent := range tenant.Agents {
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
			byName[name] = agent
		}
	}
	sort.Strings(names)
	out := make([]agents.AgentI, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out, nil
}

// Environment turns a builder context into what the validator checks against.
func (b BuilderContext) Environment() *agentspec.Environment {
	env := &agentspec.Environment{Models: []string{}, Tools: map[string][]string{}, Agents: []string{}, VectorStores: b.VectorStores}
	for _, model := range b.Models {
		env.Models = append(env.Models, model.ModelName)
	}
	for _, tool := range b.Tools {
		actions := make([]string, 0, len(tool.Actions))
		for _, action := range tool.Actions {
			actions = append(actions, action.Name)
		}
		env.Tools[tool.ToolName] = actions
	}
	for _, agent := range b.Agents {
		env.Agents = append(env.Agents, agent.AgentName)
	}
	return env
}

// AgentValidateHandler checks a config without saving it: first exactly as the
// save handler would decode it, then against the catalog and the tenant.
func AgentValidateHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		var config map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		var env *agentspec.Environment
		if builderContext, err := LoadBuilderContext(r.Context(), sh, vars["project"], vars["tenant"]); err == nil {
			env = builderContext.Environment()
		}
		result := ValidateAgentConfig(r.Context(), config, env)
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// ValidateAgentConfig runs the save path's decode and the catalog checks.
func ValidateAgentConfig(ctx context.Context, config map[string]interface{}, env *agentspec.Environment) agentspec.Result {
	result := agentspec.Validate(config, env)
	agentType, _ := config["agent_type"].(string)
	// When the catalog checks already found errors, the server's own refusal
	// is the same complaint in worse words; only report it for what they miss.
	if _, known := agentspec.TypeFor(agentType); !known || !result.Valid {
		return result
	}
	raw, err := json.Marshal(config)
	if err == nil {
		agentObj := agents_factory.GetAgent(agentType)
		agentRaw := json.RawMessage(raw)
		if err = agentObj.MakeFromJson(ctx, &agentRaw); err == nil {
			err = eru_utils.ValidateStruct(ctx, agentObj, "")
		}
	}
	if err != nil {
		result.Valid = false
		result.Issues = append([]agentspec.Issue{{Path: "", Severity: agentspec.SeverityError, Code: "server_rejects",
			Message: "the server would refuse to save this: " + err.Error()}}, result.Issues...)
	}
	return result
}

// RuleScorecardRequest is a rule set and an answer to judge it against.
type RuleScorecardRequest struct {
	ValidationRules []ruleset.RuleSet      `json:"validation_rules"`
	Answer          map[string]interface{} `json:"answer"`
}

// RuleScorecardHandler reports every rule as passed, failed, idle or needing a
// real run, against an answer the caller supplies - a test run's output or a
// sample pasted by hand. No model is called.
func RuleScorecardHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RuleScorecardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		scores := agentspec.Scorecard(req.ValidationRules, req.Answer)
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"scores": scores, "summary": agentspec.ScoreSummary(scores)})
	}
}

// ToolConnection is one saved tool as the tools screen lists it.
type ToolConnection struct {
	ToolName string `json:"tool_name"`
	ToolType string `json:"tool_type"`
	// Inherited is true for a tool saved at the project and shared into this
	// tenant: the tenant can use it but cannot edit or remove it.
	Inherited bool `json:"inherited"`
	// UsedBy names the agents in this tenant that attach the tool or let an
	// orchestrator plan with it. Workflows in eru-functions are not counted.
	UsedBy []string `json:"used_by"`
}

// ToolOverviewHandler lists a tenant's tool connections with where each one
// comes from and which agents depend on it, so a screen can refuse to delete a
// tool that agents still call.
func ToolOverviewHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		projectId, tenantId := vars["project"], vars["tenant"]
		prj, err := sh.Store.GetProjectConfig(r.Context(), projectId)
		if err != nil {
			server_handlers.FormatResponse(w, 400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		usedBy := map[string][]string{}
		if stored, sErr := StoredAgents(r.Context(), sh, projectId, tenantId); sErr == nil {
			for _, agent := range stored {
				raw, mErr := json.Marshal(agent.GetSpec())
				if mErr != nil {
					continue
				}
				var spec struct {
					AgentName  string `json:"agent_name"`
					AgentTools []struct {
						ToolName string `json:"tool_name"`
					} `json:"agent_tools"`
					AvailableTools []struct {
						ToolName string `json:"tool_name"`
					} `json:"available_tools"`
				}
				if json.Unmarshal(raw, &spec) != nil {
					continue
				}
				seen := map[string]bool{}
				for _, t := range spec.AgentTools {
					seen[t.ToolName] = true
				}
				for _, t := range spec.AvailableTools {
					seen[t.ToolName] = true
				}
				for name := range seen {
					usedBy[name] = append(usedBy[name], spec.AgentName)
				}
			}
		}
		out := []ToolConnection{}
		seen := map[string]bool{}
		for _, tid := range eru_utils.TenantLookupOrder(r.Context(), tenantId, projectId) {
			tenant, ok := prj.Tenants[tid]
			if !ok {
				continue
			}
			names := make([]string, 0, len(tenant.Tools))
			for name := range tenant.Tools {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if seen[name] {
					continue
				}
				seen[name] = true
				conn := ToolConnection{ToolName: name, Inherited: tid != tenantId, UsedBy: []string{}}
				if v, e := tenant.Tools[name].GetAttribute(r.Context(), "tool_type"); e == nil {
					conn.ToolType = fmt.Sprint(v)
				}
				if agents := usedBy[name]; agents != nil {
					sort.Strings(agents)
					conn.UsedBy = agents
				}
				out = append(out, conn)
			}
		}
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"tools": out})
	}
}
