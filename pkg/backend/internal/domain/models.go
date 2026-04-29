package domain

type Agent struct {
	ID                 string                `json:"id"`
	AgentKey           string                `json:"agent_key"`
	DisplayName        string                `json:"display_name"`
	Provider           string                `json:"provider"`
	Model              string                `json:"model"`
	Status             string                `json:"status"`
	IsDefault          bool                  `json:"is_default"`
	IsFavorite         bool                  `json:"is_favorite"`
	Icon               string                `json:"icon"`
	AgentDescription   string                `json:"agent_description"`
	CategoryPositionID string                `json:"category_position_id"`
	SystemPromptMode   string                `json:"system_prompt_mode"`
	ContextWindow      int                   `json:"context_window"`
	BudgetMonthlyCents int                   `json:"budget_monthly_cents"`
	ConfigJSON         string                `json:"config_json"`
	CreatedAt          string                `json:"created_at"`
	UpdatedAt          string                `json:"updated_at"`
	DeletedAt          string                `json:"deleted_at"`
	Settings           *AgentRuntimeSettings `json:"settings,omitempty"`
	Files              []AgentPromptFile     `json:"files,omitempty"`
}

type AgentRuntimeSettings struct {
	AgentID                           string  `json:"agent_id,omitempty"`
	SelfEvolve                        bool    `json:"self_evolve"`
	SubagentsEnabled                  bool    `json:"subagents_enabled"`
	SubagentsMaxConcurrency           int     `json:"subagents_max_concurrency"`
	SubagentsMaxGenerationDepth       int     `json:"subagents_max_generation_depth"`
	SubagentsMaxChildrenPerAgent      int     `json:"subagents_max_children_per_agent"`
	SubagentsArchiveAfterMinutes      int     `json:"subagents_archive_after_minutes"`
	SubagentsMaxRetries               int     `json:"subagents_max_retries"`
	SubagentsModelOverride            string  `json:"subagents_model_override"`
	ToolsEnabled                      bool    `json:"tools_enabled"`
	ToolsProfile                      string  `json:"tools_profile"`
	ToolsToolCallPrefix               string  `json:"tools_tool_call_prefix"`
	ToolsAllowJSON                    string  `json:"tools_allow_json"`
	ToolsDenyJSON                     string  `json:"tools_deny_json"`
	ToolsConcurrentAllowJSON          string  `json:"tools_concurrent_allow_json"`
	MemoryEnabled                     bool    `json:"memory_enabled"`
	MemoryMaxChunkLength              int     `json:"memory_max_chunk_length"`
	MemoryMaxResults                  int     `json:"memory_max_results"`
	MemoryMinScore                    float64 `json:"memory_min_score"`
	HeartbeatEnabled                  bool    `json:"heartbeat_enabled"`
	HeartbeatIntervalMinutes          int     `json:"heartbeat_interval_minutes"`
	EvolutionSelfEvolve               bool    `json:"evolution_self_evolve"`
	EvolutionSkillEvolve              bool    `json:"evolution_skill_evolve"`
	EvolutionMetricsEnabled           bool    `json:"evolution_metrics_enabled"`
	EvolutionSuggestionsEnabled       bool    `json:"evolution_suggestions_enabled"`
	GuardrailMaxChangePerPeriod       float64 `json:"guardrail_max_change_per_period"`
	GuardrailMinDataPoints            int     `json:"guardrail_min_data_points"`
	GuardrailRollbackOnDeclinePercent int     `json:"guardrail_rollback_on_decline_percent"`
	L0RecentWindowTurns               int     `json:"l0_recent_window_turns"`
	L0RecentWindowTokens              int     `json:"l0_recent_window_tokens"`
	L0SummaryThreshold                float64 `json:"l0_summary_threshold"`
	L0SummaryKeepTurns                int     `json:"l0_summary_keep_turns"`
	L0TruncateStrategy                string  `json:"l0_truncate_strategy"`
	L0InjectL1                        bool    `json:"l0_inject_l1"`
	L0InjectL3                        bool    `json:"l0_inject_l3"`
	L0InjectL4                        bool    `json:"l0_inject_l4"`
	L0L3MaxChunks                     int     `json:"l0_l3_max_chunks"`
	L0L4MaxPaths                      int     `json:"l0_l4_max_paths"`
	L0SnapshotMode                    string  `json:"l0_snapshot_mode"`
	L1Enabled                         bool    `json:"l1_enabled"`
	L1BudgetTokens                    int     `json:"l1_budget_tokens"`
	L1FieldMaxTokens                  int     `json:"l1_field_max_tokens"`
	L1HistoryKeepRevisions            int     `json:"l1_history_keep_revisions"`
	L1DefaultSchemaID                 string  `json:"l1_default_schema_id"`
	L1ArchiveOnIdleMinutes            int     `json:"l1_archive_on_idle_minutes"`
	L2EpisodeEnabled                  bool    `json:"l2_episode_enabled"`
	L2EpisodeMinImportance            float64 `json:"l2_episode_min_importance"`
	L2IndexEnabled                    bool    `json:"l2_index_enabled"`
	L2IndexEmbeddingModel             string  `json:"l2_index_embedding_model"`
	L2RecallEnabled                   bool    `json:"l2_recall_enabled"`
	L2RecallMax                       int     `json:"l2_recall_max"`
	L2RetentionDays                   int     `json:"l2_retention_days"`
	L2ArchiveAfterDays                int     `json:"l2_archive_after_days"`
	L3Enabled                         bool    `json:"l3_enabled"`
	L3RecallTopK                      int     `json:"l3_recall_top_k"`
	L3RecallMinScore                  float64 `json:"l3_recall_min_score"`
	L3RecallScopesJSON                string  `json:"l3_recall_scopes_json"`
	L3EmbeddingModel                  string  `json:"l3_embedding_model"`
	L3DecayIntervalHours              int     `json:"l3_decay_interval_hours"`
	L3ArchiveThreshold                float64 `json:"l3_archive_threshold"`
	L3MaxPerRecallChars               int     `json:"l3_max_per_recall_chars"`
	L4Enabled                         bool    `json:"l4_enabled"`
	L4GraphInjectNeighbors            bool    `json:"l4_graph_inject_neighbors"`
	L4GraphMaxNeighbors               int     `json:"l4_graph_max_neighbors"`
	L4GraphMaxHops                    int     `json:"l4_graph_max_hops"`
	L4IdentityInject                  bool    `json:"l4_identity_inject"`
	L4StrategyInject                  bool    `json:"l4_strategy_inject"`
	EvoEnabled                        bool    `json:"evo_enabled"`
	EvoAutoApply                      bool    `json:"evo_auto_apply"`
	EvoMinEpisodes                    int     `json:"evo_min_episodes"`
	EvoMinNegativeFeedback            int     `json:"evo_min_negative_feedback"`
	EvoThrottleHours                  int     `json:"evo_throttle_hours"`
	EvoProposalTTLDays                int     `json:"evo_proposal_ttl_days"`
	EvoPersonaMaxChars                int     `json:"evo_persona_max_chars"`
	EvoSystemPromptMaxAppends         int     `json:"evo_system_prompt_max_appends"`
	CreatedAt                         string  `json:"created_at,omitempty"`
	UpdatedAt                         string  `json:"updated_at,omitempty"`
}

