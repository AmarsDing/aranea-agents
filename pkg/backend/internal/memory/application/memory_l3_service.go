// MemoryL3Service 为 L3 语义记忆门面，
// 见 `aranea/docs/15 memory-L3-semantic.md`。第一阶段提供
// 事实 CRUD、指纹去重、版本历史、BM25 召回
//（向量路径已接线，在产生嵌入前保持不活跃）、
// 反馈驱动置信度、冲突跟踪、衰减批次，以及
// 供 L0 注入的提示渲染。
//
// 本服务刻意不自带 goroutine——衰减/嵌入任务调度由调用方负责。
// 测试可直接内联调用 RunDecayBatch / BuildEmbedding。
package application

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// MemoryL3Service 在 HTTP / L0 / 整合层与 SQLite 仓库之间协调。
// 独占 upsert / 去重 / 冲突检测规则，使调用方保持简单。
type MemoryL3Service struct {
	repo         repository.Store
	pii          *PIIFilter
	embedder     EmbeddingSource
	memoryL4     L4FactExtractionSource
	now          func() string
	scopeWeights map[mem.ScopeType]float64
}

// EmbeddingSource 是 MemoryL3Service 向 LLM 提供方请求文本向量嵌入的窄接口。
// 实现可包装 ProviderService 或任意 HTTP 客户端。该接缝使
// 测试中可桩替换嵌入，服务仍可用。
type EmbeddingSource interface {
	Embed(ctx context.Context, model, text string) ([]float32, error)
}

// L4FactExtractionSource 为 L3→L4 提取钩子的窄依赖，保持尽力而为且无环。
type L4FactExtractionSource interface {
	ExtractFromFact(ctx context.Context, factID string) (ExtractionReport, error)
}

