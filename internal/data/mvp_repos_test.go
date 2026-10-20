package data_test

import (
	"database/sql"
	"testing"
	"time"

	"aranea-agents/internal/biz/commerce"
	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/biz/publish"
	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

func skipIfNoTestPG(t *testing.T) {
	t.Helper()
	dsn := testhelper.TestPGDSN()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("postgres unreachable (%v); skipping MVP Ent tests", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("postgres unreachable (%v); skipping MVP Ent tests — set ARANEA_TEST_PG_DSN", err)
	}
}

func TestMVPRepos_ContentReviewPublishRoundTrip(t *testing.T) {
	skipIfNoTestPG(t)
	client, db := testhelper.SetupTestPG(t)
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())

	contentRepo := data.NewContentItemRepo(d)
	reviewRepo := data.NewReviewTaskRepo(d)
	publishRepo := data.NewPublishJobRepo(d)

	now := time.Now().UTC()
	item, err := contentRepo.Create(content.Item{
		ID: uuid.NewString(), WorkspaceID: "ws", Title: "t", Body: "b",
		SourceType: content.SourceManual, Status: content.StatusCandidate,
		DedupeHash: "abc", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := contentRepo.GetByDedupeHash("ws", "abc")
	if err != nil || !ok || got.ID != item.ID {
		t.Fatalf("dedupe: ok=%v err=%v id=%s", ok, err, got.ID)
	}

	task, err := reviewRepo.Create(review.Task{
		ID: uuid.NewString(), ContentItemID: item.ID,
		GraphExecutionID: "exec-1", GraphNodeID: "human_review", GraphTaskID: "gt-1",
		DraftTitle: "t", DraftBody: "b", Status: review.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, ok, err := reviewRepo.GetByGraphExecution("exec-1", "human_review")
	if err != nil || !ok || linked.ID != task.ID {
		t.Fatalf("graph link: ok=%v err=%v", ok, err)
	}

	job, err := publishRepo.Create(publish.Job{
		ID: uuid.NewString(), ReviewTaskID: task.ID, ContentItemID: item.ID,
		Platform: "generic", Mode: publish.ModePackage, Status: publish.StatusPending,
		IdempotencyKey: task.ID + "|generic|package", Title: "t", Body: "b",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	byKey, ok, err := publishRepo.GetByIdempotency(job.IdempotencyKey)
	if err != nil || !ok || byKey.ID != job.ID {
		t.Fatalf("idempotency: ok=%v err=%v", ok, err)
	}
}

func TestMVPRepos_OpportunitySupplier(t *testing.T) {
	skipIfNoTestPG(t)
	client, db := testhelper.SetupTestPG(t)
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())

	oppRepo := data.NewOpportunityRepo(d)
	supRepo := data.NewSupplierCandidateRepo(d)
	now := time.Now().UTC()
	opp, err := oppRepo.Create(commerce.Opportunity{
		ID: uuid.NewString(), Title: "hot sku", Audience: commerce.AudienceBoth,
		Status: commerce.OppOpen, Score: 0.9, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = supRepo.Create(commerce.SupplierCandidate{
		ID: uuid.NewString(), OpportunityID: opp.ID, Name: "Factory A",
		Status: commerce.SupplierSuggested, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := supRepo.ListByOpportunity(opp.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list suppliers: n=%d err=%v", len(list), err)
	}
}
