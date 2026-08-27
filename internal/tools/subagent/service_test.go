package subagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 3); got != "hel" {
		t.Fatalf("expected hel, got %q", got)
	}
}

// P1-4：嵌套派生必须 fail-loud 拒绝（深度上限 = 1，产品设计硬约束）。
// 子代理 run 的 ctx 带 runtimeStateSubagentRun 标记；带标记的 ctx 再调
// subagents_spawn 必须返回 BadRequest，不得静默放行或截断。
func TestSpawnTool_NestedSpawnRejected(t *testing.T) {
	tool := newSpawnTool(&Service{})
	inv := trpcagent.NewInvocation()
	inv.RunOptions.RuntimeState = map[string]any{runtimeStateSubagentRun: true}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	_, err := tool.Call(ctx, []byte(`{"task":"nested"}`))
	if err == nil {
		t.Fatal("nested spawn must be rejected")
	}
	if !strings.Contains(err.Error(), "nested subagent spawn is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// P1-4 对照组：非嵌套 ctx 不得被深度守卫拦截（后续参数校验错误属正常路径）。
func TestSpawnTool_NonNestedCtxPassesDepthGuard(t *testing.T) {
	tool := newSpawnTool(&Service{})
	_, err := tool.Call(context.Background(), []byte(`{"task":"x"}`))
	if err != nil && strings.Contains(err.Error(), "nested subagent spawn") {
		t.Fatalf("non-nested ctx wrongly blocked by depth guard: %v", err)
	}
}

func TestTruncateRunes_NoLimit(t *testing.T) {
	if got := truncateRunes("hello", 0); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestTruncateRunes_UnderLimit(t *testing.T) {
	if got := truncateRunes("hi", 10); got != "hi" {
		t.Fatalf("expected hi, got %q", got)
	}
}

func TestTruncateRunes_TrimSpace(t *testing.T) {
	if got := truncateRunes("  hello  ", 10); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestSanitizeStoredResult(t *testing.T) {
	long := strings.Repeat("x", 5000)
	got := sanitizeStoredResult(long, defaultStoredResultRunes)
	if len([]rune(got)) != defaultStoredResultRunes {
		t.Fatalf("expected %d runes, got %d", defaultStoredResultRunes, len([]rune(got)))
	}
}

func TestSummarizeResult(t *testing.T) {
	long := strings.Repeat("y", 300)
	got := summarizeResult(long, defaultStoredSummaryRunes)
	if len([]rune(got)) != defaultStoredSummaryRunes {
		t.Fatalf("expected %d runes, got %d", defaultStoredSummaryRunes, len([]rune(got)))
	}
}

func TestNewChildSessionID(t *testing.T) {
	now := time.Now()
	id := newChildSessionID("run-123", now)
	if !strings.HasPrefix(id, subagentSessionPrefix) {
		t.Fatalf("expected prefix %q, got %q", subagentSessionPrefix, id)
	}
	if !strings.Contains(id, "run-123") {
		t.Fatalf("expected run-123 in id, got %q", id)
	}
}

func TestNewRequestID(t *testing.T) {
	now := time.Now()
	id := newRequestID("run-456", now)
	if !strings.HasPrefix(id, subagentRequestPrefix) {
		t.Fatalf("expected prefix %q, got %q", subagentRequestPrefix, id)
	}
}

func TestCloneTime(t *testing.T) {
	now := time.Now()
	p := cloneTime(now)
	if p == nil {
		t.Fatal("expected non-nil")
	}
	if !p.Equal(now) {
		t.Fatal("expected equal time")
	}
}

func TestNormalizeLoadedRuns_NonTerminal(t *testing.T) {
	now := time.Now()
	runs := map[string]*runRecord{
		"r1": {Run: trpcsubagent.Run{ID: "r1", Status: trpcsubagent.StatusQueued}},
	}
	changed := normalizeLoadedRuns(runs, now)
	if !changed {
		t.Fatal("expected changed for non-terminal run")
	}
	if runs["r1"].Status != trpcsubagent.StatusFailed {
		t.Fatalf("expected failed, got %q", runs["r1"].Status)
	}
	if runs["r1"].Error != "interrupted" {
		t.Fatalf("expected interrupted, got %q", runs["r1"].Error)
	}
}

func TestNormalizeLoadedRuns_TerminalUnchanged(t *testing.T) {
	now := time.Now()
	runs := map[string]*runRecord{
		"r1": {Run: trpcsubagent.Run{ID: "r1", Status: trpcsubagent.StatusCompleted, CreatedAt: now, UpdatedAt: now}},
	}
	changed := normalizeLoadedRuns(runs, now)
	if changed {
		t.Fatal("terminal run should not be changed")
	}
}

func TestNormalizeLoadedRuns_ZeroTimes(t *testing.T) {
	now := time.Now()
	runs := map[string]*runRecord{
		"r1": {Run: trpcsubagent.Run{ID: "r1", Status: trpcsubagent.StatusCompleted}},
	}
	changed := normalizeLoadedRuns(runs, now)
	if !changed {
		t.Fatal("zero times should be changed")
	}
	if runs["r1"].CreatedAt.IsZero() || runs["r1"].UpdatedAt.IsZero() {
		t.Fatal("zero times should be filled")
	}
}

func TestLoadRuns_MissingFile(t *testing.T) {
	runs, err := loadRuns(filepath.Join(t.TempDir(), "missing.json"), loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected empty, got %d", len(runs))
	}
}

func TestLoadRuns_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "runs.json")
	os.WriteFile(p, []byte{}, 0o644)
	runs, err := loadRuns(p, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected empty, got %d", len(runs))
	}
}

func TestSaveAndLoadRuns(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "subagents", "runs.json")
	now := time.Now()
	runs := map[string]*runRecord{
		"r1": {Run: trpcsubagent.Run{ID: "r1", Status: trpcsubagent.StatusCompleted, CreatedAt: now, UpdatedAt: now}, OwnerUserID: "user-1"},
	}
	if err := saveRuns(p, runs); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRuns(p, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	if loaded["r1"].ID != "r1" {
		t.Fatalf("expected r1, got %q", loaded["r1"].ID)
	}
	if loaded["r1"].OwnerUserID != "user-1" {
		t.Fatalf("expected user-1, got %q", loaded["r1"].OwnerUserID)
	}
}

func TestNewService_EmptyStateDir(t *testing.T) {
	_, err := NewService("", nil, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty state dir")
	}
}

func TestNewService_NilRunner(t *testing.T) {
	// Nil runner is allowed at construction; SetRunner is called later at runtime.
	svc, err := NewService(t.TempDir(), nil, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	// Spawn should fail gracefully when runner is not yet configured.
	svc.Start(context.Background())
	_, spawnErr := svc.Spawn(context.Background(), SpawnRequest{
		OwnerUserID:     "u1",
		ParentSessionID: "s1",
		Task:            "do something",
	})
	if spawnErr == nil {
		t.Fatal("expected error when spawning with nil runner")
	}
}

func TestService_ListForUser_NilService(t *testing.T) {
	var s *Service
	if s.ListForUser("user", trpcsubagent.ListFilter{}) != nil {
		t.Fatal("nil service should return nil")
	}
}

func TestService_GetForUser_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.GetForUser("user", "nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestService_CancelForUser_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, _, err = svc.CancelForUser("user", "nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestService_Spawn_NilService(t *testing.T) {
	var s *Service
	_, err := s.Spawn(context.Background(), SpawnRequest{})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestService_Spawn_NotStarted(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "s", Task: "t"})
	if err == nil {
		t.Fatal("expected error for not started")
	}
}

func TestService_Spawn_EmptyOwner(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "", ParentSessionID: "s", Task: "t"})
	if err == nil {
		t.Fatal("expected error for empty owner")
	}
}

func TestService_Spawn_EmptyParentSession(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "", Task: "t"})
	if err == nil {
		t.Fatal("expected error for empty parent session")
	}
}

