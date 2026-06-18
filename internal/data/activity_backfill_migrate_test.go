package data_test

import (
	"context"
	"testing"

	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/activity"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestRunActivityBackfillMigration_reconstructsFromMessages verifies that the
// migration reconstructs Activity records from existing ChatMessages for
// Pre-AF sessions (sessions with messages but no activities).
func TestRunActivityBackfillMigration_reconstructsFromMessages(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	// Insert test messages for a session with two turns.
	// Turn 1: user → assistant → tool
	// Turn 2: user → assistant
	messages := []struct {
		id        string
		sessionID string
		turnID    string
		turnNum   int
		seqInTurn int
		role      string
		content   string
		tokenIn   int
		tokenOut  int
		createdAt string
	}{
		{"msg-1", "sess-backfill", "turn-1", 1, 1, "user", "Hello, what is 1+1?", 0, 0, "2026-01-01T10:00:00Z"},
		{"msg-2", "sess-backfill", "turn-1", 1, 2, "assistant", "1+1 equals 2.", 10, 5, "2026-01-01T10:00:05Z"},
		{"msg-3", "sess-backfill", "turn-1", 1, 3, "tool", "tool result: 2", 0, 0, "2026-01-01T10:00:03Z"},
		{"msg-4", "sess-backfill", "turn-2", 2, 1, "user", "Thanks!", 0, 0, "2026-01-01T10:01:00Z"},
		{"msg-5", "sess-backfill", "turn-2", 2, 2, "assistant", "You're welcome.", 8, 3, "2026-01-01T10:01:02Z"},
	}

	for _, m := range messages {
		_, err := client.Message.Create().
			SetID(m.id).
			SetSessionID(m.sessionID).
			SetTurnID(m.turnID).
			SetTurnNumber(m.turnNum).
			SetSeqInTurn(m.seqInTurn).
			SetRole(m.role).
			SetContentMarkdown(m.content).
			SetTokenIn(m.tokenIn).
			SetTokenOut(m.tokenOut).
			SetCreatedAt(m.createdAt).
			Save(ctx)
		if err != nil {
			t.Fatalf("create message %s: %v", m.id, err)
		}
	}

	// Run the migration
	err := data.RunActivityBackfillMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("RunActivityBackfillMigration: %v", err)
	}

	// Verify activities were created
	acts, err := client.Activity.Query().
		Where(activity.SessionIDEQ("sess-backfill")).
		Order(activity.ByTimestamp()).
		All(ctx)
	if err != nil {
		t.Fatalf("query activities: %v", err)
	}

	// Expected: 2 task (root) + 2 reply + 1 action = 5 activities
	if len(acts) != 5 {
		t.Fatalf("expected 5 activities, got %d", len(acts))
	}

	// Build a map for easier verification
	actMap := make(map[string]*ent.Activity, len(acts))
	for _, a := range acts {
		actMap[a.ID] = a
	}

	// Verify turn-1 task activity (root)
	task1ID := "backfill-task-sess-backfill-turn-1"
	task1, ok := actMap[task1ID]
	if !ok {
		t.Fatalf("turn-1 task activity %q not found", task1ID)
	}
	if task1.Kind != "task" {
		t.Errorf("task1 kind=%q want task", task1.Kind)
	}
	if task1.Status != "completed" {
		t.Errorf("task1 status=%q want completed", task1.Status)
	}
	if task1.ParentActivityID != "" {
		t.Errorf("task1 parent=%q want empty (root)", task1.ParentActivityID)
	}
	if task1.TurnID != "turn-1" {
		t.Errorf("task1 turn_id=%q want turn-1", task1.TurnID)
	}

	// Verify turn-1 reply activity (from assistant message)
	reply1, ok := actMap["backfill-msg-2"]
	if !ok {
		t.Fatalf("reply activity backfill-msg-2 not found")
	}
	if reply1.Kind != "reply" {
		t.Errorf("reply1 kind=%q want reply", reply1.Kind)
	}
	if reply1.ParentActivityID != task1ID {
		t.Errorf("reply1 parent=%q want %q", reply1.ParentActivityID, task1ID)
	}
	if reply1.Content != "1+1 equals 2." {
		t.Errorf("reply1 content=%q want %q", reply1.Content, "1+1 equals 2.")
	}
	if reply1.PromptTokens != 10 {
		t.Errorf("reply1 prompt_tokens=%d want 10", reply1.PromptTokens)
	}
	if reply1.CompletionTokens != 5 {
		t.Errorf("reply1 completion_tokens=%d want 5", reply1.CompletionTokens)
	}

	// Verify turn-1 action activity (from tool message)
	action1, ok := actMap["backfill-msg-3"]
	if !ok {
		t.Fatalf("action activity backfill-msg-3 not found")
	}
	if action1.Kind != "action" {
		t.Errorf("action1 kind=%q want action", action1.Kind)
	}
	if action1.ParentActivityID != task1ID {
		t.Errorf("action1 parent=%q want %q", action1.ParentActivityID, task1ID)
	}

	// Verify turn-2 task activity (root)
	task2ID := "backfill-task-sess-backfill-turn-2"
	task2, ok := actMap[task2ID]
	if !ok {
		t.Fatalf("turn-2 task activity %q not found", task2ID)
	}
	if task2.Kind != "task" {
		t.Errorf("task2 kind=%q want task", task2.Kind)
	}
	if task2.TurnID != "turn-2" {
		t.Errorf("task2 turn_id=%q want turn-2", task2.TurnID)
	}

	// Verify turn-2 reply activity
	reply2, ok := actMap["backfill-msg-5"]
	if !ok {
		t.Fatalf("reply activity backfill-msg-5 not found")
	}
	if reply2.Kind != "reply" {
		t.Errorf("reply2 kind=%q want reply", reply2.Kind)
	}
	if reply2.ParentActivityID != task2ID {
		t.Errorf("reply2 parent=%q want %q", reply2.ParentActivityID, task2ID)
	}
}