// FactListResult 为 GET §6.2 列表端点的线形状。
type FactListResult struct {
	Items  []mem.MemoryFact `json:"items"`
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

// FactPatch 为部分更新载荷（§6.2 PATCH）。指针表示「字段有值」；nil 表示不改动原值。
type FactPatch struct {
	Statement       *string          `json:"statement,omitempty"`
	DetailsMarkdown *string          `json:"details_markdown,omitempty"`
	Tags            *[]string        `json:"tags,omitempty"`
	Kind            *mem.FactKind `json:"fact_kind,omitempty"`
	Confidence      *float64         `json:"confidence,omitempty"`
	Importance      *float64         `json:"importance,omitempty"`
	Status          *string          `json:"status,omitempty"`
	TTLDays         *int             `json:"ttl_days,omitempty"`
	By              string           `json:"by,omitempty"`
	Reason          string           `json:"reason,omitempty"`
}

// BulkUpsertReport 汇总 §5.6 BulkUpsert 调用结果。
type BulkUpsertReport struct {
	Created    int `json:"created"`
	Updated    int `json:"updated"`
	Duplicated int `json:"duplicated"`
	Errors     int `json:"errors"`
	Conflicts  int `json:"conflicts"`
}

// DecayReport 汇总 §5.5 RunDecayBatch 调用结果。
type DecayReport struct {
	Processed      int     `json:"processed"`
	Archived       int     `json:"archived"`
	ConfidenceDrop float64 `json:"confidence_drop"`
}

// L3StatsReport 为 GET /admin/memory/l3/stats 的返回结构。
type L3StatsReport struct {
	StatusCounts map[string]int `json:"status_counts"`
}

// NewMemoryL3Service 在仓库上构建服务，带合理默认：正则 PII 过滤器、
// 无嵌入器（向量路径不活跃），以及 §5.3 的作用域权重。
func NewMemoryL3Service(repo repository.Store) *MemoryL3Service {
	return &MemoryL3Service{
		repo: repo,
		pii:  NewPIIFilter(),
		now:  nowUTC,
		scopeWeights: map[mem.ScopeType]float64{
			mem.ScopeAgent:     1.0,
			mem.ScopeUser:      0.95,
			mem.ScopeTeam:      0.9,
			mem.ScopeWorkspace: 0.85,
			mem.ScopeGlobal:    0.8,
		},
	}
}

// SetEmbeddingSource 接入 Recall 与 BuildEmbedding 使用的嵌入提供方。nil 则禁用向量路径（BM25 仍可用）。
func (s *MemoryL3Service) SetEmbeddingSource(src EmbeddingSource) { s.embedder = src }

// SetL4ExtractionSource 在 L3 事实写入后连接 L4 词典/实体提取。提取失败会审计，从不阻塞事实写入。
func (s *MemoryL3Service) SetL4ExtractionSource(src L4FactExtractionSource) { s.memoryL4 = src }

// SetClock 覆盖时钟供测试使用。
func (s *MemoryL3Service) SetClock(now func() string) {
	if now != nil {
		s.now = now
	}
}

// --- 写入路径 ------------------------------------------------------------

// UpsertFact 应用 §5.2 流程：PII 检测、规范化、
// 指纹去重、版本快照、审计日志。返回结果事实（新建或更新）。
func (s *MemoryL3Service) UpsertFact(ctx context.Context, in mem.FactUpsertInput) (mem.MemoryFact, error) {
	_ = ctx
	if !in.ScopeType.IsValid() {
		return mem.MemoryFact{}, validationError("scope_type %q is invalid", in.ScopeType)
	}
	if strings.TrimSpace(in.Statement) == "" {
		return mem.MemoryFact{}, validationError("statement is required")
	}
	statement := strings.TrimSpace(in.Statement)
	if max := 4000; len(statement) > max {
		statement = statement[:max]
	}
	details := strings.TrimSpace(in.DetailsMarkdown)
	if max := 4000; len(details) > max {
		details = details[:max]
	}
	if in.Kind == "" {
		in.Kind = mem.FactGeneric
	}
	if !in.Kind.IsValid() {
		return mem.MemoryFact{}, validationError("fact_kind %q is invalid", in.Kind)
	}

	scope := in.ScopeType
	scopeID := strings.TrimSpace(in.ScopeID)
	if scopeID == "" {
		scopeID = inferScopeID(scope, in)
	}

	piiHit, redacted := s.pii.RedactPII(statement)
	piiHitDetails, redactedDetails := s.pii.RedactPII(details)
	if piiHit || piiHitDetails {
		// 规范 §5.2 第 2 步：检测到 PII 时强制作用域为
		// user（或 agent），避免脱敏形式被共享。多数 PII 属用户级故选
		// "user"；上游若更清楚可预先设置正确作用域。
		if scope != mem.ScopeAgent && scope != mem.ScopeUser {
			scope = mem.ScopeUser
			if scopeID == "" {
				scopeID = strings.TrimSpace(in.UserID)
			}
		}
	}

	normalized := normalizeStatement(statement)
	fp := fingerprintForStatement(scope, scopeID, normalized)

	confidence := clampUnit(in.Confidence)
	if confidence == 0 {
		confidence = 0.7
	}
	importance := clampUnit(in.Importance)
	if importance == 0 {
		importance = 0.5
	}

	tags := dedupStrings(in.Tags)
	metaJSON := encodeMetaJSON(in.Metadata)

	existing, err := s.repo.GetFactByFingerprint(scope, scopeID, fp)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return mem.MemoryFact{}, err
	}
	if err == nil && existing.ID != "" {
		// 更新路径 — 提高置信度（有上限）并合并标签。
		existing.Statement = statement
		existing.StatementNormalized = normalized
		existing.DetailsMarkdown = chooseNonEmpty(details, existing.DetailsMarkdown)
		existing.Kind = in.Kind
		existing.Tags = mergeStringLists(existing.Tags, tags)
		existing.Confidence = clampUnit(existing.Confidence + 0.05)
		existing.Importance = math.Max(existing.Importance, importance)
		existing.Version++
		if in.SourceKind != "" {
			existing.SourceKind = in.SourceKind
		}
		if in.SourceEpisodeID != "" {
			existing.SourceEpisodeID = in.SourceEpisodeID
		}
		if in.SourceSessionID != "" {
			existing.SourceSessionID = in.SourceSessionID
		}
		if in.SourceMessageID != "" {
			existing.SourceMessageID = in.SourceMessageID
		}
		if in.SourceExternal != "" {
			existing.SourceExternal = in.SourceExternal
		}
		if in.TTLDays > 0 {
			existing.TTLDays = in.TTLDays
		}
		if metaJSON != "{}" {
			existing.MetadataJSON = metaJSON
		}
		existing.PIIFlag = existing.PIIFlag || piiHit || piiHitDetails
		if piiHit {
			existing.RedactedStatement = redacted
		}
		if piiHitDetails {
			existing.DetailsMarkdown = redactedDetails
		}
		if err = s.repo.UpdateFact(existing); err != nil {
			return mem.MemoryFact{}, err
		}
		_ = s.recordVersion(existing, "update", in.By)
		_ = s.refreshFTS(existing)
		_ = s.audit("memory.l3.upsert", "memory_facts", existing.ID, map[string]any{
			"scope":  string(existing.ScopeType),
			"reason": "fingerprint_match",
			"by":     in.By,
		})
		updated, err := s.repo.GetFact(existing.ID)
		if err != nil {
			return mem.MemoryFact{}, err
		}
		s.extractFactToL4(ctx, updated.ID)
		return updated, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return mem.MemoryFact{}, err
	}

	// 创建路径。
	fact := mem.MemoryFact{
		ID:                  newID(),
		ScopeType:           scope,
		ScopeID:             scopeID,
		WorkspaceID:         in.WorkspaceID,
		UserID:              in.UserID,
		TeamID:              in.TeamID,
		AgentID:             in.AgentID,
		Statement:           statement,
		StatementNormalized: normalized,
		Fingerprint:         fp,
		DetailsMarkdown:     details,
		Kind:                in.Kind,
		Tags:                tags,
		Confidence:          confidence,
		Importance:          importance,
		SourceKind:          chooseNonEmpty(in.SourceKind, "user"),
		SourceEpisodeID:     in.SourceEpisodeID,
		SourceSessionID:     in.SourceSessionID,
		SourceMessageID:     in.SourceMessageID,
		SourceExternal:      in.SourceExternal,
		Version:             1,
		Status:              mem.FactStatusActive,
		EmbeddingStatus:     "pending",
		PIIFlag:             piiHit || piiHitDetails,
		RedactedStatement:   redacted,
		TTLDays:             in.TTLDays,
		DecayFactor:         0.98,
		MetadataJSON:        metaJSON,
		LastUsedAt:          s.now(),
	}
	if piiHitDetails {
		fact.DetailsMarkdown = redactedDetails
	}
	created, err := s.repo.CreateFact(fact)
	if err != nil {
		return mem.MemoryFact{}, err
	}
	_ = s.recordVersion(created, "create", in.By)
	_ = s.refreshFTS(created)
	_ = s.audit("memory.l3.upsert", "memory_facts", created.ID, map[string]any{
		"scope":  string(created.ScopeType),
		"reason": "create",
		"by":     in.By,
	})
	s.extractFactToL4(ctx, created.ID)
	return created, nil
}

