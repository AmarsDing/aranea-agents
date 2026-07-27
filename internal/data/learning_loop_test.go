package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func setupLearningLoopTestData(t *testing.T) *Data {
	t.Helper()
	rawDB := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()

	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS learning_observations (
		  id TEXT PRIMARY KEY,
		  agent_id TEXT NOT NULL,
		  session_id TEXT NOT NULL DEFAULT '',
		  kind TEXT NOT NULL DEFAULT 'tool_call',
		  content TEXT NOT NULL DEFAULT '',
		  metadata TEXT NOT NULL DEFAULT '',
		  observed_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS learning_patterns (
		  id TEXT PRIMARY KEY,
		  agent_id TEXT NOT NULL,
		  kind TEXT NOT NULL DEFAULT '',
		  description TEXT NOT NULL DEFAULT '',
		  frequency INTEGER NOT NULL DEFAULT 0,
		  confidence REAL NOT NULL DEFAULT 0,
		  evidence TEXT NOT NULL DEFAULT '',
		  status TEXT NOT NULL DEFAULT 'detected',
		  detected_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS learning_proposals (
		  id TEXT PRIMARY KEY,
		  agent_id TEXT NOT NULL,
		  pattern_id TEXT NOT NULL DEFAULT '',
		  title TEXT NOT NULL DEFAULT '',
		  content TEXT NOT NULL DEFAULT '',
		  kind TEXT NOT NULL DEFAULT '',
		  status TEXT NOT NULL DEFAULT 'draft',
		  validated_at TEXT,
		  approved_by TEXT NOT NULL DEFAULT '',
		  created_at TEXT NOT NULL,
		  updated_at TEXT NOT NULL
		)`,
	} {
		if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, rawDB)))
	t.Cleanup(func() { client.Close() })

	d := &Data{}
	d.SetEntClientForTest(client, rawDB, loggateway.NewNoop())
	return d
}

func TestProposalRepo_ListByAgent_NullValidatedAt(t *testing.T) {
	d := setupLearningLoopTestData(t)
	repo := NewProposalRepo(d)
	ctx := context.Background()
	now := time.Now().UTC()

	draft := biz.KnowledgeProposal{
		ID:        "prop-null-va-list",
		AgentID:   "agent-1",
		PatternID: "pat-1",
		Title:     "draft proposal",
		Content:   "content",
		Kind:      "rule",
		Status:    biz.ProposalStatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := repo.Create(ctx, draft); err != nil {
		t.Fatalf("Create draft proposal: %v", err)
	}

	proposals, err := repo.ListByAgent(ctx, "agent-1", "")
	if err != nil {
		t.Fatalf("ListByAgent with NULL validated_at: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(proposals))
	}
	if proposals[0].ValidatedAt != nil {
		t.Errorf("expected ValidatedAt to be nil, got %v", proposals[0].ValidatedAt)
	}
}

func TestProposalRepo_GetByID_NullValidatedAt(t *testing.T) {
	d := setupLearningLoopTestData(t)
	repo := NewProposalRepo(d)
	ctx := context.Background()
	now := time.Now().UTC()

	draft := biz.KnowledgeProposal{
		ID:        "prop-null-va-get",
		AgentID:   "agent-1",
		PatternID: "pat-1",
		Title:     "draft proposal",
		Content:   "content",
		Kind:      "rule",
		Status:    biz.ProposalStatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := repo.Create(ctx, draft); err != nil {
		t.Fatalf("Create draft proposal: %v", err)
	}

	p, err := repo.GetByID(ctx, "prop-null-va-get")
	if err != nil {
		t.Fatalf("GetByID with NULL validated_at: %v", err)
	}
	if p.ValidatedAt != nil {
		t.Errorf("expected ValidatedAt to be nil, got %v", p.ValidatedAt)
	}
}