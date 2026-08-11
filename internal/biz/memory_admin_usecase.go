package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// conflictDetectMaxRows is the maximum number of facts scanned for conflict detection.
// This limits the cost of per-upsert conflict scanning while covering the common case
// where an agent has fewer than 100 facts in a single scope.
const conflictDetectMaxRows = 100

// negationPatterns lists prefixes that signal negation in both English and Chinese.
// Used by DetectFactConflicts for clause-level negation matching.
var negationPatterns = []string{
	"not ", "don't ", "doesn't ", "didn't ", "never ", "no longer ", "won't ", "can't ", "isn't ", "aren't ",
	"bans ", "prohibits ", "avoids ", "dislikes ", "forbids ", "disables ", "rejects ",
	"不喜欢", "不需要", "不想", "不再", "没有", "禁止", "不用", "别", "切勿", "避免", "拒绝",
	"不", "不会", "不能", "不要", "不该", "不敢", "不肯", "不宜",
}

// safeMarshalJSON marshals v to JSON. On error it logs a warning and
// returns "null" so callers never need to handle the error for
// non-critical serialization (e.g. metadata, key_decisions).
func safeMarshalJSON(v any, lg loggateway.Logger) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		lg.Warn("json.Marshal failed (using null fallback)",
			loggateway.StepID("memory.json_marshal_fail"),
			loggateway.Err(err))
		return []byte("null")
	}
	return b
}

type MemoryAdminUsecase struct {
	admin          MemoryAdminDeps
	vec            *MemoryUsecase
	indexSync      MemoryFactIndexSyncer
	factWriter     L3FactWriter
	pathBExtractor *PathBExtractor
	msgLister      RecentMessageLister
	l2AdminReader  L2EpisodeAdminReader
	l4RelReader    L4RelationAdminReader
	lg             loggateway.Logger
}

func NewMemoryAdminUsecase(admin MemoryAdminDeps, vec *MemoryUsecase, indexSync MemoryFactIndexSyncer, factWriter L3FactWriter, lg loggateway.Logger) *MemoryAdminUsecase {
	if admin == nil && vec == nil {
		return nil
	}
	return &MemoryAdminUsecase{admin: admin, vec: vec, indexSync: indexSync, factWriter: factWriter, lg: lg}
}

// SetPathBExtractor injects the Path B enhanced extractor and message lister after construction.
func (uc *MemoryAdminUsecase) SetPathBExtractor(pe *PathBExtractor, msgLister RecentMessageLister) {
	if uc != nil {
		uc.pathBExtractor = pe
		uc.msgLister = msgLister
	}
}

// SetMemoryCenterReaders injects the L2/L4 admin readers used by the memory-center
// layer overview and unified graph aggregation. Kept out of MemoryAdminDeps (DB-N3).
func (uc *MemoryAdminUsecase) SetMemoryCenterReaders(l2 L2EpisodeAdminReader, l4 L4RelationAdminReader) {
	if uc != nil {
		uc.l2AdminReader = l2
		uc.l4RelReader = l4
	}
}

func (uc *MemoryAdminUsecase) Vector() *MemoryUsecase { return uc.vec }

func (uc *MemoryAdminUsecase) requireAdmin() error {
	if uc == nil || uc.admin == nil {
		return apierror.Internal("MEMORY", "session admin store not wired")
	}
	return nil
}

func (uc *MemoryAdminUsecase) ListPIIFlaggedFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	return uc.admin.ListPIIFlaggedFacts(ctx, scopeType, scopeID, limit, offset)
}

func (uc *MemoryAdminUsecase) ApprovePIIFact(ctx context.Context, factID string) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.ApprovePIIFact(ctx, factID)
}

func (uc *MemoryAdminUsecase) RejectPIIFact(ctx context.Context, factID string) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.RejectPIIFact(ctx, factID)
}

func (uc *MemoryAdminUsecase) GetFactByID(ctx context.Context, factID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	rows, err := uc.admin.GetFactRowsByIDs(ctx, []string{factID})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, apierror.NotFound(apierror.DomainMemory, "fact not found")
	}
	return rows[0], nil
}

