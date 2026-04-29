package domain

// L1TaskStatus 枚举工作记忆任务的生命周期状态，见 `aranea/docs/13 memory-L1-working.md` §3.1。
// 字符串值与持久化在 `memory_l1_tasks.status` 及 HTTP API 暴露的一致，亦作为前端事实来源。
type L1TaskStatus string

const (
	L1TaskActive    L1TaskStatus = "active"
	L1TaskPaused    L1TaskStatus = "paused"
	L1TaskCompleted L1TaskStatus = "completed"
	L1TaskFailed    L1TaskStatus = "failed"
	L1TaskCancelled L1TaskStatus = "cancelled"
	L1TaskTimeout   L1TaskStatus = "timeout"
	L1TaskArchived  L1TaskStatus = "archived"
)

// IsTerminal 表示状态是否为「不可再写」，L1 服务可据此拒绝变更，L0 渲染器可停止注入。
func (s L1TaskStatus) IsTerminal() bool {
	switch s {
	case L1TaskCompleted, L1TaskFailed, L1TaskCancelled, L1TaskTimeout, L1TaskArchived:
		return true
	}
	return false
}

// L1FieldShare 编码按字段的跨智能体可见性。以 JSON 存在 `memory_l1_tasks.shared_with_json`。
// ReadBy 为可读取所列字段的智能体 ID（或 `team:*` 通配），即便该字段默认为私有。
type L1FieldShare struct {
	Field  string   `json:"field"`
	ReadBy []string `json:"read_by"`
}

