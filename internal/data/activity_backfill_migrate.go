package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/activity"
	"aranea-agents/internal/data/ent/message"
	"aranea-agents/pkg/loggateway"
)

// RunActivityBackfillMigration reconstructs Activity records from existing
// ChatMessages for Pre-AF sessions (sessions with messages but no activities).
//
// This migration is part of AF-GAP-03: Pre-AF historical sessions have no
// Activity records, so the Activity-First frontend cannot render them. This
// migration backfills activities by mapping message roles to activity kinds:
//   - user → task (root activity per turn)
//   - assistant → reply
//   - tool → action
//   - system → notice
//
// Limitations (due to Message schema):
//   - No reasoning_content → no thinking activities
//   - No tool_calls/tool_call_id → action activities have empty tool fields
//   - No author/agent_key → agent_key is left empty
//
// Idempotency:
//   - Migration gate (schema_migrations) prevents re-running
//   - Activity IDs are deterministic ("backfill-task-{sessionID}-{turnID}" for
//     root tasks, "backfill-{messageID}" for child activities) so partial
//     runs can be safely retried
//   - Sessions that already have activities are skipped
//
// Best-effort semantics:
//   - If a single session fails (e.g., DB error), the migration logs a warning
//     and continues with the next session. The gate is still recorded, so the
//     migration will NOT automatically retry failed sessions on next startup.
//   - This is acceptable because activity data is for display only (not
//     business-critical), and failed sessions can be manually investigated.
//   - Activity creation is idempotent (deterministic IDs + constraint-error
//     handling), so a manual re-run after gate reset is safe.
func RunActivityBackfillMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("activity backfill migration: ent client required")
	}

	applied, err := isMigrationApplied(ctx, client, MigrationActivityBackfill, lg)
	if err != nil {
		return fmt.Errorf("activity backfill migration: check gate: %w", err)
	}
	if applied {
		return nil
	}

	lg.Info("activity backfill (pre-AF): starting", loggateway.StepID("migration.activity_backfill"))

	// Find sessions that have messages but no activities.
	// We use a LEFT JOIN to find sessions in messages that don't have any
	// matching rows in activities.
	sessionIDs, err := findPreAFSessions(ctx, client)
	if err != nil {
		return fmt.Errorf("activity backfill migration: find pre-AF sessions: %w", err)
	}

	if len(sessionIDs) == 0 {
		lg.Info("activity backfill (pre-AF): no sessions to backfill",
			loggateway.StepID("migration.activity_backfill"))
		if err := recordMigrationApplied(ctx, client, d, MigrationActivityBackfill, migrationNameActivityBackfill, lg); err != nil {
			return fmt.Errorf("activity backfill migration: record: %w", err)
		}
		return nil
	}

	totalActivities := 0
	for _, sessionID := range sessionIDs {
		count, err := backfillSessionActivities(ctx, client, sessionID, lg)
		if err != nil {
			lg.Warn("activity backfill: session failed, continuing",
				loggateway.StepID("migration.activity_backfill"),
				loggateway.SessionID(sessionID),
				loggateway.Err(err))
			continue
		}
		totalActivities += count
	}

	if err := recordMigrationApplied(ctx, client, d, MigrationActivityBackfill, migrationNameActivityBackfill, lg); err != nil {
		return fmt.Errorf("activity backfill migration: record: %w", err)
	}

	lg.Info("activity backfill (pre-AF): done",
		loggateway.StepID("migration.activity_backfill"),
		loggateway.Int("sessions", len(sessionIDs)),
		loggateway.Int("activities", totalActivities))
	return nil
}

// findPreAFSessions returns session IDs that have messages but no activities.
func findPreAFSessions(ctx context.Context, client *ent.Client) ([]string, error) {
	// Query distinct session_ids from messages
	msgSessionIDs, err := client.Message.Query().
		Unique(true).
		Select(message.FieldSessionID).
		Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("query message session IDs: %w", err)
	}

	// Query distinct session_ids from activities
	actSessionIDs, err := client.Activity.Query().
		Unique(true).
		Select(activity.FieldSessionID).
		Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("query activity session IDs: %w", err)
	}

	// Build a set of sessions that already have activities
	actSet := make(map[string]struct{}, len(actSessionIDs))
	for _, sid := range actSessionIDs {
		actSet[sid] = struct{}{}
	}

	// Filter: sessions in messages but not in activities
	var result []string
	for _, sid := range msgSessionIDs {
		if sid == "" {
			continue
		}
		if _, has := actSet[sid]; !has {
			result = append(result, sid)
		}
	}
	return result, nil
}

