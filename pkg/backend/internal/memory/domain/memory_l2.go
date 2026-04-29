// Package domain – L2 情景记忆领域类型，见 `aranea/docs/14 memory-L2-episodic.md`。
// L2 记录会话的统一事件流（消息 + 工具 / 技能 / MCP 调用 + L1 任务快照），
// 并暴露按会话的片段（episode），供 L3 / L4 合并使用。
package domain

// EpisodeKind 枚举创建片段的高层级原因。
// 字符串持久化在 `memory_episodes.episode_kind`，变更需迁移。
type EpisodeKind string

const (
	EpisodeKindTask         EpisodeKind = "task"
	EpisodeKindMilestone    EpisodeKind = "milestone"
	EpisodeKindFailurePM    EpisodeKind = "failure_postmortem"
	EpisodeKindUserMarked   EpisodeKind = "user_marked"
	EpisodeKindCriticPass   EpisodeKind = "critic_pass"
)

// IsValid 在值与上述种类之一匹配时为 true。
// 调用方将空输入视为 "task"；该转换在服务层完成，领域层不含默认值。
func (k EpisodeKind) IsValid() bool {
	switch k {
	case EpisodeKindTask, EpisodeKindMilestone, EpisodeKindFailurePM, EpisodeKindUserMarked, EpisodeKindCriticPass:
		return true
	}
	return false
}

// MemoryEpisode 为 `memory_episodes` 表的持久化行。字段名与 SQL 列一致，
// JSON 标签兼作 §6.3 HTTP API 的线格式。
type MemoryEpisode struct {
	ID                  string  `json:"id"`
	SessionID           string  `json:"session_id"`
	RunID               string  `json:"run_id,omitempty"`
	TeamID              string  `json:"team_id,omitempty"`
	AgentID             string  `json:"agent_id,omitempty"`
	L1TaskID            string  `json:"l1_task_id,omitempty"`
	Kind                EpisodeKind `json:"episode_kind"`
	Title               string  `json:"title"`
	Goal                string  `json:"goal,omitempty"`
	Outcome             string  `json:"outcome,omitempty"`
	OutcomeSummary      string  `json:"outcome_summary,omitempty"`
	ResultPreview       string  `json:"result_preview,omitempty"`
	FailureReason       string  `json:"failure_reason,omitempty"`
	Importance          float64 `json:"importance"`
	Confidence          float64 `json:"confidence"`
	UserFeedback        string  `json:"user_feedback,omitempty"`
	CriticScore         float64 `json:"critic_score"`
	SpanCount           int     `json:"span_count"`
	MessageCount        int     `json:"message_count"`
	ToolCallCount       int     `json:"tool_call_count"`
	SkillCallCount      int     `json:"skill_call_count"`
	MCPCallCount        int     `json:"mcp_call_count"`
	TotalTokens         int     `json:"total_tokens"`
	TotalCostMicroUSD   int64   `json:"total_cost_micro_usd"`
	DurationMS          int     `json:"duration_ms"`
	L1SnapshotJSON      string  `json:"l1_snapshot_json,omitempty"`
	KeyDecisionsJSON    string  `json:"key_decisions_json,omitempty"`
	KeyArtifactsJSON    string  `json:"key_artifacts_json,omitempty"`
	EmbeddingStatus     string  `json:"embedding_status,omitempty"`
	EmbeddingModel      string  `json:"embedding_model,omitempty"`
	EmbeddingDim        int     `json:"embedding_dim,omitempty"`
	EmbeddingNorm       float64 `json:"embedding_norm,omitempty"`
	ConsolidationStatus string  `json:"consolidation_status"`
	ConsolidatedAt      string  `json:"consolidated_at,omitempty"`
	ConsolidatedL3Count int     `json:"consolidated_l3_count"`
	ConsolidatedL4Count int     `json:"consolidated_l4_count"`
	StartedAt           string  `json:"started_at,omitempty"`
	EndedAt             string  `json:"ended_at,omitempty"`
	MetadataJSON        string  `json:"metadata_json,omitempty"`
	CreatedAt           string  `json:"created_at,omitempty"`
	UpdatedAt           string  `json:"updated_at,omitempty"`
	ArchivedAt          string  `json:"archived_at,omitempty"`
	DeletedAt           string  `json:"deleted_at,omitempty"`
}

