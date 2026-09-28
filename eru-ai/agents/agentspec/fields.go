package agentspec

const (
	TypeReasoning    = "REASONING"
	TypeOrchestrator = "ORCHESTRATOR"
	TypeReflex       = "REFLEX"
)

var types = []TypeSpec{
	{Type: TypeReasoning, Label: "Reasoning", Family: "reasoning", Iterative: true, Creatable: true, Icon: "psychology",
		Summary: "Thinks and calls tools in a loop until its answer passes every rule. The right choice for almost every agent."},
	{Type: TypeOrchestrator, Label: "Orchestrator", Family: "orchestrator", Iterative: true, Creatable: true, Icon: "account_tree",
		Summary: "Plans a task across other agents and tools, runs the plan, and writes one answer from the results."},
	{Type: TypeReflex, Label: "Reflex", Family: "reflex", Creatable: true, Icon: "bolt",
		Summary: "Runs each of its tools once, in order, then answers in a single pass. Fast and predictable; no rules, no loop."},
}

var (
	reasoningFamily = []string{TypeReasoning, TypeOrchestrator}
	reflexFamily    = []string{TypeReflex}
)

func because(why string, agentTypes ...[]string) map[string]string {
	out := map[string]string{}
	for _, list := range agentTypes {
		for _, t := range list {
			out[t] = why
		}
	}
	return out
}

