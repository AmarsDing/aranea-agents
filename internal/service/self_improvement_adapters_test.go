package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizmonitor "aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// ── SIAnalystAgent ───────────────────────────────────────────────────────────

func TestSIAnalystAgent_AnalyzeSuccess(t *testing.T) {
	caller := &fakeLLMCaller{resp: `{"root_cause":"空指针","affected_files":["internal/biz/x.go"],"impact_scope":"local","fix_strategy":"判空","confidence":0.8}`}
	agent := NewSIAnalystAgent(caller, "openai", "gpt-x", loggateway.NewNoop())
	run := &biz.SelfImprovementRun{ID: "run-1", TriggerSource: biz.TriggerSourceErrorCluster, BaseRef: "abc123"}
	sug := &biz.UnifiedEvolutionSuggestion{ID: "sug-1", DraftName: "修复 NPE", TriggerReason: "error cluster"}

	d, err := agent.Analyze(context.Background(), run, sug)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if d.RootCause != "空指针" || d.Confidence != 0.8 {
		t.Errorf("unexpected diagnosis: %+v", d)
	}
	if len(caller.reqs) != 1 {
		t.Fatalf("caller reqs = %d, want 1", len(caller.reqs))
	}
	req := caller.reqs[0]
	if req.System != biz.SIAnalystSystemPrompt {
		t.Error("system prompt must be biz.SIAnalystSystemPrompt")
	}
	if !strings.Contains(req.User, "run-1") || !strings.Contains(req.User, "修复 NPE") {
		t.Error("user message must carry run id + suggestion context")
	}
}

func TestSIAnalystAgent_NilCaller(t *testing.T) {
	agent := NewSIAnalystAgent(nil, "openai", "gpt-x", loggateway.NewNoop())
	if _, err := agent.Analyze(context.Background(), &biz.SelfImprovementRun{}, nil); err == nil {
		t.Fatal("want not-initialized error")
	}
}

func TestSIAnalystAgent_LLMError(t *testing.T) {
	caller := &fakeLLMCaller{err: errors.New("boom")}
	agent := NewSIAnalystAgent(caller, "openai", "gpt-x", loggateway.NewNoop())
	if _, err := agent.Analyze(context.Background(), &biz.SelfImprovementRun{}, nil); err == nil {
		t.Fatal("want llm error")
	}
}

// ── S5：格式纠正重试 ─────────────────────────────────────────────────────────

// 首次输出非法 JSON → 反馈解析错误重问一次，第二次合法 → 成功。
func TestSIAnalystAgent_ParseFailureFormatRetrySucceeds(t *testing.T) {
	caller := &fakeLLMCaller{queue: []fakeLLMReply{
		{resp: "not json at all"},
		{resp: `{"root_cause":"空指针","affected_files":[],"impact_scope":"local","fix_strategy":"判空","confidence":0.7}`},
	}}
	agent := NewSIAnalystAgent(caller, "openai", "gpt-x", loggateway.NewNoop())
	run := &biz.SelfImprovementRun{ID: "run-1", TriggerSource: biz.TriggerSourceErrorCluster}

	d, err := agent.Analyze(context.Background(), run, nil)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if d.RootCause != "空指针" {
		t.Errorf("RootCause = %q", d.RootCause)
	}
	if len(caller.reqs) != 2 {
		t.Fatalf("caller reqs = %d, want 2（首次 + 格式重试）", len(caller.reqs))
	}
	retry := caller.reqs[1]
	if retry.System != biz.SIAnalystSystemPrompt {
		t.Error("retry must reuse the analyst system prompt")
	}
	if !strings.Contains(retry.User, "not json at all") || !strings.Contains(retry.User, "无法解析") {
		t.Error("retry user message must carry parse feedback + bad output")
	}
}

