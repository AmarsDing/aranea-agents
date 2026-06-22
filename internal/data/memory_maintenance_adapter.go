package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type memoryConsolidationWriterAdapter struct {
	data *Data
	lg   loggateway.Logger
}

func NewMemoryConsolidationWriterAdapter(data *Data, lg loggateway.Logger) biz.MemoryConsolidationWriter {
	if data == nil {
		return nil
	}
	return &memoryConsolidationWriterAdapter{data: data, lg: lg}
}

func (a *memoryConsolidationWriterAdapter) UpsertFactsAndEpisodeBatch(ctx context.Context, facts []biz.MemoryFactWrite, ep *biz.EpisodeWrite) (*biz.ConsolidationResult, error) {
	var factRows [][]byte
	factsWritten := 0
	factsDeduped := 0
	now := time.Now().UTC().Format(time.RFC3339Nano)

	for _, f := range facts {
		id := newUUIDString()
		fp := biz.FactFingerprint(f.Statement, f.ScopeType, f.ScopeID)
		status := strings.TrimSpace(f.Status)
		if status == "" {
			status = "active"
		}
		tags := strings.TrimSpace(f.TagsJSON)
		if tags == "" {
			tags = "[]"
		}
		meta := strings.TrimSpace(f.MetadataJSON)
		if meta == "" {
			meta = "{}"
		}
		details := strings.TrimSpace(f.DetailsMarkdown)
		_, err := a.data.RWDB().WriteDB(ctx).ExecContext(ctx, a.data.Dialect().RenumberPlaceholders(`INSERT INTO memory_facts (
			id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
			statement, statement_normalized, fingerprint, details_markdown,
			fact_kind, tags_json,
			confidence, importance, use_count, hit_count,
			positive_feedback_count, negative_feedback_count, conflict_count,
			source_kind, source_episode_id, source_session_id, source_message_id, source_external,
			version, status, superseded_by,
			pii_flag, redacted_statement,
			quality_score, metadata_json, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(scope_type, scope_id, fingerprint) DO UPDATE SET
			statement = excluded.statement, details_markdown = excluded.details_markdown,
			confidence = excluded.confidence, importance = excluded.importance,
			fact_kind = excluded.fact_kind, tags_json = excluded.tags_json,
			source_kind = excluded.source_kind, source_session_id = excluded.source_session_id,
			source_message_id = excluded.source_message_id,
			version = version + 1, status = excluded.status,
			metadata_json = excluded.metadata_json, updated_at = excluded.updated_at`),
			id,
			strings.TrimSpace(f.ScopeType),
			strings.TrimSpace(f.ScopeID),
			"", // workspace_id
			strings.TrimSpace(f.UserID),
			"", // team_id
			strings.TrimSpace(f.AgentID),
			strings.TrimSpace(f.Statement),
			strings.ToLower(strings.TrimSpace(f.Statement)),
			fp, details,
			strings.TrimSpace(f.FactKind), tags,
			f.Confidence, f.Importance, 0, 0,
			0, 0, 0,
			strings.TrimSpace(f.SourceKind), strings.TrimSpace(f.SourceEpisodeID),
			strings.TrimSpace(f.SourceSessionID),
			strings.TrimSpace(f.SourceMessageID),
			"",
			1, status, "",
			0, "",
			0, meta, now, now,
		)
		if err != nil {
			a.lg.Warn("consolidation fact upsert failed", loggateway.StepID("memory.consolidation_fact_fail"), loggateway.Err(err))
			continue
		}
		// Read back by fingerprint (works for both new inserts and ON CONFLICT upserts).
		readRows, err := a.data.RWDB().ReadDB(ctx).QueryContext(ctx,
			a.data.Dialect().RenumberPlaceholders(sqlFactSelect+` WHERE scope_type = ? AND scope_id = ? AND fingerprint = ?`),
			strings.TrimSpace(f.ScopeType), strings.TrimSpace(f.ScopeID), fp)
		if err != nil {
			a.lg.Warn("consolidation fact read-back failed", loggateway.StepID("memory.consolidation_fact_readback_fail"), loggateway.Err(err))
		} else {
			if readRows.Next() {
				if b, err := scanFactRowJSON(readRows); err == nil {
					factRows = append(factRows, b)
					// Detect dedup: if the row's ID differs from our generated ID,
					// ON CONFLICT fired (existing row was updated, not newly inserted).
					var row map[string]any
					if json.Unmarshal(b, &row) == nil {
						existingID := strings.TrimSpace(fmt.Sprint(row["id"]))
						if existingID != id {
							factsDeduped++
						} else {
							factsWritten++
						}
					} else {
						factsWritten++
					}
				}
			}
			readRows.Close()
		}
	}

	var episodeRow []byte
	if ep != nil {
		epID := strings.TrimSpace(ep.ID)
		if epID == "" {
			epID = newUUIDString()
		}
		episodeKind := strings.TrimSpace(ep.EpisodeKind)
		if episodeKind == "" {
			episodeKind = "consolidation"
		}
		keyDecisions := strings.TrimSpace(ep.KeyDecisionsJSON)
		if keyDecisions == "" {
			keyDecisions = "[]"
		}
		keyArtifacts := strings.TrimSpace(ep.KeyArtifactsJSON)
		if keyArtifacts == "" {
			keyArtifacts = "[]"
		}
		confidence := ep.Confidence
		if confidence <= 0 {
			confidence = 0.6
		}
		_, err := a.data.RWDB().WriteDB(ctx).ExecContext(ctx, a.data.Dialect().RenumberPlaceholders(`INSERT INTO memory_episodes (
			id, session_id, agent_id, episode_kind, title,
			goal, outcome, outcome_summary,
			key_decisions_json, key_artifacts_json,
			importance, confidence,
			consolidation_status, consolidated_l3_count, metadata_json, ended_at, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(session_id, title, agent_id) DO UPDATE SET
			goal = excluded.goal, outcome = excluded.outcome,
			outcome_summary = excluded.outcome_summary,
			key_decisions_json = excluded.key_decisions_json,
			key_artifacts_json = excluded.key_artifacts_json,
			importance = excluded.importance, confidence = excluded.confidence,
			consolidation_status = excluded.consolidation_status,
			consolidated_l3_count = excluded.consolidated_l3_count`),
			epID,
			strings.TrimSpace(ep.SessionID),
			strings.TrimSpace(ep.AgentID),
			episodeKind,
			strings.TrimSpace(ep.Title),
			strings.TrimSpace(ep.Goal),
			strings.TrimSpace(ep.Outcome),
			strings.TrimSpace(ep.OutcomeSummary),
			keyDecisions,
			keyArtifacts,
			ep.Importance,
			confidence,
			strings.TrimSpace(ep.ConsolidationStatus),
			ep.ConsolidatedL3,
			strings.TrimSpace(ep.MetadataJSON),
			now, now,
		)
		if err != nil {
			a.lg.Warn("consolidation episode upsert failed", loggateway.StepID("memory.consolidation_episode_fail"), loggateway.Err(err))
		} else {
			readRows, err := a.data.RWDB().ReadDB(ctx).QueryContext(ctx, a.data.Dialect().RenumberPlaceholders(sqlEpisodeSelect+` WHERE session_id = ? AND agent_id = ? AND title = ?`),
				strings.TrimSpace(ep.SessionID), strings.TrimSpace(ep.AgentID), strings.TrimSpace(ep.Title))
			if err != nil {
				a.lg.Warn("consolidation episode read-back failed", loggateway.StepID("memory.consolidation_episode_readback_fail"), loggateway.Err(err))
			} else {
				if readRows.Next() {
					if b, err := scanEpisodeRowJSON(readRows); err == nil {
						episodeRow = b
					}
				}
				readRows.Close()
			}
		}
	}

	return &biz.ConsolidationResult{
		FactRows:     factRows,
		EpisodeRow:   episodeRow,
		FactsWritten: factsWritten,
		FactsDeduped: factsDeduped,
	}, nil
}