// ReviewFact applies a single-fact user review action (confirm/reject/archive/
// dispute/deprecate/refine) via a column-targeted UPDATE — see L3FactReviewStore.
func (uc *MemoryAdminUsecase) ReviewFact(ctx context.Context, in FactReview) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ReviewFactRow(ctx, in)
}

func (uc *MemoryAdminUsecase) ListL0SnapshotRows(ctx context.Context, sessionID, agentID string, limit int32) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListL0SnapshotRows(ctx, sessionID, agentID, limit)
}

func (uc *MemoryAdminUsecase) GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.GetL0SnapshotRow(ctx, sessionID, id)
}

func (uc *MemoryAdminUsecase) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListL1TaskRows(ctx, sessionID, agentID, status, includeEnded)
}

func (uc *MemoryAdminUsecase) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool, requestingAgentID ...string) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListL1FieldRows(ctx, taskID, includeInternal, requestingAgentID...)
}

// ListFactRowsParams encapsulates parameters for ListFactRows.
// Introduced to keep the method signature under the 5-parameter limit (S4/S5 fix).
type ListFactRowsParams struct {
	ScopeType string
	ScopeID   string
	Kind      string
	Status    string
	Keyword   string
	// AgentID filters by the originating agent (memory_facts.agent_id) across
	// all scopes — the memory-center "this agent's facts" view. Independent of
	// the ScopeType/ScopeID namespace filter (AND-combined).
	AgentID string
	Limit   int32
	Offset  int32
}

func (uc *MemoryAdminUsecase) ListFactRows(ctx context.Context, p ListFactRowsParams) ([][]byte, int32, int32, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, 0, 0, err
	}
	return uc.admin.ListFactRows(ctx, p.ScopeType, p.ScopeID, p.Kind, p.Status, p.Keyword, p.AgentID, p.Limit, p.Offset)
}

// ListEntityRowsParams encapsulates parameters for ListEntityRows.
// Introduced to keep the method signature under the 5-parameter limit (S4/S5 fix).
type ListEntityRowsParams struct {
	ScopeType   string
	ScopeID     string
	WorkspaceID string
	UserID      string
	EntityType  string
	Status      string
	Keyword     string
	Limit       int32
	Offset      int32
}

func (uc *MemoryAdminUsecase) ListEntityRows(ctx context.Context, p ListEntityRowsParams) ([][]byte, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	return uc.admin.ListEntityRows(ctx, p.ScopeType, p.ScopeID, p.WorkspaceID, p.UserID, p.EntityType, p.Status, p.Keyword, p.Limit, p.Offset)
}

func (uc *MemoryAdminUsecase) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.NeighborhoodJSON(ctx, centerID, hops, maxNodes, queryAtRFC3339)
}

func (uc *MemoryAdminUsecase) AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.AgentIdentityJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.AgentStrategyJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.EvolutionProposalRows(ctx, agentID, status, limit)
}

func (uc *MemoryAdminUsecase) EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.EvolutionEventRows(ctx, agentID, limit)
}

func (uc *MemoryAdminUsecase) EvolutionMetricsJSON(ctx context.Context, agentID string, timeRange string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.EvolutionMetricsJSON(ctx, agentID, timeRange)
}

func (uc *MemoryAdminUsecase) UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error) {
	if uc == nil || uc.factWriter == nil {
		return nil, apierror.Internal("MEMORY", "fact writer not wired")
	}
	raw, err := uc.factWriter.UpsertFactRow(ctx, in)
	if err != nil {
		return nil, err
	}
	uc.syncFactIndexBestEffort(ctx, raw)
	// Best-effort conflict detection
	if err := uc.DetectFactConflicts(ctx, in.ScopeType, in.ScopeID, in.Statement); err != nil {
		uc.lg.Warn("DetectFactConflicts failed (best-effort)",
			loggateway.StepID("memory.l3_conflict_detect_fail"),
			loggateway.Err(err))
	}
	return raw, nil
}

