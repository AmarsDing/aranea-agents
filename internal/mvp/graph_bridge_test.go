package mvp_test

import (
	"testing"

	"aranea-agents/internal/mvp"
)

func TestEnqueueFromGraphPause_Idempotent(t *testing.T) {
	mvp.ResetForTest()
	task1, err := mvp.EnqueueFromGraphPause("exec-1", "human_review", "gt-1", "", "Title", "Body", []string{"generic"})
	if err != nil {
		t.Fatal(err)
	}
	if task1.GraphExecutionID != "exec-1" || task1.GraphNodeID != "human_review" || task1.GraphTaskID != "gt-1" {
		t.Fatalf("links: %+v", task1)
	}
	task2, err := mvp.EnqueueFromGraphPause("exec-1", "human_review", "gt-1", "", "Title", "Body", nil)
	if err != nil {
		t.Fatal(err)
	}
	if task1.ID != task2.ID {
		t.Fatalf("expected idempotent enqueue, got %s vs %s", task1.ID, task2.ID)
	}
}

func TestAfterReviewDecision_CallsHooks(t *testing.T) {
	mvp.ResetForTest()
	var resumed string
	var completed string
	mvp.Configure(mvp.Deps{
		GraphResume: func(executionID string, resumeValue map[string]any) error {
			resumed = executionID
			return nil
		},
		GraphTaskComplete: func(taskID, submitter, output, summary string) error {
			completed = taskID
			return nil
		},
	})
	task, err := mvp.EnqueueFromGraphPause("exec-x", "human_review", "gt-x", "", "T", "B", nil)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := mvp.Ensure().Review.Approve(task.ID, "ops", "ok")
	if err != nil {
		t.Fatal(err)
	}
	if err := mvp.AfterReviewDecision(approved); err != nil {
		t.Fatal(err)
	}
	if completed != "gt-x" {
		t.Fatalf("expected GraphTaskComplete gt-x, got %q", completed)
	}
	// With GraphTaskID set, resume is deferred to OnTaskCompleted path.
	if resumed != "" {
		t.Fatalf("expected no direct resume when GraphTaskID set, got %q", resumed)
	}

	// Reject path without GraphTaskID should resume directly.
	mvp.ResetForTest()
	resumed = ""
	mvp.Configure(mvp.Deps{
		GraphResume: func(executionID string, _ map[string]any) error {
			resumed = executionID
			return nil
		},
	})
	task2, err := mvp.EnqueueFromGraphPause("exec-y", "human_review", "", "", "T", "B", nil)
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := mvp.Ensure().Review.Reject(task2.ID, "ops", "nope")
	if err != nil {
		t.Fatal(err)
	}
	if err := mvp.AfterReviewDecision(rejected); err != nil {
		t.Fatal(err)
	}
	if resumed != "exec-y" {
		t.Fatalf("expected resume exec-y, got %q", resumed)
	}
}
