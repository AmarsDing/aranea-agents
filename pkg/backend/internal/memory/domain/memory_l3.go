// Package domain – L3 语义记忆领域类型，见 `aranea/docs/15 memory-L3-semantic.md`。
// L3 存储跨会话的结构化声明式事实（偏好、规则、术语表等），
// 作用域为智能体 / 用户 / 团队 / 工作区 / 全局，并通过 L0 组装注入 memory.l3 提示段。
package domain

// ScopeType 为记忆事实的存储作用域。字符串持久化在 `memory_facts.scope_type`，变更需迁移。
type ScopeType string

const (
	ScopeGlobal    ScopeType = "global"
	ScopeWorkspace ScopeType = "workspace"
	ScopeUser      ScopeType = "user"
	ScopeTeam      ScopeType = "team"
	ScopeAgent     ScopeType = "agent"
)

// IsValid 在值与已知作用域之一匹配时为 true。
func (s ScopeType) IsValid() bool {
	switch s {
	case ScopeGlobal, ScopeWorkspace, ScopeUser, ScopeTeam, ScopeAgent:
		return true
	}
	return false
}

// FactKind 对事实的语义内容分类。持久化在 `memory_facts.fact_kind`，并在提示渲染中暴露。
type FactKind string

const (
	FactPreference FactKind = "preference"
	FactRule       FactKind = "rule"
	FactPattern    FactKind = "pattern"
	FactPitfall    FactKind = "pitfall"
	FactGlossary   FactKind = "glossary"
	FactGeneric    FactKind = "fact"
)

// IsValid 在值与已知种类之一匹配时为 true。
func (k FactKind) IsValid() bool {
	switch k {
	case FactPreference, FactRule, FactPattern, FactPitfall, FactGlossary, FactGeneric:
		return true
	}
	return false
}

// 持久化在 `memory_facts.status` 的事实状态。
const (
	FactStatusActive     = "active"
	FactStatusArchived   = "archived"
	FactStatusDisputed   = "disputed"
	FactStatusDeprecated = "deprecated"
	FactStatusDeleted    = "deleted"
)

// 反馈类型取值。持久化在 `memory_fact_feedback.feedback_type`。
const (
	FactFeedbackConfirm = "confirm"
	FactFeedbackReject  = "reject"
	FactFeedbackRefine  = "refine"
	FactFeedbackIgnore  = "ignore"
	FactFeedbackUsed    = "used"
	FactFeedbackNotUsed = "not_used"
)

// 持久化在 `memory_fact_conflicts` 的冲突种类与状态。
const (
	FactConflictContradiction = "contradiction"
	FactConflictOverlap       = "overlap"
	FactConflictOutdated      = "outdated"
	FactConflictScopeMismatch = "scope_mismatch"

	FactConflictStatusOpen       = "open"
	FactConflictStatusResolved   = "resolved"
	FactConflictStatusIgnored    = "ignored"
	FactConflictStatusSuperseded = "superseded"
)

// MemoryFact 为 `memory_facts` 表的持久化行。字段名与 SQL 列一致，JSON 标签兼作 HTTP API §6.2 线格式。
type MemoryFact struct {
	ID                    string    `json:"id"`
	ScopeType             ScopeType `json:"scope_type"`
	ScopeID               string    `json:"scope_id"`
	WorkspaceID           string    `json:"workspace_id,omitempty"`
	UserID                string    `json:"user_id,omitempty"`
	TeamID                string    `json:"team_id,omitempty"`
	AgentID               string    `json:"agent_id,omitempty"`
	Statement             string    `json:"statement"`
	StatementNormalized   string    `json:"statement_normalized,omitempty"`
	Fingerprint           string    `json:"fingerprint,omitempty"`
	DetailsMarkdown       string    `json:"details_markdown,omitempty"`
	Kind                  FactKind  `json:"fact_kind"`
	Tags                  []string  `json:"tags"`
	Confidence            float64   `json:"confidence"`
	Importance            float64   `json:"importance"`
	UseCount              int       `json:"use_count"`
	HitCount              int       `json:"hit_count"`
	PositiveFeedbackCount int       `json:"positive_feedback_count"`
	NegativeFeedbackCount int       `json:"negative_feedback_count"`
	ConflictCount         int       `json:"conflict_count"`
	SourceKind            string    `json:"source_kind,omitempty"`
	SourceEpisodeID       string    `json:"source_episode_id,omitempty"`
	SourceSessionID       string    `json:"source_session_id,omitempty"`
	SourceMessageID       string    `json:"source_message_id,omitempty"`
	SourceExternal        string    `json:"source_external,omitempty"`
	Version               int       `json:"version"`
	Status                string    `json:"status"`
	SupersededBy          string    `json:"superseded_by,omitempty"`
	EmbeddingStatus       string    `json:"embedding_status,omitempty"`
	EmbeddingModel        string    `json:"embedding_model,omitempty"`
	EmbeddingDim          int       `json:"embedding_dim,omitempty"`
	EmbeddingBlob         []byte    `json:"-"`
	EmbeddingNorm         float64   `json:"embedding_norm,omitempty"`
	PIIFlag               bool      `json:"pii_flag,omitempty"`
	RedactedStatement     string    `json:"redacted_statement,omitempty"`
	TTLDays               int       `json:"ttl_days,omitempty"`
	DecayFactor           float64   `json:"decay_factor"`
	NextDecayAt           string    `json:"next_decay_at,omitempty"`
	LastUsedAt            string    `json:"last_used_at,omitempty"`
	ExpiresAt             string    `json:"expires_at,omitempty"`
	MetadataJSON          string    `json:"metadata_json,omitempty"`
	CreatedAt             string    `json:"created_at,omitempty"`
	UpdatedAt             string    `json:"updated_at,omitempty"`
	ArchivedAt            string    `json:"archived_at,omitempty"`
	DeletedAt             string    `json:"deleted_at,omitempty"`
}