func (uc *MemoryAdminUsecase) syncFactIndexBestEffort(ctx context.Context, raw []byte) {
	if uc == nil || len(raw) == 0 {
		return
	}
	syncer := uc.indexSync
	if syncer == nil {
		syncer = uc.vec
	}
	if syncer == nil {
		return
	}
	// Async: L4 fact index sync must not block the LLM main flow. Embed
	// calls can take up to 5s (or time out), which would delay fact
	// insertion and L3 recall. Use a detached context so the sync
	// survives request cancellation.
	safego.Go(ctx, "memory.l4_sync", func() {
		if err := syncer.SyncFactIndexFromRow(context.WithoutCancel(ctx), raw); err != nil && !errors.Is(err, ErrMemoryUnavailable) {
			uc.lg.Warn("syncFactIndexBestEffort failed", loggateway.StepID("memory.l4_fail"), loggateway.Err(err))
		}
	})
}

func (uc *MemoryAdminUsecase) InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.InsertEvolutionEventRow(ctx, in)
}

func (uc *MemoryAdminUsecase) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.DeleteSessionEventEntities(ctx, sessionID)
}

func (uc *MemoryAdminUsecase) DeleteFactRow(ctx context.Context, factID string) error {
	if uc == nil || uc.factWriter == nil {
		return apierror.Internal("MEMORY", "fact writer not wired")
	}
	return uc.factWriter.DeleteFactRow(ctx, factID)
}

func (uc *MemoryAdminUsecase) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
	if uc == nil || uc.factWriter == nil {
		return 0, apierror.Internal("MEMORY", "fact writer not wired")
	}
	return uc.factWriter.DeleteFactRowsByIDs(ctx, factIDs)
}

// --- L1 Writer Methods ---

func (uc *MemoryAdminUsecase) StartL1Task(ctx context.Context, in L1TaskInsert) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.StartL1Task(ctx, in)
}

func (uc *MemoryAdminUsecase) EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	raw, err := uc.admin.EndL1Task(ctx, sessionID, taskID, status)
	if err != nil {
		return nil, err
	}
	// Archive the task and create an L2 episode asynchronously (best-effort).
	// The hook contains a potential Path B LLM call and must not block the
	// EndL1Task caller (e.g. the working_memory_complete tool executing
	// in-turn). On failure the task stays ended-but-unarchived and is retried
	// by the L1 archive worker's retry branch (P1-2).
	safego.Go(ctx, "memory.l1_archive_hook", func() {
		uc.archiveAndCreateEpisode(context.WithoutCancel(ctx), sessionID, taskID, raw)
	})
	return raw, nil
}

