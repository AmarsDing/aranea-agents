// MemoryL4Service 为 L4 持久/知识图门面，
// 见 `aranea/docs/16 memory-L4-persistent.md`。第一阶段提供
// 带去重的实体/关系 CRUD、邻域遍历、
// 版本历史、供 L0 注入的提示渲染，以及
// 提取管线的桩（第二阶段）。
//
// 本服务刻意同步：提取任务调度与嵌入生成由调用方（cmd/server）负责，测试可内联驱动方法。所有写操作同时写审计日志，便于还原变更者。
package application

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// MemoryL4Service 在 HTTP / L0 / L2 / L3 调用方与 SQLite L4 仓库间协调。L3 依赖很窄：仅事实链接需要，未接 L3 时优雅降级。
type MemoryL4Service struct {
	repo repository.Store
	now  func() string
}

// NewMemoryL4Service 在仓库上构建服务。第二阶段可再注入嵌入/提取源。
func NewMemoryL4Service(repo repository.Store) *MemoryL4Service {
	return &MemoryL4Service{repo: repo, now: nowUTC}
}

// SetClock 覆盖时钟供测试使用。
func (s *MemoryL4Service) SetClock(now func() string) {
	if now != nil {
		s.now = now
	}
}

// --- 输入/输出 --------------------------------------------------------

// EntityUpsertInput 为 HTTP POST/PATCH 与提取管线共用的参数对象。NameNormalized 在为空时计算。
type EntityUpsertInput struct {
	ID          string               `json:"id,omitempty"`
	ScopeType   mem.ScopeType     `json:"scope_type"`
	ScopeID     string               `json:"scope_id"`
	WorkspaceID string               `json:"workspace_id,omitempty"`
	UserID      string               `json:"user_id,omitempty"`
	EntityType  mem.EntityType    `json:"entity_type"`
	Name        string               `json:"name"`
	Aliases     []string             `json:"aliases,omitempty"`
	Description string               `json:"description,omitempty"`
	Attributes  map[string]any       `json:"attributes,omitempty"`
	Importance  float64              `json:"importance,omitempty"`
	Confidence  float64              `json:"confidence,omitempty"`
	SourceKind  string               `json:"source_kind,omitempty"`
	Evidence    []mem.EvidenceRef `json:"evidence,omitempty"`
	Metadata    map[string]any       `json:"metadata,omitempty"`
	By          string               `json:"by,omitempty"`
	Reason      string               `json:"reason,omitempty"`
}

// RelationUpsertInput 为 HTTP POST 与提取管线共用的参数对象。
type RelationUpsertInput struct {
	ID            string               `json:"id,omitempty"`
	ScopeType     mem.ScopeType     `json:"scope_type"`
	ScopeID       string               `json:"scope_id"`
	WorkspaceID   string               `json:"workspace_id,omitempty"`
	SourceID      string               `json:"source_id"`
	TargetID      string               `json:"target_id"`
	RelationType  mem.RelationType  `json:"relation_type"`
	Bidirectional bool                 `json:"bidirectional,omitempty"`
	Weight        float64              `json:"weight,omitempty"`
	Confidence    float64              `json:"confidence,omitempty"`
	Importance    float64              `json:"importance,omitempty"`
	Attributes    map[string]any       `json:"attributes,omitempty"`
	Evidence      []mem.EvidenceRef `json:"evidence,omitempty"`
	SourceKind    string               `json:"source_kind,omitempty"`
	By            string               `json:"by,omitempty"`
	Reason        string               `json:"reason,omitempty"`
}