type memoryFactIndexMaintainerAdapter struct {
	data *Data
}

func NewMemoryFactIndexMaintainerAdapter(data *Data) biz.MemoryFactIndexMaintainer {
	if data == nil {
		return nil
	}
	return &memoryFactIndexMaintainerAdapter{data: data}
}

func (a *memoryFactIndexMaintainerAdapter) ListStaleIndexFacts(ctx context.Context, maxAttempts, batchSize int) ([][]byte, error) {
	if batchSize <= 0 {
		batchSize = 50
	}
	rows, err := a.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		a.data.Dialect().RenumberPlaceholders(sqlFactSelect+` WHERE embedding_status IN ('stale','failed') AND status = 'active' AND deleted_at = '' LIMIT ?`),
		batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (a *memoryFactIndexMaintainerAdapter) MarkFactIndexDisabled(ctx context.Context, factID string) error {
	_, err := a.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		a.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET embedding_status = 'disabled', updated_at = ? WHERE id = ?`),
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return err
}

type memoryEpisodeDecayerAdapter struct {
	data *Data
}

func NewMemoryEpisodeDecayerAdapter(data *Data) biz.MemoryEpisodeDecayer {
	if data == nil {
		return nil
	}
	return &memoryEpisodeDecayerAdapter{data: data}
}

func (a *memoryEpisodeDecayerAdapter) ApplyEpisodeImportanceDecay(ctx context.Context, agentID, cutoffRFC3339 string, factor float64) (int, error) {
	res, err := a.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		a.data.Dialect().RenumberPlaceholders(`UPDATE memory_episodes SET importance = importance * ?, updated_at = ? WHERE agent_id = ? AND ended_at != '' AND ended_at < ?`),
		factor, time.Now().UTC().Format(time.RFC3339Nano), agentID, cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (a *memoryEpisodeDecayerAdapter) PurgeEpisodesOlderThan(ctx context.Context, agentID, cutoffRFC3339 string) (int, error) {
	res, err := a.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		a.data.Dialect().RenumberPlaceholders(`DELETE FROM memory_episodes WHERE agent_id = ? AND ended_at != '' AND ended_at < ? AND consolidation_status = 'consolidated'`),
		agentID, cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (a *memoryEpisodeDecayerAdapter) ApplyAllEpisodeImportanceDecay(ctx context.Context, cutoffRFC3339 string, factor float64) (int, error) {
	res, err := a.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		a.data.Dialect().RenumberPlaceholders(`UPDATE memory_episodes SET importance = importance * ?, updated_at = ? WHERE ended_at != '' AND ended_at < ?`),
		factor, time.Now().UTC().Format(time.RFC3339Nano), cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

type memoryFactDecayerAdapter struct {
	data *Data
}

func NewMemoryFactDecayerAdapter(data *Data) biz.MemoryFactDecayer {
	if data == nil {
		return nil
	}
	return &memoryFactDecayerAdapter{data: data}
}

func (a *memoryFactDecayerAdapter) ApplyAgentFactImportanceDecay(ctx context.Context, agentID, cutoffRFC3339 string, factor float64) (int, error) {
	res, err := a.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		a.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET importance = importance * ?, updated_at = ? WHERE agent_id = ? AND status = 'active' AND deleted_at = '' AND updated_at < ?`),
		factor, time.Now().UTC().Format(time.RFC3339Nano), agentID, cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (a *memoryFactDecayerAdapter) ApplyAllFactImportanceDecay(ctx context.Context, cutoffRFC3339 string, factor float64) (int, error) {
	res, err := a.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		a.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET importance = importance * ?, updated_at = ? WHERE status = 'active' AND deleted_at = '' AND updated_at < ?`),
		factor, time.Now().UTC().Format(time.RFC3339Nano), cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

type memoryEpisodeBackfillReaderAdapter struct {
	data *Data
}

func NewMemoryEpisodeBackfillReaderAdapter(data *Data) biz.MemoryEpisodeBackfillReader {
	if data == nil {
		return nil
	}
	return &memoryEpisodeBackfillReaderAdapter{data: data}
}

func (a *memoryEpisodeBackfillReaderAdapter) ListEpisodesPendingEmbedding(ctx context.Context, limit int) ([]biz.EpisodeEmbedCandidate, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := a.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		a.data.Dialect().RenumberPlaceholders(`SELECT id, agent_id, title, outcome_summary FROM memory_episodes WHERE embedding_status IN ('pending','stale') AND consolidation_status = 'consolidated' LIMIT ?`),
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.EpisodeEmbedCandidate
	for rows.Next() {
		var c biz.EpisodeEmbedCandidate
		if err := rows.Scan(&c.ID, &c.AgentID, &c.Title, &c.Summary); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type memoryLegacyMigratorAdapter struct {
	data *Data
	lg   loggateway.Logger
}

func NewMemoryLegacyMigratorAdapter(data *Data, lg loggateway.Logger) biz.MemoryLegacyMigrator {
	if data == nil {
		return nil
	}
	return &memoryLegacyMigratorAdapter{data: data, lg: lg}
}

func (a *memoryLegacyMigratorAdapter) RunLegacyMigration(ctx context.Context) (int, bool, error) {
	return RunLegacyTRPCMemoryMigration(ctx, a.data, a.lg)
}

func (a *memoryLegacyMigratorAdapter) LegacyMigrationVersion() int {
	return MigrationLegacyTRPCMemoryFacts
}

// Compile-time interface checks.
var (
	_ biz.MemoryLegacyMigrator        = (*memoryLegacyMigratorAdapter)(nil)
	_ biz.MemoryConsolidationWriter   = (*memoryConsolidationWriterAdapter)(nil)
	_ biz.MemoryFactIndexMaintainer   = (*memoryFactIndexMaintainerAdapter)(nil)
	_ biz.MemoryEpisodeDecayer        = (*memoryEpisodeDecayerAdapter)(nil)
	_ biz.MemoryFactDecayer           = (*memoryFactDecayerAdapter)(nil)
	_ biz.MemoryEpisodeBackfillReader = (*memoryEpisodeBackfillReaderAdapter)(nil)
)
