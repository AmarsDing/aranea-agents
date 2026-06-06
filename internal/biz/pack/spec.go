package pack

// Pack 表示一个内存中的 Aranea 场景包。
type Pack struct {
	Manifest  ManifestSpec
	Taxonomy  *TaxonomyPackSpec
	Agents    []AgentPackSpec
	Teams     []TeamPackSpec
	Graphs    []GraphPackSpec
	AgentFiles map[string]map[string]string // agent_key → filename → content
}

// ManifestSpec 定义 .arpack 包的元数据。
type ManifestSpec struct {
	APIVersion  string               `yaml:"api_version"`
	Kind        string               `yaml:"kind"` // agent | team | industry
	Name        string               `yaml:"name"`
	Description string               `yaml:"description,omitempty"`
	Version     string               `yaml:"version"`
	Author      string               `yaml:"author,omitempty"`
	CreatedAt   string               `yaml:"created_at,omitempty"`

	Dependencies *PackDependencies   `yaml:"dependencies,omitempty"`
	Contents     *PackContents       `yaml:"contents,omitempty"`
}

// PackDependencies 声明 Pack 依赖的外部资源（为商城预留）。
type PackDependencies struct {
	Skills    []string `yaml:"skills,omitempty"`
	FuncRefs  []string `yaml:"func_refs,omitempty"`
}

// PackContents 声明 Pack 包含的实体索引。
type PackContents struct {
	Taxonomy bool              `yaml:"taxonomy,omitempty"`
	Agents   []PackContentRef  `yaml:"agents,omitempty"`
	Teams    []PackContentRef  `yaml:"teams,omitempty"`
	Graphs   []PackContentRef  `yaml:"graphs,omitempty"`
}

// PackContentRef 引用 Pack 中的一个实体。
type PackContentRef struct {
	Key string `yaml:"key"`
}

// TaxonomyPackSpec 定义行业分类树，与现有 taxonomy.yaml 格式一致。
type TaxonomyPackSpec struct {
	Industries []IndustrySpec `yaml:"industries"`
}

// IndustrySpec 行业级节点。
type IndustrySpec struct {
	Key         string           `yaml:"key"`
	Name        string           `yaml:"name"`
	Icon        string           `yaml:"icon,omitempty"`
	Description string           `yaml:"description,omitempty"`
	SortOrder   int              `yaml:"sort_order,omitempty"`
	Departments []DepartmentSpec `yaml:"departments"`
}

// DepartmentSpec 部门级节点。
type DepartmentSpec struct {
	Key         string         `yaml:"key"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description,omitempty"`
	SortOrder   int            `yaml:"sort_order,omitempty"`
	Positions   []PositionSpec `yaml:"positions"`
}

// PositionSpec 岗位级节点。
type PositionSpec struct {
	Key             string              `yaml:"key"`
	Name            string              `yaml:"name"`
	Description     string              `yaml:"description,omitempty"`
	SortOrder       int                 `yaml:"sort_order,omitempty"`
	SeniorityLevel  string              `yaml:"seniority_level,omitempty"`
	SkillsRequired  []string            `yaml:"skills_required,omitempty"`
	Responsibilities []string           `yaml:"responsibilities,omitempty"`
	Variants        []VariantSpec       `yaml:"variants,omitempty"`
}

// VariantSpec 岗位变体。
type VariantSpec struct {
	Key  string `yaml:"key"`
	Name string `yaml:"name"`
}

