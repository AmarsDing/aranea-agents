// MemoryL2Service 为 L2 情节记忆门面，
// 见 `aranea/docs/14 memory-L2-episodic.md`。第一阶段提供
// 将 L1 任务归档到 `memory_episodes`、统一事件视图
//（对 messages / tools / skills / model usage / team_run_steps 做 UNION ALL）、
// 情节 CRUD 与事件标记。第二阶段增加 BM25 索引与 Recall。
//
// 本服务刻意不自带 goroutine——索引构建与整合由调用方（cmd/server）调度，
// 以保证测试可重复。第一阶段测试可直接内联调用 BuildIndexFor。
package application

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
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

// MemoryL2Service 在 L1 任务、情节存储、统一事件视图、标记与（第二阶段）Recall 间协调。
// L1 依赖刻意收窄：仅快照契约（与 L1PromptSource 类似）保持
// 导入无环，测试可注入桩。
type MemoryL2Service struct {
	repo     repository.Store
	memoryL1 L1SnapshotSource
	memoryL4 L4EpisodeExtractionSource
	now      func() string
}

// L1SnapshotSource 为 L2 将 L1 任务归档为情节时所需的 MemoryL1Service 薄视图。由 *MemoryL1Service 实现。
type L1SnapshotSource interface {
	SnapshotForEpisode(ctx context.Context, taskID string) (mem.L1Episode, error)
}

// L4EpisodeExtractionSource 为从新创建情节提取图实体的窄依赖，避免 L2 依赖 L4 全量服务面。
type L4EpisodeExtractionSource interface {
	ExtractFromEpisode(ctx context.Context, episodeID string) (ExtractionReport, error)
}

// NewMemoryL2Service 在仓库上构建服务，并可接 L1 源。调用方通过 SetL1Source 接线，测试可不拉通完整 L1。
func NewMemoryL2Service(repo repository.Store) *MemoryL2Service {
	return &MemoryL2Service{repo: repo, now: nowUTC}
}

// SetL1Source 挂接 ArchiveL1Task 使用的 L1 快照提供方。nil 则禁用基于 L1 的归档，里程碑路径仍可用。
func (s *MemoryL2Service) SetL1Source(src L1SnapshotSource) { s.memoryL1 = src }

// SetL4ExtractionSource 在情节写入后连接 L4 提取。提取失败为尽力而为，不阻塞 L2 归档/里程碑创建。
func (s *MemoryL2Service) SetL4ExtractionSource(src L4EpisodeExtractionSource) { s.memoryL4 = src }

// SetClock 覆盖时钟供测试使用。
func (s *MemoryL2Service) SetClock(now func() string) {
	if now != nil {
		s.now = now
	}
}

// --- 输入/输出 --------------------------------------------------------