// archiveAndCreateEpisode atomically archives the L1 task and creates an L2
// episode from the snapshot within a single database transaction. If the
// episode insert fails, the L1 archive is rolled back automatically by the
// transaction, so no manual rollback is needed.
func (uc *MemoryAdminUsecase) archiveAndCreateEpisode(ctx context.Context, sessionID, taskID string, endTaskRaw []byte) {
	m, _ := jsonutil.ParseMap(endTaskRaw)
	agentID := jsonutil.IfaceStr(m, "agent_id")
	// Extract userID from task metadata if available.
	// TODO(debt): Add SessionInfoGetter dependency to reliably resolve userID
	// from sessionID, since the L1 task table does not have a user_id column.
	userID := extractUserIDFromTaskMeta(m)

	// Atomically archive the L1 task and create a bare L2 episode.
	// The data layer builds the full snapshot inside the transaction and
	// returns it for Path A extraction. If the episode insert fails, the
	// archive update is rolled back automatically.
	snapshot, err := uc.admin.ArchiveAndCreateEpisodeTx(ctx, sessionID, taskID, L1ArchiveEpisodeInsert{
		SessionID: sessionID,
		AgentID:   agentID,
		TaskID:    taskID,
	})
	if err != nil {
		uc.lg.Warn("L1 archive + episode atomic operation failed",
			loggateway.StepID("memory.l1_archive_episode_atomic_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(err))
		return
	}

	// Extract structured data from snapshot (Path A: zero-cost).
	structured := ExtractStructuredEpisode(snapshot)

	decisionsJSON := safeMarshalJSON(structured.KeyDecisions, uc.lg)
	artifactsJSON := safeMarshalJSON(structured.KeyArtifacts, uc.lg)

	// Enrich the episode with Path A structured fields (best-effort upsert).
	// The atomic tx already created a bare episode; ON CONFLICT updates it.
	if err := uc.admin.InsertL1ArchiveEpisode(ctx, L1ArchiveEpisodeInsert{
		SessionID:        sessionID,
		AgentID:          agentID,
		TaskID:           taskID,
		TaskTitle:        structured.Title,
		Status:           structured.OutcomeSummary,
		L1SnapshotJSON:   string(snapshot),
		Goal:             structured.Goal,
		Outcome:          structured.Outcome,
		OutcomeSummary:   structured.OutcomeSummary,
		KeyDecisionsJSON: string(decisionsJSON),
		KeyArtifactsJSON: string(artifactsJSON),
		EpisodeKind:      structured.EpisodeKind,
		Importance:       structured.Importance,
		Confidence:       structured.Confidence,
	}); err != nil {
		uc.lg.Warn("L1 archive episode Path A enrichment failed (best-effort)",
			loggateway.StepID("memory.l1_archive_episode_enrich_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(err))
	}

	// Path B: Check if LLM-enhanced extraction should be triggered.
	signals := extractEpisodeSignals(endTaskRaw, structured.Importance)
	if ShouldTriggerPathB(signals) {
		score := EpisodeScore(signals)
		uc.lg.Info("Path B LLM-enhanced episode eligible",
			loggateway.StepID("memory.path_b_eligible"),
			loggateway.Str("task_id", taskID),
			loggateway.Float64("importance", signals.Importance),
			loggateway.Float64("critic_score", signals.CriticScore),
			loggateway.Int("tool_call_count", signals.ToolCallCount),
			loggateway.Int("duration_ms", signals.DurationMs),
			loggateway.Str("user_mark", signals.UserMark),
			loggateway.Float64("episode_score", score))
		if uc.pathBExtractor != nil {
			uc.runPathBExtraction(ctx, sessionID, agentID, userID, taskID, structured.Title, snapshot, score)
		}
	}
}

// runPathBExtraction performs the Path B LLM-enhanced extraction as a best-effort enhancement.
// If extraction fails, the Path A (zero-cost) episode is still used.
// pathATitle is the title used by the Path A episode; Path B reuses it so the
// ON CONFLICT(session_id, l1_task_id) clause updates the existing row instead
// of inserting a duplicate.
func (uc *MemoryAdminUsecase) runPathBExtraction(ctx context.Context, sessionID, agentID, userID, taskID, pathATitle string, snapshot []byte, score float64) {
	// Build ConsolidateInput from session messages (not L1 snapshot fields).
	var input ConsolidateInput
	input.SessionID = sessionID
	input.AgentID = agentID
	input.UserID = userID
	if uc.msgLister != nil {
		msgs, err := uc.msgLister.ListRecentMessages(ctx, sessionID, 50)
		if err != nil {
			uc.lg.Warn("Path B: failed to list recent messages, falling back to snapshot",
				loggateway.StepID("memory.path_b_msg_list_fail"),
				loggateway.Str("task_id", taskID),
				loggateway.Err(err))
		} else if len(msgs) > 0 {
			input.Messages = msgs
		}
	}
	if len(input.Messages) == 0 {
		// Fallback: build from L1 snapshot if no session messages available.
		input = buildConsolidateInputFromSnapshot(snapshot, sessionID, agentID, userID)
	}

	enhancedResult, err := uc.pathBExtractor.Extract(ctx, input)
	if err != nil {
		uc.lg.Warn("Path B enhanced extraction failed (best-effort, Path A episode preserved)",
			loggateway.StepID("memory.path_b_extract_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(err))
		return
	}
	if enhancedResult == nil {
		uc.lg.Info("Path B enhanced extraction returned nil (no data worth extracting)",
			loggateway.StepID("memory.path_b_extract_nil"),
			loggateway.Str("task_id", taskID))
		return
	}

	uc.lg.Info("Path B enhanced extraction succeeded",
		loggateway.StepID("memory.path_b_extract_ok"),
		loggateway.Str("task_id", taskID),
		loggateway.Float64("importance", enhancedResult.Episode.Importance),
		loggateway.Int("entities", len(enhancedResult.Entities)),
		loggateway.Int("relations", len(enhancedResult.Relations)))

	// Update the existing Path A episode with enhanced data.
	// Reuse pathATitle so ON CONFLICT(session_id, l1_task_id) matches the
	// Path A row and updates it in-place instead of creating a duplicate.
	decisionsJSON := safeMarshalJSON(enhancedResult.Episode.KeyDecisions, uc.lg)
	artifactsJSON := safeMarshalJSON(enhancedResult.Episode.KeyArtifacts, uc.lg)
	if err := uc.admin.InsertL1ArchiveEpisode(ctx, L1ArchiveEpisodeInsert{
		SessionID:        sessionID,
		AgentID:          agentID,
		TaskID:           taskID,
		TaskTitle:        pathATitle,
		Status:           enhancedResult.Episode.OutcomeSummary,
		L1SnapshotJSON:   string(snapshot),
		Goal:             enhancedResult.Episode.Goal,
		Outcome:          enhancedResult.Episode.Outcome,
		OutcomeSummary:   enhancedResult.Episode.OutcomeSummary,
		KeyDecisionsJSON: string(decisionsJSON),
		KeyArtifactsJSON: string(artifactsJSON),
		EpisodeKind:      "l1_archive_path_b",
		Importance:       enhancedResult.Episode.Importance,
		Confidence:       enhancedResult.Episode.Confidence,
	}); err != nil {
		uc.lg.Warn("Path B episode update failed (best-effort)",
			loggateway.StepID("memory.path_b_episode_update_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(err))
	}

	// Write extracted entities and relations to L4 graph.
	uc.pathBExtractor.WriteEntities(ctx, agentID, input.UserID, enhancedResult)
}

// buildConsolidateInputFromSnapshot creates a ConsolidateInput from an L1 snapshot.
func buildConsolidateInputFromSnapshot(snapshot []byte, sessionID, agentID, userID string) ConsolidateInput {
	input := ConsolidateInput{
		SessionID: sessionID,
		AgentID:   agentID,
		UserID:    userID,
	}
	var snap l1Snapshot
	if json.Unmarshal(snapshot, &snap) == nil {
		for _, f := range snap.Fields {
			content := strVal(f, "value_text")
			if content == "" {
				continue
			}
			role := "user"
			if strings.EqualFold(strVal(f, "source"), "assistant") {
				role = "assistant"
			}
			input.Messages = append(input.Messages, ConsolidateMessage{
				Role:      role,
				Content:   content,
				MessageID: strVal(f, "id"),
			})
		}
		// Also include task-level fields as context.
		if goal := strVal(snap.Task, "task_goal"); goal != "" {
			input.Messages = append([]ConsolidateMessage{{Role: "user", Content: goal}}, input.Messages...)
		}
		if lastMsg := strVal(snap.Task, "last_assistant_message"); lastMsg != "" {
			input.Messages = append(input.Messages, ConsolidateMessage{Role: "assistant", Content: lastMsg})
		}
	}
	return input
}

// extractUserIDFromTaskMeta attempts to extract user_id from the L1 task row's
// metadata_json. Returns empty string if not found (the L1 task table does not
// have a dedicated user_id column, so this is a best-effort extraction).
func extractUserIDFromTaskMeta(taskMap map[string]any) string {
	if taskMap == nil {
		return ""
	}
	metaStr, _ := taskMap["metadata_json"].(string)
	if metaStr == "" {
		return ""
	}
	var meta map[string]any
	if json.Unmarshal([]byte(metaStr), &meta) != nil {
		return ""
	}
	uid, _ := meta["user_id"].(string)
	return uid
}

// extractEpisodeSignals builds EpisodeSignals from the EndL1Task raw response and importance.
func extractEpisodeSignals(endTaskRaw []byte, importance float64) EpisodeSignals {
	m, _ := jsonutil.ParseMap(endTaskRaw)
	signals := EpisodeSignals{
		Importance: importance,
	}
	// critic_score: default -1 (missing)
	if v, ok := m["critic_score"]; ok {
		if f, ok := v.(float64); ok {
			signals.CriticScore = f
		}
	} else {
		signals.CriticScore = -1
	}
	// tool_call_count
	if v, ok := m["tool_call_count"]; ok {
		signals.ToolCallCount = ifaceToInt(v)
	}
	// duration_ms
	if v, ok := m["duration_ms"]; ok {
		signals.DurationMs = ifaceToInt(v)
	}
	// user_mark
	signals.UserMark = jsonutil.IfaceStr(m, "user_mark")
	return signals
}

// ifaceToInt converts an interface{} from JSON unmarshaling to int.
// Handles float64 (standard JSON), int, int64, and json.Number.
func ifaceToInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
		if f, err := n.Float64(); err == nil {
			return int(f)
		}
	}
	return 0
}

func (uc *MemoryAdminUsecase) GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.GetL1TaskRow(ctx, sessionID, id)
}

func (uc *MemoryAdminUsecase) UpsertL1Field(ctx context.Context, in L1FieldInsert) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.UpsertL1Field(ctx, in)
}

func (uc *MemoryAdminUsecase) DeleteL1Field(ctx context.Context, taskID, fieldPath string) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.DeleteL1Field(ctx, taskID, fieldPath)
}

func (uc *MemoryAdminUsecase) GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.GetL1FieldRow(ctx, taskID, fieldPath)
}

func (uc *MemoryAdminUsecase) PatchL1Fields(ctx context.Context, fields []L1FieldInsert) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.PatchL1Fields(ctx, fields)
}

func (uc *MemoryAdminUsecase) ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ArchiveL1Task(ctx, sessionID, taskID)
}