func TestService_Spawn_EmptyTask(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	_, err = svc.Spawn(context.Background(), SpawnRequest{OwnerUserID: "u", ParentSessionID: "s", Task: ""})
	if err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestService_Close_NilService(t *testing.T) {
	var s *Service
	if err := s.Close(); err != nil {
		t.Fatalf("nil service Close should not error: %v", err)
	}
}

func TestService_Start_NilService(t *testing.T) {
	var s *Service
	s.Start(context.Background())
}

func TestRunRecord_PublicView_Nil(t *testing.T) {
	var r *runRecord
	if v := r.publicView(); v.ID != "" {
		t.Fatal("nil record should return zero Run")
	}
}

func TestRunRecord_Clone_Nil(t *testing.T) {
	var r *runRecord
	if c := r.clone(); c != nil {
		t.Fatal("nil clone should return nil")
	}
}

func TestRunRecord_Clone_DeepCopy(t *testing.T) {
	now := time.Now()
	r := &runRecord{
		Run: trpcsubagent.Run{
			ID:         "r1",
			StartedAt:  &now,
			FinishedAt: &now,
		},
	}
	c := r.clone()
	if c == r {
		t.Fatal("clone should be different pointer")
	}
	if c.StartedAt == r.StartedAt {
		t.Fatal("StartedAt should be deep copied")
	}
}

func TestReplyAccumulator_NilEvent(t *testing.T) {
	a := replyAccumulator{}
	a.consume(nil)
	if a.text != "" {
		t.Fatal("nil event should not produce text")
	}
}

func TestDecodeRunIDArgs_InvalidJSON(t *testing.T) {
	_, _, err := decodeRunIDArgs(context.Background(), []byte(`not json`), loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestDecodeRunIDArgs_EmptyID(t *testing.T) {
	_, _, err := decodeRunIDArgs(context.Background(), []byte(`{"id":""}`), loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

type stubRunner struct{}

func (s *stubRunner) Run(_ context.Context, _ string, _ string, _ trpcmodel.Message, _ ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
	ch := make(chan *trpcevent.Event)
	close(ch)
	return ch, nil
}

func (s *stubRunner) Close() error { return nil }

// ---- C4-③ 上行交付物压缩（S07 570K 根修）----

// subagentToolCtx 构造带 user/session 的 invocation ctx（工具 Call 的
// currentContext 依赖）。
func subagentToolCtx(userID, sessionID string) context.Context {
	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: sessionID, UserID: userID}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// seedFinishedRun 直接写入一条已完成 run（绕过执行路径，专注视图层）。
func seedFinishedRun(t *testing.T, svc *Service, runID, parentSID, owner, result, summary string) {
	t.Helper()
	svc.mu.Lock()
	svc.runs[runID] = &runRecord{
		Run: trpcsubagent.Run{
			ID:              runID,
			ParentSessionID: parentSID,
			Task:            "task",
			Status:          trpcsubagent.StatusCompleted,
			Result:          result,
			Summary:         summary,
		},
		OwnerUserID: owner,
	}
	svc.mu.Unlock()
}

// view 层单测：长结果截断到 ≤UpwardPipeMaxRunes + 标记与元数据；短结果
// 与空/缺字段不动。
func TestClipUpwardResultView(t *testing.T) {
	long := strings.Repeat("交", biz.UpwardPipeMaxRunes+100)
	view := map[string]any{"result": long}
	clipUpwardResultView(view)
	got := view["result"].(string)
	if !strings.Contains(got, "full_result=true") {
		t.Error("截断视图必须带 full_result 逃生舱标记")
	}
	if view["result_truncated"] != true {
		t.Error("result_truncated 元数据缺失")
	}
	if view["result_full_runes"] != biz.UpwardPipeMaxRunes+100 {
		t.Errorf("result_full_runes = %v, want %d", view["result_full_runes"], biz.UpwardPipeMaxRunes+100)
	}
	// 截断体 + 标记的总长应远小于原长（原长 2100 runes）。
	if n := utf8.RuneCountInString(got); n >= len([]rune(long)) {
		t.Errorf("截断后 %d runes，未有效压缩（原 %d）", n, len([]rune(long)))
	}

	short := map[string]any{"result": "短结果"}
	clipUpwardResultView(short)
	if short["result"] != "短结果" {
		t.Error("短结果不得被改动")
	}
	if _, ok := short["result_truncated"]; ok {
		t.Error("短结果不得带截断标记")
	}

	// 空/缺/非字符串字段不 panic、不改动。
	empty := map[string]any{}
	clipUpwardResultView(empty)
	nonStr := map[string]any{"result": 42}
	clipUpwardResultView(nonStr)
	if nonStr["result"] != 42 {
		t.Error("非字符串 result 不得被改动")
	}
}

// get 工具级：默认截断长结果；full_result=true 返回存储全量。
func TestGetTool_UpwardClipAndFullResultEscape(t *testing.T) {
	svc, err := NewService(t.TempDir(), &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	defer func() { _ = svc.Close() }()

	full := strings.Repeat("果", biz.UpwardPipeMaxRunes+500)
	seedFinishedRun(t, svc, "run-1", "sess-p", "u1", full, "摘要")
	ctx := subagentToolCtx("u1", "sess-p")
	tool := newGetTool(svc)

	// 默认：截断。
	res, err := tool.Call(ctx, []byte(`{"id":"run-1"}`))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	view := res.(map[string]any)
	got := view["result"].(string)
	if view["result_truncated"] != true {
		t.Fatal("默认路径必须截断长结果")
	}
	if !strings.Contains(got, "full_result=true") {
		t.Error("截断标记缺失")
	}
	if strings.Contains(got, strings.Repeat("果", biz.UpwardPipeMaxRunes+100)) {
		t.Error("截断视图仍含全量内容")
	}

	// 逃生舱：full_result=true 返回存储全量。
	res, err = tool.Call(ctx, []byte(`{"id":"run-1","full_result":true}`))
	if err != nil {
		t.Fatalf("get full_result: %v", err)
	}
	view = res.(map[string]any)
	if view["result"] != full {
		t.Error("full_result=true 必须返回存储全量")
	}
	if _, ok := view["result_truncated"]; ok {
		t.Error("全量视图不得带截断标记")
	}
}

// list 工具级：跟踪视图剥离 result、保留 summary。
func TestListTool_OmitsResultKeepsSummary(t *testing.T) {
	svc, err := NewService(t.TempDir(), &stubRunner{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	defer func() { _ = svc.Close() }()

	seedFinishedRun(t, svc, "run-1", "sess-p", "u1", strings.Repeat("果", 3000), "短摘要")
	ctx := subagentToolCtx("u1", "sess-p")
	tool := newListTool(svc)

	res, err := tool.Call(ctx, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lr := res.(listResult)
	if len(lr.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(lr.Runs))
	}
	if _, ok := lr.Runs[0]["result"]; ok {
		t.Error("list 视图不得携带 result（S07 回灌主因）")
	}
	if lr.Runs[0]["summary"] != "短摘要" {
		t.Errorf("summary = %v, want 短摘要", lr.Runs[0]["summary"])
	}
}
