package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
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
}

type MemoryAdminUsecase struct {
	admin          SessionAdminStore
	vec            *MemoryUsecase
	indexSync      MemoryFactIndexSyncer
	factWriter     L3FactWriter
	pathBExtractor *PathBExtractor
	msgLister      RecentMessageLister
	lg             loggateway.Logger
}

func NewMemoryAdminUsecase(admin SessionAdminStore, vec *MemoryUsecase, indexSync MemoryFactIndexSyncer, factWriter L3FactWriter, lg loggateway.Logger) *MemoryAdminUsecase {
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

func (uc *MemoryAdminUsecase) Vector() *MemoryUsecase { return uc.vec }

func (uc *MemoryAdminUsecase) requireAdmin() error {
	if uc == nil || uc.admin == nil {
		return kerrors.InternalServer("MEMORY", "session admin store not wired")
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

func (uc *MemoryAdminUsecase) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, 0, 0, err
	}
	return uc.admin.ListFactRows(ctx, scopeType, scopeID, kind, status, keyword, limit, offset)
}

func (uc *MemoryAdminUsecase) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	return uc.admin.ListEntityRows(ctx, scopeType, scopeID, workspaceID, userID, entityType, status, keyword, limit, offset)
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

func (uc *MemoryAdminUsecase) EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.EvolutionMetricsJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
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
	if err := syncer.SyncFactIndexFromRow(ctx, raw); err != nil && !errors.Is(err, ErrMemoryUnavailable) {
		uc.lg.Warn("syncFactIndexBestEffort failed", loggateway.StepID("memory.l4_fail"), loggateway.Err(err))
	}
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
		return kerrors.InternalServer("MEMORY", "fact writer not wired")
	}
	return uc.factWriter.DeleteFactRow(ctx, factID)
}

func (uc *MemoryAdminUsecase) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
	if uc == nil || uc.factWriter == nil {
		return 0, kerrors.InternalServer("MEMORY", "fact writer not wired")
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
	// Archive the task and create an L2 episode (best-effort, non-blocking).
	uc.archiveAndCreateEpisode(ctx, sessionID, taskID, raw)
	return raw, nil
}

// archiveAndCreateEpisode archives the L1 task and creates an L2 episode from the snapshot.
func (uc *MemoryAdminUsecase) archiveAndCreateEpisode(ctx context.Context, sessionID, taskID string, endTaskRaw []byte) {
	snapshot, err := uc.admin.ArchiveL1Task(ctx, sessionID, taskID)
	if err != nil {
		uc.lg.Warn("L1 archive failed after EndL1Task",
			loggateway.StepID("memory.l1_archive_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(err))
		return
	}

	// Extract structured data from snapshot (Path A: zero-cost)
	structured := ExtractStructuredEpisode(snapshot)

	m, _ := jsonutil.ParseMap(endTaskRaw)
	agentID := jsonutil.IfaceStr(m, "agent_id")

	decisionsJSON, _ := json.Marshal(structured.KeyDecisions)
	artifactsJSON, _ := json.Marshal(structured.KeyArtifacts)

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
		uc.lg.Warn("L1 archive episode insert failed",
			loggateway.StepID("memory.l1_archive_episode_fail"),
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
			uc.runPathBExtraction(ctx, sessionID, agentID, taskID, structured.Title, snapshot, score)
		}
	}
}

// runPathBExtraction performs the Path B LLM-enhanced extraction as a best-effort enhancement.
// If extraction fails, the Path A (zero-cost) episode is still used.
// pathATitle is the title used by the Path A episode; Path B reuses it so the
// ON CONFLICT(session_id, title, agent_id) clause updates the existing row instead
// of inserting a duplicate.
func (uc *MemoryAdminUsecase) runPathBExtraction(ctx context.Context, sessionID, agentID, taskID, pathATitle string, snapshot []byte, score float64) {
	// Build ConsolidateInput from session messages (not L1 snapshot fields).
	var input ConsolidateInput
	input.SessionID = sessionID
	input.AgentID = agentID
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
		input = buildConsolidateInputFromSnapshot(snapshot, sessionID, agentID)
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
	// Reuse pathATitle so ON CONFLICT(session_id, title, agent_id) matches the
	// Path A row and updates it in-place instead of creating a duplicate.
	decisionsJSON, _ := json.Marshal(enhancedResult.Episode.KeyDecisions)
	artifactsJSON, _ := json.Marshal(enhancedResult.Episode.KeyArtifacts)
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
func buildConsolidateInputFromSnapshot(snapshot []byte, sessionID, agentID string) ConsolidateInput {
	input := ConsolidateInput{
		SessionID: sessionID,
		AgentID:   agentID,
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
		switch n := v.(type) {
		case float64:
			signals.ToolCallCount = int(n)
		case int:
			signals.ToolCallCount = n
		}
	}
	// duration_ms
	if v, ok := m["duration_ms"]; ok {
		switch n := v.(type) {
		case float64:
			signals.DurationMs = int(n)
		case int:
			signals.DurationMs = n
		}
	}
	// user_mark
	signals.UserMark = jsonutil.IfaceStr(m, "user_mark")
	return signals
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

func (uc *MemoryAdminUsecase) ListIdleL1Tasks(ctx context.Context, cutoffRFC3339 string) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListIdleL1Tasks(ctx, cutoffRFC3339)
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

func (uc *MemoryAdminUsecase) ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	return uc.admin.ListConflictingFacts(ctx, scopeType, scopeID, limit, offset)
}

// DetectFactConflicts checks if a new fact statement conflicts with existing facts in the same scope.
// It uses clause-level negation matching: extracts core noun phrases from both statements
// and checks if one negates the other. Before conflict detection, it skips facts with
// identical fingerprints (exact dedup handled by UpsertFactRow).
func (uc *MemoryAdminUsecase) DetectFactConflicts(ctx context.Context, scopeType, scopeID, newStatement string) error {
	if uc.admin == nil {
		return nil
	}
	newNorm := NormalizeForDedup(newStatement)
	if newNorm == "" {
		return nil
	}
	newFingerprint := FactFingerprint(newStatement, scopeType, scopeID)
	rows, _, _, _, err := uc.admin.ListFactRows(ctx, scopeType, scopeID, "", "", "", conflictDetectMaxRows, 0)
	if err != nil || len(rows) == 0 {
		return nil
	}
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
			if _, err := uc.admin.IncrementConflictCount(ctx, id); err != nil {
				uc.lg.Warn("IncrementConflictCount failed (best-effort)",
					loggateway.StepID("memory.l3_conflict_increment_fail"),
					loggateway.Str("fact_id", id),
					loggateway.Err(err))
			}
		}
	}
	return nil
}

// isNegationConflict checks if two normalized statements form a negation conflict.
// It uses clause-level matching: for each negation prefix, it checks if removing
// the prefix from one statement yields a substring that appears in the other.
func isNegationConflict(a, b string, negationPrefixes []string) bool {
	aLower := " " + a + " "
	bLower := " " + b + " "
	for _, neg := range negationPrefixes {
		negL := strings.ToLower(neg)
		// Check if a = neg + X and b contains X (or vice versa).
		if stripped, ok := stripPrefix(aLower, negL); ok {
			if strings.Contains(bLower, stripped) || strings.Contains(stripped, bLower) {
				return true
			}
		}
		if stripped, ok := stripPrefix(bLower, negL); ok {
			if strings.Contains(aLower, stripped) || strings.Contains(stripped, aLower) {
				return true
			}
		}
	}
	return false
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