// MemoryL1Task 为 `memory_l1_tasks` 表一行的内存形态。
// 将工作记忆快照与会话 / 智能体 / 运行关联，供 L0 渲染器与 ChatService 在每轮定位正确状态。
type MemoryL1Task struct {
	ID            string         `json:"id"`
	SessionID     string         `json:"session_id"`
	RunID         string         `json:"run_id,omitempty"`
	TeamID        string         `json:"team_id,omitempty"`
	AgentID       string         `json:"agent_id,omitempty"`
	TaskKey       string         `json:"task_key"`
	TaskTitle     string         `json:"task_title"`
	TaskGoal      string         `json:"task_goal"`
	Status        L1TaskStatus   `json:"status"`
	SchemaVersion int            `json:"schema_version"`
	BudgetTokens  int            `json:"budget_tokens"`
	UsedTokens    int            `json:"used_tokens"`
	ParentTaskID  string         `json:"parent_task_id,omitempty"`
	SharedWith    []L1FieldShare `json:"shared_with,omitempty"`
	StartedAt     string         `json:"started_at"`
	EndedAt       string         `json:"ended_at,omitempty"`
	ArchivedAt    string         `json:"archived_at,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

// MemoryL1Field 为 `memory_l1_fields` 表一行的内存形态。
// 值分布在 text / json / ref 列，调用方可存原始载荷、结构化载荷或仅引用 id。
type MemoryL1Field struct {
	ID            string         `json:"id"`
	TaskID        string         `json:"task_id"`
	SessionID     string         `json:"session_id"`
	AgentID       string         `json:"agent_id,omitempty"`
	FieldPath     string         `json:"field_path"`
	FieldKind     string         `json:"field_kind"`
	Visibility    string         `json:"visibility"`
	PinToPrompt   bool           `json:"pin_to_prompt"`
	IsRequired    bool           `json:"is_required"`
	ValueText     string         `json:"value_text,omitempty"`
	ValueJSON     string         `json:"value_json,omitempty"`
	ValueRef      string         `json:"value_ref,omitempty"`
	Preview       string         `json:"preview,omitempty"`
	TokenEstimate int            `json:"token_estimate"`
	Source        string         `json:"source,omitempty"`
	SourceRef     string         `json:"source_ref,omitempty"`
	TTLSeconds    int            `json:"ttl_seconds,omitempty"`
	ExpiresAt     string         `json:"expires_at,omitempty"`
	Revision      int            `json:"revision"`
	LastReadAt    string         `json:"last_read_at,omitempty"`
	ReadCount     int            `json:"read_count,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

// MemoryL1FieldHistory 记录字段的一次修订。每次写入追加（见规范 §5.2），供用户回滚。
type MemoryL1FieldHistory struct {
	ID            string `json:"id"`
	FieldID       string `json:"field_id"`
	TaskID        string `json:"task_id"`
	Revision      int    `json:"revision"`
	ValueText     string `json:"value_text,omitempty"`
	ValueJSON     string `json:"value_json,omitempty"`
	ValueRef      string `json:"value_ref,omitempty"`
	Preview       string `json:"preview,omitempty"`
	TokenEstimate int    `json:"token_estimate"`
	ChangedBy     string `json:"changed_by,omitempty"`
	ChangeReason  string `json:"change_reason,omitempty"`
	DiffJSON      string `json:"diff_json,omitempty"`
	MetadataJSON  string `json:"metadata_json,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// MemoryL1Schema 表示某作用域（智能体 / 技能 / 团队 / 全局）声明的预期字段模式。
// 实际 JSON Schema 文本在 SchemaJSON。与此模式的校验在写入时进行（第二阶段）。
type MemoryL1Schema struct {
	ID            string         `json:"id"`
	ScopeType     string         `json:"scope_type"`
	ScopeID       string         `json:"scope_id,omitempty"`
	SchemaKey     string         `json:"schema_key"`
	SchemaVersion int            `json:"schema_version"`
	SchemaJSON    string         `json:"schema_json"`
	Description   string         `json:"description,omitempty"`
	Enabled       bool           `json:"enabled"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

// L1FieldPatch 为 MemoryL1Service.SetField / PatchFields 的输入。
// 服务按 FieldKind 决定写入哪一值列。IfRevision 实现规范 §5.2 第 6 步的乐观锁约定。
type L1FieldPatch struct {
	FieldPath    string         `json:"field_path"`
	FieldKind    string         `json:"field_kind,omitempty"`
	Value        any            `json:"value,omitempty"`
	ValueRef     string         `json:"value_ref,omitempty"`
	Preview      string         `json:"preview,omitempty"`
	Visibility   string         `json:"visibility,omitempty"`
	PinToPrompt  *bool          `json:"pin_to_prompt,omitempty"`
	IsRequired   *bool          `json:"is_required,omitempty"`
	TTLSeconds   *int           `json:"ttl_seconds,omitempty"`
	Source       string         `json:"source,omitempty"`
	SourceRef    string         `json:"source_ref,omitempty"`
	ChangedBy    string         `json:"changed_by,omitempty"`
	ChangeReason string         `json:"change_reason,omitempty"`
	IfRevision   *int           `json:"if_revision,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// L1TaskListQuery 为 HTTP 层列举单会话任务时使用的过滤条件。
type L1TaskListQuery struct {
	SessionID    string
	AgentID      string
	Status       string
	IncludeEnded bool
}

// L1PromptBlock 为任务在提示中可见字段的渲染结果。
// 由 MemoryL1Service.RenderForPrompt 生成，由 MemoryL0Service 消费（见规范 §5.3）。
// Content 为送入模型的 markdown / yaml；MissingFields 列出模式中仍为空的路径，供 L0 追加「请补全」提示。
type L1PromptBlock struct {
	Section       string   `json:"section"`
	Role          string   `json:"role"`
	Source        string   `json:"source"`
	Tokens        int      `json:"tokens"`
	Content       string   `json:"content"`
	Preview       string   `json:"preview,omitempty"`
	MissingFields []string `json:"missing_fields,omitempty"`
	TaskID        string   `json:"task_id,omitempty"`
}

// L1Episode 为任务结束时交付给 L2 片段流水线的快照。L2 实际模式见 `aranea/docs/14`；此处仅为传输形态。
type L1Episode struct {
	TaskID       string         `json:"task_id"`
	SessionID    string         `json:"session_id"`
	AgentID      string         `json:"agent_id,omitempty"`
	TaskKey      string         `json:"task_key"`
	TaskTitle    string         `json:"task_title"`
	TaskGoal     string         `json:"task_goal"`
	Status       L1TaskStatus   `json:"status"`
	StartedAt    string         `json:"started_at"`
	EndedAt      string         `json:"ended_at"`
	UsedTokens   int            `json:"used_tokens"`
	BudgetTokens int            `json:"budget_tokens"`
	Snapshot     map[string]any `json:"snapshot"`
	Stats        map[string]int `json:"stats"`
}
