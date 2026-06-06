package data

import (
	"context"

	"aranea-agents/internal/data/ent/agentpromptfile"
	"aranea-agents/internal/data/ent/session"
	"aranea-agents/pkg/loggateway"
)

// cascadeDeleteByAgent cleans up related records when an agent is soft-deleted.
// Failures are logged but do not block the primary delete operation.
func cascadeDeleteByAgent(ctx context.Context, d *Data, agentID string) {
	if d == nil || agentID == "" {
		return
	}
	lg := d.lg.With(loggateway.StepID("data.cascade.agent"), loggateway.Str("agent_id", agentID))

	// Delete agent_runtime_settings (1:1 with agent, PK = agent_id)
	if err := d.RW().Write(ctx).AgentRuntimeSetting.DeleteOneID(agentID).Exec(ctx); err != nil {
		lg.Warn("cascade: delete agent_runtime_setting failed", loggateway.Err(err))
	}

	// Delete agent_prompt_files
	if n, err := d.RW().Write(ctx).AgentPromptFile.Delete().
		Where(agentpromptfile.AgentIDEQ(agentID)).
		Exec(ctx); err != nil {
		lg.Warn("cascade: delete agent_prompt_files failed", loggateway.Err(err))
	} else if n > 0 {
		lg.Debug("cascade: deleted agent_prompt_files", loggateway.Int("count", n))
	}

	// Soft-delete sessions owned by this agent
	now := nowRFC3339()
	if _, err := d.RW().Write(ctx).Session.Update().
		Where(session.AgentIDEQ(agentID), session.DeletedAtEQ("")).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		lg.Warn("cascade: soft-delete sessions by agent failed", loggateway.Err(err))
	}

	// Hard-delete tool_agent_overrides for this agent
	execer := d.RWDB().WriteDB(ctx)
	if _, err := execer.ExecContext(ctx, `DELETE FROM tool_agent_overrides WHERE agent_id = ?`, agentID); err != nil {
		lg.Warn("cascade: delete tool_agent_overrides failed", loggateway.Err(err))
	}
}