// AgentPackSpec 定义 Agent 完整配置（导出/导入用）。
type AgentPackSpec struct {
	Key               string              `yaml:"key"`
	DisplayName       string              `yaml:"display_name"`
	Description       string              `yaml:"description,omitempty"`
	Icon              string              `yaml:"icon,omitempty"`

	// 身份属性
	PositionKey       string              `yaml:"position_key,omitempty"` // taxonomy 路径格式：industry/dept/pos
	Variant           string              `yaml:"variant,omitempty"`
	VariantDescription string             `yaml:"variant_description,omitempty"`

	// 模型配置
	Provider          string              `yaml:"provider,omitempty"`
	Model             string              `yaml:"model,omitempty"`
	ModelTier         string              `yaml:"model_tier,omitempty"` // fast | strong
	SystemPromptMode  string              `yaml:"system_prompt_mode,omitempty"`
	ContextWindow     int                 `yaml:"context_window,omitempty"`

	// 工具配置
	ToolsProfile      string              `yaml:"tools_profile,omitempty"`
	ToolsAllow        []string            `yaml:"tools_allow,omitempty"`
	ToolsDeny         []string            `yaml:"tools_deny,omitempty"`
	ToolsParallel     *bool               `yaml:"tools_parallel,omitempty"`

	// 技能引用
	Skills            *AgentSkillsSpec    `yaml:"skills,omitempty"`

	// 子代理
	SubagentsEnabled  *bool               `yaml:"subagents_enabled,omitempty"`
	SubagentsMaxConcurrency int            `yaml:"subagents_max_concurrency,omitempty"`
	SubagentsMaxGenerationDepth int        `yaml:"subagents_max_generation_depth,omitempty"`

	// 代码执行
	CodeExecutor      string              `yaml:"code_executor,omitempty"`

	// 运行时设置（可移植部分）
	Runtime           *AgentRuntimePackSpec `yaml:"runtime,omitempty"`

	// 文件引用（指向 agents/<key>/ 目录下的文件）
	Files             []AgentFileRef      `yaml:"files,omitempty"`

	// Team 角色
	TeamRole          string              `yaml:"team_role,omitempty"`

	// Agent 类型
	Kind              string              `yaml:"kind,omitempty"`               // llm | a2a_proxy (technical type)
	OwnershipKind     string              `yaml:"ownership_kind,omitempty"`     // user | system_builtin | ecosystem_preset | marketplace | certified (ownership classification)
	Source            string              `yaml:"source,omitempty"`             // user | system | imported (origin tracking)
	A2AProxy          *A2AProxyPackSpec   `yaml:"a2a_proxy,omitempty"`
}

// AgentSkillsSpec Agent 的技能引用。
type AgentSkillsSpec struct {
	Allowed  []string `yaml:"allowed,omitempty"`
	Denied   []string `yaml:"denied,omitempty"`
	LoadMode string   `yaml:"load_mode,omitempty"`
}

// AgentRuntimePackSpec Agent 可移植运行时配置。
type AgentRuntimePackSpec struct {
	Memory   *RuntimeMemorySpec   `yaml:"memory,omitempty"`
	Tools    *RuntimeToolsSpec    `yaml:"tools,omitempty"`
	Evolution *RuntimeEvolutionSpec `yaml:"evolution,omitempty"`
	Reasoning *RuntimeReasoningSpec `yaml:"reasoning,omitempty"`
	RalphLoop *RuntimeRalphLoopSpec `yaml:"ralph_loop,omitempty"`
	Context  *RuntimeContextSpec  `yaml:"context,omitempty"`
}

// RuntimeMemorySpec 内存域可移植配置。
type RuntimeMemorySpec struct {
	Enabled              bool    `yaml:"enabled"`
	L0RecentWindowTurns  int     `yaml:"l0_recent_window_turns,omitempty"`
	L0RecentWindowTokens int     `yaml:"l0_recent_window_tokens,omitempty"`
	L0SummaryThreshold   float64 `yaml:"l0_summary_threshold,omitempty"`
	L0SummaryKeepTurns   int     `yaml:"l0_summary_keep_turns,omitempty"`
	L0InjectL1           bool    `yaml:"l0_inject_l1,omitempty"`
	L0InjectL3           bool    `yaml:"l0_inject_l3,omitempty"`
	L0InjectL4           bool    `yaml:"l0_inject_l4,omitempty"`
	L0SnapshotMode       string  `yaml:"l0_snapshot_mode,omitempty"`
	L1Enabled            bool    `yaml:"l1_enabled,omitempty"`
	L1BudgetTokens       int     `yaml:"l1_budget_tokens,omitempty"`
	L2EpisodeEnabled     bool    `yaml:"l2_episode_enabled,omitempty"`
	L2EpisodeMinImportance float64 `yaml:"l2_episode_min_importance,omitempty"`
	L2RecallEnabled      bool    `yaml:"l2_recall_enabled,omitempty"`
	L2RecallMax          int     `yaml:"l2_recall_max,omitempty"`
	L2RetentionDays      int     `yaml:"l2_retention_days,omitempty"`
	L3Enabled            bool    `yaml:"l3_enabled,omitempty"`
	L3RecallTopK         int     `yaml:"l3_recall_top_k,omitempty"`
	L3RecallMinScore     float64 `yaml:"l3_recall_min_score,omitempty"`
	L4Enabled            bool    `yaml:"l4_enabled,omitempty"`
	L4GraphInjectNeighbors bool  `yaml:"l4_graph_inject_neighbors,omitempty"`
	L4GraphMaxNeighbors  int     `yaml:"l4_graph_max_neighbors,omitempty"`
	L4IdentityInject     bool    `yaml:"l4_identity_inject,omitempty"`
}

