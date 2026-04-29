package sqlite

import (
	mem "arenea/backend/internal/memory/domain"

	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"arenea/backend/internal/kernel/contracts"
)

// L4Repository 为 L4 知识图的 SQLite 实现。
type L4Repository struct {
	db *sql.DB
}

// NewL4Repository 使用与 monolith 相同的 *sql.DB。
func NewL4Repository(db *sql.DB) *L4Repository {
	return &L4Repository{db: db}
}

// memoryEntitySelectColumns 为 memory_entities 查询的标准列清单。共享可保证 Get/List/Neighborhood 与迁移 schema 一致。
const memoryEntitySelectColumns = `
	id, scope_type, scope_id, workspace_id, user_id,
	entity_type, name, name_normalized, aliases_json, description, attributes_json,
	importance, confidence, use_count, source_kind,
	embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
	status, merged_into,
	metadata_json, created_at, updated_at, archived_at, deleted_at
`

const memoryRelationSelectColumns = `
	id, scope_type, scope_id, workspace_id,
	source_id, target_id, relation_type, bidirectional,
	weight, confidence, importance, use_count,
	attributes_json, evidence_json, status, source_kind,
	metadata_json, created_at, updated_at, archived_at, deleted_at
`

func scanMemoryEntity(s scanner) (mem.MemoryEntity, error) {
	var (
		e             mem.MemoryEntity
		scopeType     string
		entityType    string
		aliasesJSON   string
		attributesJSON string
		metadataJSON  string
		embeddingBlob []byte
	)
	if err := s.Scan(
		&e.ID, &scopeType, &e.ScopeID, &e.WorkspaceID, &e.UserID,
		&entityType, &e.Name, &e.NameNormalized, &aliasesJSON, &e.Description, &attributesJSON,
		&e.Importance, &e.Confidence, &e.UseCount, &e.SourceKind,
		&e.EmbeddingStatus, &e.EmbeddingModel, &e.EmbeddingDim, &embeddingBlob, &e.EmbeddingNorm,
		&e.Status, &e.MergedInto,
		&metadataJSON, &e.CreatedAt, &e.UpdatedAt, &e.ArchivedAt, &e.DeletedAt,
	); err != nil {
		return mem.MemoryEntity{}, err
	}
	e.ScopeType = mem.ScopeType(scopeType)
	e.EntityType = mem.EntityType(entityType)
	e.EmbeddingBlob = embeddingBlob
	e.Aliases = decodeJSONStringSlice(aliasesJSON)
	e.Attributes = decodeJSONObject(attributesJSON)
	e.Metadata = decodeJSONObject(metadataJSON)
	return e, nil
}

func scanMemoryRelation(s scanner) (mem.MemoryRelation, error) {
	var (
		r              mem.MemoryRelation
		scopeType      string
		relationType   string
		bidirectional  int
		attributesJSON string
		evidenceJSON   string
		metadataJSON   string
	)
	if err := s.Scan(
		&r.ID, &scopeType, &r.ScopeID, &r.WorkspaceID,
		&r.SourceID, &r.TargetID, &relationType, &bidirectional,
		&r.Weight, &r.Confidence, &r.Importance, &r.UseCount,
		&attributesJSON, &evidenceJSON, &r.Status, &r.SourceKind,
		&metadataJSON, &r.CreatedAt, &r.UpdatedAt, &r.ArchivedAt, &r.DeletedAt,
	); err != nil {
		return mem.MemoryRelation{}, err
	}
	r.ScopeType = mem.ScopeType(scopeType)
	r.RelationType = mem.RelationType(relationType)
	r.Bidirectional = bidirectional != 0
	r.Attributes = decodeJSONObject(attributesJSON)
	r.Evidence = decodeEvidenceList(evidenceJSON)
	r.Metadata = decodeJSONObject(metadataJSON)
	return r, nil
}

func decodeJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func decodeJSONObject(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func decodeEvidenceList(raw string) []mem.EvidenceRef {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []mem.EvidenceRef
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeJSONStringSlice(in []string) string {
	if len(in) == 0 {
		return "[]"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func encodeJSONObject(in map[string]any) string {
	if len(in) == 0 {
		return "{}"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func encodeEvidenceList(in []mem.EvidenceRef) string {
	if len(in) == 0 {
		return "[]"
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// UpsertEntity 按 (scope_type, scope_id, entity_type, name_normalized) 插入或更新 memory_entities。
// 已存在同自然键时保留原 id 并更新可变字段。
func (r *L4Repository) UpsertEntity(e mem.MemoryEntity) (mem.MemoryEntity, error) {
	if e.ScopeType == "" {
		return mem.MemoryEntity{}, errors.New("entity scope_type is required")
	}
	if e.EntityType == "" {
		return mem.MemoryEntity{}, errors.New("entity_type is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		return mem.MemoryEntity{}, errors.New("entity name is required")
	}
	if e.NameNormalized == "" {
		e.NameNormalized = strings.ToLower(strings.TrimSpace(e.Name))
	}
	if e.Status == "" {
		e.Status = mem.EntityStatusActive
	}
	if e.SourceKind == "" {
		e.SourceKind = mem.GraphSourceUser
	}
	if e.EmbeddingStatus == "" {
		e.EmbeddingStatus = "pending"
	}
	if e.Confidence == 0 {
		e.Confidence = 0.7
	}
	if e.Importance == 0 {
		e.Importance = 0.5
	}
	now := nowISO()
	if e.CreatedAt == "" {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	aliases := encodeJSONStringSlice(e.Aliases)
	attributes := encodeJSONObject(e.Attributes)
	metadata := encodeJSONObject(e.Metadata)

	if e.ID == "" {
		existing, err := r.GetEntityByName(e.ScopeType, e.ScopeID, e.EntityType, e.NameNormalized)
		if err == nil {
			e.ID = existing.ID
			e.CreatedAt = existing.CreatedAt
		} else if !errors.Is(err, sql.ErrNoRows) {
			return mem.MemoryEntity{}, err
		}
	}
	if e.ID == "" {
		return mem.MemoryEntity{}, errors.New("entity id is required (caller must populate)")
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_entities(
			id, scope_type, scope_id, workspace_id, user_id,
			entity_type, name, name_normalized, aliases_json, description, attributes_json,
			importance, confidence, use_count, source_kind,
			embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
			status, merged_into,
			metadata_json, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, entity_type, name_normalized) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			user_id = excluded.user_id,
			name = excluded.name,
			aliases_json = excluded.aliases_json,
			description = excluded.description,
			attributes_json = excluded.attributes_json,
			importance = excluded.importance,
			confidence = excluded.confidence,
			source_kind = excluded.source_kind,
			embedding_status = excluded.embedding_status,
			embedding_model = excluded.embedding_model,
			embedding_dim = excluded.embedding_dim,
			embedding_blob = excluded.embedding_blob,
			embedding_norm = excluded.embedding_norm,
			status = excluded.status,
			merged_into = excluded.merged_into,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at,
			archived_at = excluded.archived_at,
			deleted_at = excluded.deleted_at`,
		e.ID, string(e.ScopeType), e.ScopeID, e.WorkspaceID, e.UserID,
		string(e.EntityType), e.Name, e.NameNormalized, aliases, e.Description, attributes,
		e.Importance, e.Confidence, e.UseCount, e.SourceKind,
		e.EmbeddingStatus, e.EmbeddingModel, e.EmbeddingDim, e.EmbeddingBlob, e.EmbeddingNorm,
		e.Status, e.MergedInto,
		metadata, e.CreatedAt, e.UpdatedAt, e.ArchivedAt, e.DeletedAt,
	)
	if err != nil {
		return mem.MemoryEntity{}, err
	}
	stored, err := r.GetEntity(e.ID)
	if err != nil {
		return mem.MemoryEntity{}, err
	}
	return stored, nil
}

func (r *L4Repository) GetEntity(id string) (mem.MemoryEntity, error) {
	row := r.db.QueryRow(`SELECT `+memoryEntitySelectColumns+` FROM memory_entities WHERE id = ?`, id)
	return scanMemoryEntity(row)
}

func (r *L4Repository) GetEntityByName(scope mem.ScopeType, scopeID string, t mem.EntityType, normalized string) (mem.MemoryEntity, error) {
	row := r.db.QueryRow(`SELECT `+memoryEntitySelectColumns+` FROM memory_entities WHERE scope_type = ? AND scope_id = ? AND entity_type = ? AND name_normalized = ?`, string(scope), scopeID, string(t), normalized)
	return scanMemoryEntity(row)
}

func (r *L4Repository) ListEntities(q contracts.EntityListQuery) ([]mem.MemoryEntity, int, error) {
	conds := []string{}
	args := []any{}
	if q.ScopeType != "" {
		conds = append(conds, "scope_type = ?")
		args = append(args, string(q.ScopeType))
	}
	if q.ScopeID != "" {
		conds = append(conds, "scope_id = ?")
		args = append(args, q.ScopeID)
	}
	if q.WorkspaceID != "" {
		conds = append(conds, "workspace_id = ?")
		args = append(args, q.WorkspaceID)
	}
	if q.UserID != "" {
		conds = append(conds, "user_id = ?")
		args = append(args, q.UserID)
	}
	if q.EntityType != "" {
		conds = append(conds, "entity_type = ?")
		args = append(args, string(q.EntityType))
	}
	if q.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, q.Status)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		conds = append(conds, "(LOWER(name) LIKE ? OR LOWER(description) LIKE ?)")
		like := "%" + strings.ToLower(kw) + "%"
		args = append(args, like, like)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM memory_entities`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, limit, offset)
	rows, err := r.db.Query(`SELECT `+memoryEntitySelectColumns+` FROM memory_entities`+where+` ORDER BY importance DESC, updated_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []mem.MemoryEntity{}
	for rows.Next() {
		v, err := scanMemoryEntity(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *L4Repository) UpdateEntityStatus(id, status, mergedInto, archivedAt, deletedAt string) error {
	if id == "" {
		return errors.New("entity id is required")
	}
	_, err := r.db.Exec(
		`UPDATE memory_entities SET status = ?, merged_into = ?, archived_at = ?, deleted_at = ?, updated_at = ? WHERE id = ?`,
		status, mergedInto, archivedAt, deletedAt, nowISO(), id,
	)
	return err
}

func (r *L4Repository) UpdateEntityName(id, name, normalized string) error {
	if id == "" {
		return errors.New("entity id is required")
	}
	if name == "" {
		return errors.New("entity name is required")
	}
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(name))
	}
	_, err := r.db.Exec(
		`UPDATE memory_entities SET name = ?, name_normalized = ?, updated_at = ? WHERE id = ?`,
		name, normalized, nowISO(), id,
	)
	return err
}

func (r *L4Repository) UpsertEntityFact(entityID, factID string, weight float64) error {
	if entityID == "" || factID == "" {
		return errors.New("entity_id and fact_id are required")
	}
	if weight <= 0 {
		weight = 1.0
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_entity_facts(entity_id, fact_id, weight, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(entity_id, fact_id) DO UPDATE SET weight = excluded.weight`,
		entityID, factID, weight, nowISO(),
	)
	return err
}

func (r *L4Repository) ListFactsForEntity(entityID string, limit int) ([]mem.MemoryEntityFactLink, error) {
	if entityID == "" {
		return nil, errors.New("entity id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.Query(
		`SELECT entity_id, fact_id, weight, created_at FROM memory_entity_facts WHERE entity_id = ? ORDER BY weight DESC, created_at DESC LIMIT ?`,
		entityID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mem.MemoryEntityFactLink{}
	for rows.Next() {
		var v mem.MemoryEntityFactLink
		if err := rows.Scan(&v.EntityID, &v.FactID, &v.Weight, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *L4Repository) InsertEntityVersion(v mem.MemoryEntityVersion) error {
	if v.ID == "" {
		return errors.New("version id is required")
	}
	if v.EntityID == "" {
		return errors.New("entity id is required")
	}
	if v.SnapshotJSON == "" {
		return errors.New("snapshot is required")
	}
	if v.CreatedAt == "" {
		v.CreatedAt = nowISO()
	}
	if v.DiffJSON == "" {
		v.DiffJSON = "{}"
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_entity_versions(id, entity_id, version, snapshot_json, changed_by, change_reason, diff_json, metadata_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.EntityID, v.Version, v.SnapshotJSON, v.ChangedBy, v.ChangeReason, v.DiffJSON, encodeJSONObject(v.Metadata), v.CreatedAt,
	)
	return err
}

func (r *L4Repository) ListEntityVersions(entityID string, limit int) ([]mem.MemoryEntityVersion, error) {
	if entityID == "" {
		return nil, errors.New("entity id is required")
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, err := r.db.Query(
		`SELECT id, entity_id, version, snapshot_json, changed_by, change_reason, diff_json, metadata_json, created_at
		 FROM memory_entity_versions WHERE entity_id = ? ORDER BY version DESC LIMIT ?`,
		entityID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mem.MemoryEntityVersion{}
	for rows.Next() {
		var v mem.MemoryEntityVersion
		var metadata string
		if err := rows.Scan(&v.ID, &v.EntityID, &v.Version, &v.SnapshotJSON, &v.ChangedBy, &v.ChangeReason, &v.DiffJSON, &metadata, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Metadata = decodeJSONObject(metadata)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *L4Repository) BumpEntityUseCount(id string, atISO string) error {
	if id == "" {
		return errors.New("entity id is required")
	}
	if atISO == "" {
		atISO = nowISO()
	}
	_, err := r.db.Exec(`UPDATE memory_entities SET use_count = use_count + 1, updated_at = ? WHERE id = ?`, atISO, id)
	return err
}

// UpsertRelation 按 (scope_type, scope_id, source_id, target_id, relation_type) 插入或更新 memory_relations。
func (r *L4Repository) UpsertRelation(rel mem.MemoryRelation) (mem.MemoryRelation, error) {
	if rel.ScopeType == "" {
		return mem.MemoryRelation{}, errors.New("relation scope_type is required")
	}
	if rel.SourceID == "" || rel.TargetID == "" {
		return mem.MemoryRelation{}, errors.New("relation source_id and target_id are required")
	}
	if rel.RelationType == "" {
		return mem.MemoryRelation{}, errors.New("relation_type is required")
	}
	if rel.Status == "" {
		rel.Status = mem.RelationStatusActive
	}
	if rel.SourceKind == "" {
		rel.SourceKind = mem.GraphSourceUser
	}
	if rel.Weight == 0 {
		rel.Weight = 1.0
	}
	if rel.Confidence == 0 {
		rel.Confidence = 0.7
	}
	if rel.Importance == 0 {
		rel.Importance = 0.5
	}
	now := nowISO()
	if rel.CreatedAt == "" {
		rel.CreatedAt = now
	}
	rel.UpdatedAt = now
	if rel.ID == "" {
		return mem.MemoryRelation{}, errors.New("relation id is required (caller must populate)")
	}
	_, err := r.db.Exec(
		`INSERT INTO memory_relations(
			id, scope_type, scope_id, workspace_id,
			source_id, target_id, relation_type, bidirectional,
			weight, confidence, importance, use_count,
			attributes_json, evidence_json, status, source_kind,
			metadata_json, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, source_id, target_id, relation_type) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			bidirectional = excluded.bidirectional,
			weight = excluded.weight,
			confidence = excluded.confidence,
			importance = excluded.importance,
			attributes_json = excluded.attributes_json,
			evidence_json = excluded.evidence_json,
			status = excluded.status,
			source_kind = excluded.source_kind,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at,
			archived_at = excluded.archived_at,
			deleted_at = excluded.deleted_at`,
		rel.ID, string(rel.ScopeType), rel.ScopeID, rel.WorkspaceID,
		rel.SourceID, rel.TargetID, string(rel.RelationType), boolToInt(rel.Bidirectional),
		rel.Weight, rel.Confidence, rel.Importance, rel.UseCount,
		encodeJSONObject(rel.Attributes), encodeEvidenceList(rel.Evidence), rel.Status, rel.SourceKind,
		encodeJSONObject(rel.Metadata), rel.CreatedAt, rel.UpdatedAt, rel.ArchivedAt, rel.DeletedAt,
	)
	if err != nil {
		return mem.MemoryRelation{}, err
	}
	return r.GetRelation(rel.ID)
}

func (r *L4Repository) GetRelation(id string) (mem.MemoryRelation, error) {
	row := r.db.QueryRow(`SELECT `+memoryRelationSelectColumns+` FROM memory_relations WHERE id = ?`, id)
	return scanMemoryRelation(row)
}

func (r *L4Repository) ListRelationsForNode(nodeID string, limit int) ([]mem.MemoryRelation, error) {
	if nodeID == "" {
		return nil, errors.New("node id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(
		`SELECT `+memoryRelationSelectColumns+` FROM memory_relations
		 WHERE (source_id = ? OR target_id = ?) AND status = 'active'
		 ORDER BY weight DESC, updated_at DESC LIMIT ?`,
		nodeID, nodeID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mem.MemoryRelation{}
	for rows.Next() {
		v, err := scanMemoryRelation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *L4Repository) UpdateRelationStatus(id, status, archivedAt, deletedAt string) error {
	if id == "" {
		return errors.New("relation id is required")
	}
	_, err := r.db.Exec(
		`UPDATE memory_relations SET status = ?, archived_at = ?, deleted_at = ?, updated_at = ? WHERE id = ?`,
		status, archivedAt, deletedAt, nowISO(), id,
	)
	return err
}

func (r *L4Repository) BumpRelationUseCount(id string, atISO string) error {
	if id == "" {
		return errors.New("relation id is required")
	}
	if atISO == "" {
		atISO = nowISO()
	}
	_, err := r.db.Exec(`UPDATE memory_relations SET use_count = use_count + 1, updated_at = ? WHERE id = ?`, atISO, id)
	return err
}

// GetNeighborhood 从 `centerID` 向外最多扩展 `hops` 层，返回至多 `maxNodes` 个不同实体（不含中心）及连接关系。
// 用纯 Go 多次 SELECT 实现，避免依赖各 SQLite 版本上 `WITH RECURSIVE` 的差异。
func (r *L4Repository) GetNeighborhood(centerID string, hops, maxNodes int) (mem.GraphNeighborhood, error) {
	center, err := r.GetEntity(centerID)
	if err != nil {
		return mem.GraphNeighborhood{}, err
	}
	if hops <= 0 {
		hops = 1
	}
	if hops > 3 {
		hops = 3
	}
	if maxNodes <= 0 || maxNodes > 200 {
		maxNodes = 25
	}

	visited := map[string]bool{centerID: true}
	frontier := []string{centerID}
	relSeen := map[string]bool{}
	entities := []mem.MemoryEntity{}
	relations := []mem.MemoryRelation{}

	for h := 0; h < hops && len(visited) < maxNodes+1 && len(frontier) > 0; h++ {
		next := []string{}
		for _, node := range frontier {
			rels, err := r.ListRelationsForNode(node, 100)
			if err != nil {
				return mem.GraphNeighborhood{}, err
			}
			for _, rel := range rels {
				if relSeen[rel.ID] {
					continue
				}
				relSeen[rel.ID] = true
				relations = append(relations, rel)
				other := rel.TargetID
				if other == node {
					other = rel.SourceID
				}
				if other == "" || visited[other] {
					continue
				}
				if len(visited) >= maxNodes+1 {
					continue
				}
				visited[other] = true
				ent, err := r.GetEntity(other)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						continue
					}
					return mem.GraphNeighborhood{}, err
				}
				entities = append(entities, ent)
				next = append(next, other)
			}
		}
		frontier = next
	}

	return mem.GraphNeighborhood{
		Center:    center,
		Hops:      hops,
		Entities:  entities,
		Relations: relations,
	}, nil
}

