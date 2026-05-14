package biz

// Agent is the catalog agent aggregate (legacy agents table + hydrated runtime state).
type Agent struct {
	ID                 string
	AgentKey           string
	DisplayName        string
	Provider           string
	Model              string
	Status             string
	IsDefault          bool
	IsFavorite         bool
	Icon               string
	AgentDescription   string
	CategoryPositionID string
	SystemPromptMode   string
	ContextWindow      int
	BudgetMonthlyCents int
	ConfigJSON         string
	CreatedAt          string
	UpdatedAt          string
	DeletedAt          string
	Settings           *AgentRuntimeSettings
	Files              []AgentPromptFile
}

// AgentRuntimeSettings mirrors agent_runtime_settings row.
type AgentRuntimeSettings struct {
	AgentID                           string
	SelfEvolve                        bool
	SubagentsEnabled                  bool
	SubagentsMaxConcurrency           int
	SubagentsMaxGenerationDepth       int
	SubagentsMaxChildrenPerAgent      int
	SubagentsArchiveAfterMinutes      int
	SubagentsMaxRetries               int
	SubagentsModelOverride            string
	ToolsEnabled                      bool
	ToolsProfile                      string
	ToolsToolCallPrefix               string
	ToolsAllowJSON                    string
	ToolsDenyJSON                     string
	ToolsConcurrentAllowJSON          string
	MemoryEnabled                     bool
	MemoryMaxChunkLength              int
	MemoryMaxResults                  int
	MemoryMinScore                    float64
	HeartbeatEnabled                  bool
	HeartbeatIntervalMinutes          int
	EvolutionSelfEvolve               bool
	EvolutionSkillEvolve              bool
	EvolutionMetricsEnabled           bool
	EvolutionSuggestionsEnabled       bool
	GuardrailMaxChangePerPeriod       float64
	GuardrailMinDataPoints            int
	GuardrailRollbackOnDeclinePercent int
	L0RecentWindowTurns               int
	L0RecentWindowTokens              int
	L0SummaryThreshold                float64
	L0SummaryKeepTurns                int
	// L0CompressProvider / L0CompressModel select a cheaper catalog model for session summarization; empty → use agent/session chat model.
	L0CompressProvider string
	L0CompressModel    string
	L0TruncateStrategy string
	L0InjectL1                        bool
	L0InjectL3                        bool
	L0InjectL4                        bool
	L0L3MaxChunks                     int
	L0L4MaxPaths                      int
	L0SnapshotMode                    string
	L1Enabled                         bool
	L1BudgetTokens                    int
	L1FieldMaxTokens                  int
	L1HistoryKeepRevisions            int
	L1DefaultSchemaID                 string
	L1ArchiveOnIdleMinutes            int
	L2EpisodeEnabled                  bool
	L2EpisodeMinImportance            float64
	L2IndexEnabled                    bool
	L2IndexEmbeddingModel             string
	L2RecallEnabled                   bool
	L2RecallMax                       int
	L2RetentionDays                   int
	L2ArchiveAfterDays                int
	L3Enabled                         bool
	L3RecallTopK                      int
	L3RecallMinScore                  float64
	L3RecallScopesJSON                string
	L3EmbeddingModel                  string
	L3DecayIntervalHours              int
	L3ArchiveThreshold                float64
	L3MaxPerRecallChars               int
	L4Enabled                         bool
	L4GraphInjectNeighbors            bool
	L4GraphMaxNeighbors               int
	L4GraphMaxHops                    int
	L4IdentityInject                  bool
	L4StrategyInject                  bool
	EvoEnabled                        bool
	EvoAutoApply                      bool
	EvoMinEpisodes                    int
	EvoMinNegativeFeedback            int
	EvoThrottleHours                  int
	EvoProposalTTLDays                int
	EvoPersonaMaxChars                int
	EvoSystemPromptMaxAppends         int
	// SkillRuntimeJSON is agent_runtime_settings.skill_runtime_json (whitelist/deny/tags + routing caps).
	SkillRuntimeJSON string
	// IntentPassEnabled runs the optional pre-turn intent classification pass (see intent package); default true in DB/UI.
	IntentPassEnabled bool
	CreatedAt         string
	UpdatedAt         string
}

// AgentPromptFile is one row in agent_prompt_files (API name field maps to file_name).
type AgentPromptFile struct {
	ID        string
	AgentID   string
	Name      string
	Body      string
	SortOrder int
	CreatedAt string
	UpdatedAt string
}

// AgentListQuery filters the agent catalog list.
type AgentListQuery struct {
	Keyword    string
	Status     string
	Provider   string
	CategoryID string
	Limit      int
	Offset     int
}

// AgentListResult is a page of agents without per-row hydration unless noted.
type AgentListResult struct {
	Items  []Agent
	Total  int
	Limit  int
	Offset int
}

// FileTokenEstimate is the token estimate for a single prompt file.
type FileTokenEstimate struct {
	FileID          string
	FileName        string
	EstimatedTokens int
}

// FileTokenEstimates is the aggregate token estimate for all prompt files of an agent.
type FileTokenEstimates struct {
	TotalTokens    int
	FileEstimates  []FileTokenEstimate
}