// RuntimeToolsSpec 工具域可移植配置。
type RuntimeToolsSpec struct {
	RetryEnabled           bool    `yaml:"retry_enabled,omitempty"`
	RetryMaxAttempts       int     `yaml:"retry_max_attempts,omitempty"`
	RetryInitialIntervalMs int     `yaml:"retry_initial_interval_ms,omitempty"`
	StreamingEnabled       bool    `yaml:"streaming_enabled,omitempty"`
	CircuitBreakerEnabled  bool    `yaml:"circuit_breaker_enabled,omitempty"`
	CommandSafetyEnabled   bool    `yaml:"command_safety_enabled,omitempty"`
}

// RuntimeEvolutionSpec 进化域可移植配置。
type RuntimeEvolutionSpec struct {
	SelfEvolve        bool `yaml:"self_evolve,omitempty"`
	SkillEvolve       bool `yaml:"skill_evolve,omitempty"`
	MetricsEnabled    bool `yaml:"metrics_enabled,omitempty"`
	SuggestionsEnabled bool `yaml:"suggestions_enabled,omitempty"`
}

// RuntimeReasoningSpec 推理域可移植配置。
type RuntimeReasoningSpec struct {
	Mode  string `yaml:"mode,omitempty"`
	Level string `yaml:"level,omitempty"`
}

// RuntimeRalphLoopSpec RalphLoop 域可移植配置。
type RuntimeRalphLoopSpec struct {
	MaxIterations        int    `yaml:"max_iterations,omitempty"`
	CompletionPromise    string `yaml:"completion_promise,omitempty"`
	VerifyCommand        string `yaml:"verify_command,omitempty"`
	VerifyTimeoutSeconds int    `yaml:"verify_timeout_seconds,omitempty"`
}

// RuntimeContextSpec 上下文域可移植配置。
type RuntimeContextSpec struct {
	CompactionEnabled    bool `yaml:"compaction_enabled,omitempty"`
	SessionSummaryEnabled bool `yaml:"session_summary_enabled,omitempty"`
	IntentPassEnabled    bool `yaml:"intent_pass_enabled,omitempty"`
}

// AgentFileRef Agent 文件引用。
type AgentFileRef struct {
	Name string `yaml:"name"`
}

// A2AProxyPackSpec A2A 代理配置。
type A2AProxyPackSpec struct {
	RemoteURL       string `yaml:"remote_url"`
	AgentCardURL    string `yaml:"agent_card_url,omitempty"`
	EnableStreaming  bool   `yaml:"enable_streaming,omitempty"`
	AuthType        string `yaml:"auth_type,omitempty"`
	TimeoutSeconds  int    `yaml:"timeout_seconds,omitempty"`
}