// backfillSessionActivities reconstructs activities for a single session.
// Returns the number of activities created.
func backfillSessionActivities(ctx context.Context, client *ent.Client, sessionID string, lg loggateway.Logger) (int, error) {
	// Query all messages for this session, ordered by turn_id, seq_in_turn, created_at
	msgs, err := client.Message.Query().
		Where(message.SessionIDEQ(sessionID)).
		Order(
			ent.Asc(message.FieldTurnID),
			ent.Asc(message.FieldSeqInTurn),
			ent.Asc(message.FieldCreatedAt),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("query messages for session %s: %w", sessionID, err)
	}

	if len(msgs) == 0 {
		return 0, nil
	}

	// Group messages by turn_id and create activities
	created := 0
	turnRootIDs := make(map[string]string) // turnID → root task activity ID

	for _, msg := range msgs {
		turnID := msg.TurnID
		if turnID == "" {
			// Messages without turn_id get a synthetic turn ID based on session
			turnID = "no-turn"
		}

		// Ensure root task activity exists for this turn
		rootID, ok := turnRootIDs[turnID]
		if !ok {
			rootID = fmt.Sprintf("backfill-task-%s-%s", sessionID, turnID)
			// Create root task activity from the first message (typically user)
			rootTimestamp := msg.CreatedAt
			if rootTimestamp == "" {
				rootTimestamp = time.Now().UTC().Format(time.RFC3339Nano)
			}

			rootContent := ""
			if msg.Role == "user" {
				rootContent = msg.ContentMarkdown
			}

			if err := createBackfillActivity(ctx, client, rootID, "task", "completed",
				sessionID, turnID, "", rootTimestamp, rootContent, 0, 0); err != nil {
				return created, fmt.Errorf("create root task for turn %s: %w", turnID, err)
			}
			turnRootIDs[turnID] = rootID
			created++
		}

		// Skip creating child activity for the user message that became the root task
		if msg.Role == "user" {
			continue
		}

		// Map message role to activity kind
		kind, status := messageRoleToActivityKind(msg.Role, msg.Status)

		// Skip roles that don't map to any activity kind
		if kind == "" {
			continue
		}

		childID := fmt.Sprintf("backfill-%s", msg.ID)
		timestamp := msg.CreatedAt
		if timestamp == "" {
			timestamp = time.Now().UTC().Format(time.RFC3339Nano)
		}

		promptTokens := int64(msg.TokenIn)
		completionTokens := int64(msg.TokenOut)

		if err := createBackfillActivity(ctx, client, childID, kind, status,
			sessionID, turnID, rootID, timestamp, msg.ContentMarkdown,
			promptTokens, completionTokens); err != nil {
			lg.Warn("activity backfill: create child activity failed, skipping",
				loggateway.StepID("migration.activity_backfill"),
				loggateway.Str("message_id", msg.ID),
				loggateway.Err(err))
			continue
		}
		created++
	}

	return created, nil
}

// createBackfillActivity creates a single activity record via Ent.
// Uses Create (not Upsert) — if the ID already exists (partial re-run),
// the constraint error is logged and the caller continues.
func createBackfillActivity(ctx context.Context, client *ent.Client,
	id, kind, status, sessionID, turnID, parentID, timestamp, content string,
	promptTokens, completionTokens int64) error {

	builder := client.Activity.Create().
		SetID(id).
		SetKind(kind).
		SetStatus(status).
		SetSessionID(sessionID).
		SetTurnID(turnID).
		SetParentActivityID(parentID).
		SetTimestamp(timestamp).
		SetDurationMs(0).
		SetPromptTokens(promptTokens).
		SetCompletionTokens(completionTokens).
		SetContent(content).
		SetCollapsed(kind != "reply") // reply is expanded by default, others collapsed

	_, err := builder.Save(ctx)
	if err != nil {
		// If the activity already exists (partial re-run), treat as success
		if ent.IsConstraintError(err) {
			return nil
		}
		return err
	}
	return nil
}

// messageRoleToActivityKind maps a message role to the corresponding activity
// kind and status. Returns empty kind for roles that should be skipped.
func messageRoleToActivityKind(role, msgStatus string) (kind, status string) {
	switch role {
	case "assistant":
		if msgStatus != "" && msgStatus != "ok" {
			return "reply", "failed"
		}
		return "reply", "completed"
	case "tool":
		return "action", "completed"
	case "system":
		return "notice", "completed"
	default:
		// user, unknown roles, etc. are not mapped to child activities
		return "", ""
	}
}
