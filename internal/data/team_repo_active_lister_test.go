package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func TestTeamRepo_ListActiveRunTeamIDs(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRepo(d)
	ctx := context.Background()

	mk := func(id, teamID, status string) {
		t.Helper()
		if _, err := repo.CreateTeamRun(ctx, biz.TeamRunRecord{ID: id, TeamID: teamID, Status: status}); err != nil {
			t.Fatalf("CreateTeamRun(%s): %v", id, err)
		}
	}
	mk("tr-1", "t1", biz.TeamRunStatusRunning)
	mk("tr-2", "t2", biz.TeamRunStatusSuccess)
	mk("tr-3", "t3", biz.TeamRunStatusWaitingHuman)

	got, err := repo.ListActiveRunTeamIDs(ctx, []string{"t1", "t2", "t3", "t4"})
	if err != nil {
		t.Fatalf("ListActiveRunTeamIDs: %v", err)
	}
	if !got["t1"] || !got["t3"] || got["t2"] || got["t4"] || len(got) != 2 {
		t.Fatalf("unexpected active set: %v", got)
	}

	empty, err := repo.ListActiveRunTeamIDs(ctx, nil)
	if err != nil {
		t.Fatalf("ListActiveRunTeamIDs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty map for nil input, got %v", empty)
	}
}
