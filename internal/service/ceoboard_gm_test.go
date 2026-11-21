package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	officev1 "aranea-agents/api/kratos/office/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/ceoboard/orggraph"
	"aranea-agents/internal/biz/inbox"
	"aranea-agents/pkg/auth"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// gmFakeCron 是 cron.Repo + GMCronStore 的内存实现（只演练看板 / 总经理用到的读路径）。
type gmFakeCron struct {
	tasks    []biz.CronTask
	runs     []biz.CronTaskRun
	resolved []string
}

func (f *gmFakeCron) ListCronTasks(context.Context) ([]biz.CronTask, error) { return f.tasks, nil }
func (f *gmFakeCron) GetCronTask(context.Context, string) (biz.CronTask, error) {
	return biz.CronTask{}, errors.New("unused")
}
func (f *gmFakeCron) CreateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	f.tasks = append(f.tasks, t)
	return t, nil
}
func (f *gmFakeCron) UpdateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	return t, nil
}
func (f *gmFakeCron) DeleteCronTask(context.Context, string) error { return nil }
func (f *gmFakeCron) GetCronTaskRun(context.Context, string) (biz.CronTaskRun, error) {
	return biz.CronTaskRun{}, errors.New("unused")
}
func (f *gmFakeCron) ListCronTaskRuns(_ context.Context, q biz.CronTaskRunQuery) ([]biz.CronTaskRun, int, error) {
	var out []biz.CronTaskRun
	for _, r := range f.runs {
		if (q.TaskID == "" || r.TaskID == q.TaskID) && (q.Status == "" || r.Status == q.Status) && (q.IncludeResolved || r.ResolvedAt == "") {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, len(out), nil
}
func (f *gmFakeCron) InsertCronTaskRun(context.Context, biz.CronTaskRunInput) error { return nil }
func (f *gmFakeCron) UpdateCronTaskRun(context.Context, string, string, string, string, string) error {
	return nil
}
func (f *gmFakeCron) ResolveCronTaskFailures(context.Context, string, string) (biz.CronTaskRun, int, error) {
	return biz.CronTaskRun{}, 0, nil
}
func (f *gmFakeCron) CronTaskKeyExists(_ context.Context, key string) (bool, error) {
	for _, t := range f.tasks {
		if t.TaskKey == key {
			return true, nil
		}
	}
	return false, nil
}
func (f *gmFakeCron) ResolveCronTaskRunsThrough(_ context.Context, runID, _, _ string) (int, error) {
	f.resolved = append(f.resolved, runID)
	return 1, nil
}

func gmOut(t *testing.T, out gmRunOutput) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"gm": out})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func gmFixture(t *testing.T) *gmFakeCron {
	report := gmRunOutput{Kind: "report", Patrol: orggraph.Patrol{CompanyID: "co-1", Company: "一号公司", State: orggraph.StateAttention},
		AgentKey: "gm-key", Conclusion: "要看一眼：等你拍板 2 件", Reasons: []string{"调仓是高风险"}, Writer: "model"}
	older := report
	older.Conclusion = "旧的"
	return &gmFakeCron{
		tasks: []biz.CronTask{
			{ID: "tp", TaskKey: "gm_patrol:co-1", Enabled: true, AgentID: "ceo",
				MetadataJSON: `{"last_run_at":"2026-11-16T09:10:00+08:00","last_run_status":"success","next_run_at":"2026-11-16T10:10:00+08:00"}`},
			{ID: "tr", TaskKey: "gm_report:co-1", Enabled: true, AgentID: "ceo",
				MetadataJSON: `{"next_run_at":"2026-11-16T12:20:00+08:00"}`},
			{ID: "other", TaskKey: "daily", Enabled: true},
		},
		runs: []biz.CronTaskRun{
			{ID: "r-old", TaskID: "tr", Status: "success", CreatedAt: "2026-11-16T00:20:00Z", FinishedAt: "2026-11-16T00:21:00Z", OutputJSON: gmOut(t, older)},
			{ID: "r-new", TaskID: "tr", Status: "success", CreatedAt: "2026-11-16T04:20:00Z", FinishedAt: "2026-11-16T04:21:00Z", OutputJSON: gmOut(t, report)},
			{ID: "r-pending", TaskID: "tr", Status: "pending", CreatedAt: "2026-11-16T05:20:00Z"},
		},
	}
}

func superCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: 1, Roles: []string{"superadmin"}})
}