// TeamPackSpec 定义 Team 完整配置（导出/导入用）。
type TeamPackSpec struct {
	Key               string              `yaml:"key"`
	DisplayName       string              `yaml:"display_name"`
	Description       string              `yaml:"description,omitempty"`
	Mode              string              `yaml:"mode"` // coordinator | sequential | parallel

	// 编排配置
	MaxConcurrency    int                 `yaml:"max_concurrency,omitempty"`
	TimeoutSeconds    int                 `yaml:"timeout_seconds,omitempty"`
	RunTimeoutSec     int                 `yaml:"run_timeout_sec,omitempty"`
	TurnTimeoutSec    int                 `yaml:"turn_timeout_sec,omitempty"`
	FirstByteTimeoutSec int               `yaml:"first_byte_timeout_sec,omitempty"`
	LoopMaxIter       int                 `yaml:"loop_max_iter,omitempty"`
	EnableCheckpoint  bool                `yaml:"enable_checkpoint,omitempty"`
	RuntimeEngine     string              `yaml:"runtime_engine,omitempty"`
	TeamGraphRuntime  bool                `yaml:"team_graph_runtime,omitempty"`

	// 成员通过 agent_key 引用
	Members           []TeamMemberPackSpec `yaml:"members,omitempty"`

	// 意图锚定与合成器
	IntentAnchorKey   string              `yaml:"intent_anchor_key,omitempty"`
	SynthesizerKey    string              `yaml:"synthesizer_key,omitempty"`

	// 关联 Graph
	Graph             *TeamGraphPackSpec  `yaml:"graph,omitempty"`

	// 失败策略
	FailurePolicy     *TeamFailurePolicySpec `yaml:"failure_policy,omitempty"`

	// Critic Loop
	CriticLoop        *CriticLoopPackSpec `yaml:"critic_loop,omitempty"`

	// 所有权与来源
	OwnershipKind     string              `yaml:"ownership_kind,omitempty"`     // user | system_builtin | ecosystem_preset | marketplace | certified
	Source            string              `yaml:"source,omitempty"`             // user | system | imported
}

// TeamMemberPackSpec Team 成员定义（通过 agent_key 引用）。
type TeamMemberPackSpec struct {
	AgentKey   string `yaml:"agent_key"`
	Role       string `yaml:"role"`
	Name       string `yaml:"name,omitempty"`
	TaskPrompt string `yaml:"task_prompt,omitempty"`
	Enabled    *bool  `yaml:"enabled,omitempty"`
	SortOrder  int    `yaml:"sort_order,omitempty"`
}

// TeamGraphPackSpec Team 关联的 Graph 定义。
type TeamGraphPackSpec struct {
	Linked bool                `yaml:"linked,omitempty"` // true=外部链接, false=内嵌定义
	// 内嵌图定义（Linked=false 时使用）
	Layout string              `yaml:"layout,omitempty"`
	Nodes  []TeamGraphNodeSpec `yaml:"nodes,omitempty"`
	Edges  []TeamGraphEdgeSpec `yaml:"edges,omitempty"`
	// 外部链接（Linked=true 时使用，引用 graphs/ 目录中的 Graph ID）
	LinkedGraphID string `yaml:"linked_graph_id,omitempty"`
}

// TeamGraphNodeSpec Team 内嵌 Graph 节点（通过 agent_key 引用 Agent）。
type TeamGraphNodeSpec struct {
	ID               string   `yaml:"id"`
	Type             string   `yaml:"type"`
	Label            string   `yaml:"label,omitempty"`
	AgentKey         string   `yaml:"agent_key,omitempty"`
	Role             string   `yaml:"role,omitempty"`
	InterruptBefore  bool     `yaml:"interrupt_before,omitempty"`
	InterruptAfter   bool     `yaml:"interrupt_after,omitempty"`
	Destinations     []string `yaml:"destinations,omitempty"`
	RetryMaxAttempts int      `yaml:"retry_max_attempts,omitempty"`
	FallbackAgent    string   `yaml:"fallback_agent,omitempty"`
}

// TeamGraphEdgeSpec Team 内嵌 Graph 边。
type TeamGraphEdgeSpec struct {
	ID        string `yaml:"id,omitempty"`
	Source    string `yaml:"source"`
	Target    string `yaml:"target"`
	Label     string `yaml:"label,omitempty"`
	Condition string `yaml:"condition,omitempty"`
}