// TestRunActivityBackfillMigration_skipsSessionsWithActivities verifies that
// sessions that already have Activity records are not backfilled (idempotent).
func TestRunActivityBackfillMigration_skipsSessionsWithActivities(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	// Insert a message for a session
	_, err := client.Message.Create().
		SetID("msg-existing").
		SetSessionID("sess-existing").
		SetTurnID("turn-1").
		SetTurnNumber(1).
		SetSeqInTurn(1).
		SetRole("user").
		SetContentMarkdown("Hello").
		SetCreatedAt("2026-01-01T10:00:00Z").
		Save(ctx)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	// Insert an existing activity for the same session (simulating AF already ran)
	_, err = client.Activity.Create().
		SetID("act-existing").
		SetKind("task").
		SetStatus("completed").
		SetSessionID("sess-existing").
		SetTurnID("turn-1").
		SetTimestamp("2026-01-01T10:00:00Z").
		Save(ctx)
	if err != nil {
		t.Fatalf("create existing activity: %v", err)
	}

	// Run the migration
	err = data.RunActivityBackfillMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("RunActivityBackfillMigration: %v", err)
	}

	// Verify no new activities were created for this session
	count, err := client.Activity.Query().
		Where(activity.SessionIDEQ("sess-existing")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count activities: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 activity (existing), got %d", count)
	}
}

// TestRunActivityBackfillMigration_gateSkipsOnSecondRun verifies the migration
// gate prevents re-running the migration on subsequent calls.
func TestRunActivityBackfillMigration_gateSkipsOnSecondRun(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	// Insert a message
	_, err := client.Message.Create().
		SetID("msg-gate").
		SetSessionID("sess-gate").
		SetTurnID("turn-1").
		SetTurnNumber(1).
		SetSeqInTurn(1).
		SetRole("user").
		SetContentMarkdown("Gate test").
		SetCreatedAt("2026-01-01T10:00:00Z").
		Save(ctx)
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	// First run
	err = data.RunActivityBackfillMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	countAfterFirst, err := client.Activity.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count after first: %v", err)
	}
	if countAfterFirst == 0 {
		t.Fatal("expected activities after first run, got 0")
	}

	// Second run — should be skipped by gate
	err = data.RunActivityBackfillMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	countAfterSecond, err := client.Activity.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count after second: %v", err)
	}
	if countAfterSecond != countAfterFirst {
		t.Errorf("expected %d activities after second run (gate skip), got %d", countAfterFirst, countAfterSecond)
	}
}