func (uc *MemoryAdminUsecase) ListIdleL1Tasks(ctx context.Context, idleCutoffRFC3339, retryCutoffRFC3339 string) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListIdleL1Tasks(ctx, idleCutoffRFC3339, retryCutoffRFC3339)
}

func (uc *MemoryAdminUsecase) InsertL1ArchiveEpisode(ctx context.Context, in L1ArchiveEpisodeInsert) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.InsertL1ArchiveEpisode(ctx, in)
}

func (uc *MemoryAdminUsecase) IncrementConflictCount(ctx context.Context, factID string) (int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return 0, err
	}
	return uc.admin.IncrementConflictCount(ctx, factID)
}

func (uc *MemoryAdminUsecase) ListConflictingFacts(ctx context.Context, scopeType, scopeID, agentID string, limit, offset int32) ([][]byte, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	return uc.admin.ListConflictingFacts(ctx, scopeType, scopeID, agentID, limit, offset)
}

// DetectFactConflicts checks if a new fact statement conflicts with existing facts in the same scope.
// It uses clause-level negation matching: extracts core noun phrases from both statements
// and checks if one negates the other. Before conflict detection, it skips facts with
// identical fingerprints (exact dedup handled by UpsertFactRow).
// Conflicting fact IDs are collected first, then conflict_count is incremented in a
// single batch query to avoid N+1 writes.
func (uc *MemoryAdminUsecase) DetectFactConflicts(ctx context.Context, scopeType, scopeID, newStatement string) error {
	if uc.admin == nil {
		return nil
	}
	newNorm := NormalizeForDedup(newStatement)
	if newNorm == "" {
		return nil
	}
	newFingerprint := FactFingerprint(newStatement, scopeType, scopeID)
	rows, _, _, _, err := uc.admin.ListFactRows(ctx, scopeType, scopeID, "", "", "", "", conflictDetectMaxRows, 0)
	if err != nil || len(rows) == 0 {
		return nil
	}
	if len(rows) >= conflictDetectMaxRows {
		uc.lg.Warn("conflict detection hit row limit — some conflicts may be undetected",
			loggateway.StepID("memory.conflict_detect_limit"),
			loggateway.Str("scope_type", scopeType),
			loggateway.Str("scope_id", scopeID),
			loggateway.Int("limit", conflictDetectMaxRows))
	}
	var conflictIDs []string
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		existing := jsonutil.IfaceStr(m, "statement")
		id := jsonutil.IfaceStr(m, "id")
		if existing == "" || id == "" {
			continue
		}
		// Skip if fingerprint matches (exact dedup — no conflict, just duplicate).
		existingFingerprint := FactFingerprint(existing, scopeType, scopeID)
		if existingFingerprint == newFingerprint {
			continue
		}
		existingNorm := NormalizeForDedup(existing)
		if existingNorm == "" {
			continue
		}
		if isNegationConflict(newNorm, existingNorm, negationPatterns) {
			conflictIDs = append(conflictIDs, id)
		}
	}
	if len(conflictIDs) > 0 {
		if err := uc.admin.BatchIncrementConflictCounts(ctx, conflictIDs); err != nil {
			uc.lg.Warn("BatchIncrementConflictCounts failed (best-effort)",
				loggateway.StepID("memory.l3_conflict_batch_increment_fail"),
				loggateway.Int("count", len(conflictIDs)),
				loggateway.Err(err))
		}
	}
	return nil
}