// MemoryL2Event 为 §6.2 暴露的只读统一事件行。
// 物理来源可为 `messages`、`session_trace_spans`、`tool_invocations`、`skill_invocation`、
// `team_run_steps`、`monitor_events` — `RefTable` / `RefID` 供调用方回链到权威行。
type MemoryL2Event struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	SessionID  string         `json:"session_id"`
	RunID      string         `json:"run_id,omitempty"`
	TurnID     string         `json:"turn_id,omitempty"`
	SpanID     string         `json:"span_id,omitempty"`
	ActorType  string         `json:"actor_type,omitempty"`
	ActorID    string         `json:"actor_id,omitempty"`
	ActorName  string         `json:"actor_name,omitempty"`
	Status     string         `json:"status,omitempty"`
	Title      string         `json:"title,omitempty"`
	Preview    string         `json:"preview,omitempty"`
	OccurredAt string         `json:"occurred_at"`
	DurationMS int            `json:"duration_ms,omitempty"`
	TokensIn   int            `json:"tokens_in,omitempty"`
	TokensOut  int            `json:"tokens_out,omitempty"`
	CostMicro  int64          `json:"cost_micro_usd,omitempty"`
	RefTable   string         `json:"ref_table"`
	RefID      string         `json:"ref_id"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// MemoryL2EventQuery 为 §6.2 接受的过滤集合。空字段视为「匹配任意」；
// 空 session_id 在服务层拒绝，避免误返回跨会话数据。
type MemoryL2EventQuery struct {
	SessionID    string   `json:"session_id"`
	TurnID       string   `json:"turn_id,omitempty"`
	SpanID       string   `json:"span_id,omitempty"`
	Kinds        []string `json:"kinds,omitempty"`
	ActorIDs     []string `json:"actor_ids,omitempty"`
	StatusIn     []string `json:"status_in,omitempty"`
	StartTimeUTC string   `json:"start_time,omitempty"`
	EndTimeUTC   string   `json:"end_time,omitempty"`
	Keyword      string   `json:"keyword,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	Offset       int      `json:"offset,omitempty"`
}

// MemoryL2RecallQuery 为 RecallByQuery / POST /l2/recall 的输入。
// QueryEmbedding 可选：为空时服务回退为仅 BM25 排序（第二阶段）。
type MemoryL2RecallQuery struct {
	SessionID      string        `json:"session_id"`
	AgentID        string        `json:"agent_id,omitempty"`
	Query          string        `json:"query"`
	QueryEmbedding []float32     `json:"query_embedding,omitempty"`
	MinImportance  float64       `json:"min_importance,omitempty"`
	TopK           int           `json:"top_k,omitempty"`
	IncludeKinds   []EpisodeKind `json:"include_kinds,omitempty"`
}

// MemoryL2RecallResult 为 §6.5 返回的行形态。序列化前会裁剪 Episode（仅摘要字段），以控制提示增长。
type MemoryL2RecallResult struct {
	Episode   MemoryEpisode `json:"episode"`
	BM25Score float64       `json:"bm25_score,omitempty"`
	VectorSim float64       `json:"vector_sim,omitempty"`
	FinalRank float64       `json:"final_rank"`
}

// MemoryEventMark 为 `memory_event_marks` 表的持久化行。
// (RefKind, RefID, MarkType, MarkedBy) 唯一，同一主体对同一事件重复标记幂等（UPSERT）。
type MemoryEventMark struct {
	ID           string         `json:"id"`
	SessionID    string         `json:"session_id"`
	EpisodeID    string         `json:"episode_id,omitempty"`
	RefKind      string         `json:"ref_kind"`
	RefID        string         `json:"ref_id"`
	MarkType     string         `json:"mark_type"`
	MarkedBy     string         `json:"marked_by,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Weight       float64        `json:"weight"`
	MetadataJSON string         `json:"metadata_json,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    string         `json:"created_at,omitempty"`
	DeletedAt    string         `json:"deleted_at,omitempty"`
}

// MemoryL2IndexEntry 对应 `memory_l2_index_meta`。第二阶段对每个 (episode, text_kind) 写入一行；
// FTS5 虚拟表保存分词文本供 BM25 排序。
type MemoryL2IndexEntry struct {
	ID             string  `json:"id"`
	EpisodeID      string  `json:"episode_id"`
	SessionID      string  `json:"session_id"`
	AgentID        string  `json:"agent_id,omitempty"`
	TextKind       string  `json:"text_kind"`
	TextPreview    string  `json:"text_preview,omitempty"`
	TokenEstimate  int     `json:"token_estimate"`
	EmbeddingModel string  `json:"embedding_model,omitempty"`
	EmbeddingDim   int     `json:"embedding_dim,omitempty"`
	EmbeddingNorm  float64 `json:"embedding_norm,omitempty"`
	Importance     float64 `json:"importance"`
	CreatedAt      string  `json:"created_at,omitempty"`
	UpdatedAt      string  `json:"updated_at,omitempty"`
}

// L2KeyDecision 与 L2KeyArtifact 为打包进 `key_decisions_json` / `key_artifacts_json` 的结构形态。
// 以 JSON 字符串持久化可控制列数，同时 API / 前端仍可将其作为一等列表渲染。
type L2KeyDecision struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale,omitempty"`
	At        string `json:"at,omitempty"`
	SpanID    string `json:"span_id,omitempty"`
}

type L2KeyArtifact struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Preview string `json:"preview,omitempty"`
}
