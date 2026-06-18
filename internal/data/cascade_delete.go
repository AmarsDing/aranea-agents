package data

import (
	"context"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agentpromptfile"
	"aranea-agents/internal/data/ent/session"
)

// cascadeDeleteByAgent cleans up related records when an agent is soft-deleted.
// All operations are expected to run inside a transaction; any error aborts the whole transaction.
func cascadeDeleteByAgent(ctx context.Context, d *Data, agentID string) error {
	if d == nil || agentID == "" {
		return nil
	}

	// Delete agent_runtime_settings (1:1 with agent, PK = agent_id).
	// The record is optional — agents created via non-atomic paths may lack it.
	if err := d.RW().Write(ctx).AgentRuntimeSetting.DeleteOneID(agentID).Exec(ctx); err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
	}

	// Delete agent_prompt_files
	if _, err := d.RW().Write(ctx).AgentPromptFile.Delete().
		Where(agentpromptfile.AgentIDEQ(agentID)).
		Exec(ctx); err != nil {
		return err
	}

	// Soft-delete sessions owned by this agent
	now := nowRFC3339()
	if _, err := d.RW().Write(ctx).Session.Update().
		Where(session.AgentIDEQ(agentID), session.DeletedAtEQ("")).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return err
	}

	// Hard-delete tool_agent_overrides for this agent
	execer := d.RWDB().WriteDB(ctx)
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM tool_agent_overrides WHERE agent_id = ?`), agentID); err != nil {
		return err
	}

	return nil
}

// cascadeDeleteBySession cleans up related records when a session is soft-deleted.
// All 14 DELETE operations are wrapped in a single transaction to prevent
// partial success leaving the database in an inconsistent state.
// When called from within an ExecInTx, the inner ExecInTx reuses the outer transaction.
func cascadeDeleteBySession(ctx context.Context, d *Data, sessionID string) error {
	if d == nil || sessionID == "" {
		return nil
	}

	return d.ExecInTx(ctx, func(txCtx context.Context) error {
		execer := d.RWDB().WriteDB(txCtx)

		// Hard-delete session_turns (no soft-delete support)
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM session_turns WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete session_participants
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM session_participants WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete session_run_checkpoints
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM session_run_checkpoints WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete tool_invocation_params + tool_invocations
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM tool_invocation_params WHERE invocation_id IN (SELECT id FROM tool_invocations WHERE session_id = ?)`), sessionID); err != nil {
			return err
		}
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM tool_invocations WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete skill_invocations
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM skill_invocation WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete tool_result_blobs + tool_result_replacements
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM tool_result_replacements WHERE session_id = ?`), sessionID); err != nil {
			return err
		}
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM tool_result_blobs WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete messages
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM messages WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete event_store entries
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM event_store WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete session_runs (created by DDL migration, not Ent schema)
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM session_runs WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete session_runtime + session_metrics (1:1 with session)
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM session_runtime WHERE id = ?`), sessionID); err != nil {
			return err
		}
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM session_metrics WHERE id = ?`), sessionID); err != nil {
			return err
		}

		// Hard-delete channel_turn_jobs
		if _, err := execer.ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`DELETE FROM channel_turn_job WHERE session_id = ?`), sessionID); err != nil {
			return err
		}

		return nil
	})
}

// cascadeDeleteByTeam cleans up related records when a team is soft-deleted.
// All operations are expected to run inside a transaction; any error aborts the whole transaction.
func cascadeDeleteByTeam(ctx context.Context, d *Data, teamID string) error {
	if d == nil || teamID == "" {
		return nil
	}
	execer := d.RWDB().WriteDB(ctx)

	// Hard-delete team_run_steps (child of team_runs)
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM team_run_steps WHERE team_id = ?`), teamID); err != nil {
		return err
	}

	// Hard-delete team_runs
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM team_runs WHERE team_id = ?`), teamID); err != nil {
		return err
	}

	// Hard-delete compiled_teams
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM compiled_teams WHERE team_id = ?`), teamID); err != nil {
		return err
	}

	return nil
}

// cascadeDeleteByChannel cleans up related records when a channel is soft-deleted.
// All operations are expected to run inside a transaction; any error aborts the whole transaction.
func cascadeDeleteByChannel(ctx context.Context, d *Data, channelID string) error {
	if d == nil || channelID == "" {
		return nil
	}
	execer := d.RWDB().WriteDB(ctx)

	// Hard-delete channel_peer_sessions
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM channel_peer_session WHERE channel_id = ?`), channelID); err != nil {
		return err
	}

	// Hard-delete channel_credentials
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM channel_credential WHERE channel_id = ?`), channelID); err != nil {
		return err
	}

	// Hard-delete channel_deliveries
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM channel_delivery WHERE channel_id = ?`), channelID); err != nil {
		return err
	}

	// Hard-delete channel_inbound_receipts
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM channel_inbound_receipt WHERE channel_id = ?`), channelID); err != nil {
		return err
	}

	// Hard-delete channel_turn_jobs
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM channel_turn_job WHERE channel_id = ?`), channelID); err != nil {
		return err
	}

	// Hard-delete channel_runtime_leases
	if _, err := execer.ExecContext(ctx, d.Dialect().RenumberPlaceholders(`DELETE FROM channel_runtime_lease WHERE channel_id = ?`), channelID); err != nil {
		return err
	}

	return nil
}