// CreateEpisodeInput 为 ArchiveL1Task（L1 快照提取后）与手工
// CreateMilestoneEpisode 共用的参数对象。HTTP 层也复用于 §6.3 POST。
type CreateEpisodeInput struct {
	SessionID      string                 `json:"session_id"`
	RunID          string                 `json:"run_id,omitempty"`
	TeamID         string                 `json:"team_id,omitempty"`
	AgentID        string                 `json:"agent_id,omitempty"`
	L1TaskID       string                 `json:"l1_task_id,omitempty"`
	Kind           mem.EpisodeKind     `json:"episode_kind,omitempty"`
	Title          string                 `json:"title"`
	Goal           string                 `json:"goal,omitempty"`
	Outcome        string                 `json:"outcome,omitempty"`
	OutcomeSummary string                 `json:"outcome_summary,omitempty"`
	ResultPreview  string                 `json:"result_preview,omitempty"`
	FailureReason  string                 `json:"failure_reason,omitempty"`
	Importance     float64                `json:"importance,omitempty"`
	Confidence     float64                `json:"confidence,omitempty"`
	UserFeedback   string                 `json:"user_feedback,omitempty"`
	CriticScore    float64                `json:"critic_score,omitempty"`
	KeyDecisions   []mem.L2KeyDecision `json:"key_decisions,omitempty"`
	KeyArtifacts   []mem.L2KeyArtifact `json:"key_artifacts,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
	StartedAt      string                 `json:"started_at,omitempty"`
	EndedAt        string                 `json:"ended_at,omitempty"`
}

// EpisodeListResult 为 GET §6.3 的线形状。
type EpisodeListResult struct {
	Items  []mem.MemoryEpisode `json:"items"`
	Total  int                    `json:"total"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

// EpisodeDetail 为 GET §6.3 单条情节的线形状。标记来自 `memory_event_marks`；
// 最近事件来自同会话的 `ListL2Events` 窗口，受 `started_at` / `ended_at` 约束。
type EpisodeDetail struct {
	Episode mem.MemoryEpisode     `json:"episode"`
	Events  []mem.MemoryL2Event   `json:"events,omitempty"`
	Marks   []mem.MemoryEventMark `json:"marks,omitempty"`
	Summary string                   `json:"summary,omitempty"`
}

// EventListResult 为 GET §6.2 的线形状。
type EventListResult struct {
	Items  []mem.MemoryL2Event `json:"items"`
	Total  int                    `json:"total"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

// MarkInput 为 POST §6.4 的线形状。
type MarkInput struct {
	EpisodeID string         `json:"episode_id,omitempty"`
	RefKind   string         `json:"ref_kind"`
	RefID     string         `json:"ref_id"`
	MarkType  string         `json:"mark_type"`
	MarkedBy  string         `json:"marked_by,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Weight    float64        `json:"weight,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// RetentionReport 由 ApplyRetention 返回，供定时任务记录指标/审计行。
type RetentionReport struct {
	ArchivedEpisodes int `json:"archived_episodes"`
	DeletedEpisodes  int `json:"deleted_episodes"`
}

// --- 情节生命周期 -------------------------------------------------------

// ArchiveL1Task 实现 §5.2：拉取 L1 快照、汇总计数、
// 推导重要性并创建情节（状态=pending consolidation）。
// 在 (session_id, l1_task_id) 上幂等——若任务已有活动情节则原样返回。
func (s *MemoryL2Service) ArchiveL1Task(ctx context.Context, l1TaskID string) (mem.MemoryEpisode, error) {
	if l1TaskID == "" {
		return mem.MemoryEpisode{}, validationError("l1_task_id is required")
	}
	if s.memoryL1 == nil {
		return mem.MemoryEpisode{}, errors.New("L1 source not configured")
	}
	snap, err := s.memoryL1.SnapshotForEpisode(ctx, l1TaskID)
	if err != nil {
		return mem.MemoryEpisode{}, err
	}
	if snap.SessionID == "" {
		return mem.MemoryEpisode{}, validationError("snapshot session_id is empty")
	}
	settings := s.resolveSettings(snap.AgentID)
	if !settings.EpisodeEnabled {
		return mem.MemoryEpisode{}, nil
	}
	if existing, ok := s.findExistingEpisodeForTask(snap.SessionID, l1TaskID); ok {
		return existing, nil
	}

	stats := s.collectSessionStats(snap.SessionID, snap.AgentID, snap.StartedAt, snap.EndedAt)
	keyDecisions, keyArtifacts := extractKeyDecisionsArtifacts(snap)
	importance := computeImportance(snap, stats)
	if importance < 0 {
		importance = 0
	} else if importance > 1 {
		importance = 1
	}

	now := s.now()
	endedAt := snap.EndedAt
	if endedAt == "" {
		endedAt = now
	}
	startedAt := snap.StartedAt
	if startedAt == "" {
		startedAt = endedAt
	}
	durationMS := computeDurationMS(startedAt, endedAt)

	title := snap.TaskTitle
	if title == "" {
		title = snap.TaskKey
	}
	if title == "" {
		title = "L1 task"
	}

	snapJSON, _ := json.Marshal(snap)
	metadataJSON := encodeMetadataJSON(map[string]any{
		"l1_used_tokens":   snap.UsedTokens,
		"l1_budget_tokens": snap.BudgetTokens,
		"l1_status":        string(snap.Status),
	})

	episode := mem.MemoryEpisode{
		ID:                  newID(),
		SessionID:           snap.SessionID,
		AgentID:             snap.AgentID,
		L1TaskID:            l1TaskID,
		Kind:                mem.EpisodeKindTask,
		Title:               title,
		Goal:                snap.TaskGoal,
		Outcome:             outcomeForStatus(snap.Status),
		OutcomeSummary:      "",
		ResultPreview:       previewText(snap.TaskGoal, l0PreviewLimit),
		Importance:          importance,
		Confidence:          0.7,
		CriticScore:         -1,
		MessageCount:        stats.MessageCount,
		ToolCallCount:       stats.ToolCallCount,
		SkillCallCount:      stats.SkillCallCount,
		MCPCallCount:        stats.MCPCallCount,
		TotalTokens:         stats.TotalTokens,
		TotalCostMicroUSD:   stats.TotalCostMicroUSD,
		DurationMS:          durationMS,
		L1SnapshotJSON:      string(snapJSON),
		KeyDecisionsJSON:    encodeKeyDecisions(keyDecisions),
		KeyArtifactsJSON:    encodeKeyArtifacts(keyArtifacts),
		ConsolidationStatus: "pending",
		EmbeddingStatus:     "pending",
		StartedAt:           startedAt,
		EndedAt:             endedAt,
		MetadataJSON:        metadataJSON,
	}
	created, err := s.repo.CreateEpisode(episode)
	if err != nil {
		return mem.MemoryEpisode{}, err
	}
	_ = s.audit("l2.archive_task", "memory_episodes", created.ID, map[string]any{
		"session":    created.SessionID,
		"agent":      created.AgentID,
		"l1_task":    l1TaskID,
		"importance": created.Importance,
	})
	if settings.IndexEnabled {
		// 尽力：索引失败不得阻塞归档。
		_ = s.BuildIndexFor(ctx, created.ID)
	}
	s.extractEpisodeToL4(ctx, created.ID)
	return created, nil
}

// CreateMilestoneEpisode 为 §5.4 用户/Critic/插件入口。
// 与 ArchiveL1Task 不同，不需要 L1 快照——调用方直接提供 title/goal/outcome。
// 重要性默认 0.6（高于一般整合门槛），整合任务会较快拾取。
func (s *MemoryL2Service) CreateMilestoneEpisode(ctx context.Context, in CreateEpisodeInput) (mem.MemoryEpisode, error) {
	_ = ctx
	if in.SessionID == "" {
		return mem.MemoryEpisode{}, validationError("session_id is required")
	}
	if in.Title == "" {
		return mem.MemoryEpisode{}, validationError("title is required")
	}
	kind := in.Kind
	if kind == "" {
		kind = mem.EpisodeKindMilestone
	}
	if !kind.IsValid() {
		return mem.MemoryEpisode{}, validationError("invalid episode_kind: %q", string(kind))
	}
	importance := in.Importance
	if importance == 0 {
		importance = 0.6
	}
	confidence := in.Confidence
	if confidence == 0 {
		confidence = 0.7
	}
	criticScore := in.CriticScore
	if criticScore == 0 {
		criticScore = -1
	}
	now := s.now()
	endedAt := in.EndedAt
	if endedAt == "" {
		endedAt = now
	}
	startedAt := in.StartedAt
	if startedAt == "" {
		startedAt = endedAt
	}
	episode := mem.MemoryEpisode{
		ID:                  newID(),
		SessionID:           in.SessionID,
		RunID:               in.RunID,
		TeamID:              in.TeamID,
		AgentID:             in.AgentID,
		L1TaskID:            in.L1TaskID,
		Kind:                kind,
		Title:               in.Title,
		Goal:                in.Goal,
		Outcome:             firstNonEmptyString(in.Outcome, "success"),
		OutcomeSummary:      in.OutcomeSummary,
		ResultPreview:       previewText(firstNonEmptyString(in.ResultPreview, in.OutcomeSummary, in.Goal), l0PreviewLimit),
		FailureReason:       in.FailureReason,
		Importance:          importance,
		Confidence:          confidence,
		UserFeedback:        in.UserFeedback,
		CriticScore:         criticScore,
		KeyDecisionsJSON:    encodeKeyDecisions(in.KeyDecisions),
		KeyArtifactsJSON:    encodeKeyArtifacts(in.KeyArtifacts),
		ConsolidationStatus: "pending",
		EmbeddingStatus:     "pending",
		StartedAt:           startedAt,
		EndedAt:             endedAt,
		MetadataJSON:        encodeMetadataJSON(in.Metadata),
		DurationMS:          computeDurationMS(startedAt, endedAt),
	}
	created, err := s.repo.CreateEpisode(episode)
	if err != nil {
		return mem.MemoryEpisode{}, err
	}
	_ = s.audit("l2.create_milestone", "memory_episodes", created.ID, map[string]any{
		"session": created.SessionID,
		"agent":   created.AgentID,
		"kind":    string(created.Kind),
	})
	settings := s.resolveSettings(in.AgentID)
	if settings.IndexEnabled {
		_ = s.BuildIndexFor(ctx, created.ID)
	}
	s.extractEpisodeToL4(ctx, created.ID)
	return created, nil
}

// UpdateEpisode 修改情节可编辑字段。调用方应传完整行（典型：GET → 修改 → PATCH）。仓库
// 保留嵌入/整合状态，工作线程可继续运行。
func (s *MemoryL2Service) UpdateEpisode(ctx context.Context, ep mem.MemoryEpisode) (mem.MemoryEpisode, error) {
	_ = ctx
	if ep.ID == "" {
		return mem.MemoryEpisode{}, validationError("id is required")
	}
	if err := s.repo.UpdateEpisode(ep); err != nil {
		return mem.MemoryEpisode{}, err
	}
	updated, err := s.repo.GetEpisode(ep.ID)
	if err != nil {
		return mem.MemoryEpisode{}, err
	}
	_ = s.audit("l2.update_episode", "memory_episodes", updated.ID, map[string]any{
		"session": updated.SessionID,
	})
	return updated, nil
}

// DeleteEpisode 为软删除。底层事件仍可查。
func (s *MemoryL2Service) DeleteEpisode(ctx context.Context, id string) error {
	_ = ctx
	if id == "" {
		return validationError("id is required")
	}
	if err := s.repo.SoftDeleteEpisode(id); err != nil {
		return err
	}
	_ = s.audit("l2.delete_episode", "memory_episodes", id, nil)
	return nil
}

// GetEpisode 加载情节及其标记与最近事件切片。
// 在可用时使用情节时间戳作为事件窗口，使调用方只见相关行。
func (s *MemoryL2Service) GetEpisode(ctx context.Context, id string) (EpisodeDetail, error) {
	_ = ctx
	if id == "" {
		return EpisodeDetail{}, validationError("id is required")
	}
	ep, err := s.repo.GetEpisode(id)
	if err != nil {
		return EpisodeDetail{}, err
	}
	marks, _ := s.repo.ListMarksForEpisode(ep.ID)
	events, _, _ := s.repo.ListL2Events(mem.MemoryL2EventQuery{
		SessionID:    ep.SessionID,
		StartTimeUTC: ep.StartedAt,
		EndTimeUTC:   ep.EndedAt,
		Limit:        100,
	})
	return EpisodeDetail{
		Episode: ep,
		Events:  events,
		Marks:   marks,
		Summary: ep.OutcomeSummary,
	}, nil
}

// ListEpisodes 分页返回某会话的情节。
func (s *MemoryL2Service) ListEpisodes(ctx context.Context, sessionID, kind string, limit, offset int) (EpisodeListResult, error) {
	_ = ctx
	if sessionID == "" {
		return EpisodeListResult{}, validationError("session_id is required")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	items, total, err := s.repo.ListEpisodes(sessionID, kind, limit, offset)
	if err != nil {
		return EpisodeListResult{}, err
	}
	if items == nil {
		items = []mem.MemoryEpisode{}
	}
	return EpisodeListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// ListEvents 代理到仓库的 UNION ALL 查询。
func (s *MemoryL2Service) ListEvents(ctx context.Context, q mem.MemoryL2EventQuery) (EventListResult, error) {
	_ = ctx
	if q.SessionID == "" {
		return EventListResult{}, validationError("session_id is required")
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	q.Limit = limit
	q.Offset = offset
	items, total, err := s.repo.ListL2Events(q)
	if err != nil {
		return EventListResult{}, err
	}
	if items == nil {
		items = []mem.MemoryL2Event{}
	}
	return EventListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// --- 标记 -------------------------------------------------------------------

// Mark 应用 §3.4 标记并按 §5.4 提高关联情节重要性（star ⇒ +0.2；consolidate ⇒ +0.15 等，上限 1.0）。当
// EpisodeID 为空且引用本身为情节时，使用 ref_id。
func (s *MemoryL2Service) Mark(ctx context.Context, in MarkInput) (mem.MemoryEventMark, error) {
	_ = ctx
	if in.RefKind == "" || in.RefID == "" || in.MarkType == "" {
		return mem.MemoryEventMark{}, validationError("ref_kind, ref_id and mark_type are required")
	}
	mark := mem.MemoryEventMark{
		EpisodeID: in.EpisodeID,
		RefKind:   in.RefKind,
		RefID:     in.RefID,
		MarkType:  in.MarkType,
		MarkedBy:  in.MarkedBy,
		Reason:    in.Reason,
		Weight:    in.Weight,
		Metadata:  in.Metadata,
	}
	if mark.EpisodeID == "" && in.RefKind == "episode" {
		mark.EpisodeID = in.RefID
	}
	// session_id 解析：优先关联情节；否则为 ""（仓库会拒绝 ""）。非情节引用须显式传 episode_id 以解析会话。
	sessionID, err := s.resolveSessionForMark(mark)
	if err != nil {
		return mem.MemoryEventMark{}, err
	}
	mark.SessionID = sessionID

	stored, err := s.repo.UpsertEventMark(mark)
	if err != nil {
		return mem.MemoryEventMark{}, err
	}
	if stored.EpisodeID != "" {
		s.adjustImportanceForMark(stored.EpisodeID, in.MarkType)
	}
	_ = s.audit("l2.mark", "memory_event_marks", stored.ID, map[string]any{
		"ref_kind":  stored.RefKind,
		"ref_id":    stored.RefID,
		"mark_type": stored.MarkType,
		"episode":   stored.EpisodeID,
	})
	return stored, nil
}

// UnMark 按 ID 软删标记。重要性不变，避免再次标记时重复累加。
func (s *MemoryL2Service) UnMark(ctx context.Context, id string) error {
	_ = ctx
	if id == "" {
		return validationError("id is required")
	}
	if err := s.repo.SoftDeleteEventMark(id); err != nil {
		return err
	}
	_ = s.audit("l2.unmark", "memory_event_marks", id, nil)
	return nil
}

// ListMarks 返回某会话的最近标记（可按类型过滤）。
func (s *MemoryL2Service) ListMarks(ctx context.Context, sessionID, markType string, limit int) ([]mem.MemoryEventMark, error) {
	_ = ctx
	if sessionID == "" {
		return nil, validationError("session_id is required")
	}
	return s.repo.ListEventMarks(sessionID, markType, limit)
}

// --- 召回（第二阶段能力，仅 BM25） ------------------------------------

// RecallByQuery 对 BM25 索引执行 §5.3 融合。向量召回在接入嵌入后可用（第三阶段）；函数签名稳定，调用方无需后续迁移。
func (s *MemoryL2Service) RecallByQuery(ctx context.Context, q mem.MemoryL2RecallQuery) ([]mem.MemoryL2RecallResult, error) {
	_ = ctx
	if q.SessionID == "" {
		return nil, validationError("session_id is required")
	}
	settings := s.resolveSettings(q.AgentID)
	if !settings.RecallEnabled && q.AgentID != "" {
		return nil, nil
	}
	topK := q.TopK
	if topK <= 0 {
		topK = settings.RecallMax
	}
	if topK <= 0 {
		topK = 5
	}
	min := q.MinImportance
	if min <= 0 {
		min = 0
	}
	results, err := s.repo.SearchL2BM25(q.SessionID, q.Query, min, topK*2)
	if err != nil {
		return nil, err
	}
	results = applyKindFilter(results, q.IncludeKinds)
	results = fuseRecallScores(results)
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// RecallSegmentForL0 在启用 L2 召回且有匹配情节时返回可注入提示的片段。L0 与 L3/L4 片段一并注入。函数永不返回错误——缺数据时静默 ok=false，L0 主路径无分支。
func (s *MemoryL2Service) RecallSegmentForL0(ctx context.Context, sessionID, agentID, query string) (mem.L0Segment, bool) {
	if sessionID == "" {
		return mem.L0Segment{}, false
	}
	settings := s.resolveSettings(agentID)
	if !settings.RecallEnabled {
		return mem.L0Segment{}, false
	}
	results, err := s.RecallByQuery(ctx, mem.MemoryL2RecallQuery{
		SessionID: sessionID,
		AgentID:   agentID,
		Query:     query,
		TopK:      settings.RecallMax,
	})
	if err != nil || len(results) == 0 {
		return mem.L0Segment{}, false
	}
	body := renderRecallMarkdown(results)
	if strings.TrimSpace(body) == "" {
		return mem.L0Segment{}, false
	}
	return mem.L0Segment{
		Section: "memory.l2",
		Role:    "system",
		Source:  fmt.Sprintf("memory.l2:recall(%d)", len(results)),
		Tokens:  estimateTokensApprox(body),
		Content: body,
		Preview: previewText(body, l0PreviewLimit),
	}, true
}

// --- 索引 -------------------------------------------------------------------

// BuildIndexFor 为情节生成 FTS5 行。第一阶段串接 (title, goal, outcome_summary, result_preview, key decisions) 的小文本——足够 BM25 排序；第三阶段可再加嵌入。
func (s *MemoryL2Service) BuildIndexFor(ctx context.Context, episodeID string) error {
	_ = ctx
	if episodeID == "" {
		return validationError("episode_id is required")
	}
	ep, err := s.repo.GetEpisode(episodeID)
	if err != nil {
		return err
	}
	text := buildIndexText(ep)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	entry := mem.MemoryL2IndexEntry{
		EpisodeID:     ep.ID,
		SessionID:     ep.SessionID,
		AgentID:       ep.AgentID,
		TextKind:      "episode",
		TextPreview:   previewText(text, l0PreviewLimit),
		TokenEstimate: estimateTokensApprox(text),
		Importance:    ep.Importance,
	}
	if err := s.repo.UpsertL2Index(entry, text); err != nil {
		return err
	}
	if err := s.repo.UpdateEpisodeEmbedding(ep.ID, "skipped", "", 0, 0); err != nil {
		return err
	}
	return nil
}

// --- 保留策略 --------------------------------------------------------------

// ApplyRetention 将超过 `archive_after_days` 的情节归档，并
// 硬删除早于 `retention_days` 的已归档/已删行。两阈值从智能体运行时设置读取；缺省回退规范默认（保留 90 天 / 归档 30 天）。
func (s *MemoryL2Service) ApplyRetention(ctx context.Context) (RetentionReport, error) {
	_ = ctx
	settings := s.resolveSettings("")
	now := time.Now().UTC()
	archiveBefore := now.Add(-time.Duration(settings.ArchiveAfterDays) * 24 * time.Hour).Format(time.RFC3339)
	retentionBefore := now.Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour).Format(time.RFC3339)
	archived, err := s.repo.ArchiveEpisodesBeforeDate("", archiveBefore)
	if err != nil {
		return RetentionReport{}, err
	}
	deleted, err := s.repo.DeleteArchivedEpisodesBefore(retentionBefore)
	if err != nil {
		return RetentionReport{}, err
	}
	_ = s.audit("l2.retention", "memory_episodes", "", map[string]any{
		"archived": archived,
		"deleted":  deleted,
	})
	return RetentionReport{ArchivedEpisodes: archived, DeletedEpisodes: deleted}, nil
}

// --- 内部实现 ---------------------------------------------------------------

type l2Settings struct {
	EpisodeEnabled       bool
	EpisodeMinImportance float64
	IndexEnabled         bool
	IndexEmbeddingModel  string
	RecallEnabled        bool
	RecallMax            int
	RetentionDays        int
	ArchiveAfterDays     int
}

func (s *MemoryL2Service) resolveSettings(agentID string) l2Settings {
	out := l2Settings{
		EpisodeEnabled:       true,
		EpisodeMinImportance: 0.3,
		IndexEnabled:         true,
		RecallEnabled:        false,
		RecallMax:            3,
		RetentionDays:        90,
		ArchiveAfterDays:     30,
	}
	if agentID == "" {
		return out
	}
	row, err := s.repo.GetAgentRuntimeSettings(agentID)
	if err != nil {
		return out
	}
	out.EpisodeEnabled = row.L2EpisodeEnabled
	if row.L2EpisodeMinImportance > 0 {
		out.EpisodeMinImportance = row.L2EpisodeMinImportance
	}
	out.IndexEnabled = row.L2IndexEnabled
	out.IndexEmbeddingModel = row.L2IndexEmbeddingModel
	out.RecallEnabled = row.L2RecallEnabled
	if row.L2RecallMax > 0 {
		out.RecallMax = row.L2RecallMax
	}
	if row.L2RetentionDays > 0 {
		out.RetentionDays = row.L2RetentionDays
	}
	if row.L2ArchiveAfterDays > 0 {
		out.ArchiveAfterDays = row.L2ArchiveAfterDays
	}
	return out
}

func (s *MemoryL2Service) findExistingEpisodeForTask(sessionID, l1TaskID string) (mem.MemoryEpisode, bool) {
	episodes, _, err := s.repo.ListEpisodes(sessionID, "", 50, 0)
	if err != nil {
		return mem.MemoryEpisode{}, false
	}
	for _, ep := range episodes {
		if ep.L1TaskID == l1TaskID && ep.DeletedAt == "" {
			return ep, true
		}
	}
	return mem.MemoryEpisode{}, false
}

// resolveSessionForMark 解析新标记的 session_id。情节级标记从关联情节读取；其余由调用方（前端）显式提供，避免反查来源表。
func (s *MemoryL2Service) resolveSessionForMark(m mem.MemoryEventMark) (string, error) {
	if m.EpisodeID != "" {
		ep, err := s.repo.GetEpisode(m.EpisodeID)
		if err == nil && ep.SessionID != "" {
			return ep.SessionID, nil
		}
	}
	if m.SessionID != "" {
		return m.SessionID, nil
	}
	return "", validationError("session_id (or episode_id) is required for mark")
}

// adjustImportanceForMark 按 §5.4 提高重要性。尽力而为——
// 失败静默，标记本身仍会落库。
func (s *MemoryL2Service) adjustImportanceForMark(episodeID, markType string) {
	delta := 0.0
	switch strings.ToLower(strings.TrimSpace(markType)) {
	case "star", "pin", "good_example":
		delta = 0.2
	case "consolidate", "critic_pass":
		delta = 0.15
	case "postmortem", "bad_example":
		delta = 0.1
	case "forget":
		delta = -0.3
	}
	if delta == 0 {
		return
	}
	ep, err := s.repo.GetEpisode(episodeID)
	if err != nil {
		return
	}
	next := ep.Importance + delta
	if next > 1 {
		next = 1
	} else if next < 0 {
		next = 0
	}
	if math.Abs(next-ep.Importance) < 1e-6 {
		return
	}
	ep.Importance = next
	_ = s.repo.UpdateEpisode(ep)
}

func (s *MemoryL2Service) audit(action, resource, resourceID string, detail map[string]any) error {
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

func (s *MemoryL2Service) extractEpisodeToL4(ctx context.Context, episodeID string) {
	if s.memoryL4 == nil || episodeID == "" {
		return
	}
	report, err := s.memoryL4.ExtractFromEpisode(ctx, episodeID)
	if err != nil {
		_ = s.audit("l2.l4_extract_failed", "memory_episodes", episodeID, map[string]any{"error": err.Error()})
		return
	}
	_ = s.audit("l2.l4_extract", "memory_episodes", episodeID, map[string]any{
		"new_entities":     report.NewEntities,
		"updated_entities": report.UpdatedEntities,
		"errors":           report.Errors,
		"note":             report.Note,
	})
}

// --- 纯函数辅助 -----------------------------------------------------------

// l2SessionStats 汇总情节头部的快速计数。
type l2SessionStats struct {
	MessageCount      int
	ToolCallCount     int
	SkillCallCount    int
	MCPCallCount      int
	TotalTokens       int
	TotalCostMicroUSD int64
}

// collectSessionStats 遍历统一事件视图填充情节上存储的计数。
// 若同时设置 (started_at, ended_at) 则尊重该窗口；否则使用整段会话。
func (s *MemoryL2Service) collectSessionStats(sessionID, agentID, startedAt, endedAt string) l2SessionStats {
	var stats l2SessionStats
	events, _, err := s.repo.ListL2Events(mem.MemoryL2EventQuery{
		SessionID:    sessionID,
		StartTimeUTC: startedAt,
		EndTimeUTC:   endedAt,
		Limit:        500,
	})
	if err != nil {
		return stats
	}
	for _, ev := range events {
		if agentID != "" && ev.ActorID != "" && ev.ActorID != agentID {
			continue
		}
		switch ev.Kind {
		case "message":
			stats.MessageCount++
		case "tool_call":
			stats.ToolCallCount++
		case "skill_call":
			stats.SkillCallCount++
		case "mcp_call":
			stats.MCPCallCount++
		case "model_call":
			stats.TotalTokens += ev.TokensIn + ev.TokensOut
			stats.TotalCostMicroUSD += ev.CostMicro
		}
	}
	return stats
}

// computeImportance 为 §5.2 第 5 步评分公式。刻意为线性有界，
// 便于前端展示各因素贡献。
func computeImportance(snap mem.L1Episode, stats l2SessionStats) float64 {
	imp := 0.3
	if snap.Status == mem.L1TaskCompleted {
		imp += 0.2
	}
	if stats.MessageCount > 0 {
		imp += 0.1
	}
	if stats.ToolCallCount > 0 || stats.SkillCallCount > 0 {
		imp += 0.1
	}
	used := snap.UsedTokens
	budget := snap.BudgetTokens
	if budget > 0 {
		ratio := float64(used) / float64(budget)
		if ratio > 0.5 {
			imp += 0.1
		}
		if ratio > 0.8 {
			imp += 0.1
		}
	}
	if imp > 1 {
		imp = 1
	}
	return imp
}

// extractKeyDecisionsArtifacts 遍历 L1 快照，抽取路径以 "decisions." 或 "artifacts." 开头的字段。第一阶段保持简单——仅取渲染值；后续可从 `session_trace_spans` 丰富元数据。
func extractKeyDecisionsArtifacts(snap mem.L1Episode) ([]mem.L2KeyDecision, []mem.L2KeyArtifact) {
	var decisions []mem.L2KeyDecision
	var artifacts []mem.L2KeyArtifact
	for path, raw := range snap.Snapshot {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value := fmt.Sprintf("%v", entry["value"])
		switch {
		case strings.HasPrefix(path, "decisions."):
			decisions = append(decisions, mem.L2KeyDecision{
				Decision:  strings.TrimPrefix(path, "decisions."),
				Rationale: value,
				At:        fmt.Sprintf("%v", entry["updated_at"]),
			})
		case strings.HasPrefix(path, "artifacts."):
			artifacts = append(artifacts, mem.L2KeyArtifact{
				Kind:    strings.TrimPrefix(path, "artifacts."),
				Ref:     value,
				Preview: previewText(value, 120),
			})
		}
	}
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].Decision < decisions[j].Decision })
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Kind < artifacts[j].Kind })
	return decisions, artifacts
}

func encodeKeyDecisions(values []mem.L2KeyDecision) string {
	if len(values) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func encodeKeyArtifacts(values []mem.L2KeyArtifact) string {
	if len(values) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func encodeMetadataJSON(meta map[string]any) string {
	if len(meta) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func outcomeForStatus(status mem.L1TaskStatus) string {
	switch status {
	case mem.L1TaskCompleted:
		return "success"
	case mem.L1TaskFailed:
		return "failed"
	case mem.L1TaskCancelled:
		return "cancelled"
	case mem.L1TaskArchived:
		return "archived"
	}
	return "partial"
}

func computeDurationMS(startedAt, endedAt string) int {
	if startedAt == "" || endedAt == "" {
		return 0
	}
	start, err1 := time.Parse(time.RFC3339, startedAt)
	end, err2 := time.Parse(time.RFC3339, endedAt)
	if err1 != nil || err2 != nil {
		return 0
	}
	d := end.Sub(start)
	if d < 0 {
		return 0
	}
	return int(d.Milliseconds())
}

func applyKindFilter(results []mem.MemoryL2RecallResult, kinds []mem.EpisodeKind) []mem.MemoryL2RecallResult {
	if len(kinds) == 0 {
		return results
	}
	allowed := make(map[mem.EpisodeKind]bool, len(kinds))
	for _, k := range kinds {
		allowed[k] = true
	}
	out := results[:0]
	for _, r := range results {
		if allowed[r.Episode.Kind] {
			out = append(out, r)
		}
	}
	return out
}

// fuseRecallScores 将 BM25（已翻转为「越高越好」）归一化到 [0,1] 并计算 §5.3 加权最终排名。第一阶段仅 BM25 信号时融合分主要由其决定；重要性项避免高价值情节被极新低分命中埋没。
func fuseRecallScores(results []mem.MemoryL2RecallResult) []mem.MemoryL2RecallResult {
	if len(results) == 0 {
		return results
	}
	max := results[0].BM25Score
	for _, r := range results {
		if r.BM25Score > max {
			max = r.BM25Score
		}
	}
	if max <= 0 {
		max = 1
	}
	for i := range results {
		bm := results[i].BM25Score / max
		final := 0.7*bm + 0.3*results[i].Episode.Importance
		results[i].FinalRank = final
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].FinalRank > results[j].FinalRank })
	return results
}

// renderRecallMarkdown 将召回结果格式化为紧凑列表，降低提示开销。仅渲染摘要字段（§9：「Recall 只返回摘要」）。
func renderRecallMarkdown(results []mem.MemoryL2RecallResult) string {
	var b strings.Builder
	b.WriteString("## Episodic Memory (recall)\n")
	for _, r := range results {
		ep := r.Episode
		title := ep.Title
		if title == "" {
			title = ep.Goal
		}
		summary := ep.OutcomeSummary
		if summary == "" {
			summary = ep.ResultPreview
		}
		fmt.Fprintf(&b, "- **%s** (%s, importance=%.2f): %s\n",
			title, ep.Outcome, ep.Importance, previewText(summary, 160))
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildIndexText 串接情节的可搜索表面。标题重复一次以在 BM25 中加大权重（第一阶段 FTS5 不支持按字段加权）。
func buildIndexText(ep mem.MemoryEpisode) string {
	var parts []string
	if ep.Title != "" {
		parts = append(parts, ep.Title, ep.Title)
	}
	if ep.Goal != "" {
		parts = append(parts, ep.Goal)
	}
	if ep.OutcomeSummary != "" {
		parts = append(parts, ep.OutcomeSummary)
	}
	if ep.ResultPreview != "" {
		parts = append(parts, ep.ResultPreview)
	}
	if ep.FailureReason != "" {
		parts = append(parts, ep.FailureReason)
	}
	if ep.KeyDecisionsJSON != "" && ep.KeyDecisionsJSON != "[]" {
		parts = append(parts, ep.KeyDecisionsJSON)
	}
	return strings.Join(parts, "\n")
}
