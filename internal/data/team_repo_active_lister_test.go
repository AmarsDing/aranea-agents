package data

import (
	"context"
	"fmt"
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
	all, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", nil, 100)
	if err != nil {
		t.Fatalf("ListTeamRunsForStatsExport(all): %v", err)
	}
	if len(all) != 3 || all[0].ID != "tr-e3" || all[2].ID != "tr-e1" {
		t.Fatalf("all = %+v, want 3 rows DESC tr-e3..tr-e1", all)
	}

	// from/to 窗口（UTC 下推，含边界）。
	from := time.Date(2026, 8, 27, 11, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)
	win, err := repo.ListTeamRunsForStatsExport(ctx, from, to, "", nil, 100)
	if err != nil {
		t.Fatalf("ListTeamRunsForStatsExport(window): %v", err)
	}
	if len(win) != 1 || win[0].ID != "tr-e2" {
		t.Fatalf("window = %+v, want only tr-e2", win)
	}

	// session_id 过滤跨 team。
	bySession, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "s-a", nil, 100)
	if err != nil {
		t.Fatalf("ListTeamRunsForStatsExport(session): %v", err)
	}
	if len(bySession) != 2 || bySession[0].ID != "tr-e2" || bySession[1].ID != "tr-e1" {
		t.Fatalf("session = %+v, want tr-e2,tr-e1 DESC", bySession)
	}

	// limit 截断。
	lim, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", nil, 2)
	if err != nil {
		t.Fatalf("ListTeamRunsForStatsExport(limit): %v", err)
	}
	if len(lim) != 2 || lim[0].ID != "tr-e3" {
		t.Fatalf("limit = %+v, want 2 rows head tr-e3", lim)
	}
}

// P5.1 M1：租户可见 team 过滤必须下推 SQL。nil=不限（system）；空非 nil
// 短路空集（租户无可见 team）；非空子集只命中这些 team 的 run。
func TestTeamRepo_ListTeamRunsForStatsExport_TeamIDsPushdown(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRepo(d)
	ctx := context.Background()

	mk := func(id, teamID string) {
		t.Helper()
		if _, err := repo.CreateTeamRun(ctx, biz.TeamRunRecord{
			ID: id, TeamID: teamID, Status: biz.TeamRunStatusSuccess,
			CreatedAt: "2026-08-27T10:00:00Z",
		}); err != nil {
			t.Fatalf("CreateTeamRun(%s): %v", id, err)
		}
	}
	mk("tr-p1", "t-a")
	mk("tr-p2", "t-b")
	mk("tr-p3", "t-sh")

	// nil = 不限：3 行全返回。
	all, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", nil, 100)
	if err != nil || len(all) != 3 {
		t.Fatalf("nil teamIDs = %d rows, err %v; want 3", len(all), err)
	}
	// 子集下推：只命中 t-a + t-sh（t-b 被 SQL 排除）。
	sub, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", []string{"t-a", "t-sh"}, 100)
	if err != nil {
		t.Fatalf("subset: %v", err)
	}
	if len(sub) != 2 {
		t.Fatalf("subset = %+v, want tr-p1+tr-p3", sub)
	}
	for _, r := range sub {
		if r.TeamID == "t-b" {
			t.Fatalf("subset leaked t-b run: %+v", sub)
		}
	}
	// 空非 nil = 租户无可见 team：短路空集，不报错。
	empty, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", []string{}, 100)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty teamIDs = %d rows, err %v; want 0 rows nil err", len(empty), err)
	}
}