// isNegationConflict checks if two normalized statements form a negation conflict.
// It uses clause-level matching: for each negation prefix, it checks if removing
// the prefix from one statement yields a core clause that closely matches the
// other statement. To avoid false positives from loose substring containment,
// the core clause must either:
//   - exactly equal the other statement, or
//   - constitute a significant portion of the other statement (≥75% overlap),
//     ensuring the core clause is the semantic subject of both statements.
func isNegationConflict(a, b string, negationPrefixes []string) bool {
	aLower := " " + strings.ToLower(a) + " "
	bLower := " " + strings.ToLower(b) + " "
	for _, neg := range negationPrefixes {
		negL := strings.ToLower(neg)
		// Check if a = neg + X and b matches X (or vice versa).
		if stripped, ok := stripPrefix(aLower, negL); ok {
			if negationCoreMatches(stripped, bLower) {
				return true
			}
		}
		if stripped, ok := stripPrefix(bLower, negL); ok {
			if negationCoreMatches(stripped, aLower) {
				return true
			}
		}
	}
	return false
}

// negationCoreMatches checks whether the stripped core clause (from a negated
// statement) semantically matches the other statement. It requires either an
// exact match or a significant word overlap (≥75%) to avoid false positives
// from loose substring containment (e.g. "not a" matching "banana").
func negationCoreMatches(core, other string) bool {
	core = strings.TrimSpace(core)
	other = strings.TrimSpace(other)
	if core == "" || other == "" {
		return false
	}
	// Exact match: "not like X" vs "like X"
	if core == other {
		return true
	}
	// Word-overlap match: count how many words from core appear in other.
	coreWords := strings.Fields(core)
	otherWords := strings.Fields(other)
	if len(coreWords) == 0 {
		return false
	}
	otherSet := make(map[string]struct{}, len(otherWords))
	for _, w := range otherWords {
		otherSet[strings.ToLower(w)] = struct{}{}
	}
	matched := 0
	for _, w := range coreWords {
		if _, ok := otherSet[strings.ToLower(w)]; ok {
			matched++
		}
	}
	overlapRatio := float64(matched) / float64(len(coreWords))
	return overlapRatio >= 0.75
}

// stripPrefix removes a negation prefix from the beginning of a statement (after
// leading whitespace). Returns the stripped core clause and true if the prefix was found.
func stripPrefix(s, prefix string) (string, bool) {
	trimmed := strings.TrimLeft(s, " ")
	if strings.HasPrefix(trimmed, prefix) {
		core := strings.TrimSpace(trimmed[len(prefix):])
		if core != "" {
			return core, true
		}
	}
	// Try matching after an auxiliary verb within the first 3 words.
	// E.g. "he does not like X" → check if word[2] matches prefix.
	words := strings.SplitN(trimmed, " ", 4)
	for i := 1; i < len(words)-1 && i < 3; i++ {
		if strings.EqualFold(words[i], strings.TrimSpace(prefix)) {
			core := strings.TrimSpace(strings.Join(words[i+1:], " "))
			if core != "" {
				return core, true
			}
		}
	}
	return "", false
}