// TestRunActivityBackfillMigration_emptyDB verifies the migration handles an
// empty database (no messages) gracefully.
func TestRunActivityBackfillMigration_emptyDB(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	err := data.RunActivityBackfillMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("RunActivityBackfillMigration on empty DB: %v", err)
	}

	count, err := client.Activity.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count activities: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 activities on empty DB, got %d", count)
	}
}

// TestRunActivityBackfillMigration_messagesWithoutTurnID verifies that
// messages with an empty turn_id are grouped under a synthetic "no-turn" turn.
func TestRunActivityBackfillMigration_messagesWithoutTurnID(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	// Insert messages without turn_id
	messages := []struct {
		id        string
		sessionID string
		turnID    string
		role      string
		content   string
		createdAt string
	}{
		{"msg-noturn-1", "sess-noturn", "", "user", "Hello", "2026-01-01T10:00:00Z"},
		{"msg-noturn-2", "sess-noturn", "", "assistant", "Hi there", "2026-01-01T10:00:05Z"},
	}

	for _, m := range messages {
		_, err := client.Message.Create().
			SetID(m.id).
			SetSessionID(m.sessionID).
			SetTurnID(m.turnID).
			SetRole(m.role).
			SetContentMarkdown(m.content).
			SetCreatedAt(m.createdAt).
			Save(ctx)
		if err != nil {
			t.Fatalf("create message %s: %v", m.id, err)
		}
	}

	err := data.RunActivityBackfillMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("RunActivityBackfillMigration: %v", err)
	}

	// Expected: 1 task (root, synthetic "no-turn") + 1 reply = 2 activities
	acts, err := client.Activity.Query().
		Where(activity.SessionIDEQ("sess-noturn")).
		All(ctx)
	if err != nil {
		t.Fatalf("query activities: %v", err)
	}
	if len(acts) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(acts))
	}

	// Verify the root task uses the synthetic "no-turn" turn ID
	rootID := "backfill-task-sess-noturn-no-turn"
	actMap := make(map[string]*ent.Activity, len(acts))
	for _, a := range acts {
		actMap[a.ID] = a
	}
	root, ok := actMap[rootID]
	if !ok {
		t.Fatalf("root task %q not found", rootID)
	}
	if root.TurnID != "no-turn" {
		t.Errorf("root turn_id=%q want no-turn", root.TurnID)
	}
}

// TestRunActivityBackfillMigration_assistantFailedStatus verifies that an
// assistant message with a non-"ok" status maps to a reply activity with
// "failed" status.
func TestRunActivityBackfillMigration_assistantFailedStatus(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	// Insert a user message and a failed assistant message
	messages := []struct {
		id        string
		role      string
		status    string
		content   string
		createdAt string
	}{
		{"msg-fail-1", "user", "ok", "Do something", "2026-01-01T10:00:00Z"},
		{"msg-fail-2", "assistant", "error", "I failed", "2026-01-01T10:00:05Z"},
	}

	for _, m := range messages {
		_, err := client.Message.Create().
			SetID(m.id).
			SetSessionID("sess-fail").
			SetTurnID("turn-1").
			SetTurnNumber(1).
			SetRole(m.role).
			SetStatus(m.status).
			SetContentMarkdown(m.content).
			SetCreatedAt(m.createdAt).
			Save(ctx)
		if err != nil {
			t.Fatalf("create message %s: %v", m.id, err)
		}
	}

	err := data.RunActivityBackfillMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("RunActivityBackfillMigration: %v", err)
	}

	// Find the reply activity from the failed assistant message
	acts, err := client.Activity.Query().
		Where(activity.SessionIDEQ("sess-fail")).
		All(ctx)
	if err != nil {
		t.Fatalf("query activities: %v", err)
	}

	actMap := make(map[string]*ent.Activity, len(acts))
	for _, a := range acts {
		actMap[a.ID] = a
	}

	reply, ok := actMap["backfill-msg-fail-2"]
	if !ok {
		t.Fatalf("reply activity backfill-msg-fail-2 not found")
	}
	if reply.Kind != "reply" {
		t.Errorf("reply kind=%q want reply", reply.Kind)
	}
	if reply.Status != "failed" {
		t.Errorf("reply status=%q want failed", reply.Status)
	}
}