// 两次都非法 → 返回解析错误，调用恰好 2 次。
func TestSIAnalystAgent_ParseFailureRetryExhausted(t *testing.T) {
	caller := &fakeLLMCaller{queue: []fakeLLMReply{
		{resp: "bad"},
		{resp: `{"root_cause":""}`},
	}}
	agent := NewSIAnalystAgent(caller, "openai", "gpt-x", loggateway.NewNoop())
	if _, err := agent.Analyze(context.Background(), &biz.SelfImprovementRun{ID: "run-1"}, nil); err == nil {
		t.Fatal("want parse error after retry exhausted")
	}
	if len(caller.reqs) != 2 {
		t.Fatalf("caller reqs = %d, want exactly 2", len(caller.reqs))
	}
}

// LLM 调用本身失败（非解析失败）→ 直接返回，不触发格式重试。
func TestSIAnalystAgent_LLMErrorNoFormatRetry(t *testing.T) {
	caller := &fakeLLMCaller{queue: []fakeLLMReply{
		{err: errors.New("boom")},
		{resp: `{"root_cause":"x","impact_scope":"local","fix_strategy":"y","confidence":0.5}`},
	}}
	agent := NewSIAnalystAgent(caller, "openai", "gpt-x", loggateway.NewNoop())
	if _, err := agent.Analyze(context.Background(), &biz.SelfImprovementRun{ID: "run-1"}, nil); err == nil {
		t.Fatal("want llm error")
	}
	if len(caller.reqs) != 1 {
		t.Fatalf("llm 错误不应格式重试, reqs = %d, want 1", len(caller.reqs))
	}
}

// 首次合法 → 不重试（只调用 1 次）。
func TestSIAnalystAgent_ValidFirstTryNoRetry(t *testing.T) {
	caller := &fakeLLMCaller{resp: `{"root_cause":"x","impact_scope":"local","fix_strategy":"y","confidence":0.5}`}
	agent := NewSIAnalystAgent(caller, "openai", "gpt-x", loggateway.NewNoop())
	if _, err := agent.Analyze(context.Background(), &biz.SelfImprovementRun{ID: "run-1"}, nil); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(caller.reqs) != 1 {
		t.Fatalf("合法输出不应重试, reqs = %d, want 1", len(caller.reqs))
	}
}

// ── SIPatcherAgent ───────────────────────────────────────────────────────────

func siPatchArgs(worktree string) biz.SIPatchRequest {
	return biz.SIPatchRequest{
		Run:          &biz.SelfImprovementRun{ID: "run-1", TriggerSource: biz.TriggerSourceErrorCluster},
		Diagnosis:    &biz.Diagnosis{RootCause: "空指针", AffectedFiles: []string{"internal/biz/x.go"}, FixStrategy: "判空"},
		WorktreePath: worktree,
		Attempt:      1,
	}
}

func TestSIPatcherAgent_PatchSuccess_InlinesWorktreeFile(t *testing.T) {
	worktree := t.TempDir()
	full := filepath.Join(worktree, "internal", "biz")
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "x.go"), []byte("package biz\n// v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	caller := &fakeLLMCaller{resp: `{"diff":"diff --git a/internal/biz/x.go b/internal/biz/x.go\n--- a/internal/biz/x.go\n+++ b/internal/biz/x.go\n@@ -1,2 +1,2 @@\n-// v1\n+// v2\n","kind":"code"}`}
	agent := NewSIPatcherAgent(caller, "openai", "gpt-x", 20, loggateway.NewNoop())

	out, err := agent.Patch(context.Background(), siPatchArgs(worktree))
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if out.Kind != biz.PatchKindCode {
		t.Errorf("kind = %q, want code", out.Kind)
	}
	req := caller.reqs[0]
	if !strings.Contains(req.User, "// v1") {
		t.Error("user message must inline affected worktree file content")
	}
	if !strings.Contains(req.User, "空指针") {
		t.Error("user message must carry the diagnosis")
	}
}