// BulkUpsert 遍历 UpsertFact 并汇总每行结果。
// 错误不中止整批；记录在报告中。
func (s *MemoryL3Service) BulkUpsert(ctx context.Context, ins []mem.FactUpsertInput) (BulkUpsertReport, error) {
	out := BulkUpsertReport{}
	for _, in := range ins {
		before, _ := s.repo.GetFactByFingerprint(in.ScopeType, in.ScopeID, fingerprintForStatement(in.ScopeType, in.ScopeID, normalizeStatement(in.Statement)))
		fact, err := s.UpsertFact(ctx, in)
		if err != nil {
			out.Errors++
			continue
		}
		if before.ID == "" || before.ID != fact.ID {
			out.Created++
		} else {
			out.Updated++
		}
	}
	return out, nil
}

// UpdateFact 应用部分补丁并写入版本快照。
func (s *MemoryL3Service) UpdateFact(ctx context.Context, id string, patch FactPatch) (mem.MemoryFact, error) {
	_ = ctx
	if id == "" {
		return mem.MemoryFact{}, validationError("id is required")
	}
	fact, err := s.repo.GetFact(id)
	if err != nil {
		return mem.MemoryFact{}, err
	}
	changed := false
	if patch.Statement != nil && strings.TrimSpace(*patch.Statement) != "" {
		stmt := strings.TrimSpace(*patch.Statement)
		piiHit, redacted := s.pii.RedactPII(stmt)
		fact.Statement = stmt
		fact.StatementNormalized = normalizeStatement(stmt)
		fact.Fingerprint = fingerprintForStatement(fact.ScopeType, fact.ScopeID, fact.StatementNormalized)
		fact.PIIFlag = fact.PIIFlag || piiHit
		if piiHit {
			fact.RedactedStatement = redacted
		}
		changed = true
	}
	if patch.DetailsMarkdown != nil {
		details := strings.TrimSpace(*patch.DetailsMarkdown)
		piiHit, redacted := s.pii.RedactPII(details)
		if piiHit {
			details = redacted
			fact.PIIFlag = true
		}
		fact.DetailsMarkdown = details
		changed = true
	}
	if patch.Tags != nil {
		fact.Tags = dedupStrings(*patch.Tags)
		changed = true
	}
	if patch.Kind != nil && (*patch.Kind).IsValid() {
		fact.Kind = *patch.Kind
		changed = true
	}
	if patch.Confidence != nil {
		fact.Confidence = clampUnit(*patch.Confidence)
		changed = true
	}
	if patch.Importance != nil {
		fact.Importance = clampUnit(*patch.Importance)
		changed = true
	}
	if patch.Status != nil && strings.TrimSpace(*patch.Status) != "" {
		fact.Status = strings.TrimSpace(*patch.Status)
		changed = true
	}
	if patch.TTLDays != nil {
		fact.TTLDays = *patch.TTLDays
		changed = true
	}
	if !changed {
		return fact, nil
	}
	fact.Version++
	if err = s.repo.UpdateFact(fact); err != nil {
		return mem.MemoryFact{}, err
	}
	_ = s.recordVersion(fact, chooseNonEmpty(patch.Reason, "update"), patch.By)
	_ = s.refreshFTS(fact)
	_ = s.audit("memory.l3.update", "memory_facts", fact.ID, map[string]any{
		"by":     patch.By,
		"reason": patch.Reason,
	})
	updated, err := s.repo.GetFact(fact.ID)
	if err != nil {
		return mem.MemoryFact{}, err
	}
	s.extractFactToL4(ctx, updated.ID)
	return updated, nil
}

// DeleteFact 软删除事实并从索引中移除。
func (s *MemoryL3Service) DeleteFact(ctx context.Context, id, by string) error {
	_ = ctx
	if id == "" {
		return validationError("id is required")
	}
	now := s.now()
	if err := s.repo.UpdateFactStatus(id, mem.FactStatusDeleted, "", now); err != nil {
		return err
	}
	_ = s.repo.DeleteFactIndex(id)
	_ = s.audit("memory.l3.delete", "memory_facts", id, map[string]any{"by": by})
	return nil
}

// RollbackFact 恢复到先前版本。写入新版本行
//（回滚本身可审计）并提高当前版本号而非直接覆盖。
func (s *MemoryL3Service) RollbackFact(ctx context.Context, id string, toVersion int, by string) (mem.MemoryFact, error) {
	_ = ctx
	if id == "" || toVersion <= 0 {
		return mem.MemoryFact{}, validationError("id and to_version are required")
	}
	fact, err := s.repo.GetFact(id)
	if err != nil {
		return mem.MemoryFact{}, err
	}
	target, err := s.repo.GetFactVersion(id, toVersion)
	if err != nil {
		return mem.MemoryFact{}, err
	}
	fact.Statement = target.Statement
	fact.StatementNormalized = normalizeStatement(target.Statement)
	fact.Fingerprint = fingerprintForStatement(fact.ScopeType, fact.ScopeID, fact.StatementNormalized)
	fact.DetailsMarkdown = target.Details
	fact.Tags = target.Tags
	fact.Confidence = target.Confidence
	fact.Status = chooseNonEmpty(target.Status, fact.Status)
	fact.Version++
	if err = s.repo.UpdateFact(fact); err != nil {
		return mem.MemoryFact{}, err
	}
	_ = s.recordVersion(fact, fmt.Sprintf("rollback_to_v%d", toVersion), by)
	_ = s.refreshFTS(fact)
	_ = s.audit("memory.l3.rollback", "memory_facts", id, map[string]any{"to": toVersion, "by": by})
	return s.repo.GetFact(id)
}

