// Package domain – L4 持久 / 进化记忆领域类型，见 `aranea/docs/16 memory-L4-persistent.md`。
// L4 存储长生命期知识图谱（实体 + 关系）及智能体演进的身份 / 策略画像，
// 为五层记忆架构的最顶层。
package domain

// EntityType 对知识图谱节点分类。字符串持久化在 `memory_entities.entity_type`，变更需迁移。
type EntityType string

const (
	EntityPerson     EntityType = "person"
	EntityProject    EntityType = "project"
	EntityRepository EntityType = "repository"
	EntityTech       EntityType = "tech"
	EntityFramework  EntityType = "framework"
	EntityCompany    EntityType = "company"
	EntityTopic      EntityType = "topic"
	EntityFile       EntityType = "file"
	EntityEndpoint   EntityType = "endpoint"
	EntityCustom     EntityType = "custom"
)

// IsValid 表示实体类型是否为已知枚举值。
// 未知类型仍可通过 EntityCustom；接受插件提供的分类体系时调用方可放宽此检查。
func (t EntityType) IsValid() bool {
	switch t {
	case EntityPerson, EntityProject, EntityRepository, EntityTech, EntityFramework,
		EntityCompany, EntityTopic, EntityFile, EntityEndpoint, EntityCustom:
		return true
	}
	return false
}

// RelationType 对知识图谱边分类。字符串持久化在 `memory_relations.relation_type`。
type RelationType string

const (
	RelWorksOn    RelationType = "works_on"
	RelUses       RelationType = "uses"
	RelDependsOn  RelationType = "depends_on"
	RelAuthoredBy RelationType = "authored_by"
	RelPartOf     RelationType = "part_of"
	RelSimilarTo  RelationType = "similar_to"
	RelReplaces   RelationType = "replaces"
	RelMemberOf   RelationType = "member_of"
	RelSupports   RelationType = "supports"
	RelBlocks     RelationType = "blocks"
)

// IsValid 表示关系类型是否为已知枚举值。允许自定义类型，但应在更上游审核。
func (r RelationType) IsValid() bool {
	switch r {
	case RelWorksOn, RelUses, RelDependsOn, RelAuthoredBy, RelPartOf,
		RelSimilarTo, RelReplaces, RelMemberOf, RelSupports, RelBlocks:
		return true
	}
	return false
}

// 持久化在 `status` 列的实体 / 关系状态取值。
const (
	EntityStatusActive   = "active"
	EntityStatusMerged   = "merged"
	EntityStatusArchived = "archived"
	EntityStatusDeleted  = "deleted"

	RelationStatusActive   = "active"
	RelationStatusArchived = "archived"
	RelationStatusDeleted  = "deleted"
)

// 来源种类，描述实体 / 关系的创建方式。
const (
	GraphSourceExtracted   = "extracted"
	GraphSourceUser        = "user"
	GraphSourcePlugin      = "plugin"
	GraphSourceAgent       = "agent"
	GraphSourceConsolidate = "consolidator"
)

// EvidenceRef 为指向事实 / 片段 / 消息等的结构化引用，支撑实体、关系或进化变更。
// 作为 JSON 元素持久化在各 `evidence_json` 列中。
type EvidenceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// MemoryEntity 为 `memory_entities` 表的持久化行。JSON 标签与 §6.2 HTTP API 线格式一致。
type MemoryEntity struct {
	ID             string         `json:"id"`
	ScopeType      ScopeType      `json:"scope_type"`
	ScopeID        string         `json:"scope_id"`
	WorkspaceID    string         `json:"workspace_id,omitempty"`
	UserID         string         `json:"user_id,omitempty"`
	EntityType     EntityType     `json:"entity_type"`
	Name           string         `json:"name"`
	NameNormalized string         `json:"name_normalized,omitempty"`
	Aliases        []string       `json:"aliases"`
	Description    string         `json:"description,omitempty"`
	Attributes     map[string]any `json:"attributes,omitempty"`

	Importance float64 `json:"importance"`
	Confidence float64 `json:"confidence"`
	UseCount   int     `json:"use_count"`
	SourceKind string  `json:"source_kind,omitempty"`

	EmbeddingStatus string  `json:"embedding_status,omitempty"`
	EmbeddingModel  string  `json:"embedding_model,omitempty"`
	EmbeddingDim    int     `json:"embedding_dim,omitempty"`
	EmbeddingBlob   []byte  `json:"-"`
	EmbeddingNorm   float64 `json:"embedding_norm,omitempty"`

	Status     string `json:"status"`
	MergedInto string `json:"merged_into,omitempty"`

	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
	ArchivedAt string         `json:"archived_at,omitempty"`
	DeletedAt  string         `json:"deleted_at,omitempty"`
}

// MemoryRelation 为 `memory_relations` 表的持久化行。
type MemoryRelation struct {
	ID          string    `json:"id"`
	ScopeType   ScopeType `json:"scope_type"`
	ScopeID     string    `json:"scope_id"`
	WorkspaceID string    `json:"workspace_id,omitempty"`

	SourceID      string       `json:"source_id"`
	TargetID      string       `json:"target_id"`
	RelationType  RelationType `json:"relation_type"`
	Bidirectional bool         `json:"bidirectional,omitempty"`

	Weight     float64 `json:"weight"`
	Confidence float64 `json:"confidence"`
	Importance float64 `json:"importance"`
	UseCount   int     `json:"use_count"`

	Attributes map[string]any `json:"attributes,omitempty"`
	Evidence   []EvidenceRef  `json:"evidence,omitempty"`
	Status     string         `json:"status"`
	SourceKind string         `json:"source_kind,omitempty"`

	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
	ArchivedAt string         `json:"archived_at,omitempty"`
	DeletedAt  string         `json:"deleted_at,omitempty"`
}

// MemoryEntityVersion 捕获 `memory_entities` 行的一次历史快照。`change_reason` 为 create / update / merge / split / rename / restore 之一。
type MemoryEntityVersion struct {
	ID           string         `json:"id"`
	EntityID     string         `json:"entity_id"`
	Version      int            `json:"version"`
	SnapshotJSON string         `json:"snapshot_json"`
	ChangedBy    string         `json:"changed_by,omitempty"`
	ChangeReason string         `json:"change_reason,omitempty"`
	DiffJSON     string         `json:"diff_json,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    string         `json:"created_at,omitempty"`
}

// MemoryEntityFactLink 为实体 ↔ L3 事实的反向索引行。
type MemoryEntityFactLink struct {
	EntityID  string  `json:"entity_id"`
	FactID    string  `json:"fact_id"`
	Weight    float64 `json:"weight"`
	CreatedAt string  `json:"created_at,omitempty"`
}

// GraphNeighborhood 为 `MemoryL4GraphService.Neighborhood` 的返回结果。`Hops` 为实际返回的跳数（受请求上限约束）。
type GraphNeighborhood struct {
	Center    MemoryEntity     `json:"center"`
	Hops      int              `json:"hops"`
	Entities  []MemoryEntity   `json:"entities"`
	Relations []MemoryRelation `json:"relations"`
}