func TestBoardAttachGM(t *testing.T) {
	fake := gmFixture(t)
	s := NewBoardService(BoardDeps{Cron: biz.NewCronUsecase(fake, nil)}, nil)
	board := orggraph.Board{Companies: []orggraph.CompanyNode{{ID: "co-1"}, {ID: "co-2"}}}
	s.attachGM(superCtx(), &board, fake.tasks)

	gm := board.Companies[0].GM
	if gm == nil || !gm.Enabled || gm.LastPatrolAt != "2026-11-16T09:10:00+08:00" || gm.NextPatrolAt == "" || gm.NextReportAt != "2026-11-16T12:20:00+08:00" {
		t.Fatalf("gm = %+v", gm)
	}
	if r := gm.LastReport; r == nil || r.RunID != "r-new" || r.Conclusion != "要看一眼：等你拍板 2 件" || r.Writer != "model" || r.Acked {
		t.Fatalf("last report = %+v", gm.LastReport)
	}
	if board.Companies[1].GM != nil {
		t.Fatal("company without GM tasks must have no gm block")
	}
}

func TestInboxGMReportSource(t *testing.T) {
	fake := gmFixture(t)
	src := &inboxGMReportSource{cron: biz.NewCronUsecase(fake, nil), store: fake}

	if items, total, _ := src.List(context.Background(), 10); len(items) != 0 || total != 0 {
		t.Fatal("no principal → empty")
	}
	items, total, err := src.List(superCtx(), 10)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("items=%+v total=%d err=%v", items, total, err)
	}
	it := items[0]
	if it.NativeID != "r-new" || it.Kind != inbox.KindReport || it.AgentKey != "gm-key" || it.Risk != inbox.RiskMedium ||
		it.Summary != "要看一眼：等你拍板 2 件" || len(it.Actions) != 1 || it.Actions[0] != inbox.ActionAck {
		t.Fatalf("item = %+v", it)
	}
	if err := src.Decide(superCtx(), it, inbox.ActionApprove, ""); !errors.Is(err, inbox.ErrActionNotAllowed) {
		t.Fatalf("approve must be rejected, got %v", err)
	}
	if err := src.Decide(superCtx(), it, inbox.ActionAck, ""); err != nil || len(fake.resolved) != 1 || fake.resolved[0] != "r-new" {
		t.Fatalf("ack: err=%v resolved=%v", err, fake.resolved)
	}

	fake.runs[1].ResolvedAt = "2026-11-16T05:00:00Z"
	if items, _, _ := src.List(superCtx(), 10); len(items) != 0 {
		t.Fatalf("acked latest report must leave the inbox (older ones are superseded): %+v", items)
	}
}

func TestPickWorkspaceOwner(t *testing.T) {
	admins := []*biz.Admin{
		{ID: 5, Status: biz.AdminStatusActive, Roles: []string{"superadmin"}},
		{ID: 2, Status: "disabled", Roles: []string{"superadmin"}},
		{ID: 3, Status: biz.AdminStatusActive, Roles: []string{"user"}},
		{ID: 4, Status: biz.AdminStatusActive, Access: "admin"}, // 未迁移账号：按 access 平移角色
		{ID: 1, Status: biz.AdminStatusActive, WorkspaceID: "other", Roles: []string{"superadmin"}},
	}
	// 4 号按 access=admin 平移为 superadmin，是本工作区 id 最小的在用超管；1 号在别的工作区、2 号停用。
	if owner := pickWorkspaceOwner(admins, "default"); owner == nil || owner.ID != 4 {
		t.Fatalf("owner = %+v", owner)
	}
	if pickWorkspaceOwner(admins, "nobody") != nil {
		t.Fatal("workspace without superadmin → nil")
	}
}

func TestClassifyDelegated(t *testing.T) {
	created := timestamppb.New(time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC))
	tasks := []*officev1.OfficeTask{
		{Id: "a", Title: "写周报\n细节", SourceRef: "s1", AssigneeKind: "agent", AssigneeId: "ag1", CreatedAt: created},
		{Id: "b", Title: "", SourceRef: "s2", AssigneeKind: "team", AssigneeId: "tm1", CreatedAt: created},
		{Id: "c", Title: "没会话", AssigneeKind: "agent", AssigneeId: "ag2", CreatedAt: created},
	}
	latest := map[string]*sessionv1.TaskSituationV2{
		"s1": {SessionId: "s1", Status: "failed", UpdatedAt: ptrString("2026-11-16T02:00:00Z")},
		"s2": {SessionId: "s2", Status: "completed", UpdatedAt: ptrString("2026-11-16T03:00:00Z")},
	}
	got := classifyDelegated(tasks, latest)
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[0].ID != "office:a" || got[0].Title != "写周报" || got[0].State != orggraph.WorkStuck || got[0].At != "2026-11-16T02:00:00Z" || got[0].To != "/chat?session=s1&agent=ag1" {
		t.Fatalf("a = %+v", got[0])
	}
	if got[1].Title != "b" || got[1].State != orggraph.WorkReview || got[1].To != "/chat?session=s2&team=tm1" {
		t.Fatalf("b = %+v", got[1])
	}
	// 没有承载会话的任务落到会话中心里的这条任务
	if got[2].State != orggraph.WorkRunning || got[2].To != "/chat?task=office%3Ac" || got[2].At != "2026-11-16T01:00:00Z" {
		t.Fatalf("c = %+v", got[2])
	}
}