func TestSIPatcherAgent_QuotaExhausted(t *testing.T) {
	caller := &fakeLLMCaller{resp: `{"diff":"d","kind":"code"}`}
	agent := NewSIPatcherAgent(caller, "openai", "gpt-x", 1, loggateway.NewNoop())
	req := siPatchArgs("")
	if _, err := agent.Patch(context.Background(), req); err != nil {
		t.Fatalf("first Patch: %v", err)
	}
	if _, err := agent.Patch(context.Background(), req); !errors.Is(err, ErrSIPatcherQuotaExceeded) {
		t.Fatalf("second Patch err = %v, want ErrSIPatcherQuotaExceeded", err)
	}
}

func TestSIPatcherAgent_NilCaller(t *testing.T) {
	agent := NewSIPatcherAgent(nil, "openai", "gpt-x", 20, loggateway.NewNoop())
	if _, err := agent.Patch(context.Background(), siPatchArgs("")); err == nil {
		t.Fatal("want not-initialized error")
	}
}

// S5：首次输出非法 JSON → 反馈重问，第二次合法 → 成功（消耗 2 单位配额）。
func TestSIPatcherAgent_ParseFailureFormatRetrySucceeds(t *testing.T) {
	caller := &fakeLLMCaller{queue: []fakeLLMReply{
		{resp: "here is your patch: …"},
		{resp: `{"diff":"diff --git a/internal/biz/x.go b/internal/biz/x.go\n--- a/internal/biz/x.go\n+++ b/internal/biz/x.go\n@@ -1 +1 @@\n-a\n+b\n","kind":"code"}`},
	}}
	agent := NewSIPatcherAgent(caller, "openai", "gpt-x", 20, loggateway.NewNoop())
	out, err := agent.Patch(context.Background(), siPatchArgs(""))
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if out.Kind != biz.PatchKindCode {
		t.Errorf("kind = %q, want code", out.Kind)
	}
	if len(caller.reqs) != 2 {
		t.Fatalf("caller reqs = %d, want 2（首次 + 格式重试）", len(caller.reqs))
	}
	if !strings.Contains(caller.reqs[1].User, "here is your patch") || !strings.Contains(caller.reqs[1].User, "无法解析") {
		t.Error("retry user message must carry parse feedback + bad output")
	}
	// 重试后配额只剩 18：再发起一次 Patch 仍应成功（queue 尾元素重复）。
	if _, err := agent.Patch(context.Background(), siPatchArgs("")); err != nil {
		t.Fatalf("third Patch: %v", err)
	}
}

// S5：两次都非法 → 返回解析错误；调用恰好 2 次。
func TestSIPatcherAgent_ParseFailureRetryExhausted(t *testing.T) {
	caller := &fakeLLMCaller{queue: []fakeLLMReply{
		{resp: "bad"},
		{resp: `{"diff":"","kind":"code"}`},
	}}
	agent := NewSIPatcherAgent(caller, "openai", "gpt-x", 20, loggateway.NewNoop())
	if _, err := agent.Patch(context.Background(), siPatchArgs("")); err == nil {
		t.Fatal("want parse error after retry exhausted")
	}
	if len(caller.reqs) != 2 {
		t.Fatalf("caller reqs = %d, want exactly 2", len(caller.reqs))
	}
}

// S5：重试时配额耗尽 → 放弃重试，返回原始解析错误（不吞配额错误掩盖格式问题）。
func TestSIPatcherAgent_RetryQuotaExhaustedReturnsParseError(t *testing.T) {
	caller := &fakeLLMCaller{resp: "not json"}
	agent := NewSIPatcherAgent(caller, "openai", "gpt-x", 1, loggateway.NewNoop())
	_, err := agent.Patch(context.Background(), siPatchArgs(""))
	if err == nil {
		t.Fatal("want parse error")
	}
	if errors.Is(err, ErrSIPatcherQuotaExceeded) {
		t.Fatal("重试配额耗尽时应返回解析错误而非配额错误")
	}
	if len(caller.reqs) != 1 {
		t.Fatalf("配额耗尽不应发起第二次 LLM 调用, reqs = %d, want 1", len(caller.reqs))
	}
}