// P5.1 排序稳定 + 亚秒边界：同 created_at 秒的多 run 按 id DESC 决胜
// （limit 截断可复现）；窗口边界带亚秒时按存储精度（秒）比较——修复前
// RFC3339Nano 格式化撞字典序陷阱（'.'<'Z'），10:00:00Z 行会被
// to=10:00:00.5 错误排除。
func TestTeamRepo_ListTeamRunsForStatsExport_StableOrderSubSecond(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRepo(d)
	ctx := context.Background()

	mk := func(id string) {
		t.Helper()
		if _, err := repo.CreateTeamRun(ctx, biz.TeamRunRecord{
			ID: id, TeamID: "t1", Status: biz.TeamRunStatusSuccess,
			CreatedAt: "2026-08-27T10:00:00Z",
		}); err != nil {
			t.Fatalf("CreateTeamRun(%s): %v", id, err)
		}
	}
	mk("tr-s1")
	mk("tr-s2")
	mk("tr-s3")

	// 同秒三行：id DESC 决胜（tr-s3 > tr-s2 > tr-s1）。
	rows, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", nil, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 || rows[0].ID != "tr-s3" || rows[1].ID != "tr-s2" || rows[2].ID != "tr-s1" {
		t.Fatalf("same-second order = %+v, want id DESC tr-s3,tr-s2,tr-s1", rows)
	}
	// limit=1 截断头部确定（排序不稳定时此行会随机失败）。
	head, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", nil, 1)
	if err != nil || len(head) != 1 || head[0].ID != "tr-s3" {
		t.Fatalf("limit-1 head = %+v, err %v; want tr-s3", head, err)
	}

	// 亚秒边界：to=10:00:00.5 必须命中 10:00:00Z 行（存储精度秒，边界
	// 截断后 LTE 10:00:00Z 成立）；from=10:00:00.5 同样命中（GTE 截断
	// 到 10:00:00Z，窗口向两侧各宽一秒是可接受语义）。
	from := time.Date(2026, 8, 27, 10, 0, 0, 500000000, time.UTC)
	to := from
	win, err := repo.ListTeamRunsForStatsExport(ctx, from, to, "", nil, 100)
	if err != nil {
		t.Fatalf("sub-second window: %v", err)
	}
	if len(win) != 3 {
		t.Fatalf("sub-second window = %d rows, want 3（亚秒截断到存储精度）", len(win))
	}
	// 远离窗口的亚秒边界仍排除：to=09:59:59.9 → 截断 09:59:59Z < 10:00:00Z。
	before := time.Date(2026, 8, 27, 9, 59, 59, 900000000, time.UTC)
	out, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, before, "", nil, 100)
	if err != nil {
		t.Fatalf("before window: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("before window = %d rows, want 0", len(out))
	}
}

// P5.1 M2：limit 单点收口于 repo——<=0 落默认 500；>1000 收硬上限 1000
// （修复前 >1000 被重置为 500，请求 1500 反比 1000 拿得少）。
func TestTeamRepo_ListTeamRunsForStatsExport_LimitClamp(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRepo(d)
	ctx := context.Background()

	// 造 1001 行（超过硬上限一行，验证精确收口）。
	for i := 0; i < 1001; i++ {
		if _, err := repo.CreateTeamRun(ctx, biz.TeamRunRecord{
			ID:        fmt.Sprintf("tr-l%04d", i),
			TeamID:    "t1",
			Status:    biz.TeamRunStatusSuccess,
			CreatedAt: time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("CreateTeamRun(%d): %v", i, err)
		}
	}

	// limit=0 → 默认 500。
	def, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", nil, 0)
	if err != nil || len(def) != 500 {
		t.Fatalf("limit=0 = %d rows, err %v; want 500（默认收口）", len(def), err)
	}
	// limit=1500 → 硬上限 1000（不是 500）。
	hard, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", nil, 1500)
	if err != nil || len(hard) != 1000 {
		t.Fatalf("limit=1500 = %d rows, err %v; want 1000（硬上限）", len(hard), err)
	}
	// limit=1000 边界值原样生效。
	exact, err := repo.ListTeamRunsForStatsExport(ctx, time.Time{}, time.Time{}, "", nil, 1000)
	if err != nil || len(exact) != 1000 {
		t.Fatalf("limit=1000 = %d rows, err %v; want 1000", len(exact), err)
	}
}
