package domain

// L0Settings 记录驱动 L0（感知 / 上下文窗口）记忆层的智能体级配置，见 `12 memory-L0-sensory.md`。
// 各开关持久化在 agent_runtime_settings，与既有 memory_*、tools_* 配置并列。
type L0Settings struct {
	RecentWindowTurns  int     `json:"recent_window_turns"`
	RecentWindowTokens int     `json:"recent_window_tokens"`
	SummaryThreshold   float64 `json:"summary_threshold"`
	SummaryKeepTurns   int     `json:"summary_keep_turns"`
	TruncateStrategy   string  `json:"truncate_strategy"`
	InjectL1           bool    `json:"inject_l1"`
	InjectL3           bool    `json:"inject_l3"`
	InjectL4           bool    `json:"inject_l4"`
	L3MaxChunks        int     `json:"l3_max_chunks"`
	L4MaxPaths         int     `json:"l4_max_paths"`
	SnapshotMode       string  `json:"snapshot_mode"`
}

// L0Segment 是组装后提示中的一块。Content 为送入模型的内容；Preview 为持久化供调试 / 审计的摘要。
type L0Segment struct {
	Section string `json:"section"`
	Role    string `json:"role"`
	Source  string `json:"source"`
	Tokens  int    `json:"tokens"`
	Content string `json:"content,omitempty"`
	Preview string `json:"preview"`
}

// L0AssemblyRequest 为 ChatService / TeamRuntime 构建模型提示时的输入。
// ContextWindow / ReservedForOutput 取自所选提供商模型，故预算按每次调用计算。
type L0AssemblyRequest struct {
	SessionID         string      `json:"session_id"`
	RunID             string      `json:"run_id,omitempty"`
	TurnID            string      `json:"turn_id,omitempty"`
	SpanID            string      `json:"span_id,omitempty"`
	AgentID           string      `json:"agent_id,omitempty"`
	TeamID            string      `json:"team_id,omitempty"`
	UserID            string      `json:"user_id,omitempty"`
	WorkspaceID       string      `json:"workspace_id,omitempty"`
	Provider          string      `json:"provider,omitempty"`
	Model             string      `json:"model,omitempty"`
	ContextWindow     int         `json:"context_window,omitempty"`
	ReservedForOutput int         `json:"reserved_for_output,omitempty"`
	UserMessage       string      `json:"user_message"`
	UserMessageID     string      `json:"user_message_id,omitempty"`
	ExtraSystemBlocks []L0Segment `json:"extra_system_blocks,omitempty"`
}

// L0MemoryScopeContext 将调用方在 L0 可见的记忆作用域传入 L3 / L4 回忆。
// 旧调用点仅提供 session_id 与 agent_id，导致运行时设置中的用户 / 团队 / 工作区回忆作用域无法参与主聊天组装路径。
type L0MemoryScopeContext struct {
	SessionID   string `json:"session_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	TeamID      string `json:"team_id,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Query       string `json:"query,omitempty"`
}

// L0ChatMessage 为交还给 ChatService 的角色 / 内容二元组，可直接传入运行时适配器而不泄露布局细节。
type L0ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// L0AssemblyResult 为 MemoryL0Service.Assemble 的返回值。PromptMessages 为最终送入模型的数组；
// 其余元数据供快照、追踪跨度与提示调试 UI 使用。
type L0AssemblyResult struct {
	Segments              []L0Segment     `json:"segments"`
	PromptMessages        []L0ChatMessage `json:"prompt_messages"`
	BudgetTokens          int             `json:"budget_tokens"`
	PromptTokenEstimate   int             `json:"prompt_token_estimate"`
	UsedRatioEstimate     float64         `json:"used_ratio_estimate"`
	RecentWindowTurns     int             `json:"recent_window_turns"`
	RecentWindowTokens    int             `json:"recent_window_tokens"`
	SummarizedTurnFrom    int             `json:"summarized_turn_from"`
	SummarizedTurnTo      int             `json:"summarized_turn_to"`
	TruncateStrategy      string          `json:"truncate_strategy"`
	TruncatedMessageCount int             `json:"truncated_message_count"`
	WarningCodes          []string        `json:"warning_codes"`
	SnapshotID            string          `json:"snapshot_id,omitempty"`
}

// L0AssemblySnapshot 对应 memory_l0_assembly_snapshots 表的一行。
// 为单次 L0 组装的可审计记录，供运维 / 智能体进化分析回放提示构造过程。
type L0AssemblySnapshot struct {
	ID                    string  `json:"id"`
	SessionID             string  `json:"session_id"`
	RunID                 string  `json:"run_id"`
	TurnID                string  `json:"turn_id"`
	SpanID                string  `json:"span_id"`
	AgentID               string  `json:"agent_id"`
	TeamID                string  `json:"team_id"`
	Provider              string  `json:"provider"`
	Model                 string  `json:"model"`
	ContextWindowTokens   int     `json:"context_window_tokens"`
	BudgetTokens          int     `json:"budget_tokens"`
	RecentWindowTurns     int     `json:"recent_window_turns"`
	RecentWindowTokens    int     `json:"recent_window_tokens"`
	SummaryTokenEstimate  int     `json:"summary_token_estimate"`
	L1FieldCount          int     `json:"l1_field_count"`
	L1TokenEstimate       int     `json:"l1_token_estimate"`
	L3ChunkCount          int     `json:"l3_chunk_count"`
	L3TokenEstimate       int     `json:"l3_token_estimate"`
	L4PathCount           int     `json:"l4_path_count"`
	L4TokenEstimate       int     `json:"l4_token_estimate"`
	PromptTokenEstimate   int     `json:"prompt_token_estimate"`
	PromptTokenActual     int     `json:"prompt_token_actual"`
	UsedRatio             float64 `json:"used_ratio"`
	TruncateStrategy      string  `json:"truncate_strategy"`
	TruncatedMessageCount int     `json:"truncated_message_count"`
	SummarizedTurnFrom    int     `json:"summarized_turn_from"`
	SummarizedTurnTo      int     `json:"summarized_turn_to"`
	SegmentsJSON          string  `json:"segments_json"`
	WarningCodesJSON      string  `json:"warning_codes_json"`
	MetadataJSON          string  `json:"metadata_json"`
	CreatedAt             string  `json:"created_at"`
}

// SessionSummary 为 SummaryService 写入的 `from_turn..to_turn` 区间浓缩纪要，由 L0 消费以在预算内保留较早历史。
type SessionSummary struct {
	ID              string `json:"id"`
	SessionID       string `json:"session_id"`
	SummaryMarkdown string `json:"summary_markdown"`
	FromTurn        int    `json:"from_turn"`
	ToTurn          int    `json:"to_turn"`
	TokenEstimate   int    `json:"token_estimate"`
	CreatedAt       string `json:"created_at"`
}