func merge(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

var orchestratorOnly = []string{TypeOrchestrator}

const (
	whySpecialistSchema  = "This specialist brings its own output schema, which replaces this one."
	whyReflexNoRules     = "Reflex agents answer in one pass and never check their answer."
	whyOrchestratorRules = "The orchestrator does not run the answer-checking loop; put rules on the sub-agents."
)

var rulesIgnored = merge(because(whyReflexNoRules, reflexFamily), because(whyOrchestratorRules, orchestratorOnly))

var groups = []GroupSpec{
	{Group: "basics", Label: "Basics", Icon: "badge", Level: "basic", Summary: "What the agent is called, what kind it is, and which model runs it."},
	{Group: "instructions", Label: "Instructions", Icon: "description", Level: "basic", Summary: "What the agent is for and how it should work."},
	{Group: "tools", Label: "Tools", Icon: "construction", Level: "basic", Summary: "What the agent can look up or do."},
	{Group: "output", Label: "Output", Icon: "data_object", Level: "basic", Summary: "The shape of the answer. Leave empty for a free-text reply."},
	{Group: "rules", Label: "Rules", Icon: "rule", Level: "advanced", Summary: "Checks the answer must pass. A failed check is sent back to the model with your message."},
	{Group: "evidence", Label: "Evidence", Icon: "fact_check", Level: "advanced", Summary: "Values to remember from tool calls, so a rule can require the answer to use only what was really seen."},
	{Group: "claims", Label: "Claims", Icon: "verified", Level: "advanced", Summary: "Hold the answer to what it says it did, against the calls that actually ran."},
	{Group: "orchestration", Label: "Orchestration", Icon: "account_tree", Level: "basic", Summary: "Which agents and tools the planner may use, and how it writes the final answer."},
	{Group: "limits", Label: "Limits", Icon: "speed", Level: "advanced", Summary: "Retries, loop length and a hard budget for one run."},
	{Group: "guardrails", Label: "Guardrails", Icon: "shield", Level: "advanced", Summary: "Rules of conduct stated as non-negotiable, appended last to the prompt."},
	{Group: "memory", Label: "Memory", Icon: "memory", Level: "advanced", Summary: "Whether the agent remembers a conversation, and where."},
	{Group: "specialist", Label: "Specialist", Icon: "tune", Level: "advanced", Summary: "Settings only one specialist type reads."},
}

var redisTypes = []string{"REDIS", "REDIS_CLUSTER", "ELASTICACHE"}

var fields = []FieldSpec{
	{Key: "agent_name", Label: "Name", Kind: "string", Required: true, Pattern: "^[a-zA-Z0-9_-]+$", Group: "basics", Level: "basic",
		Help: "Unique in this workspace, and how other agents, tools and the API refer to it. Letters, digits, _ and - only. Cannot be changed later."},
	{Key: "agent_type", Label: "Type", Kind: "enum", Required: true, Default: TypeReasoning, Group: "basics", Level: "basic",
		Help: "How the agent runs. Reasoning suits almost everything; Orchestrator coordinates other agents."},
	{Key: "description", Label: "Description", Kind: "text", Group: "basics", Level: "basic",
		Help: "One or two sentences on what it does. Planners and other agents read this to decide whether to call it, so say what it is FOR."},
	{Key: "model", Label: "Model", Kind: "ref_model", Required: true, Group: "basics", Level: "basic",
		Help: "A model configured in this workspace."},
	{Key: "is_system", Label: "Hide from lists", Kind: "bool", Group: "basics", Level: "advanced",
		Help: "System agents are left out of agent lists and pickers unless asked for."},

	{Key: "system_prompt", Label: "Instructions", Kind: "text", Group: "instructions", Level: "basic",
		Help: "The agent's brief: its job, how to approach it, what good looks like. Rules and guardrails are added after this automatically.",
		Ignored: merge(
			because("The orchestrator plans with a built-in prompt. Use the synthesis prompt and guardrails to steer it.", orchestratorOnly),
		)},
	{Key: "guardrail_prompt", Label: "Guardrails", Kind: "text", Group: "guardrails", Level: "advanced",
		Help: "Conduct that must never be broken - what not to do, what never to reveal. Framed as non-negotiable and placed last in the prompt."},

	{Key: "agent_tools", Label: "Tools", Kind: "array", Group: "tools", Level: "basic",
		Help: "Tools the agent may call. Attach the same tool more than once to give it several actions, each with its own key.",
		Children: []FieldSpec{
			{Key: "tool_name", Label: "Tool", Kind: "ref_tool", Required: true, Level: "basic", Help: "A tool configured in this workspace."},
			{Key: "action_name", Label: "Action", Kind: "ref_action", Level: "basic", Help: "Which of the tool's actions the agent may call; the model sees that action's parameters."},
			{Key: "tool_key", Label: "Name shown to the model", Kind: "string", Level: "basic",
				Help: "What the model calls this tool. Required to be distinct when one tool is attached more than once; defaults to the tool name."},
			{Key: "effect", Label: "What it does", Kind: "object", Level: "advanced",
				Help: "Declaring effects lets the agent avoid repeating a write when it retries.",
				Children: []FieldSpec{
					{Key: "read_only", Label: "Read only", Kind: "bool", Level: "advanced", Help: "The call changes nothing - a lookup or a list."},
					{Key: "idempotent", Label: "Safe to repeat", Kind: "bool", Level: "advanced", Help: "Calling twice with the same arguments does the same as calling once."},
					{Key: "destructive", Label: "Destructive", Kind: "bool", Level: "advanced", Help: "The call removes or overwrites data. Declared for visibility; nothing enforces it yet."},
				}},
			{Key: "action_prompt", Label: "When to call it", Kind: "text", Level: "advanced",
				Help:    "Reflex agents: the instruction for this tool call.",
				Ignored: because("Only reflex agents read this. A reasoning agent decides when to call a tool from its instructions and the tool's description.", reasoningFamily)},
			{Key: "tool_output_type", Label: "Result as", Kind: "enum", Enum: []string{"string", "json"}, Default: "string", Level: "advanced",
				Help:    "Reflex agents: keep the result as text, or merge it as JSON.",
				Ignored: because("Only reflex agents read this.", reasoningFamily)},
			{Key: "dependent_tools", Label: "Then run", Kind: "array", Level: "advanced",
				Help:    "Reflex agents: tools that run after this one, with its result available.",
				Ignored: because("Only reflex agents run dependent tools.", reasoningFamily)},
		}},

	{Key: "output_schema", Label: "Output schema", Kind: "json_schema", Group: "output", Level: "basic",
		Help: "JSON schema of the answer. The agent is made to return exactly this shape; rules can then check its contents. Supports type, properties, required, items, enum, description.",
		Ignored: merge(
			because("Advertised to callers only; the orchestrator's plan uses a fixed schema.", orchestratorOnly),
		)},

	{Key: "validation_rules", Label: "Rules", Kind: "array", Group: "rules", Level: "advanced",
		Help:    "Rule sets, each judging one part of the answer. A failed error rule sends the answer back with its message; a failed quality rule earns one more attempt.",
		Ignored: rulesIgnored,
		Children: []FieldSpec{
			{Key: "subjects", Label: "Applies to", Kind: "string", Level: "advanced",
				Help: "Dotted path into the answer, e.g. inv or invoice.lines. Lists along the path are walked. Empty means the whole answer."},
			{Key: "name_key", Label: "Name each item by", Kind: "string", Level: "advanced",
				Help: "Property used for {subject} in messages, e.g. inv_no. Falls back to name, id, type."},
			{Key: "rules", Label: "Rules", Kind: "array", Level: "advanced", Help: "See rule_kinds for what each kind needs."},
		}},

	{Key: "evidence", Label: "Evidence", Kind: "array", Group: "evidence", Level: "advanced",
		Help:    "Named sets of values collected from tool calls. A 'was actually seen' rule checks an answer value against one.",
		Ignored: rulesIgnored,
		Children: []FieldSpec{
			{Key: "name", Label: "Set name", Kind: "string", Required: true, Level: "advanced", Help: "What rules call this set."},
			{Key: "action", Label: "From action", Kind: "ref_action", Required: true, Level: "advanced", Help: "The tool action whose calls fill the set."},
			{Key: "from_arg", Label: "Collect argument", Kind: "string", Level: "advanced", Help: "Record this argument of each call."},
			{Key: "from_result", Label: "Collect from result", Kind: "string", Level: "advanced", Help: "Dotted path into each result; lists are walked."},
			{Key: "failed_too", Label: "Include failed calls", Kind: "bool", Level: "advanced", Help: "Also collect from calls that returned an error."},
		}},

	{Key: "claims", Label: "Claims", Kind: "array", Group: "claims", Level: "advanced",
		Help:    "Things the answer says it did, each checked against the tool calls that really ran.",
		Ignored: rulesIgnored,
		Children: []FieldSpec{
			{Key: "claims", Label: "Claimed items at", Kind: "string", Required: true, Level: "advanced", Help: "Dotted path to the list of things the answer claims, e.g. created."},
			{Key: "action", Label: "Must have called", Kind: "ref_action", Required: true, Level: "advanced", Help: "The action that must have run for each claim."},
			{Key: "arg_key", Label: "Matching argument", Kind: "string", Level: "advanced", Help: "The call argument that must equal each claimed item."},
			{Key: "unchanged", Label: "Claimed unchanged at", Kind: "string", Level: "advanced", Help: "Path to items the answer says were already correct; they must have been looked at."},
			{Key: "reject", Label: "Reject unsupported claims", Kind: "bool", Level: "advanced", Help: "On: an unsupported claim is an error. Off: it only lowers quality."},
		}},

	{Key: "available_agents", Label: "Agents it may call", Kind: "ref_agent_list", Types: orchestratorOnly, Group: "orchestration", Level: "basic",
		Help: "Leave empty to allow every agent in the workspace except itself."},
	{Key: "available_tools", Label: "Tools it may plan with", Kind: "array", Types: orchestratorOnly, Group: "orchestration", Level: "basic",
		Help: "Tool actions the plan may use directly. Leave empty to allow all.",
		Children: []FieldSpec{
			{Key: "tool_name", Label: "Tool", Kind: "ref_tool", Required: true, Level: "basic", Help: "A tool configured in this workspace."},
			{Key: "actions", Label: "Actions", Kind: "string_list", Level: "basic", Help: "Allowed actions; empty allows all of the tool's actions."},
		}},
	{Key: "client_output_agents", Label: "Pass results straight to the app", Kind: "ref_agent_list", Types: orchestratorOnly, Group: "orchestration", Level: "advanced",
		Help: "Sub-agents whose structured output goes to the calling app intact, as well as into the summary - e.g. a page builder whose page the app renders."},
	{Key: "synthesis_prompt", Label: "How to write the final answer", Kind: "text", Types: orchestratorOnly, Group: "orchestration", Level: "basic",
		Help: "Instruction for combining sub-agent results into one reply. Grounding rules are always added."},
	{Key: "max_replans", Label: "Re-plans after a failure", Kind: "int", Default: 2, Types: orchestratorOnly, Group: "orchestration", Level: "advanced",
		Help: "How many times the planner may revise the plan when a step fails."},
	{Key: "delegation_strategy", Label: "Delegation strategy", Kind: "enum", Enum: []string{"adaptive", "sequential", "parallel"}, Default: "adaptive",
		Types: orchestratorOnly, Group: "orchestration", Level: "advanced",
		Help: "Adaptive and parallel run independent steps at the same time (the default). Sequential runs one step at a time, in order - for rate-limited tools or steps that must not overlap."},

	{Key: "retry_count", Label: "Retries", Kind: "int", Default: 0, Group: "limits", Level: "basic",
		Help: "Reasoning: how many times a rejected answer is sent back to be fixed. Orchestrator: plan repair attempts (at least 2). Reflex: attempts on a JSON error."},
	{Key: "max_iterations", Label: "Steps per attempt", Kind: "int", Default: 10, Types: reasoningFamily, Group: "limits", Level: "advanced",
		Help: "Most model/tool round-trips in one attempt."},
	{Key: "thinking_budget", Label: "Thinking budget", Kind: "int", Default: 10000, Types: reasoningFamily, Group: "limits", Level: "advanced",
		Help: "Tokens the model may spend thinking before it answers, on models that support it."},
	{Key: "enable_clarification", Label: "May ask the user", Kind: "bool", Default: false, Types: reasoningFamily, Group: "instructions", Level: "basic",
		Help: "Lets the agent stop and ask a question when a request is genuinely ambiguous. The answer comes back in the same conversation."},
	{Key: "budget", Label: "Budget", Kind: "object", Group: "limits", Level: "advanced",
		Help:    "Hard ceilings for one run. Zero means no limit. When it runs out, only an already-validated answer is returned.",
		Ignored: merge(because("Reflex agents make a single pass.", reflexFamily), because("The orchestrator does not apply a run budget.", orchestratorOnly)),
		Children: []FieldSpec{
			{Key: "max_attempts", Label: "Attempts", Kind: "int", Level: "advanced", Help: "Trips round the loop of any kind - retries, repairs, quality improvements."},
			{Key: "max_total_tokens", Label: "Total tokens", Kind: "int", Level: "advanced", Help: "Tokens across all attempts."},
			{Key: "max_duration_seconds", Label: "Seconds", Kind: "int", Level: "advanced", Help: "Wall-clock time."},
		}},
	{Key: "max_delegation_depth", Label: "Delegation depth", Kind: "int", Default: 3, Group: "limits", Level: "advanced",
		Help: "How many agents deep a chain starting here may go. An agent appearing twice in one chain is always refused."},

	{Key: "chat_memory", Label: "Conversation memory", Kind: "object", Group: "memory", Level: "advanced",
		Help: "Where conversation turns are kept. Without it the agent treats every request as new.",
		Children: []FieldSpec{
			{Key: "cache_store_type", Label: "Store", Kind: "enum", Enum: []string{"INMEMORY", "REDIS", "REDIS_CLUSTER", "ETCD"}, Default: "INMEMORY", Level: "advanced",
				Help: "In-memory is per server; Redis shares memory across servers."},
			{Key: "persist_enabled", Label: "Save conversations", Kind: "bool", Default: true, Level: "advanced",
				Help: "Write conversations to the database so they survive restarts and can be listed."},
			{Key: "cache_db_alias", Label: "Database alias", Kind: "string", Default: "pdb", Level: "advanced",
				ShowWhen: map[string][]string{"persist_enabled": {"true"}}, Help: "Database the conversations are saved to."},
			{Key: "persist_error", Label: "Save failed turns", Kind: "bool", Level: "advanced", Help: "Also save turns that ended in an error."},
			{Key: "session_ttl", Label: "Session lifetime", Kind: "duration", Level: "advanced", Help: "How long an unsaved conversation stays in memory, e.g. 2h. Default 2h."},
			{Key: "redis_addr", Label: "Redis address", Kind: "string", Required: true, Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}, Help: "host:port"},
			{Key: "redis_username", Label: "Redis username", Kind: "string", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "redis_password", Label: "Redis password", Kind: "string", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}, Help: "Use a secret variable rather than the value."},
			{Key: "redis_db", Label: "Redis DB", Kind: "int", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": {"REDIS"}}},
			{Key: "hash_tag", Label: "Hash tag", Kind: "string", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": {"REDIS_CLUSTER", "ELASTICACHE"}}},
			{Key: "tls_enabled", Label: "TLS", Kind: "bool", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "tls_skip_verify", Label: "Skip TLS verification", Kind: "bool", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "tls_server_name", Label: "TLS server name", Kind: "string", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "pool_size", Label: "Pool size", Kind: "int", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "min_idle_conns", Label: "Min idle connections", Kind: "int", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "read_timeout_ms", Label: "Read timeout (ms)", Kind: "int", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "write_timeout_ms", Label: "Write timeout (ms)", Kind: "int", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "max_retries", Label: "Max retries", Kind: "int", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "tag_set_ttl_sec_max", Label: "Tag set TTL cap (s)", Kind: "int", Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": redisTypes}},
			{Key: "etcd_endpoints", Label: "etcd endpoints", Kind: "string", Required: true, Level: "advanced", ShowWhen: map[string][]string{"cache_store_type": {"ETCD"}}},
		}},
	{Key: "conversation_config", Label: "Conversation window", Kind: "object", Group: "memory", Level: "advanced",
		Help: "How much history the model sees, and when older turns are summarised. Leave unset for defaults tuned to the model.",
		Children: []FieldSpec{
			{Key: "max_recent_messages", Label: "Recent messages kept whole", Kind: "int", Level: "advanced"},
			{Key: "max_tokens", Label: "History token limit", Kind: "int", Level: "advanced"},
			{Key: "summary_threshold", Label: "Summarise above (tokens)", Kind: "int", Level: "advanced"},
			{Key: "enable_summarization", Label: "Summarise old turns", Kind: "bool", Level: "advanced"},
			{Key: "summary_model", Label: "Summary model", Kind: "ref_model", Level: "advanced", Help: "Empty uses the agent's model."},
			{Key: "max_conversation_age", Label: "Summarise after (ns)", Kind: "int", Level: "advanced", Help: "Nanoseconds; 86400000000000 is 24h."},
		}},
	{Key: "memory_store", Label: "Long-term memory", Kind: "ref_vectorstore", Group: "memory", Level: "advanced",
		Help:    "A vector store to remember things in across conversations. The agent gets recall_memory and save_memory tools, and is told when to use them. The store needs an embedding model.",
		Ignored: merge(because("Only reasoning agents are given the memory tools.", reflexFamily), because("Only reasoning agents are given the memory tools; give memory to the sub-agents.", orchestratorOnly))},
	{Key: "memory_namespace", Label: "Memory namespace", Kind: "string", Group: "memory", Level: "advanced",
		Help:    "Agents with the same namespace share what they remember. Defaults to the agent name. Always kept separate per workspace.",
		Ignored: merge(because("Only reasoning agents are given the memory tools.", reflexFamily), because("Only reasoning agents are given the memory tools.", orchestratorOnly))},
}