// cascadeDeleteBySession cleans up related records when a session is soft-deleted.
// All 14 DELETE operations are wrapped in a single transaction to prevent
// partial success leaving the database in an inconsistent state.
func cascadeDeleteBySession(ctx context.Context, d *Data, sessionID string) {
	if d == nil || sessionID == "" {
		return
	}
	lg := d.lg.With(loggateway.StepID("data.cascade.session"), loggateway.Str("session_id", sessionID))

	err := d.ExecInTx(ctx, func(txCtx context.Context) error {
		execer := d.RWDB().WriteDB(txCtx)

		// Hard-delete session_turns (no soft-delete support)
		if _, err := execer.ExecContext(txCtx, `DELETE FROM session_turns WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete session_participants
		if _, err := execer.ExecContext(txCtx, `DELETE FROM session_participants WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete session_run_checkpoints
		if _, err := execer.ExecContext(txCtx, `DELETE FROM session_run_checkpoints WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete tool_invocation_params + tool_invocations
		if _, err := execer.ExecContext(txCtx, `DELETE FROM tool_invocation_params WHERE invocation_id IN (SELECT id FROM tool_invocations WHERE session_id = ?)`, sessionID); err != nil {
			return err
		}
		if _, err := execer.ExecContext(txCtx, `DELETE FROM tool_invocations WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete skill_invocations
		if _, err := execer.ExecContext(txCtx, `DELETE FROM skill_invocation WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete tool_result_blobs + tool_result_replacements
		if _, err := execer.ExecContext(txCtx, `DELETE FROM tool_result_replacements WHERE session_id = ?`, sessionID); err != nil {
			return err
		}
		if _, err := execer.ExecContext(txCtx, `DELETE FROM tool_result_blobs WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete messages
		if _, err := execer.ExecContext(txCtx, `DELETE FROM messages WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete event_store entries
		if _, err := execer.ExecContext(txCtx, `DELETE FROM event_store WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete session_runs (created by DDL migration, not Ent schema)
		if _, err := execer.ExecContext(txCtx, `DELETE FROM session_runs WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete session_runtime + session_metrics (1:1 with session)
		if _, err := execer.ExecContext(txCtx, `DELETE FROM session_runtime WHERE id = ?`, sessionID); err != nil {
			return err
		}
		if _, err := execer.ExecContext(txCtx, `DELETE FROM session_metrics WHERE id = ?`, sessionID); err != nil {
			return err
		}

		// Hard-delete channel_turn_jobs
		if _, err := execer.ExecContext(txCtx, `DELETE FROM channel_turn_job WHERE session_id = ?`, sessionID); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		lg.Warn("cascade: delete session related records failed", loggateway.Err(err))
	}
}

// cascadeDeleteByTeam cleans up related records when a team is soft-deleted.
// Failures are logged but do not block the primary delete operation.
func cascadeDeleteByTeam(ctx context.Context, d *Data, teamID string) {
	if d == nil || teamID == "" {
		return
	}
	lg := d.lg.With(loggateway.StepID("data.cascade.team"), loggateway.Str("team_id", teamID))
	execer := d.RWDB().WriteDB(ctx)

	// Hard-delete team_run_steps (child of team_runs)
	if _, err := execer.ExecContext(ctx, `DELETE FROM team_run_steps WHERE team_id = ?`, teamID); err != nil {
		lg.Warn("cascade: delete team_run_steps failed", loggateway.Err(err))
	}

	// Hard-delete team_runs
	if _, err := execer.ExecContext(ctx, `DELETE FROM team_runs WHERE team_id = ?`, teamID); err != nil {
		lg.Warn("cascade: delete team_runs failed", loggateway.Err(err))
	}

	// Hard-delete compiled_teams
	if _, err := execer.ExecContext(ctx, `DELETE FROM compiled_teams WHERE team_id = ?`, teamID); err != nil {
		lg.Warn("cascade: delete compiled_teams failed", loggateway.Err(err))
	}
}

// cascadeDeleteByChannel cleans up related records when a channel is soft-deleted.
// Failures are logged but do not block the primary delete operation.
func cascadeDeleteByChannel(ctx context.Context, d *Data, channelID string) {
	if d == nil || channelID == "" {
		return
	}
	lg := d.lg.With(loggateway.StepID("data.cascade.channel"), loggateway.Str("channel_id", channelID))
	execer := d.RWDB().WriteDB(ctx)

	// Hard-delete channel_peer_sessions
	if _, err := execer.ExecContext(ctx, `DELETE FROM channel_peer_session WHERE channel_id = ?`, channelID); err != nil {
		lg.Warn("cascade: delete channel_peer_sessions failed", loggateway.Err(err))
	}

	// Hard-delete channel_credentials
	if _, err := execer.ExecContext(ctx, `DELETE FROM channel_credential WHERE channel_id = ?`, channelID); err != nil {
		lg.Warn("cascade: delete channel_credentials failed", loggateway.Err(err))
	}

	// Hard-delete channel_deliveries
	if _, err := execer.ExecContext(ctx, `DELETE FROM channel_delivery WHERE channel_id = ?`, channelID); err != nil {
		lg.Warn("cascade: delete channel_deliveries failed", loggateway.Err(err))
	}

	// Hard-delete channel_inbound_receipts
	if _, err := execer.ExecContext(ctx, `DELETE FROM channel_inbound_receipt WHERE channel_id = ?`, channelID); err != nil {
		lg.Warn("cascade: delete channel_inbound_receipts failed", loggateway.Err(err))
	}

	// Hard-delete channel_turn_jobs
	if _, err := execer.ExecContext(ctx, `DELETE FROM channel_turn_job WHERE channel_id = ?`, channelID); err != nil {
		lg.Warn("cascade: delete channel_turn_jobs failed", loggateway.Err(err))
	}

	// Hard-delete channel_runtime_leases
	if _, err := execer.ExecContext(ctx, `DELETE FROM channel_runtime_lease WHERE channel_id = ?`, channelID); err != nil {
		lg.Warn("cascade: delete channel_runtime_leases failed", loggateway.Err(err))
	}
}