// EntityListResult 为 GET §6.2 列表端点的线形状。
type EntityListResult struct {
	Items  []mem.MemoryEntity `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

// ExtractionReport 汇总单次 ExtractFromEpisode / Fact 调用。第一阶段桩恒返回零，但形态与文档一致，HTTP 可先接线。
type ExtractionReport struct {
	NewEntities      int    `json:"new_entities"`
	UpdatedEntities  int    `json:"updated_entities"`
	NewRelations     int    `json:"new_relations"`
	UpdatedRelations int    `json:"updated_relations"`
	Skipped          int    `json:"skipped"`
	Errors           int    `json:"errors"`
	Note             string `json:"note,omitempty"`
}

// --- 实体 CRUD -------------------------------------------------------------

// UpsertEntity 存储或更新实体行，在同一事务中写审计与 `memory_entity_versions` 快照。
func (s *MemoryL4Service) UpsertEntity(ctx context.Context, in EntityUpsertInput) (mem.MemoryEntity, error) {
	if in.ScopeType == "" {
		return mem.MemoryEntity{}, validationError("scope_type is required")
	}
	if !in.ScopeType.IsValid() {
		return mem.MemoryEntity{}, validationError("scope_type must be one of global/workspace/user/team/agent")
	}
	if in.EntityType == "" {
		return mem.MemoryEntity{}, validationError("entity_type is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return mem.MemoryEntity{}, validationError("name is required")
	}

	normalized := normalizeEntityName(in.Name)
	now := s.now()

	entity := mem.MemoryEntity{
		ID:             in.ID,
		ScopeType:      in.ScopeType,
		ScopeID:        in.ScopeID,
		WorkspaceID:    in.WorkspaceID,
		UserID:         in.UserID,
		EntityType:     in.EntityType,
		Name:           strings.TrimSpace(in.Name),
		NameNormalized: normalized,
		Aliases:        normalizeAliases(in.Aliases),
		Description:    strings.TrimSpace(in.Description),
		Attributes:     in.Attributes,
		Importance:     clamp01OrDefault(in.Importance, 0.5),
		Confidence:     clamp01OrDefault(in.Confidence, 0.7),
		SourceKind:     defaultIfEmpty(in.SourceKind, mem.GraphSourceUser),
		Status:         mem.EntityStatusActive,
		Metadata:       in.Metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	existing, getErr := s.repo.GetEntityByName(in.ScopeType, in.ScopeID, in.EntityType, normalized)
	isUpdate := getErr == nil
	if isUpdate {
		entity.ID = existing.ID
		entity.CreatedAt = existing.CreatedAt
		entity.UseCount = existing.UseCount
	}
	if entity.ID == "" {
		entity.ID = newID()
	}

	stored, err := s.repo.UpsertEntity(entity)
	if err != nil {
		return mem.MemoryEntity{}, err
	}

	version := 1
	if isUpdate {
		prev, _ := s.repo.ListEntityVersions(stored.ID, 1)
		if len(prev) > 0 {
			version = prev[0].Version + 1
		} else {
			version = 2
		}
	}
	snapshotJSON, _ := json.Marshal(stored)
	reason := defaultIfEmpty(in.Reason, "create")
	if isUpdate {
		reason = defaultIfEmpty(in.Reason, "update")
	}
	if vErr := s.repo.InsertEntityVersion(mem.MemoryEntityVersion{
		ID:           newID(),
		EntityID:     stored.ID,
		Version:      version,
		SnapshotJSON: string(snapshotJSON),
		ChangedBy:    in.By,
		ChangeReason: reason,
		CreatedAt:    now,
	}); vErr != nil {
		return stored, vErr
	}
	action := "memory.l4.entity.create"
	if isUpdate {
		action = "memory.l4.entity.update"
	}
	_ = s.audit(action, "memory_entities", stored.ID, map[string]any{
		"name":        stored.Name,
		"entity_type": stored.EntityType,
		"scope":       stored.ScopeType,
		"scope_id":    stored.ScopeID,
		"by":          in.By,
		"reason":      in.Reason,
	})
	return stored, nil
}

// GetEntity 按 ID 返回单个实体。
func (s *MemoryL4Service) GetEntity(ctx context.Context, id string) (mem.MemoryEntity, error) {
	if id == "" {
		return mem.MemoryEntity{}, validationError("id is required")
	}
	return s.repo.GetEntity(id)
}

// ListEntities 按查询条件分页返回实体列表。
func (s *MemoryL4Service) ListEntities(ctx context.Context, q repository.EntityListQuery) (EntityListResult, error) {
	items, total, err := s.repo.ListEntities(q)
	if err != nil {
		return EntityListResult{}, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	return EntityListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// ArchiveEntity 将实体状态置为 `archived` 并记审计。用于管理端软删。
func (s *MemoryL4Service) ArchiveEntity(ctx context.Context, id, by, reason string) error {
	if id == "" {
		return validationError("id is required")
	}
	now := s.now()
	if err := s.repo.UpdateEntityStatus(id, mem.EntityStatusArchived, "", now, ""); err != nil {
		return err
	}
	_ = s.audit("memory.l4.entity.archive", "memory_entities", id, map[string]any{"by": by, "reason": reason})
	return nil
}

// DeleteEntity 将实体状态置为 `deleted` 并记审计。不支持硬删——软删保留仍引用该实体的关系完整性。
func (s *MemoryL4Service) DeleteEntity(ctx context.Context, id, by, reason string) error {
	if id == "" {
		return validationError("id is required")
	}
	now := s.now()
	if err := s.repo.UpdateEntityStatus(id, mem.EntityStatusDeleted, "", "", now); err != nil {
		return err
	}
	_ = s.audit("memory.l4.entity.delete", "memory_entities", id, map[string]any{"by": by, "reason": reason})
	return nil
}

// RenameEntity 更新实体名称（及规范归一形式）。写入新版本快照以保留历史。
func (s *MemoryL4Service) RenameEntity(ctx context.Context, id, newName, by, reason string) (mem.MemoryEntity, error) {
	if id == "" {
		return mem.MemoryEntity{}, validationError("id is required")
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return mem.MemoryEntity{}, validationError("name is required")
	}
	existing, err := s.repo.GetEntity(id)
	if err != nil {
		return mem.MemoryEntity{}, err
	}
	normalized := normalizeEntityName(newName)
	if err := s.repo.UpdateEntityName(id, newName, normalized); err != nil {
		return mem.MemoryEntity{}, err
	}
	stored, err := s.repo.GetEntity(id)
	if err != nil {
		return mem.MemoryEntity{}, err
	}
	prev, _ := s.repo.ListEntityVersions(id, 1)
	nextVersion := 1
	if len(prev) > 0 {
		nextVersion = prev[0].Version + 1
	}
	snap, _ := json.Marshal(stored)
	_ = s.repo.InsertEntityVersion(mem.MemoryEntityVersion{
		ID:           newID(),
		EntityID:     id,
		Version:      nextVersion,
		SnapshotJSON: string(snap),
		ChangedBy:    by,
		ChangeReason: defaultIfEmpty(reason, "rename"),
		DiffJSON:     fmt.Sprintf(`{"name":{"before":%q,"after":%q}}`, existing.Name, newName),
		CreatedAt:    s.now(),
	})
	_ = s.audit("memory.l4.entity.rename", "memory_entities", id, map[string]any{
		"before": existing.Name, "after": newName, "by": by, "reason": reason,
	})
	return stored, nil
}

// MergeEntities 将各 `mergeIDs` 行标记为合并入 `primaryID`，
// 重连关系到主实体并写快照。将成自环的关系改为归档而非迁移。
func (s *MemoryL4Service) MergeEntities(ctx context.Context, primaryID string, mergeIDs []string, by, reason string) error {
	if primaryID == "" {
		return validationError("primary id is required")
	}
	if len(mergeIDs) == 0 {
		return validationError("merge ids are required")
	}
	primary, err := s.repo.GetEntity(primaryID)
	if err != nil {
		return err
	}
	now := s.now()
	for _, mid := range mergeIDs {
		if mid == "" || mid == primaryID {
			continue
		}
		victim, err := s.repo.GetEntity(mid)
		if err != nil {
			continue
		}
		rels, err := s.repo.ListRelationsForNode(mid, 500)
		if err != nil {
			return err
		}
		for _, rel := range rels {
			rewritten := rel
			selfLoop := false
			if rewritten.SourceID == mid {
				rewritten.SourceID = primaryID
			}
			if rewritten.TargetID == mid {
				rewritten.TargetID = primaryID
			}
			if rewritten.SourceID == rewritten.TargetID {
				selfLoop = true
			}
			if selfLoop {
				_ = s.repo.UpdateRelationStatus(rel.ID, mem.RelationStatusArchived, now, "")
				continue
			}
			rewritten.ID = newID()
			rewritten.UpdatedAt = now
			if _, err := s.repo.UpsertRelation(rewritten); err != nil {
				return err
			}
			_ = s.repo.UpdateRelationStatus(rel.ID, mem.RelationStatusArchived, now, "")
		}
		if err := s.repo.UpdateEntityStatus(mid, mem.EntityStatusMerged, primaryID, now, ""); err != nil {
			return err
		}
		snap, _ := json.Marshal(victim)
		prev, _ := s.repo.ListEntityVersions(mid, 1)
		next := 1
		if len(prev) > 0 {
			next = prev[0].Version + 1
		}
		_ = s.repo.InsertEntityVersion(mem.MemoryEntityVersion{
			ID:           newID(),
			EntityID:     mid,
			Version:      next,
			SnapshotJSON: string(snap),
			ChangedBy:    by,
			ChangeReason: defaultIfEmpty(reason, "merge"),
			DiffJSON:     fmt.Sprintf(`{"merged_into":%q}`, primaryID),
			CreatedAt:    now,
		})
		_ = s.audit("memory.l4.entity.merge", "memory_entities", mid, map[string]any{
			"merged_into": primaryID,
			"by":          by,
			"reason":      reason,
		})
	}
	_ = s.audit("memory.l4.entity.merge_target", "memory_entities", primary.ID, map[string]any{
		"sources": mergeIDs,
		"by":      by,
		"reason":  reason,
	})
	return nil
}

// ListEntityFacts 返回与实体链接的事实 id。第二阶段在 L3 提取落地时接线双向 fact ↔ entity 索引。
func (s *MemoryL4Service) ListEntityFacts(ctx context.Context, entityID string, limit int) ([]mem.MemoryEntityFactLink, error) {
	if entityID == "" {
		return nil, validationError("entity id is required")
	}
	return s.repo.ListFactsForEntity(entityID, limit)
}

// ListEntityVersions 返回实体快照历史，最新在前。
func (s *MemoryL4Service) ListEntityVersions(ctx context.Context, entityID string, limit int) ([]mem.MemoryEntityVersion, error) {
	if entityID == "" {
		return nil, validationError("entity id is required")
	}
	return s.repo.ListEntityVersions(entityID, limit)
}

// LinkEntityToFact 对 L3 提取（第二阶段）使用的 entity ↔ fact 反向索引行做 upsert。对插件/提取管线公开。
func (s *MemoryL4Service) LinkEntityToFact(ctx context.Context, entityID, factID string, weight float64) error {
	if entityID == "" || factID == "" {
		return validationError("entity_id and fact_id are required")
	}
	return s.repo.UpsertEntityFact(entityID, factID, weight)
}

// --- 关系 CRUD -----------------------------------------------------------

// UpsertRelation 按 (scope_type, scope_id, source, target, relation_type) 存储或更新关系行。
func (s *MemoryL4Service) UpsertRelation(ctx context.Context, in RelationUpsertInput) (mem.MemoryRelation, error) {
	if in.ScopeType == "" {
		return mem.MemoryRelation{}, validationError("scope_type is required")
	}
	if !in.ScopeType.IsValid() {
		return mem.MemoryRelation{}, validationError("scope_type must be one of global/workspace/user/team/agent")
	}
	if in.SourceID == "" || in.TargetID == "" {
		return mem.MemoryRelation{}, validationError("source_id and target_id are required")
	}
	if in.SourceID == in.TargetID {
		return mem.MemoryRelation{}, validationError("source_id and target_id must differ")
	}
	if in.RelationType == "" {
		return mem.MemoryRelation{}, validationError("relation_type is required")
	}
	now := s.now()
	relation := mem.MemoryRelation{
		ID:            in.ID,
		ScopeType:     in.ScopeType,
		ScopeID:       in.ScopeID,
		WorkspaceID:   in.WorkspaceID,
		SourceID:      in.SourceID,
		TargetID:      in.TargetID,
		RelationType:  in.RelationType,
		Bidirectional: in.Bidirectional,
		Weight:        clampPositiveOrDefault(in.Weight, 1.0),
		Confidence:    clamp01OrDefault(in.Confidence, 0.7),
		Importance:    clamp01OrDefault(in.Importance, 0.5),
		Attributes:    in.Attributes,
		Evidence:      in.Evidence,
		SourceKind:    defaultIfEmpty(in.SourceKind, mem.GraphSourceUser),
		Status:        mem.RelationStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if relation.ID == "" {
		relation.ID = newID()
	}
	stored, err := s.repo.UpsertRelation(relation)
	if err != nil {
		return mem.MemoryRelation{}, err
	}
	_ = s.audit("memory.l4.relation.upsert", "memory_relations", stored.ID, map[string]any{
		"source": stored.SourceID, "target": stored.TargetID,
		"relation_type": stored.RelationType,
		"by":            in.By, "reason": in.Reason,
	})
	return stored, nil
}

// GetRelation 按 ID 返回单条关系。
func (s *MemoryL4Service) GetRelation(ctx context.Context, id string) (mem.MemoryRelation, error) {
	if id == "" {
		return mem.MemoryRelation{}, validationError("id is required")
	}
	return s.repo.GetRelation(id)
}

// ListRelationsForNode 返回连接到节点的活动关系。
func (s *MemoryL4Service) ListRelationsForNode(ctx context.Context, nodeID string, limit int) ([]mem.MemoryRelation, error) {
	if nodeID == "" {
		return nil, validationError("node id is required")
	}
	return s.repo.ListRelationsForNode(nodeID, limit)
}

// DeleteRelation 将关系状态置为 `deleted` 并记审计。
func (s *MemoryL4Service) DeleteRelation(ctx context.Context, id, by, reason string) error {
	if id == "" {
		return validationError("id is required")
	}
	now := s.now()
	if err := s.repo.UpdateRelationStatus(id, mem.RelationStatusDeleted, "", now); err != nil {
		return err
	}
	_ = s.audit("memory.l4.relation.delete", "memory_relations", id, map[string]any{"by": by, "reason": reason})
	return nil
}

// --- 邻域/搜索 ----------------------------------------------------------------

// Neighborhood 从 `centerID` 向外最多 `hops` 跳，返回至多 `maxNodes` 个不同实体及连接它们的关系。跳数上限 3 以控制延迟。
func (s *MemoryL4Service) Neighborhood(ctx context.Context, centerID string, hops, maxNodes int) (mem.GraphNeighborhood, error) {
	if centerID == "" {
		return mem.GraphNeighborhood{}, validationError("center id is required")
	}
	return s.repo.GetNeighborhood(centerID, hops, maxNodes)
}

// SearchByText 为基于名称/别名/描述的简单关键词查找。产生嵌入后（第二阶段）可换为向量搜索。
func (s *MemoryL4Service) SearchByText(ctx context.Context, scope mem.ScopeType, scopeID, query string, topK int) ([]mem.MemoryEntity, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if topK <= 0 || topK > 50 {
		topK = 10
	}
	q := repository.EntityListQuery{
		ScopeType: scope,
		ScopeID:   scopeID,
		Status:    mem.EntityStatusActive,
		Keyword:   query,
		Limit:     topK,
	}
	items, _, err := s.repo.ListEntities(q)
	return items, err
}

// --- 管线桩 ----------------------------------------------------------------

// ExtractFromEpisode 对情节的 title / goal / outcome / failure_reason 运行第二阶段基于词典的提取器。每次匹配在智能体作用域下创建（或刷新）知识图实体；情节本身*不*反链，因模式尚无 episode↔entity 索引——第三阶段将扩展链接表。无匹配时返回 Skipped=N。
func (s *MemoryL4Service) ExtractFromEpisode(ctx context.Context, episodeID string) (ExtractionReport, error) {
	if episodeID == "" {
		return ExtractionReport{}, validationError("episode id is required")
	}
	episode, err := s.repo.GetEpisode(episodeID)
	if err != nil {
		return ExtractionReport{}, err
	}

	text := strings.Join([]string{
		episode.Title,
		episode.Goal,
		episode.Outcome,
		episode.OutcomeSummary,
		episode.ResultPreview,
		episode.FailureReason,
	}, "\n")

	matches := scanExtractionMatches(text)
	if len(matches) == 0 {
		return ExtractionReport{Note: "no dictionary matches"}, nil
	}

	scopeType, scopeID := extractionScopeFromEpisode(episode)
	if scopeType == "" {
		return ExtractionReport{Skipped: len(matches), Note: "episode lacks a usable scope"}, nil
	}

	report := ExtractionReport{}
	for _, m := range matches {
		existing, _ := s.repo.GetEntityByName(scopeType, scopeID, m.Type, normalizeEntityName(m.Name))
		_, err := s.UpsertEntity(ctx, EntityUpsertInput{
			ScopeType:   scopeType,
			ScopeID:     scopeID,
			WorkspaceID: episode.TeamID,
			EntityType:  m.Type,
			Name:        m.Name,
			Aliases:     m.Aliases,
			Importance:  0.4,
			Confidence:  0.55,
			SourceKind:  mem.GraphSourceExtracted,
			By:          "extractor",
			Reason:      "extract_from_episode:" + episode.ID,
			Metadata: map[string]any{
				"source_episode_id": episode.ID,
			},
		})
		if err != nil {
			report.Errors++
			continue
		}
		if existing.ID == "" {
			report.NewEntities++
		} else {
			report.UpdatedEntities++
		}
	}
	return report, nil
}

// ExtractFromFact 对事实的 statement + details_markdown 运行第二阶段基于词典的提取器。每次匹配在事实作用域下创建（或刷新）知识图实体，并通过 `memory_entity_facts` 反链，便于后续邻域查询同时展示来源事实与实体。
func (s *MemoryL4Service) ExtractFromFact(ctx context.Context, factID string) (ExtractionReport, error) {
	if factID == "" {
		return ExtractionReport{}, validationError("fact id is required")
	}
	fact, err := s.repo.GetFact(factID)
	if err != nil {
		return ExtractionReport{}, err
	}

	text := strings.TrimSpace(fact.Statement)
	if fact.DetailsMarkdown != "" {
		text = text + "\n" + fact.DetailsMarkdown
	}

	matches := scanExtractionMatches(text)
	if len(matches) == 0 {
		return ExtractionReport{Note: "no dictionary matches"}, nil
	}

	report := ExtractionReport{}
	for _, m := range matches {
		existing, _ := s.repo.GetEntityByName(fact.ScopeType, fact.ScopeID, m.Type, normalizeEntityName(m.Name))
		entity, err := s.UpsertEntity(ctx, EntityUpsertInput{
			ScopeType:   fact.ScopeType,
			ScopeID:     fact.ScopeID,
			WorkspaceID: fact.WorkspaceID,
			UserID:      fact.UserID,
			EntityType:  m.Type,
			Name:        m.Name,
			Aliases:     m.Aliases,
			Importance:  0.45,
			Confidence:  0.6,
			SourceKind:  mem.GraphSourceExtracted,
			By:          "extractor",
			Reason:      "extract_from_fact:" + fact.ID,
			Metadata: map[string]any{
				"source_fact_id": fact.ID,
			},
		})
		if err != nil {
			report.Errors++
			continue
		}
		if existing.ID == "" {
			report.NewEntities++
		} else {
			report.UpdatedEntities++
		}
		if err := s.repo.UpsertEntityFact(entity.ID, fact.ID, 1.0); err != nil {
			report.Errors++
		}
	}
	return report, nil
}

// extractionScopeFromEpisode 选择情节可挂的最细作用域。属智能体则挂智能体作用域以按人格提取；属团队则挂团队作用域；否则跳过（无明确归属）。
func extractionScopeFromEpisode(episode mem.MemoryEpisode) (mem.ScopeType, string) {
	if episode.AgentID != "" {
		return mem.ScopeAgent, episode.AgentID
	}
	if episode.TeamID != "" {
		return mem.ScopeTeam, episode.TeamID
	}
	return "", ""
}

// --- L0 提示渲染 ------------------------------------------------------------

// l4MaxNeighborChars 限制渲染的邻域块在 L0 提示中可占用的字符量。与 §5.8 提示预算默认一致。
const l4MaxNeighborChars = 1500

// RenderForPrompt 将 GraphNeighborhood 格式化为适合 L0 注入的 markdown。邻域无有效数据（如孤立节点）时 ok=false。
func (s *MemoryL4Service) RenderForPrompt(n mem.GraphNeighborhood, maxChars int) (string, bool) {
	if maxChars <= 0 || maxChars > 4000 {
		maxChars = l4MaxNeighborChars
	}
	if n.Center.ID == "" || (len(n.Entities) == 0 && len(n.Relations) == 0) {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# memory.l4.graph\nCenter: %s (%s)", n.Center.Name, n.Center.EntityType)
	if desc := strings.TrimSpace(n.Center.Description); desc != "" {
		fmt.Fprintf(&b, "\n  - %s", desc)
	}

	entityIndex := map[string]string{n.Center.ID: n.Center.Name}
	for _, e := range n.Entities {
		entityIndex[e.ID] = e.Name
	}
	rels := append([]mem.MemoryRelation(nil), n.Relations...)
	sort.SliceStable(rels, func(i, j int) bool {
		return rels[i].Weight > rels[j].Weight
	})
	if len(rels) > 0 {
		b.WriteString("\nRelations:")
		for _, rel := range rels {
			src := entityIndex[rel.SourceID]
			if src == "" {
				src = rel.SourceID
			}
			dst := entityIndex[rel.TargetID]
			if dst == "" {
				dst = rel.TargetID
			}
			fmt.Fprintf(&b, "\n  - %s --[%s]--> %s (w=%.2f)", src, rel.RelationType, dst, rel.Weight)
			if b.Len() > maxChars {
				break
			}
		}
	}
	body := strings.TrimSpace(b.String())
	if body == "" {
		return "", false
	}
	if len(body) > maxChars {
		body = body[:maxChars] + "..."
	}
	return body, true
}

// NeighborhoodSegmentForL0 为 L0RecallSource 垫片：给定 session/agent/query，在智能体可见作用域（agent → workspace → user → global）内关键词选中心实体，扩展 k 跳邻域，渲染 `memory.l4.graph` 片段。
//
// 各限制来自 `agent_runtime_settings`（§3.3）：片段需 `l4_enabled` 与 `l4_graph_inject_neighbors`；邻居数受 `l4_graph_max_neighbors` 限制；跳数受 `l4_graph_max_hops` 限制（≤3 以控制延迟）。
//
// 功能关闭、查询为空、无匹配实体或邻域渲染为空块时 ok=false，L0 省略该片段。
func (s *MemoryL4Service) NeighborhoodSegmentForL0(ctx context.Context, sessionID, agentID, query string) (mem.L0Segment, bool) {
	return s.NeighborhoodSegmentForL0WithContext(ctx, mem.L0MemoryScopeContext{
		SessionID: sessionID,
		AgentID:   agentID,
		Query:     query,
	})
}

// NeighborhoodSegmentForL0WithContext 为带完整上下文的 L0 接缝。选中心实体时使用团队、用户与工作区作用域 ID。
func (s *MemoryL4Service) NeighborhoodSegmentForL0WithContext(ctx context.Context, scope mem.L0MemoryScopeContext) (mem.L0Segment, bool) {
	if strings.TrimSpace(scope.Query) == "" || scope.AgentID == "" {
		return mem.L0Segment{}, false
	}
	settings, _ := s.repo.GetAgentRuntimeSettings(scope.AgentID)
	if !settings.L4Enabled || !settings.L4GraphInjectNeighbors {
		return mem.L0Segment{}, false
	}

	hops := firstPositive(settings.L4GraphMaxHops, 1)
	if hops > 3 {
		hops = 3
	}
	maxNodes := firstPositive(settings.L4GraphMaxNeighbors, 6)
	if maxNodes > 20 {
		maxNodes = 20
	}

	center, ok := s.findCenterEntity(scope, scope.Query)
	if !ok {
		return mem.L0Segment{}, false
	}

	n, err := s.repo.GetNeighborhood(center.ID, hops, maxNodes)
	if err != nil {
		return mem.L0Segment{}, false
	}
	if n.Center.ID == "" {
		n.Center = center
	}
	body, ok := s.RenderForPrompt(n, l4MaxNeighborChars)
	if !ok {
		return mem.L0Segment{}, false
	}

	_ = s.repo.BumpEntityUseCount(center.ID, s.now())

	return mem.L0Segment{
		Section: "memory.l4.graph",
		Role:    "system",
		Source:  fmt.Sprintf("memory.l4:%s", center.ID),
		Tokens:  estimateTokensApprox(body),
		Content: body,
		Preview: previewText(body, l0PreviewLimit),
	}, true
}

// findCenterEntity 按智能体可见作用域（agent → workspace → user → global）遍历，返回名称/别名/描述与查询匹配的首个活动实体。第二阶段的 attention 管线将把关键词搜索换为向量召回。
func (s *MemoryL4Service) findCenterEntity(scope mem.L0MemoryScopeContext, query string) (mem.MemoryEntity, bool) {
	candidates := []repository.EntityListQuery{
		{ScopeType: mem.ScopeAgent, ScopeID: scope.AgentID},
		{ScopeType: mem.ScopeTeam, ScopeID: scope.TeamID},
		{ScopeType: mem.ScopeWorkspace, ScopeID: scope.WorkspaceID},
		{ScopeType: mem.ScopeUser, ScopeID: scope.UserID},
		{ScopeType: mem.ScopeGlobal},
	}
	for _, q := range candidates {
		if q.ScopeType != mem.ScopeGlobal && q.ScopeID == "" {
			continue
		}
		q.Status = mem.EntityStatusActive
		q.Keyword = query
		q.Limit = 1
		items, _, err := s.repo.ListEntities(q)
		if err != nil || len(items) == 0 {
			continue
		}
		return items[0], true
	}
	return mem.MemoryEntity{}, false
}

// --- 审计辅助 ----------------------------------------------------------------

func (s *MemoryL4Service) audit(action, resource, resourceID string, detail map[string]any) error {
	body, _ := json.Marshal(detail)
	if len(body) == 0 {
		body = []byte("{}")
	}
	return s.repo.AddAuditLog(domain.AuditLog{
		ID:         newID(),
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Detail:     string(body),
	})
}

// --- 纯函数辅助 --------------------------------------------------------------

func normalizeEntityName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

func normalizeAliases(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}

func clamp01OrDefault(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampPositiveOrDefault(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

func defaultIfEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