// TeamFailurePolicySpec Team 失败策略。
type TeamFailurePolicySpec struct {
	Default        string                       `yaml:"default,omitempty"` // retry_then_block | skip | fail_fast
	Retry          *TeamRetryPolicySpec         `yaml:"retry,omitempty"`
	NodeOverrides  map[string]TeamNodeFailureOverrideSpec `yaml:"node_overrides,omitempty"`
	ParallelFail   string                       `yaml:"parallel_fail,omitempty"`
	CircuitBreaker *CircuitBreakerPolicySpec    `yaml:"circuit_breaker,omitempty"`
	OnError        string                       `yaml:"on_error,omitempty"`
}

// TeamRetryPolicySpec Team 重试策略。
type TeamRetryPolicySpec struct {
	MaxAttempts     int     `yaml:"max_attempts,omitempty"`
	InitialIntervalMs int   `yaml:"initial_interval_ms,omitempty"`
	BackoffFactor   float64 `yaml:"backoff_factor,omitempty"`
}

// TeamNodeFailureOverrideSpec 节点级失败策略覆盖。
type TeamNodeFailureOverrideSpec struct {
	Action string `yaml:"action,omitempty"` // retry | skip | fail
}

// CircuitBreakerPolicySpec 熔断策略。
type CircuitBreakerPolicySpec struct {
	FailureThreshold int     `yaml:"failure_threshold,omitempty"`
	RecoveryTimeoutMs int   `yaml:"recovery_timeout_ms,omitempty"`
	HalfOpenMaxCalls int    `yaml:"half_open_max_calls,omitempty"`
}

// CriticLoopPackSpec Critic Loop 配置。
type CriticLoopPackSpec struct {
	MaxIterations  int     `yaml:"max_iterations,omitempty"`
	ScoreThreshold float64 `yaml:"score_threshold,omitempty"`
}

// GraphPackSpec 定义 Graph 模板（导出/导入用）。
type GraphPackSpec struct {
	ID               string                `yaml:"id"`
	Name             string                `yaml:"name"`
	Description      string                `yaml:"description,omitempty"`
	Category         string                `yaml:"category,omitempty"`
	ExecutionEngine  string                `yaml:"execution_engine,omitempty"`
	EnableCheckpoint bool                  `yaml:"enable_checkpoint,omitempty"`
	EntryPoint       string                `yaml:"entry_point"`
	FinishPoint      string                `yaml:"finish_point,omitempty"`
	Version          int                   `yaml:"version,omitempty"`
	SortOrder        int                   `yaml:"sort_order,omitempty"`
	StateFields      []StateFieldPackSpec  `yaml:"state_fields,omitempty"`
	Nodes            []GraphNodePackSpec   `yaml:"nodes,omitempty"`
	Edges            []GraphEdgePackSpec   `yaml:"edges,omitempty"`
	ConditionalEdges []GraphCondEdgePackSpec `yaml:"conditional_edges,omitempty"`
	Subgraphs        []SubgraphPackSpec    `yaml:"subgraphs,omitempty"`
	InterruptBefore  []string              `yaml:"interrupt_before,omitempty"`
	InterruptAfter   []string              `yaml:"interrupt_after,omitempty"`
}

// StateFieldPackSpec Graph 状态字段。
type StateFieldPackSpec struct {
	Name            string `yaml:"name"`
	Type            string `yaml:"type"`
	Reducer         string `yaml:"reducer,omitempty"`
	DefaultValue    any    `yaml:"default_value,omitempty"`
	Required        bool   `yaml:"required,omitempty"`
	DisableDeepCopy bool   `yaml:"disable_deep_copy,omitempty"`
}

