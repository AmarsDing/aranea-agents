package data

import (
	"context"
	"testing"
	"time"

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

// 79-runtime-governance R7：stats JSONL 导出的窗口列举。覆盖 created_at
// DESC 排序、from/to 窗口过滤、session_id 过滤与跨 team 列举。
func TestTeamRepo_ListTeamRunsForStatsExport(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRepo(d)
	ctx := context.Background()

	at := func(h int) string {
		return time.Date(2026, 8, 27, h, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	}
	mk := func(id, teamID, sessionID, createdAt string) {
		t.Helper()
		if _, err := repo.CreateTeamRun(ctx, biz.TeamRunRecord{
			ID: id, TeamID: teamID, SessionID: sessionID,
			Status: biz.TeamRunStatusSuccess, CreatedAt: createdAt,
		}); err != nil {
			t.Fatalf("CreateTeamRun(%s): %v", id, err)
		}
	}
	mk("tr-e1", "t1", "s-a", at(10))
	mk("tr-e2", "t2", "s-a", at(12))
	mk("tr-e3", "t1", "s-b", at(14))

	// 无窗口：全量，created_at DESC。
	all, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", 100)
	if err != nil {
		t.Fatalf("ListTeamRunsForStatsExport(all): %v", err)
	}
	if len(all) != 3 || all[0].ID != "tr-e3" || all[2].ID != "tr-e1" {
		t.Fatalf("all = %+v, want 3 rows DESC tr-e3..tr-e1", all)
	}

	// from/to 窗口（UTC 下推，含边界）。
	from := time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	win, err := repo.ListTeamRunsForStatsExport(ctx, from, to, "", 100)
	if err != nil {
		t.Fatalf("ListTeamRunsForStatsExport(window): %v", err)
	}
	if len(win) != 1 || win[0].ID != "tr-e2" {
		t.Fatalf("window = %+v, want only tr-e2", win)
	}

	// session_id 过滤跨 team。
	bySession, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "s-a", 100)
	if err != nil {
		t.Fatalf("ListTeamRunsForStatsExport(session): %v", err)
	}
	if len(bySession) != 2 || bySession[0].ID != "tr-e2" || bySession[1].ID != "tr-e1" {
		t.Fatalf("session = %+v, want tr-e2,tr-e1 DESC", bySession)
	}

	// limit 截断。
	lim, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", 2)
	if err != nil {
		t.Fatalf("ListTeamRunsForStatsExport(limit): %v", err)
	}
	if len(lim) != 2 || lim[0].ID != "tr-e3" {
		t.Fatalf("limit = %+v, want 2 rows head tr-e3", lim)
	}
}