// --- 读取路径 -------------------------------------------------------------

// Get 按 ID 返回单条事实。
func (s *MemoryL3Service) Get(ctx context.Context, id string) (mem.MemoryFact, error) {
	_ = ctx
	if id == "" {
		return mem.MemoryFact{}, validationError("id is required")
	}
	return s.repo.GetFact(id)
}

// List 使用仓库过滤器结构体分页返回事实。
func (s *MemoryL3Service) List(ctx context.Context, q repository.FactListQuery) (FactListResult, error) {
	_ = ctx
	items, total, err := s.repo.ListFacts(q)
	if err != nil {
		return FactListResult{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	return FactListResult{Items: items, Total: total, Limit: limit, Offset: q.Offset}, nil
}

// ListVersions 返回某事实的历史行（最新在前）。
func (s *MemoryL3Service) ListVersions(ctx context.Context, factID string, limit int) ([]mem.FactVersion, error) {
	_ = ctx
	return s.repo.ListFactVersions(factID, limit)
}

// ListFeedback 返回最近反馈记录。
func (s *MemoryL3Service) ListFeedback(ctx context.Context, factID string, limit int) ([]mem.FactFeedback, error) {
	_ = ctx
	return s.repo.ListFactFeedback(factID, limit)
}

// Recall 实现 §5.3 混合检索。向量与 BM25 结果按事实 id 合并；
// 最终分数按规范系数组合 vector / BM25 / 置信度 / 新近性 / scope_weight。
func (s *MemoryL3Service) Recall(ctx context.Context, q mem.FactRecallQuery) ([]mem.FactRecallHit, error) {
	if q.TopK <= 0 {
		q.TopK = 5
	}
	if q.TopK > 50 {
		q.TopK = 50
	}
	if q.MinScore <= 0 {
		q.MinScore = 0.3
	}
	includes := q.IncludeScopes
	if len(includes) == 0 {
		includes = []mem.ScopeType{mem.ScopeAgent, mem.ScopeUser, mem.ScopeTeam, mem.ScopeWorkspace}
	}
	scopes, scopeIDs := s.expandScopes(includes, q)
	if len(scopes) == 0 {
		return nil, nil
	}

	queryText := strings.TrimSpace(q.Query)
	wantVector := len(q.QueryEmbedding) > 0
	if !wantVector && queryText != "" && s.embedder != nil {
		// 尽力请求嵌入器；失败静默，BM25 仍运行。
		if vec, err := s.embedder.Embed(ctx, "", queryText); err == nil {
			q.QueryEmbedding = vec
			wantVector = true
		}
	}

	var bm25Hits, vectorHits []mem.FactRecallHit
	if queryText != "" {
		var err error
		bm25Hits, err = s.repo.SearchFactsBM25(scopes, scopeIDs, queryText, q.TopK*4)
		if err != nil {
			return nil, err
		}
	}
	if wantVector {
		var err error
		vectorHits, err = s.repo.SearchFactsVector(scopes, scopeIDs, q.QueryEmbedding, q.TopK*4)
		if err != nil {
			return nil, err
		}
	}

	merged := mergeRecallHits(bm25Hits, vectorHits)
	merged = applyRecallFilters(merged, q)
	scoreHits(merged, s.scopeWeights, s.now())
	sort.Slice(merged, func(i, j int) bool { return merged[i].FinalScore > merged[j].FinalScore })

	out := make([]mem.FactRecallHit, 0, q.TopK)
	for _, h := range merged {
		if h.FinalScore < q.MinScore {
			continue
		}
		out = append(out, h)
		if len(out) >= q.TopK {
			break
		}
	}
	for _, h := range out {
		_ = s.repo.BumpFactUseStat(h.Fact.ID, true, s.now())
	}
	return out, nil
}

// Feedback 应用 §5.4 流程：插入行、调整置信度、
// 低于阈值自动归档，以及连续三次拒绝后自动创建冲突。
func (s *MemoryL3Service) Feedback(ctx context.Context, fb mem.FactFeedback) error {
	_ = ctx
	if fb.FactID == "" {
		return validationError("fact_id is required")
	}
	if fb.Type == "" {
		return validationError("type is required")
	}
	fact, err := s.repo.GetFact(fb.FactID)
	if err != nil {
		return err
	}
	if fb.ID == "" {
		fb.ID = newID()
	}
	if fb.Weight == 0 {
		fb.Weight = 1.0
	}
	if fb.CreatedAt == "" {
		fb.CreatedAt = s.now()
	}
	if _, err = s.repo.InsertFactFeedback(fb); err != nil {
		return err
	}
	delta := 0.0
	posInc, negInc := 0, 0
	switch fb.Type {
	case mem.FactFeedbackConfirm:
		delta = 0.10 * fb.Weight
		posInc = 1
	case mem.FactFeedbackReject:
		delta = -0.20 * fb.Weight
		negInc = 1
	case mem.FactFeedbackUsed:
		delta = 0.02 * fb.Weight
	case mem.FactFeedbackNotUsed:
		delta = -0.01 * fb.Weight
	case mem.FactFeedbackRefine:
		// refine：保持置信度，仅提高重要性。
		_ = s.repo.UpdateFact(mem.MemoryFact{
			ID: fact.ID, Statement: fact.Statement, StatementNormalized: fact.StatementNormalized,
			Fingerprint: fact.Fingerprint, DetailsMarkdown: fact.DetailsMarkdown, Kind: fact.Kind,
			Tags: fact.Tags, Confidence: fact.Confidence, Importance: clampUnit(fact.Importance + 0.05),
			SourceKind: fact.SourceKind, SourceEpisodeID: fact.SourceEpisodeID, SourceSessionID: fact.SourceSessionID,
			SourceMessageID: fact.SourceMessageID, SourceExternal: fact.SourceExternal,
			Version: fact.Version, Status: fact.Status, SupersededBy: fact.SupersededBy,
			PIIFlag: fact.PIIFlag, RedactedStatement: fact.RedactedStatement,
			TTLDays: fact.TTLDays, DecayFactor: fact.DecayFactor,
			NextDecayAt: fact.NextDecayAt, LastUsedAt: fact.LastUsedAt, ExpiresAt: fact.ExpiresAt,
			MetadataJSON: fact.MetadataJSON, ArchivedAt: fact.ArchivedAt, DeletedAt: fact.DeletedAt,
		})
	}
	if delta != 0 || posInc != 0 || negInc != 0 {
		newConf := clampUnit(fact.Confidence + delta)
		if err = s.repo.UpdateFactConfidence(fact.ID, newConf, 0, posInc, negInc); err != nil {
			return err
		}
		// 低于阈值时归档
		threshold := 0.2
		settings, sErr := s.repo.GetAgentRuntimeSettings(fb.AgentID)
		if sErr == nil && settings.L3ArchiveThreshold > 0 {
			threshold = settings.L3ArchiveThreshold
		}
		if newConf < threshold {
			_ = s.repo.UpdateFactStatus(fact.ID, mem.FactStatusArchived, "", s.now())
		}
	}
	if fb.Type == mem.FactFeedbackReject {
		recent, _ := s.repo.CountRecentFactFeedback(fact.ID, mem.FactFeedbackReject, 3)
		if recent >= 3 {
			_ = s.markSelfConflict(fact)
		}
	}
	_ = s.audit("memory.l3.feedback", "memory_facts", fact.ID, map[string]any{
		"type":   fb.Type,
		"source": fb.Source,
		"weight": fb.Weight,
	})
	return nil
}

// DetectConflicts 将事实与同作用域高相似邻居比较。
// 第一阶段用 BM25 代理：得分高于下限且指纹不同者
// 标为待人工复核的冲突候选。
func (s *MemoryL3Service) DetectConflicts(ctx context.Context, factID string) ([]mem.FactConflict, error) {
	_ = ctx
	if factID == "" {
		return nil, validationError("fact_id is required")
	}
	fact, err := s.repo.GetFact(factID)
	if err != nil {
		return nil, err
	}
	hits, err := s.repo.SearchFactsBM25(
		[]mem.ScopeType{fact.ScopeType},
		[]string{fact.ScopeID},
		fact.Statement,
		10,
	)
	if err != nil {
		return nil, err
	}
	var out []mem.FactConflict
	for _, h := range hits {
		if h.Fact.ID == fact.ID || h.Fact.Fingerprint == fact.Fingerprint {
			continue
		}
		c := mem.FactConflict{
			FactAID:    fact.ID,
			FactBID:    h.Fact.ID,
			ScopeType:  fact.ScopeType,
			ScopeID:    fact.ScopeID,
			Kind:       mem.FactConflictOverlap,
			Similarity: h.BM25Score,
			Status:     mem.FactConflictStatusOpen,
			DetectedBy: "runtime",
		}
		saved, err := s.repo.UpsertFactConflict(c)
		if err != nil {
			continue
		}
		out = append(out, saved)
	}
	return out, nil
}

// ResolveConflict 按选定操作标记冲突已解决。
// 当 resolution 为 keep_a / keep_b 时自动归档败方事实。
func (s *MemoryL3Service) ResolveConflict(ctx context.Context, conflictID, resolution, by string) error {
	_ = ctx
	if conflictID == "" {
		return validationError("conflict_id is required")
	}
	c, err := s.repo.GetFactConflict(conflictID)
	if err != nil {
		return err
	}
	if err = s.repo.UpdateFactConflictResolution(conflictID, mem.FactConflictStatusResolved, resolution, by, s.now()); err != nil {
		return err
	}
	switch resolution {
	case "keep_a":
		_ = s.repo.UpdateFactStatus(c.FactBID, mem.FactStatusArchived, c.FactAID, s.now())
	case "keep_b":
		_ = s.repo.UpdateFactStatus(c.FactAID, mem.FactStatusArchived, c.FactBID, s.now())
	case "mark_disputed":
		_ = s.repo.UpdateFactStatus(c.FactAID, mem.FactStatusDisputed, "", s.now())
		_ = s.repo.UpdateFactStatus(c.FactBID, mem.FactStatusDisputed, "", s.now())
	}
	_ = s.audit("memory.l3.conflict.resolve", "memory_fact_conflicts", conflictID, map[string]any{
		"resolution": resolution,
		"by":         by,
	})
	return nil
}

// ListOpenConflicts 代理仓库调用。
func (s *MemoryL3Service) ListOpenConflicts(ctx context.Context, scope mem.ScopeType, scopeID string) ([]mem.FactConflict, error) {
	_ = ctx
	return s.repo.ListOpenFactConflicts(scope, scopeID, 100)
}

// --- 异步/批处理 ----------------------------------------------------------

// BuildEmbedding 向配置的嵌入器请求事实向量并存储。可重复调用；覆盖已有 blob。
func (s *MemoryL3Service) BuildEmbedding(ctx context.Context, factID string) error {
	if s.embedder == nil {
		return errors.New("embedding source is not configured")
	}
	if factID == "" {
		return validationError("fact_id is required")
	}
	fact, err := s.repo.GetFact(factID)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(fact.Statement)
	if fact.DetailsMarkdown != "" {
		text = text + "\n" + fact.DetailsMarkdown
	}
	vec, err := s.embedder.Embed(ctx, "", text)
	if err != nil {
		return err
	}
	blob := repository.EncodeFloat32Blob(vec)
	norm := vectorL2Norm(vec)
	return s.repo.UpsertFactEmbedding(factID, "", len(vec), blob, norm)
}

// RunDecayBatch 扫描衰减窗口已到期的事实并应用
// §5.5 算法。返回计数以供调用方上报指标。
func (s *MemoryL3Service) RunDecayBatch(ctx context.Context) (DecayReport, error) {
	_ = ctx
	report := DecayReport{}
	now := s.now()
	facts, err := s.repo.ListFactsDueForDecay(now, 200)
	if err != nil {
		return report, err
	}
	threshold := 0.2
	intervalHours := 24
	report.Processed = len(facts)
	for _, f := range facts {
		factor := f.DecayFactor
		if factor <= 0 || factor >= 1 {
			factor = 0.98
		}
		newConf := clampUnit(f.Confidence * factor)
		report.ConfidenceDrop += f.Confidence - newConf
		nextAt := time.Now().UTC().Add(time.Duration(intervalHours) * time.Hour).Format(time.RFC3339)
		if newConf < threshold {
			_ = s.repo.UpdateFactStatus(f.ID, mem.FactStatusArchived, "", now)
			report.Archived++
			continue
		}
		_ = s.repo.ApplyFactDecay(f.ID, factor, nextAt)
	}
	if report.Archived == 0 && report.Processed > 0 {
		// 同时捕获经直接编辑/反馈而低于阈值的事实
		extra, _ := s.repo.ArchiveFactsBelowConfidence(threshold, 500)
		report.Archived += extra
	}
	return report, nil
}

// Stats 返回 §6.6 管理端统计载荷。
func (s *MemoryL3Service) Stats(ctx context.Context, scope mem.ScopeType, scopeID string) (L3StatsReport, error) {
	_ = ctx
	counts, err := s.repo.CountFactsByStatus(scope, scopeID)
	if err != nil {
		return L3StatsReport{}, err
	}
	return L3StatsReport{StatusCounts: counts}, nil
}

// --- L0 渲染 -----------------------------------------------------------

// RenderForPrompt 将召回命中格式化为可注入 L0 组装的系统块。
// 输出内容由 maxChars 限制，使 L0 预算逻辑可预期。
func (s *MemoryL3Service) RenderForPrompt(ctx context.Context, hits []mem.FactRecallHit, maxChars int) (mem.FactPromptBlock, error) {
	_ = ctx
	if maxChars <= 0 {
		maxChars = 1500
	}
	if len(hits) == 0 {
		return mem.FactPromptBlock{Section: "memory.l3", Role: "system"}, nil
	}
	var b strings.Builder
	b.WriteString("Relevant long-term knowledge:\n")
	used := []mem.FactRecallHit{}
	for _, h := range hits {
		statement := h.Fact.Statement
		if h.Fact.PIIFlag && h.Fact.RedactedStatement != "" {
			statement = h.Fact.RedactedStatement
		}
		line := fmt.Sprintf("- [%s/%s · conf %.2f] %s\n", h.Fact.Kind, h.Fact.ScopeType, h.Fact.Confidence, statement)
		if b.Len()+len(line) > maxChars {
			break
		}
		b.WriteString(line)
		used = append(used, h)
	}
	content := strings.TrimRight(b.String(), "\n")
	return mem.FactPromptBlock{
		Section: "memory.l3",
		Role:    "system",
		Tokens:  estimateTokensApprox(content),
		Content: content,
		Items:   used,
	}, nil
}

// RecallSegmentForL0 供 MemoryL0Service 使用的接缝。遵守
// 智能体运行时设置（top_k / min_score / scopes / max_chars），L0 无需了解 L3 内部。
func (s *MemoryL3Service) RecallSegmentForL0(ctx context.Context, sessionID, agentID, query string) (mem.L0Segment, bool) {
	return s.RecallSegmentForL0WithContext(ctx, mem.L0MemoryScopeContext{
		SessionID: sessionID,
		AgentID:   agentID,
		Query:     query,
	})
}

// RecallSegmentForL0WithContext 为带完整上下文的 L0 接缝。包含
// user/team/workspace 作用域 ID，使 `l3_recall_scopes_json=["agent","team","workspace"]` 等设置在普通对话中生效。
func (s *MemoryL3Service) RecallSegmentForL0WithContext(ctx context.Context, scope mem.L0MemoryScopeContext) (mem.L0Segment, bool) {
	if strings.TrimSpace(scope.Query) == "" {
		return mem.L0Segment{}, false
	}
	settings, _ := s.repo.GetAgentRuntimeSettings(scope.AgentID)
	if !settings.L3Enabled {
		return mem.L0Segment{}, false
	}
	q := mem.FactRecallQuery{
		WorkspaceID:   scope.WorkspaceID,
		UserID:        scope.UserID,
		TeamID:        scope.TeamID,
		AgentID:       scope.AgentID,
		Query:         scope.Query,
		IncludeScopes: parseScopeList(settings.L3RecallScopesJSON),
		TopK:          firstPositive(settings.L3RecallTopK, 5),
		MinScore:      firstPositiveFloat(settings.L3RecallMinScore, 0.55),
		MaxChars:      firstPositive(settings.L3MaxPerRecallChars, 1500),
	}
	hits, err := s.Recall(ctx, q)
	if err != nil || len(hits) == 0 {
		return mem.L0Segment{}, false
	}
	block, err := s.RenderForPrompt(ctx, hits, q.MaxChars)
	if err != nil || strings.TrimSpace(block.Content) == "" {
		return mem.L0Segment{}, false
	}
	return mem.L0Segment{
		Section: block.Section,
		Role:    block.Role,
		Source:  fmt.Sprintf("memory.l3:%d", len(hits)),
		Tokens:  block.Tokens,
		Content: block.Content,
		Preview: previewText(block.Content, l0PreviewLimit),
	}, true
}

// --- 内部实现 --------------------------------------------------------------

func (s *MemoryL3Service) recordVersion(f mem.MemoryFact, reason, by string) error {
	if reason == "" {
		reason = "update"
	}
	return s.repo.InsertFactVersion(mem.FactVersion{
		ID:           newID(),
		FactID:       f.ID,
		Version:      f.Version,
		Statement:    f.Statement,
		Details:      f.DetailsMarkdown,
		Tags:         f.Tags,
		Confidence:   f.Confidence,
		Status:       f.Status,
		ChangedBy:    by,
		ChangeReason: reason,
		CreatedAt:    s.now(),
	})
}

func (s *MemoryL3Service) refreshFTS(f mem.MemoryFact) error {
	if f.Status != mem.FactStatusActive {
		return s.repo.UpsertFactsFTS(f.ID, f.ScopeType, f.ScopeID, string(f.Kind), "")
	}
	parts := []string{f.Statement}
	if f.DetailsMarkdown != "" {
		parts = append(parts, f.DetailsMarkdown)
	}
	if len(f.Tags) > 0 {
		parts = append(parts, strings.Join(f.Tags, " "))
	}
	return s.repo.UpsertFactsFTS(f.ID, f.ScopeType, f.ScopeID, string(f.Kind), strings.Join(parts, " "))
}

func (s *MemoryL3Service) extractFactToL4(ctx context.Context, factID string) {
	if s.memoryL4 == nil || factID == "" {
		return
	}
	report, err := s.memoryL4.ExtractFromFact(ctx, factID)
	if err != nil {
		_ = s.audit("memory.l3.l4_extract_failed", "memory_facts", factID, map[string]any{"error": err.Error()})
		return
	}
	_ = s.audit("memory.l3.l4_extract", "memory_facts", factID, map[string]any{
		"new_entities":     report.NewEntities,
		"updated_entities": report.UpdatedEntities,
		"errors":           report.Errors,
		"note":             report.Note,
	})
}

func (s *MemoryL3Service) audit(action, resource, resourceID string, detail map[string]any) error {
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
		CreatedAt:  s.now(),
	})
}