// GraphNodePackSpec Graph 节点。
type GraphNodePackSpec struct {
	ID                   string   `yaml:"id"`
	Type                 string   `yaml:"type,omitempty"`
	Label                string   `yaml:"label,omitempty"`
	Description          string   `yaml:"description,omitempty"`
	FuncRef              string   `yaml:"func_ref,omitempty"`
	Instruction          string   `yaml:"instruction,omitempty"`
	ModelName            string   `yaml:"model_name,omitempty"`
	ToolNames            []string `yaml:"tool_names,omitempty"`
	AgentKey             string   `yaml:"agent_key,omitempty"`
	InterruptBefore      bool     `yaml:"interrupt_before,omitempty"`
	InterruptAfter       bool     `yaml:"interrupt_after,omitempty"`
	Destinations         []string `yaml:"destinations,omitempty"`
	RetryMaxAttempts     int      `yaml:"retry_max_attempts,omitempty"`
	FailureAction        string   `yaml:"failure_action,omitempty"`
	FallbackAgent        string   `yaml:"fallback_agent,omitempty"`
	InputMapperJSON      string   `yaml:"input_mapper_json,omitempty"`
	OutputMapperJSON     string   `yaml:"output_mapper_json,omitempty"`
	IsolatedMessages     bool     `yaml:"isolated_messages,omitempty"`
	InputFromLastResponse bool   `yaml:"input_from_last_response,omitempty"`
	CacheEnabled         bool     `yaml:"cache_enabled,omitempty"`
	CacheTTLSeconds      int      `yaml:"cache_ttl_seconds,omitempty"`
}

// GraphEdgePackSpec Graph 普通边。
type GraphEdgePackSpec struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	Kind string `yaml:"kind,omitempty"`
}

// GraphCondEdgePackSpec Graph 条件边。
type GraphCondEdgePackSpec struct {
	From        string            `yaml:"from"`
	CondFuncRef string            `yaml:"cond_func_ref,omitempty"`
	PathMap     map[string]string `yaml:"path_map,omitempty"`
}

// SubgraphPackSpec Graph 子图定义。
type SubgraphPackSpec struct {
	ID              string              `yaml:"id"`
	Name            string              `yaml:"name,omitempty"`
	EntryPoint      string              `yaml:"entry_point"`
	FinishPoint     string              `yaml:"finish_point,omitempty"`
	InterruptBefore bool                `yaml:"interrupt_before,omitempty"`
	InterruptAfter  bool                `yaml:"interrupt_after,omitempty"`
	Nodes           []GraphNodePackSpec `yaml:"nodes,omitempty"`
	Edges           []GraphEdgePackSpec `yaml:"edges,omitempty"`
}

// ConflictStrategy 冲突策略。
type ConflictStrategy string

const (
	ConflictSkip      ConflictStrategy = "skip"
	ConflictOverwrite ConflictStrategy = "overwrite"
	ConflictDuplicate ConflictStrategy = "duplicate"
)

// ValidationResult 校验结果。
type ValidationResult struct {
	Valid          bool              `yaml:"valid"`
	Conflicts      []ConflictItem    `yaml:"conflicts,omitempty"`
	MissingSkills  []string          `yaml:"missing_skills,omitempty"`
	MissingFuncRefs []string         `yaml:"missing_func_refs,omitempty"`
	Errors         []string          `yaml:"errors,omitempty"`
}

// ConflictItem 冲突项。
type ConflictItem struct {
	EntityType string `yaml:"entity_type"` // agent | team | taxonomy
	Key        string `yaml:"key"`
}

// ImportResult 导入结果。
type ImportResult struct {
	AgentsCreated int          `yaml:"agents_created"`
	AgentsUpdated int          `yaml:"agents_updated"`
	AgentsSkipped int          `yaml:"agents_skipped"`
	TeamsCreated  int          `yaml:"teams_created"`
	TeamsUpdated  int          `yaml:"teams_updated"`
	TeamsSkipped  int          `yaml:"teams_skipped"`
	GraphsCreated int          `yaml:"graphs_created"`
	GraphsUpdated int          `yaml:"graphs_updated"`
	GraphsSkipped int          `yaml:"graphs_skipped"`
	TaxonomyNodes int          `yaml:"taxonomy_nodes"`
	Failures      []ImportFailure `yaml:"failures,omitempty"`
	Warnings      []string        `yaml:"warnings,omitempty"`
}

// ImportFailure 导入失败项。
type ImportFailure struct {
	EntityType string `yaml:"entity_type"`
	Key        string `yaml:"key"`
	Reason     string `yaml:"reason"`
}
