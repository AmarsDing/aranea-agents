package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func testNoopLogger() loggateway.Logger { return loggateway.NewNoop() }

// seedPagedSteps inserts n steps (seq 1..n) into sessionID via the repo.
func seedPagedSteps(t *testing.T, repo biz.StepV2Repo, sessionID string, n int) {
	t.Helper()
	base := time.Now().UTC().Add(-time.Hour)
	for i := 1; i <= n; i++ {
		_, err := repo.CreateStep(context.Background(), biz.Step{
			ID:              fmt.Sprintf("step-%s-%d", sessionID, i),
			SessionID:       sessionID,
			SpiritSessionID: sessionID,
			Kind:            biz.StepKindReply,
			AuthorAgentKey:  "agent-1",
			Seq:             int64(i),
			Status:          biz.StepStatusCompleted,
			StartedAt:       base.Add(time.Duration(i) * time.Second),
			Version:         1,
		})
		if err != nil {
			t.Fatalf("seed step %d: %v", i, err)
		}
	}
}

func stepSeqs(steps []biz.Step) []int64 {
	out := make([]int64, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Seq)
	}
	return out
}

func TestStepV2Repo_ListStepsBySessionPaged(t *testing.T) {
	d := newTestDataPG(t)
	repo := NewStepV2Repo(d, testNoopLogger())
	seedPagedSteps(t, repo, "sess-paged", 10)
	ctx := context.Background()

	t.Run("limit window returns latest N asc with hasMore", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if !hasMore {
			t.Error("hasMore=false, want true")
		}
		got := stepSeqs(steps)
		want := []int64{8, 9, 10}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("seqs=%v, want %v (ascending)", got, want)
		}
	})

	t.Run("before_seq walks towards older", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3, BeforeSeq: 8})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if !hasMore {
			t.Error("hasMore=false, want true")
		}
		if got, want := fmt.Sprint(stepSeqs(steps)), "[5 6 7]"; got != want {
			t.Errorf("seqs=%s, want %s", got, want)
		}
	})

	t.Run("last page hasMore=false", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3, BeforeSeq: 3})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if hasMore {
			t.Error("hasMore=true, want false")
		}
		if got, want := fmt.Sprint(stepSeqs(steps)), "[1 2]"; got != want {
			t.Errorf("seqs=%s, want %s", got, want)
		}
	})

	t.Run("limit=0 degrades to full list", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if hasMore {
			t.Error("hasMore=true, want false")
		}
		if len(steps) != 10 {
			t.Errorf("len=%d, want 10", len(steps))
		}
	})
}