func (s *MemoryL3Service) markSelfConflict(f mem.MemoryFact) error {
	c := mem.FactConflict{
		FactAID:    f.ID,
		FactBID:    f.ID,
		ScopeType:  f.ScopeType,
		ScopeID:    f.ScopeID,
		Kind:       mem.FactConflictContradiction,
		Status:     mem.FactConflictStatusOpen,
		DetectedBy: "feedback_streak",
		Similarity: 1.0,
	}
	_, err := s.repo.UpsertFactConflict(c)
	return err
}

func (s *MemoryL3Service) expandScopes(includes []mem.ScopeType, q mem.FactRecallQuery) ([]mem.ScopeType, []string) {
	var scopes []mem.ScopeType
	var ids []string
	for _, sc := range includes {
		switch sc {
		case mem.ScopeAgent:
			if q.AgentID != "" {
				scopes = append(scopes, sc)
				ids = append(ids, q.AgentID)
			}
		case mem.ScopeUser:
			if q.UserID != "" {
				scopes = append(scopes, sc)
				ids = append(ids, q.UserID)
			}
		case mem.ScopeTeam:
			if q.TeamID != "" {
				scopes = append(scopes, sc)
				ids = append(ids, q.TeamID)
			}
		case mem.ScopeWorkspace:
			if q.WorkspaceID != "" {
				scopes = append(scopes, sc)
				ids = append(ids, q.WorkspaceID)
			}
		case mem.ScopeGlobal:
			scopes = append(scopes, sc)
			ids = append(ids, "")
		}
	}
	return scopes, ids
}