type AgentPromptFile struct {
	ID        string `json:"id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type AgentListQuery struct {
	Keyword    string `json:"keyword"`
	Status     string `json:"status"`
	Provider   string `json:"provider"`
	CategoryID string `json:"category_id"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

type AgentListResult struct {
	Items  []Agent `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type Team struct {
	ID             string `json:"id"`
	TeamKey        string `json:"team_key"`
	DisplayName    string `json:"display_name"`
	Status         string `json:"status"`
	IsDefault      bool   `json:"is_default"`
	DefinitionJSON string `json:"definition_json"`
	ADKAppName     string `json:"adk_app_name"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	DeletedAt      string `json:"deleted_at"`
}

type TeamRun struct {
	ID            string `json:"id"`
	TeamID        string `json:"team_id"`
	SessionID     string `json:"session_id"`
	MessageID     string `json:"message_id"`
	Mode          string `json:"mode"`
	Status        string `json:"status"`
	InputPreview  string `json:"input_preview"`
	OutputPreview string `json:"output_preview"`
	TokenIn       int    `json:"token_in"`
	TokenOut      int    `json:"token_out"`
	CostMicroUSD  int64  `json:"cost_micro_usd"`
	DurationMS    int    `json:"duration_ms"`
	ErrorMessage  string `json:"error_message"`
	TopologyJSON  string `json:"topology_json"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type TeamRunStep struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	TeamID        string `json:"team_id"`
	AgentID       string `json:"agent_id"`
	AgentKey      string `json:"agent_key"`
	AgentName     string `json:"agent_name"`
	Role          string `json:"role"`
	SortOrder     int    `json:"sort_order"`
	Status        string `json:"status"`
	InputPreview  string `json:"input_preview"`
	OutputPreview string `json:"output_preview"`
	TokenIn       int    `json:"token_in"`
	TokenOut      int    `json:"token_out"`
	CostMicroUSD  int64  `json:"cost_micro_usd"`
	DurationMS    int    `json:"duration_ms"`
	ErrorMessage  string `json:"error_message"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	CreatedAt     string `json:"created_at"`
}

type Session struct {
	ID                      string  `json:"id"`
	OwnerType               string  `json:"owner_type"`
	AgentID                 string  `json:"agent_id"`
	TeamID                  string  `json:"team_id"`
	Title                   string  `json:"title"`
	Summary                 string  `json:"summary"`
	ContextUsedRatio        float64 `json:"context_used_ratio"`
	ContextUsedTokens       int     `json:"context_used_tokens"`
	MaxContextUsedRatio     float64 `json:"max_context_used_ratio"`
	LastContextWindowTokens int     `json:"last_context_window_tokens"`
	ContextStatus           string  `json:"context_status"`
	DialogMode              string  `json:"dialog_mode"`
	Provider                string  `json:"provider"`
	Model                   string  `json:"model"`
	Status                  string  `json:"status"`
	MessageCount            int     `json:"message_count"`
	RunCount                int     `json:"run_count"`
	ModelCallCount          int     `json:"model_call_count"`
	ToolCallCount           int     `json:"tool_call_count"`
	SkillCallCount          int     `json:"skill_call_count"`
	MCPCallCount            int     `json:"mcp_call_count"`
	InputTokens             int     `json:"input_tokens"`
	OutputTokens            int     `json:"output_tokens"`
	TotalTokens             int     `json:"total_tokens"`
	TotalCostMicroUSD       int64   `json:"total_cost_micro_usd"`
	LastMessageAt           string  `json:"last_message_at"`
	CreatedAt               string  `json:"created_at"`
	UpdatedAt               string  `json:"updated_at"`
	ArchivedAt              string  `json:"archived_at"`
	DeletedAt               string  `json:"deleted_at"`
}

type SessionSearchQuery struct {
	OwnerType     string `json:"owner_type"`
	AgentID       string `json:"agent_id"`
	TeamID        string `json:"team_id"`
	Status        string `json:"status"`
	ContextStatus string `json:"context_status"`
	Keyword       string `json:"keyword"`
	Limit         int    `json:"limit"`
	Offset        int    `json:"offset"`
}

type SessionListResult struct {
	Items  []Session `json:"items"`
	Total  int       `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

type Message struct {
	ID               string `json:"id"`
	SessionID        string `json:"session_id"`
	ParentMessageID  string `json:"parent_message_id"`
	TurnIndex        int    `json:"turn_index"`
	Role             string `json:"role"`
	Content          string `json:"content_markdown"`
	ModelName        string `json:"model_name"`
	TokenIn          int    `json:"token_in"`
	TokenOut         int    `json:"token_out"`
	LatencyMS        int    `json:"latency_ms"`
	Status           string `json:"status"`
	AttachmentsCount int    `json:"attachments_count"`
	OptionsJSON      string `json:"options_json"`
	ErrorMessage     string `json:"error_message"`
	CreatedAt        string `json:"created_at"`
}

type SessionTimelineItem struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Side            string   `json:"side"`
	Title           string   `json:"title"`
	Subtitle        string   `json:"subtitle"`
	ActorID         string   `json:"actor_id"`
	ActorName       string   `json:"actor_name"`
	Status          string   `json:"status"`
	OccurredAt      string   `json:"occurred_at"`
	DurationMS      int      `json:"duration_ms"`
	ContentMarkdown string   `json:"content_markdown"`
	Preview         string   `json:"preview"`
	DetailJSON      string   `json:"detail_json"`
	Tags            []string `json:"tags"`
}

type SessionTimelineSummary struct {
	Total        int `json:"total"`
	MessageCount int `json:"message_count"`
	ToolCount    int `json:"tool_count"`
	SkillCount   int `json:"skill_count"`
	MCPCount     int `json:"mcp_count"`
}

type SessionTimeline struct {
	SessionID string                 `json:"session_id"`
	Items     []SessionTimelineItem  `json:"items"`
	Summary   SessionTimelineSummary `json:"summary"`
}

type ModelTokenUsageEvent struct {
	ID                            string  `json:"id"`
	OccurredAt                    string  `json:"occurred_at"`
	DateKey                       string  `json:"date_key"`
	HourKey                       string  `json:"hour_key"`
	WorkspaceID                   string  `json:"workspace_id"`
	UserID                        string  `json:"user_id"`
	TeamID                        string  `json:"team_id"`
	AgentID                       string  `json:"agent_id"`
	AgentKey                      string  `json:"agent_key"`
	SessionID                     string  `json:"session_id"`
	MessageID                     string  `json:"message_id"`
	RequestID                     string  `json:"request_id"`
	ProviderCode                  string  `json:"provider_code"`
	ProviderType                  string  `json:"provider_type"`
	ProviderDisplayName           string  `json:"provider_display_name"`
	ModelAPIID                    string  `json:"model_api_id"`
	ModelDisplayName              string  `json:"model_display_name"`
	ModelCategoryJSON             string  `json:"model_category_json"`
	UsageKind                     string  `json:"usage_kind"`
	CallCount                     int     `json:"call_count"`
	InputTokens                   int     `json:"input_tokens"`
	OutputTokens                  int     `json:"output_tokens"`
	CachedInputTokens             int     `json:"cached_input_tokens"`
	ReasoningTokens               int     `json:"reasoning_tokens"`
	EmbeddingTokens               int     `json:"embedding_tokens"`
	TotalTokens                   int     `json:"total_tokens"`
	InputPriceMicroUSDPer1K       int64   `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64   `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64   `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64   `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64   `json:"embedding_price_micro_usd_per_1k"`
	InputCostMicroUSD             int64   `json:"input_cost_micro_usd"`
	OutputCostMicroUSD            int64   `json:"output_cost_micro_usd"`
	CachedInputCostMicroUSD       int64   `json:"cached_input_cost_micro_usd"`
	ReasoningCostMicroUSD         int64   `json:"reasoning_cost_micro_usd"`
	EmbeddingCostMicroUSD         int64   `json:"embedding_cost_micro_usd"`
	TotalCostMicroUSD             int64   `json:"total_cost_micro_usd"`
	LatencyMS                     int     `json:"latency_ms"`
	TimeToFirstTokenMS            int     `json:"time_to_first_token_ms"`
	TokensPerSecond               float64 `json:"tokens_per_second"`
	Status                        string  `json:"status"`
	ErrorCode                     string  `json:"error_code"`
	ErrorMessage                  string  `json:"error_message"`
	RetryCount                    int     `json:"retry_count"`
	PromptMode                    string  `json:"prompt_mode"`
	MaxOutputTokens               int     `json:"max_output_tokens"`
	ContextWindowK                int     `json:"context_window_k"`
	StreamEnabled                 bool    `json:"stream_enabled"`
	MetadataJSON                  string  `json:"metadata_json"`
	CreatedAt                     string  `json:"created_at"`
}

type ModelPricingRule struct {
	ID                            string `json:"id"`
	ProviderCode                  string `json:"provider_code"`
	ModelAPIID                    string `json:"model_api_id"`
	Currency                      string `json:"currency"`
	InputPriceMicroUSDPer1K       int64  `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64  `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64  `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64  `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64  `json:"embedding_price_micro_usd_per_1k"`
	EffectiveFrom                 string `json:"effective_from"`
	EffectiveTo                   string `json:"effective_to"`
	IsActive                      bool   `json:"is_active"`
	Source                        string `json:"source"`
	MetadataJSON                  string `json:"metadata_json"`
	CreatedAt                     string `json:"created_at"`
	UpdatedAt                     string `json:"updated_at"`
}

type ModelTokenUsageDaily struct {
	ID                 string  `json:"id"`
	DateKey            string  `json:"date_key"`
	WorkspaceID        string  `json:"workspace_id"`
	AgentID            string  `json:"agent_id"`
	AgentKey           string  `json:"agent_key"`
	ProviderCode       string  `json:"provider_code"`
	ModelAPIID         string  `json:"model_api_id"`
	UsageKind          string  `json:"usage_kind"`
	CallCount          int     `json:"call_count"`
	RequestCount       int     `json:"request_count"`
	SuccessCount       int     `json:"success_count"`
	FailedCount        int     `json:"failed_count"`
	CancelledCount     int     `json:"cancelled_count"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	CachedInputTokens  int     `json:"cached_input_tokens"`
	ReasoningTokens    int     `json:"reasoning_tokens"`
	EmbeddingTokens    int     `json:"embedding_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	TotalCostMicroUSD  int64   `json:"total_cost_micro_usd"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type ModelUsageQuery struct {
	Range        string `json:"range"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	ProviderCode string `json:"provider_code"`
	ModelAPIID   string `json:"model_api_id"`
	AgentID      string `json:"agent_id"`
	Status       string `json:"status"`
	Limit        int    `json:"limit"`
}

type ModelUsageSummary struct {
	CallCount          int     `json:"call_count"`
	RequestCount       int     `json:"request_count"`
	SuccessCount       int     `json:"success_count"`
	FailedCount        int     `json:"failed_count"`
	CancelledCount     int     `json:"cancelled_count"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	TotalCostMicroUSD  int64   `json:"total_cost_micro_usd"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
	SuccessRate        float64 `json:"success_rate"`
}

type ModelUsageTrendPoint struct {
	DateKey            string  `json:"date_key"`
	CallCount          int     `json:"call_count"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	TotalCostMicroUSD  int64   `json:"total_cost_micro_usd"`
	SuccessCount       int     `json:"success_count"`
	FailedCount        int     `json:"failed_count"`
	CancelledCount     int     `json:"cancelled_count"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
}

type ModelUsageBreakdownRow struct {
	ProviderCode       string  `json:"provider_code"`
	ModelAPIID         string  `json:"model_api_id"`
	ModelDisplayName   string  `json:"model_display_name"`
	AgentID            string  `json:"agent_id"`
	AgentKey           string  `json:"agent_key"`
	CallCount          int     `json:"call_count"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	TotalTokens        int     `json:"total_tokens"`
	TotalCostMicroUSD  int64   `json:"total_cost_micro_usd"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`
	SuccessRate        float64 `json:"success_rate"`
}

type ModelUsageOverview struct {
	Today     ModelUsageSummary        `json:"today"`
	Yesterday ModelUsageSummary        `json:"yesterday"`
	Month     ModelUsageSummary        `json:"month"`
	Range     ModelUsageSummary        `json:"range"`
	Trends    []ModelUsageTrendPoint   `json:"trends"`
	TopModels []ModelUsageBreakdownRow `json:"top_models"`
	TopAgents []ModelUsageBreakdownRow `json:"top_agents"`
	Anomalies []ModelTokenUsageEvent   `json:"anomalies"`
}

type ChatAttachment struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	MessageID    string `json:"message_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	SizeBytes    int64  `json:"size_bytes"`
	StorageKey   string `json:"storage_key"`
	Checksum     string `json:"checksum"`
	UploadStatus string `json:"upload_status"`
	CreatedAt    string `json:"created_at"`
	DeletedAt    string `json:"deleted_at"`
}

type ChatOption struct {
	Type         string `json:"type"`
	Key          string `json:"key"`
	Label        string `json:"label"`
	Enabled      bool   `json:"enabled"`
	SortOrder    int    `json:"sort_order"`
	MetadataJSON string `json:"metadata_json"`
}

type PlatformResource struct {
	ID           string `json:"id"`
	Resource     string `json:"resource"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Enabled      bool   `json:"enabled"`
	SortOrder    int    `json:"sort_order"`
	ParentID     string `json:"parent_id"`
	Level        string `json:"level"`
	AgentID      string `json:"agent_id"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	ConfigJSON   string `json:"config_json"`
	MetadataJSON string `json:"metadata_json"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	DeletedAt    string `json:"deleted_at"`
}

type PlatformResourceTreeNode struct {
	PlatformResource
	Children []PlatformResourceTreeNode `json:"children"`
}

type MCPServerTestResult struct {
	OK      bool           `json:"ok"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type CronTaskRunQuery struct {
	TaskID string `json:"cron_task_id"`
	Status string `json:"status"`
	Limit  int    `json:"limit"`
}

type CronTaskRun struct {
	ID           string `json:"id"`
	TaskID       string `json:"task_id"`
	TaskName     string `json:"task_name"`
	Status       string `json:"status"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	Trigger      string `json:"trigger"`
	RunID        string `json:"run_id"`
	OutputJSON   string `json:"output_json"`
	ErrorMessage string `json:"error_message"`
	CreatedAt    string `json:"created_at"`
}

type ChannelCatalogItem struct {
	Type             string         `json:"type"`
	Label            string         `json:"label"`
	Description      string         `json:"description"`
	Group            string         `json:"group"`
	ReceiveModes     []string       `json:"receive_modes"`
	Icon             string         `json:"icon"`
	Bundled          bool           `json:"bundled"`
	SupportsTest     bool           `json:"supports_test"`
	SupportsWebhook  bool           `json:"supports_webhook"`
	ConfigSchema     map[string]any `json:"config_schema"`
	CredentialSchema map[string]any `json:"credential_schema"`
	UIHints          map[string]any `json:"ui_hints"`
	SortOrder        int            `json:"sort_order"`
}

type ChannelCredential struct {
	ID            string `json:"id"`
	ChannelID     string `json:"channel_id"`
	CredentialKey string `json:"credential_key"`
	Status        string `json:"status"`
	SecretRef     string `json:"secret_ref,omitempty"`
	MetadataJSON  string `json:"metadata_json"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	DeletedAt     string `json:"deleted_at"`
	Configured    bool   `json:"configured"`
	MaskedPreview string `json:"masked_preview,omitempty"`
}

type ChannelCredentialInput struct {
	CredentialKey string `json:"credential_key"`
	Secret        string `json:"secret,omitempty"`
	SecretRef     string `json:"secret_ref,omitempty"`
	Status        string `json:"status,omitempty"`
	MetadataJSON  string `json:"metadata_json,omitempty"`
}

type ChannelDelivery struct {
	ID           string `json:"id"`
	ChannelID    string `json:"channel_id"`
	AgentID      string `json:"agent_id"`
	Status       string `json:"status"`
	PayloadJSON  string `json:"payload_json"`
	ErrorMessage string `json:"error_message"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type ChannelRuntimeConfig struct {
	ID           string              `json:"id"`
	Key          string              `json:"key"`
	Type         string              `json:"type"`
	Enabled      bool                `json:"enabled"`
	Status       string              `json:"status"`
	ConfigJSON   string              `json:"config_json"`
	MetadataJSON string              `json:"metadata_json"`
	Credentials  []ChannelCredential `json:"credentials"`
}

type ChannelTestResult struct {
	OK      bool           `json:"ok"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type PluginPermissions struct {
	CanView       bool `json:"can_view"`
	CanToggle     bool `json:"can_toggle"`
	CanEditConfig bool `json:"can_edit_config"`
	CanViewLogs   bool `json:"can_view_logs"`
}

type Plugin struct {
	ID                string            `json:"id"`
	Key               string            `json:"key"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Category          string            `json:"category"`
	RiskLevel         string            `json:"risk_level"`
	Enabled           bool              `json:"enabled"`
	Scope             string            `json:"scope"`
	CallbackPoints    []string          `json:"callback_points"`
	SortOrder         int               `json:"sort_order"`
	ConfigSchemaJSON  string            `json:"config_schema_json"`
	ConfigJSON        string            `json:"config_json"`
	DefaultConfigJSON string            `json:"default_config_json"`
	InvokeCount       int               `json:"invoke_count"`
	BlockCount        int               `json:"block_count"`
	ErrorCount        int               `json:"error_count"`
	LastInvokedAt     string            `json:"last_invoked_at,omitempty"`
	LastStatus        string            `json:"last_status,omitempty"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
	Permissions       PluginPermissions `json:"permissions"`
}

type PluginListQuery struct {
	Search        string `json:"search"`
	Category      string `json:"category"`
	Enabled       string `json:"enabled"`
	CallbackPoint string `json:"callback_point"`
	Limit         int    `json:"limit"`
	Offset        int    `json:"offset"`
}

type PluginListResult struct {
	Items  []Plugin `json:"items"`
	Total  int      `json:"total"`
	Limit  int      `json:"limit"`
	Offset int      `json:"offset"`
}

type PluginConfigUpdate struct {
	ConfigJSON string `json:"config_json"`
}

type ToolPermissions struct {
	CanManage bool `json:"can_manage"`
}

type Tool struct {
	ID                   string          `json:"id"`
	Key                  string          `json:"key"`
	DisplayName          string          `json:"display_name"`
	Description          string          `json:"description"`
	Category             string          `json:"category"`
	Source               string          `json:"source"`
	RiskLevel            string          `json:"risk_level"`
	Enabled              bool            `json:"enabled"`
	Readonly             bool            `json:"readonly"`
	RequiresConfirmation bool            `json:"requires_confirmation"`
	SupportsStreaming    bool            `json:"supports_streaming"`
	SupportsConcurrency  bool            `json:"supports_concurrency"`
	ParametersSchemaJSON string          `json:"parameters_schema_json"`
	ResultSchemaJSON     string          `json:"result_schema_json"`
	ConfigSchemaJSON     string          `json:"config_schema_json"`
	ConfigJSON           string          `json:"config_json"`
	DefaultConfigJSON    string          `json:"default_config_json"`
	MetadataJSON         string          `json:"metadata_json"`
	RuntimeStatus        string          `json:"runtime_status,omitempty"`
	RuntimeKind          string          `json:"runtime_kind,omitempty"`
	InvokeCount          int             `json:"invoke_count"`
	InvokeCount24h       int             `json:"invoke_count_24h"`
	SuccessCount         int             `json:"success_count"`
	FailureCount         int             `json:"failure_count"`
	BlockedCount         int             `json:"blocked_count"`
	AgentOverrideCount   int             `json:"agent_override_count"`
	AvgDurationMS        *float64        `json:"avg_duration_ms"`
	LastInvokedAt        string          `json:"last_invoked_at,omitempty"`
	LastStatus           string          `json:"last_status,omitempty"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	DeletedAt            string          `json:"deleted_at,omitempty"`
	Permissions          ToolPermissions `json:"permissions"`
}

type ToolUpsertInput struct {
	ID                   string `json:"id"`
	Key                  string `json:"key"`
	DisplayName          string `json:"display_name"`
	Description          string `json:"description"`
	Category             string `json:"category"`
	Source               string `json:"source"`
	RiskLevel            string `json:"risk_level"`
	Enabled              bool   `json:"enabled"`
	Readonly             bool   `json:"readonly"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
	SupportsStreaming    bool   `json:"supports_streaming"`
	SupportsConcurrency  bool   `json:"supports_concurrency"`
	ParametersSchemaJSON string `json:"parameters_schema_json"`
	ResultSchemaJSON     string `json:"result_schema_json"`
	ConfigSchemaJSON     string `json:"config_schema_json"`
	ConfigJSON           string `json:"config_json"`
	DefaultConfigJSON    string `json:"default_config_json"`
	MetadataJSON         string `json:"metadata_json"`
}

type ToolListQuery struct {
	Search    string `json:"search"`
	Category  string `json:"category"`
	Source    string `json:"source"`
	RiskLevel string `json:"risk_level"`
	Enabled   string `json:"enabled"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

type ToolListResult struct {
	Items   []Tool      `json:"items"`
	Total   int         `json:"total"`
	Limit   int         `json:"limit"`
	Offset  int         `json:"offset"`
	Summary ToolSummary `json:"summary"`
}

type ToolSummary struct {
	TotalTools      int     `json:"total_tools"`
	EnabledTools    int     `json:"enabled_tools"`
	HighRiskEnabled int     `json:"high_risk_enabled"`
	Calls24h        int     `json:"calls_24h"`
	FailureRate24h  float64 `json:"failure_rate_24h"`
}

type ToolInvocation struct {
	ID               string `json:"id"`
	RequestID        string `json:"request_id"`
	InvocationID     string `json:"invocation_id"`
	ToolID           string `json:"tool_id"`
	ToolKey          string `json:"tool_key"`
	ToolDisplayName  string `json:"tool_display_name"`
	AgentID          string `json:"agent_id"`
	AgentKey         string `json:"agent_key"`
	AgentDisplayName string `json:"agent_display_name"`
	SessionID        string `json:"session_id"`
	MessageID        string `json:"message_id"`
	UserID           string `json:"user_id"`
	Source           string `json:"source"`
	Status           string `json:"status"`
	StartedAt        string `json:"started_at"`
	EndedAt          string `json:"ended_at"`
	DurationMS       int    `json:"duration_ms"`
	InputPreview     string `json:"input_preview"`
	InputHash        string `json:"input_hash"`
	OutputPreview    string `json:"output_preview"`
	OutputHash       string `json:"output_hash"`
	ErrorCode        string `json:"error_code"`
	ErrorMessage     string `json:"error_message"`
	RedactionApplied bool   `json:"redaction_applied"`
	MetadataJSON     string `json:"metadata_json"`
	CreatedAt        string `json:"created_at"`
}

type ToolRunQuery struct {
	ToolKey   string `json:"tool_key"`
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	From      string `json:"from"`
	To        string `json:"to"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

type ToolRunResult struct {
	Items  []ToolInvocation `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

type EffectiveAgentTool struct {
	ToolKey        string `json:"tool_key"`
	DisplayName    string `json:"display_name"`
	Category       string `json:"category"`
	Source         string `json:"source"`
	Enabled        bool   `json:"enabled"`
	EffectiveState string `json:"effective_state"`
	Reason         string `json:"reason"`
}

type AgentEffectiveTools struct {
	ToolsEnabled bool                 `json:"tools_enabled"`
	Profile      string               `json:"profile"`
	Allow        []string             `json:"allow"`
	Deny         []string             `json:"deny"`
	Items        []EffectiveAgentTool `json:"items"`
}

type SkillTag struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type SkillVersionSummary struct {
	ID               string `json:"id"`
	Version          string `json:"version"`
	ValidationStatus string `json:"validation_status"`
	PublishedAt      string `json:"published_at"`
}

type SkillPermissions struct {
	CanEdit          bool `json:"can_edit"`
	CanDelete        bool `json:"can_delete"`
	CanToggleEnabled bool `json:"can_toggle_enabled"`
	CanDuplicate     bool `json:"can_duplicate"`
}

type Skill struct {
	ID                   string               `json:"id"`
	Name                 string               `json:"name"`
	Slug                 string               `json:"slug"`
	Description          string               `json:"description"`
	Tags                 []SkillTag           `json:"tags"`
	ExtendsSkillID       string               `json:"extends_skill_id,omitempty"`
	Status               string               `json:"status"`
	Enabled              bool                 `json:"enabled"`
	CurrentVersion       *SkillVersionSummary `json:"current_version"`
	InvokeCount          int                  `json:"invoke_count"`
	SuccessCount         int                  `json:"success_count"`
	FailureCount         int                  `json:"failure_count"`
	UsageCount7d         int                  `json:"usage_count_7d"`
	AvgDurationMS        *float64             `json:"avg_duration_ms"`
	LastAgentID          string               `json:"last_agent_id,omitempty"`
	LastAgentDisplayName string               `json:"last_agent_display_name,omitempty"`
	LastInvokedAt        string               `json:"last_invoked_at,omitempty"`
	LastDurationMS       *int                 `json:"last_duration_ms"`
	CreatedAt            string               `json:"created_at"`
	UpdatedAt            string               `json:"updated_at"`
	Permissions          SkillPermissions     `json:"permissions"`
}

type SkillListQuery struct {
	Search  string `json:"search"`
	Tags    string `json:"tags"`
	Enabled string `json:"enabled"`
	Status  string `json:"status"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

type SkillListResult struct {
	Items  []Skill `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type SkillInvocationPermissions struct {
	CanViewDetail bool `json:"can_view_detail"`
}

type SkillInvocation struct {
	ID               string                     `json:"id"`
	SkillID          string                     `json:"skill_id"`
	SkillName        string                     `json:"skill_name"`
	SkillVersion     string                     `json:"skill_version"`
	AgentID          string                     `json:"agent_id"`
	AgentDisplayName string                     `json:"agent_display_name"`
	UserID           string                     `json:"user_id,omitempty"`
	SessionID        string                     `json:"session_id,omitempty"`
	Status           string                     `json:"status"`
	DurationMS       int                        `json:"duration_ms"`
	StartedAt        string                     `json:"started_at"`
	EndedAt          string                     `json:"ended_at,omitempty"`
	InputPreview     string                     `json:"input_preview,omitempty"`
	InputHash        string                     `json:"input_hash,omitempty"`
	OutputPreview    string                     `json:"output_preview,omitempty"`
	ErrorCode        string                     `json:"error_code,omitempty"`
	ErrorMessage     string                     `json:"error_message,omitempty"`
	Permissions      SkillInvocationPermissions `json:"permissions"`
}

type SkillRunQuery struct {
	SkillID   string `json:"skill_id"`
	AgentID   string `json:"agent_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	From      string `json:"from"`
	To        string `json:"to"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

type SkillRunResult struct {
	Items  []SkillInvocation `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

type SkillImportJob struct {
	JobID            string                 `json:"job_id"`
	Status           string                 `json:"status"`
	ValidationStatus string                 `json:"validation_status"`
	StorageRoot      string                 `json:"storage_root"`
	Candidates       []SkillImportCandidate `json:"candidates"`
	ConflictGroups   []SkillConflictGroup   `json:"conflict_groups"`
	Message          string                 `json:"message,omitempty"`
}

type SkillImportCandidate struct {
	CandidateID      string             `json:"candidate_id"`
	Name             string             `json:"name"`
	Slug             string             `json:"slug"`
	Description      string             `json:"description"`
	BodyPreview      string             `json:"body_preview"`
	TargetDir        string             `json:"target_dir"`
	ValidationStatus string             `json:"validation_status"`
	StatusIcon       string             `json:"status_icon"`
	Warnings         []SkillImportIssue `json:"warnings"`
	Blocks           []SkillImportIssue `json:"blocks"`
}

type SkillImportIssue struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type SkillSimilarityMetrics struct {
	SimilarityScore       float64 `json:"similarity_score"`
	NameSimilarity        float64 `json:"name_similarity"`
	DescriptionSimilarity float64 `json:"description_similarity"`
	BodySimilarity        float64 `json:"body_similarity"`
	TriggerSimilarity     float64 `json:"trigger_similarity"`
	ToolSimilarity        float64 `json:"tool_similarity"`
	ConflictRisk          string  `json:"conflict_risk"`
	Recommendation        string  `json:"recommendation"`
	Confidence            float64 `json:"confidence"`
}

type SkillConflictGroup struct {
	GroupID                string                  `json:"group_id"`
	HighestSimilarityScore float64                 `json:"highest_similarity_score"`
	Metrics                SkillSimilarityMetrics  `json:"metrics"`
	Reason                 string                  `json:"reason"`
	Evidence               []string                `json:"evidence"`
	CandidateIDs           []string                `json:"candidate_ids"`
	ExistingSkills         []SkillSimilaritySource `json:"existing_skills"`
	CanRefine              bool                    `json:"can_refine"`
}

type SkillSimilaritySource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Version     string `json:"version"`
	BodyPreview string `json:"body_preview"`
	Body        string `json:"-"`
}

type SkillRefineRequest struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

type SkillRefineResult struct {
	MergedName             string     `json:"merged_name"`
	MergedDescription      string     `json:"merged_description"`
	MergedBody             string     `json:"merged_body"`
	MergedTags             []SkillTag `json:"merged_tags"`
	SourceCandidateIDs     []string   `json:"source_candidate_ids"`
	SourceExistingSkillIDs []string   `json:"source_existing_skill_ids"`
}

type SkillImportDecision struct {
	CandidateID       string     `json:"candidate_id,omitempty"`
	GroupID           string     `json:"group_id,omitempty"`
	Action            string     `json:"action"`
	MergedName        string     `json:"merged_name,omitempty"`
	MergedDescription string     `json:"merged_description,omitempty"`
	MergedBody        string     `json:"merged_body,omitempty"`
	MergedTags        []SkillTag `json:"merged_tags,omitempty"`
}

type SkillImportApplyRequest struct {
	Decisions []SkillImportDecision `json:"decisions"`
}

type SkillImportApplyResult struct {
	CreatedSkillIDs     []string `json:"created_skill_ids"`
	SkippedCandidateIDs []string `json:"skipped_candidate_ids"`
	Message             string   `json:"message"`
}

type SkillCreateInput struct {
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description string     `json:"description"`
	Body        string     `json:"body"`
	Tags        []SkillTag `json:"tags"`
	StorageDir  string     `json:"storage_dir"`
}

type SkillFile struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updated_at"`
}

type SkillFileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language"`
}

type SkillFileUpdateInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type AvatarAsset struct {
	ID            string `json:"id"`
	Key           string `json:"key"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	MimeType      string `json:"mime_type"`
	WorkspaceID   string `json:"workspace_id"`
	OwnerUserID   string `json:"owner_user_id"`
	Source        string `json:"source"`
	IsSystem      bool   `json:"is_system"`
	FileSizeBytes int    `json:"file_size_bytes"`
	WidthPx       int    `json:"width_px"`
	HeightPx      int    `json:"height_px"`
	SortOrder     int    `json:"sort_order"`
	CreatedAt     string `json:"created_at"`
}

type AvatarImage struct {
	ID       string
	MimeType string
	Data     []byte
}

type ValidateModelInput struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ValidateModelResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type InspectProviderModelInput struct {
	ResourceID   string `json:"resource_id"`
	ProviderCode string `json:"provider_code"`
	ProviderType string `json:"provider_type"`
	ModelAPIID   string `json:"model_api_id"`
	APIBaseURL   string `json:"api_base_url"`
	APIKey       string `json:"api_key"`
}

type InspectProviderModelResult struct {
	OK                            bool   `json:"ok"`
	Message                       string `json:"message"`
	ProviderCode                  string `json:"provider_code"`
	ProviderType                  string `json:"provider_type"`
	ModelAPIID                    string `json:"model_api_id"`
	ModelDisplayName              string `json:"model_display_name"`
	ModelSizeLabel                string `json:"model_size_label"`
	ContextWindowK                int    `json:"context_window_k"`
	MaxOutputTokens               int    `json:"max_output_tokens"`
	InputPriceMicroUSDPer1K       int64  `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64  `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64  `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64  `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64  `json:"embedding_price_micro_usd_per_1k"`
	Source                        string `json:"source"`
	RawMetadataJSON               string `json:"raw_metadata_json"`
}

type AuditLog struct {
	ID         string `json:"id"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	RequestID  string `json:"request_id"`
	Detail     string `json:"detail"`
	CreatedAt  string `json:"created_at"`
}
