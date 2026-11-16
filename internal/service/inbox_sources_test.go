package service

import (
	"testing"
	"time"

	cronv1 "aranea-agents/api/kratos/cron/v1"
	decisionv1 "aranea-agents/api/kratos/decision/v1"
	"aranea-agents/internal/biz/inbox"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var inboxTestNow = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

func hitlRecord(t *testing.T, key, outcome, cid, sessionID string, age time.Duration, agent string) *decisionv1.DecisionRecord {
	t.Helper()
	r := &decisionv1.DecisionRecord{
		DecisionKey: key,
		Category:    "hitl_approval",
		Outcome:     outcome,
		Scenario:    "shell.exec rm -rf build",
		Reasoning:   "需要确认：删除构建目录",
		ActorKey:    "actor-" + key,
		CreatedAt:   timestamppb.New(inboxTestNow.Add(-age)),
	}
	if cid != "" {
		md, err := structpb.NewStruct(map[string]any{"correlation_id": cid})
		if err != nil {
			t.Fatal(err)
		}
		r.Metadata = md
	}
	if sessionID != "" {
		r.SessionId = &sessionID
	}
	if agent != "" {
		lv, err := structpb.NewList([]any{map[string]any{"type": "agent", "key": agent}})
		if err != nil {
			t.Fatal(err)
		}
		r.RelatedEntities = lv
	}
	return r
}

func TestInboxPendingHITL(t *testing.T) {
	records := []*decisionv1.DecisionRecord{
		hitlRecord(t, "open", "asked", "c-open", "s1", 5*time.Minute, "coder"),
		// 已决：同 correlation_id 存在非 asked 记录
		hitlRecord(t, "done-ask", "asked", "c-done", "s1", 6*time.Minute, ""),
		hitlRecord(t, "done-ok", "approved", "c-done", "s1", 5*time.Minute, ""),
		// 超出可签窗口
		hitlRecord(t, "expired", "asked", "c-old", "s1", 31*time.Minute, ""),
		// 缺 correlation_id / 缺会话：无法定位决议目标
		hitlRecord(t, "no-cid", "asked", "", "s1", time.Minute, ""),
		hitlRecord(t, "no-session", "asked", "c-ns", "", time.Minute, ""),
	}
	got := inboxPendingHITL(records, inboxTestNow)
	if len(got) != 1 {
		t.Fatalf("want exactly the open item, got %d: %+v", len(got), got)
	}
	it := got[0]
	if it.NativeID != "open" || it.Kind != inbox.KindConfirm || it.Risk != inbox.RiskHigh {
		t.Fatalf("item = %+v", it)
	}
	if it.AgentKey != "coder" || it.Requester != "coder" {
		t.Fatalf("attribution = %q / %q", it.AgentKey, it.Requester)
	}
	if it.Ref["session_id"] != "s1" || it.Ref["activity_id"] != "c-open" {
		t.Fatalf("ref = %v", it.Ref)
	}
	if want := inboxTestNow.Add(-5 * time.Minute).Add(inboxHITLWindow); !it.DueAt.Equal(want) {
		t.Fatalf("due = %v want %v", it.DueAt, want)
	}
	if it.Link != "/chat?session=s1" {
		t.Fatalf("link = %q", it.Link)
	}
}

func TestInboxPendingHITLFallsBackToSourceRefAndActor(t *testing.T) {
	r := hitlRecord(t, "k", "ASKED", "c1", "", time.Minute, "")
	ref, _ := structpb.NewStruct(map[string]any{"session_id": "from-ref"})
	r.SourceRef = ref
	got := inboxPendingHITL([]*decisionv1.DecisionRecord{r}, inboxTestNow)
	if len(got) != 1 || got[0].Ref["session_id"] != "from-ref" {
		t.Fatalf("got %+v", got)
	}
	if got[0].AgentKey != "" || got[0].Requester != "actor-k" {
		t.Fatalf("unattributed record must not invent an agent: %+v", got[0])
	}
}

func cronRun(id, taskID, name string, age time.Duration, resolved bool) *cronv1.CronTaskRun {
	r := &cronv1.CronTaskRun{
		Id:           id,
		TaskId:       taskID,
		TaskName:     name,
		Status:       "failure",
		StartedAt:    inboxTestNow.Add(-age).Format(time.RFC3339),
		ErrorMessage: "boom " + id,
	}
	if resolved {
		r.ResolvedAt = inboxTestNow.Format(time.RFC3339)
	}
	return r
}

func TestInboxCronFailuresDedupesByTaskAndKeepsLatest(t *testing.T) {
	runs := []*cronv1.CronTaskRun{
		cronRun("r1", "t1", "日报", 3*time.Hour, false),
		cronRun("r2", "t1", "日报", time.Hour, false), // 同任务更近的一次
		cronRun("r3", "t2", "同步", 2*time.Hour, false),
		cronRun("r4", "t3", "太旧", 8*24*time.Hour, false), // 窗口外
		cronRun("r5", "t4", "已核销", time.Hour, true),
	}
	got := inboxCronFailures(runs, inboxTestNow)
	if len(got) != 2 {
		t.Fatalf("want 2 tasks, got %d: %+v", len(got), got)
	}
	if got[0].NativeID != "r2" || got[0].Title != "日报" || got[0].Summary != "boom r2" {
		t.Fatalf("t1 should resolve to its latest run: %+v", got[0])
	}
	if got[1].NativeID != "r3" {
		t.Fatalf("second = %+v", got[1])
	}
	for _, it := range got {
		if it.Kind != inbox.KindException || len(it.Actions) != 1 || it.Actions[0] != inbox.ActionResolve {
			t.Fatalf("item shape = %+v", it)
		}
	}
}

func TestNewInboxAggregatorRegistersOnlyWiredSources(t *testing.T) {
	if got := NewInboxAggregator(InboxDeps{}).Sources(); len(got) != 0 {
		t.Fatalf("no deps → no sources, got %v", got)
	}
	got := NewInboxAggregator(InboxDeps{Review: &ReviewService{}, Cron: &CronService{}, Decisions: &DecisionRecordService{}}).Sources()
	want := []string{inboxSourcePublish, inboxSourceOpportunity, inboxSourceCronFailure}
	if len(got) != len(want) {
		t.Fatalf("sources = %v want %v (hitl needs both decisions and chat)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sources = %v want %v", got, want)
		}
	}
}

func TestInboxOpportunityOpen(t *testing.T) {
	for status, want := range map[string]bool{"": true, "Pending": true, "new": true, " open ": true, "acked": false, "closed": false} {
		if got := inboxOpportunityOpen(status); got != want {
			t.Errorf("status %q: got %v want %v", status, got, want)
		}
	}
}