// mergeRecallHits 按事实 id 合并两组命中，对每个 id 保留
// 最大的 BM25 / 向量分数。
func mergeRecallHits(bm, vec []mem.FactRecallHit) []mem.FactRecallHit {
	idx := map[string]*mem.FactRecallHit{}
	push := func(h mem.FactRecallHit) {
		cur, ok := idx[h.Fact.ID]
		if !ok {
			cp := h
			idx[h.Fact.ID] = &cp
			return
		}
		if h.BM25Score > cur.BM25Score {
			cur.BM25Score = h.BM25Score
		}
		if h.VectorScore > cur.VectorScore {
			cur.VectorScore = h.VectorScore
		}
		if h.Reason != "" && cur.Reason != "" && h.Reason != cur.Reason {
			cur.Reason = "hybrid"
		}
	}
	for _, h := range bm {
		push(h)
	}
	for _, h := range vec {
		push(h)
	}
	out := make([]mem.FactRecallHit, 0, len(idx))
	for _, p := range idx {
		out = append(out, *p)
	}
	return out
}

// applyRecallFilters 丢弃不符合请求标签/种类过滤的命中。仓库已过滤作用域与状态。
func applyRecallFilters(hits []mem.FactRecallHit, q mem.FactRecallQuery) []mem.FactRecallHit {
	if len(q.Tags) == 0 && len(q.Kinds) == 0 {
		return hits
	}
	tagSet := map[string]bool{}
	for _, t := range q.Tags {
		tagSet[strings.ToLower(strings.TrimSpace(t))] = true
	}
	kindSet := map[mem.FactKind]bool{}
	for _, k := range q.Kinds {
		kindSet[k] = true
	}
	out := hits[:0]
	for _, h := range hits {
		if len(kindSet) > 0 && !kindSet[h.Fact.Kind] {
			continue
		}
		if len(tagSet) > 0 {
			match := false
			for _, t := range h.Fact.Tags {
				if tagSet[strings.ToLower(t)] {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, h)
	}
	return out
}

// scoreHits 原地应用 §5.3 最终分数公式。FTS5 的 BM25 分数无界，
// 故用软上限压到 0..1；向量分数已按余弦归一化。
func scoreHits(hits []mem.FactRecallHit, scopeWeights map[mem.ScopeType]float64, nowISOValue string) {
	now, _ := time.Parse(time.RFC3339, nowISOValue)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for i := range hits {
		h := &hits[i]
		bm := normaliseBM25(h.BM25Score)
		vec := h.VectorScore
		if vec < 0 {
			vec = 0
		}
		recency := recencyBoost(h.Fact.LastUsedAt, now)
		weight := scopeWeights[h.Fact.ScopeType]
		if weight == 0 {
			weight = 0.7
		}
		h.ScopeWeight = weight
		h.FinalScore = clampUnit(0.65*vec + 0.15*bm + 0.10*h.Fact.Confidence + 0.05*recency + 0.05*weight)
	}
}

func normaliseBM25(score float64) float64 {
	if score <= 0 {
		return 0
	}
	v := score / (score + 5.0)
	if v > 1 {
		v = 1
	}
	return v
}

func recencyBoost(lastUsed string, now time.Time) float64 {
	if lastUsed == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, lastUsed)
	if err != nil {
		return 0
	}
	delta := now.Sub(t).Hours() / 24
	if delta < 0 {
		delta = 0
	}
	// 约 30 天半衰期。
	boost := math.Exp(-delta / 30)
	if boost > 1 {
		boost = 1
	}
	return boost
}

// --- 纯函数辅助 -----------------------------------------------------------

func normalizeStatement(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 0x7F:
			b.WriteRune(r)
			prevSpace = false
		default:
			// 丢弃标点
		}
	}
	return strings.TrimSpace(b.String())
}

func fingerprintForStatement(scope mem.ScopeType, scopeID, normalized string) string {
	h := sha256.Sum256([]byte(string(scope) + ":" + scopeID + ":" + normalized))
	return hex.EncodeToString(h[:])
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func dedupStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	return out
}

func mergeStringLists(a, b []string) []string {
	merged := append([]string{}, a...)
	merged = append(merged, b...)
	return dedupStrings(merged)
}

func chooseNonEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func encodeMetaJSON(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func inferScopeID(scope mem.ScopeType, in mem.FactUpsertInput) string {
	switch scope {
	case mem.ScopeAgent:
		return in.AgentID
	case mem.ScopeUser:
		return in.UserID
	case mem.ScopeTeam:
		return in.TeamID
	case mem.ScopeWorkspace:
		return in.WorkspaceID
	}
	return ""
}

func parseScopeList(raw string) []mem.ScopeType {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	out := make([]mem.ScopeType, 0, len(values))
	for _, v := range values {
		sc := mem.ScopeType(strings.ToLower(strings.TrimSpace(v)))
		if sc.IsValid() {
			out = append(out, sc)
		}
	}
	return out
}

func firstPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstPositiveFloat(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func vectorL2Norm(v []float32) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	return math.Sqrt(sum)
}