// ── siReadWorktreeFile path safety ───────────────────────────────────────────

func TestSIReadWorktreeFile_BlocksTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := siReadWorktreeFile(root, "../outside.txt", 1024); ok {
		t.Error("path traversal must be blocked")
	}
	if _, ok := siReadWorktreeFile(root, "ok.txt", 1024); !ok {
		t.Error("in-root file must be readable")
	}
	if _, ok := siReadWorktreeFile(root, "missing.txt", 1024); ok {
		t.Error("missing file must report not-ok")
	}
}

func TestSIReadWorktreeFile_BudgetTruncates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(strings.Repeat("x", 2048)), 0o644); err != nil {
		t.Fatal(err)
	}
	content, ok := siReadWorktreeFile(root, "big.txt", 64)
	if !ok {
		t.Fatal("want readable")
	}
	if len(content) > 128 || !strings.Contains(content, "truncated") {
		t.Errorf("content not truncated within budget: len=%d", len(content))
	}
}

// ── SIMonitorApprovalSink idempotency ────────────────────────────────────────

type siFakeMonitorEvents struct {
	inserted []bizmonitor.EventWrite
	items    []bizmonitor.PlatformRow
}

func (f *siFakeMonitorEvents) InsertMonitorEvent(_ context.Context, ev bizmonitor.EventWrite) error {
	f.inserted = append(f.inserted, ev)
	return nil
}
func (f *siFakeMonitorEvents) ListMonitorEvents(_ context.Context, _ bizmonitor.EventsQuery) (bizmonitor.ListResult, error) {
	return bizmonitor.ListResult{Items: f.items}, nil
}
func (f *siFakeMonitorEvents) GetMonitorEvent(_ context.Context, id string) (bizmonitor.PlatformRow, error) {
	return bizmonitor.PlatformRow{}, nil
}
func (f *siFakeMonitorEvents) CountMonitorEventsSince(_ context.Context, _, _, _, _ string) (int32, error) {
	return 0, nil
}
func (f *siFakeMonitorEvents) DeleteMonitorEventsOlderThan(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

func TestSIMonitorApprovalSink_IdempotentPerRun(t *testing.T) {
	events := &siFakeMonitorEvents{}
	sink := NewSIMonitorApprovalSink(events, loggateway.NewNoop())
	run := &biz.SelfImprovementRun{ID: "run-9", RiskLevel: "high", PatchKind: biz.PatchKindCode}

	id1, err := sink.SubmitApproval(context.Background(), run)
	if err != nil {
		t.Fatalf("SubmitApproval: %v", err)
	}
	if id1 != "si-approval:run-9" {
		t.Errorf("approval id = %q", id1)
	}
	// 模拟重启后：近期事件里已有该 run 的请求 → 不再插入。
	events.items = []bizmonitor.PlatformRow{{MetadataJSON: `{"run_id":"run-9"}`}}
	if _, err := sink.SubmitApproval(context.Background(), run); err != nil {
		t.Fatalf("second SubmitApproval: %v", err)
	}
	if len(events.inserted) != 1 {
		t.Fatalf("inserted = %d, want 1 (idempotent)", len(events.inserted))
	}
}

// ── SIKBNegativePatternSink ──────────────────────────────────────────────────

type siFakePatternKB struct {
	byHash     map[string]*bizmonitor.FailurePattern
	created    []bizmonitor.FailurePattern
	incrementd []string
}

func (f *siFakePatternKB) GetByPatternHash(_ context.Context, hash string) (*bizmonitor.FailurePattern, error) {
	return f.byHash[hash], nil
}
func (f *siFakePatternKB) Create(_ context.Context, p bizmonitor.FailurePattern) error {
	f.created = append(f.created, p)
	return nil
}
func (f *siFakePatternKB) IncrementFail(_ context.Context, id string) error {
	f.incrementd = append(f.incrementd, id)
	return nil
}

func TestSIKBNegativePatternSink_CreateNew(t *testing.T) {
	kb := &siFakePatternKB{byHash: map[string]*bizmonitor.FailurePattern{}}
	sink := NewSIKBNegativePatternSink(kb, loggateway.NewNoop())
	rec := biz.SINegativePatternRecord{RunID: "run-1", TriggerSource: biz.TriggerSourceErrorCluster, PatternHash: "h1", PatternRegex: "x.go"}
	if err := sink.RecordNegativePattern(context.Background(), rec); err != nil {
		t.Fatalf("RecordNegativePattern: %v", err)
	}
	if len(kb.created) != 1 {
		t.Fatalf("created = %d, want 1", len(kb.created))
	}
	p := kb.created[0]
	if string(p.Source) != "self_improvement" || p.FailCount != 1 || !p.IsActive {
		t.Errorf("unexpected pattern: %+v", p)
	}
	if p.FixAction.Type != "log_only" {
		t.Errorf("negative pattern must be log_only, got %q", p.FixAction.Type)
	}
}

func TestSIKBNegativePatternSink_ExistingIncrementsFail(t *testing.T) {
	kb := &siFakePatternKB{byHash: map[string]*bizmonitor.FailurePattern{
		"h1": {ID: "p1", PatternHash: "h1"},
	}}
	sink := NewSIKBNegativePatternSink(kb, loggateway.NewNoop())
	rec := biz.SINegativePatternRecord{RunID: "run-2", TriggerSource: biz.TriggerSourceErrorCluster, PatternHash: "h1"}
	if err := sink.RecordNegativePattern(context.Background(), rec); err != nil {
		t.Fatalf("RecordNegativePattern: %v", err)
	}
	if len(kb.created) != 0 {
		t.Fatalf("created = %d, want 0 (dedup)", len(kb.created))
	}
	if len(kb.incrementd) != 1 || kb.incrementd[0] != "p1" {
		t.Fatalf("IncrementFail = %v, want [p1]", kb.incrementd)
	}
}

// ── SIOrchestratorFeedbackSink ───────────────────────────────────────────────

func TestSIOrchestratorFeedbackSink_Escalates(t *testing.T) {
	orch := biz.NewSkillEvolutionOrchestrator(nil, nil, nil, loggateway.NewNoop())
	sink := NewSIOrchestratorFeedbackSink(orch)
	if err := sink.EscalateTriggerCooldown(context.Background(), biz.TriggerSourceErrorCluster, 2); err != nil {
		t.Fatalf("EscalateTriggerCooldown: %v", err)
	}
	// 无读取接口——再升一次验证不 panic 且链路可用即可（乘数上限语义见 biz 单测）。
	if err := sink.EscalateTriggerCooldown(context.Background(), biz.TriggerSourceErrorCluster, 2); err != nil {
		t.Fatalf("second EscalateTriggerCooldown: %v", err)
	}
}

func TestSIMonitorNotifier_InsertsWarnEvent(t *testing.T) {
	events := &siFakeMonitorEvents{}
	n := NewSIMonitorNotifier(events, loggateway.NewNoop())
	run := &biz.SelfImprovementRun{ID: "run-1", SuggestionID: "sug-1"}
	if err := n.NotifySelfImprovement(context.Background(), run, "已自动应用"); err != nil {
		t.Fatalf("NotifySelfImprovement: %v", err)
	}
	if len(events.inserted) != 1 {
		t.Fatalf("inserted = %d, want 1", len(events.inserted))
	}
	ev := events.inserted[0]
	if ev.EventKey != "self_improvement.notify" || ev.Status != "warn" {
		t.Errorf("unexpected event: %+v", ev)
	}
	if !strings.Contains(ev.MetadataJSON, `"run_id":"run-1"`) {
		t.Errorf("metadata must carry run_id: %s", ev.MetadataJSON)
	}
}