// FactUpsertInput 为服务层接受的应用级写入载荷。Tags 与 Metadata 以 Go 原生类型传入，在仓库内序列化为 JSON。
type FactUpsertInput struct {
	ScopeType       ScopeType      `json:"scope_type"`
	ScopeID         string         `json:"scope_id,omitempty"`
	WorkspaceID     string         `json:"workspace_id,omitempty"`
	UserID          string         `json:"user_id,omitempty"`
	TeamID          string         `json:"team_id,omitempty"`
	AgentID         string         `json:"agent_id,omitempty"`
	Statement       string         `json:"statement"`
	DetailsMarkdown string         `json:"details_markdown,omitempty"`
	Kind            FactKind       `json:"fact_kind,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	Confidence      float64        `json:"confidence,omitempty"`
	Importance      float64        `json:"importance,omitempty"`
	SourceKind      string         `json:"source_kind,omitempty"`
	SourceEpisodeID string         `json:"source_episode_id,omitempty"`
	SourceSessionID string         `json:"source_session_id,omitempty"`
	SourceMessageID string         `json:"source_message_id,omitempty"`
	SourceExternal  string         `json:"source_external,omitempty"`
	TTLDays         int            `json:"ttl_days,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	By              string         `json:"by,omitempty"`
	Reason          string         `json:"reason,omitempty"`
}

// FactRecallQuery 为 §5.3 Recall 的输入。Query（文本）或 QueryEmbedding（向量）至少其一必填；
// 二者皆有时，文本用于 BM25 回退，嵌入用于向量检索。
type FactRecallQuery struct {
	WorkspaceID    string      `json:"workspace_id,omitempty"`
	UserID         string      `json:"user_id,omitempty"`
	TeamID         string      `json:"team_id,omitempty"`
	AgentID        string      `json:"agent_id,omitempty"`
	IncludeScopes  []ScopeType `json:"include_scopes,omitempty"`
	Query          string      `json:"query,omitempty"`
	QueryEmbedding []float32   `json:"-"`
	Tags           []string    `json:"tags,omitempty"`
	Kinds          []FactKind  `json:"kinds,omitempty"`
	TopK           int         `json:"top_k,omitempty"`
	MinScore       float64     `json:"min_score,omitempty"`
	MaxChars       int         `json:"max_chars,omitempty"`
}

// FactRecallHit 表示 Recall 的单条结果及用于最终排序的分项分数。Reason 为检索路径标签（"bm25" / "vector" / "hybrid"）。
type FactRecallHit struct {
	Fact        MemoryFact `json:"fact"`
	VectorScore float64    `json:"vector_score"`
	BM25Score   float64    `json:"bm25_score"`
	FinalScore  float64    `json:"final_score"`
	ScopeWeight float64    `json:"scope_weight"`
	Reason      string     `json:"reason,omitempty"`
}

// FactFeedback 为用户 / 运行时信号，用于调整事实的置信度与重要性（§5.4）。
type FactFeedback struct {
	ID        string         `json:"id,omitempty"`
	FactID    string         `json:"fact_id"`
	SessionID string         `json:"session_id,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
	Type      string         `json:"type"`
	Source    string         `json:"source"`
	Weight    float64        `json:"weight,omitempty"`
	Comment   string         `json:"comment,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
}

// FactConflict 记录同作用域内两事实间的检测矛盾及解决元数据（§5.x）。
type FactConflict struct {
	ID         string    `json:"id"`
	FactAID    string    `json:"fact_a_id"`
	FactBID    string    `json:"fact_b_id"`
	ScopeType  ScopeType `json:"scope_type"`
	ScopeID    string    `json:"scope_id,omitempty"`
	Kind       string    `json:"conflict_kind"`
	Similarity float64   `json:"similarity"`
	Status     string    `json:"status"`
	DetectedBy string    `json:"detected_by,omitempty"`
	Resolution string    `json:"resolution,omitempty"`
	ResolvedBy string    `json:"resolved_by,omitempty"`
	ResolvedAt string    `json:"resolved_at,omitempty"`
	CreatedAt  string    `json:"created_at,omitempty"`
	UpdatedAt  string    `json:"updated_at,omitempty"`
}

// FactVersion 为 `memory_fact_versions` 的快照行。diff 以原始 JSON 存储，调用方可展示逐字段变更而无需从现行行反推。
type FactVersion struct {
	ID           string   `json:"id"`
	FactID       string   `json:"fact_id"`
	Version      int      `json:"version"`
	Statement    string   `json:"statement"`
	Details      string   `json:"details_markdown,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Confidence   float64  `json:"confidence"`
	Status       string   `json:"status"`
	ChangedBy    string   `json:"changed_by,omitempty"`
	ChangeReason string   `json:"change_reason,omitempty"`
	DiffJSON     string   `json:"diff_json,omitempty"`
	CreatedAt    string   `json:"created_at,omitempty"`
}

// FactPromptBlock 为 L0 作为 `memory.l3` 系统块注入的渲染输出。
type FactPromptBlock struct {
	Section string          `json:"section"`
	Role    string          `json:"role"`
	Tokens  int             `json:"tokens"`
	Content string          `json:"content"`
	Items   []FactRecallHit `json:"items,omitempty"`
}
