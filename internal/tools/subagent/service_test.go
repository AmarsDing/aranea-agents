package subagent

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

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

// Nil construction-time runner is allowed; the per-session turn context is
// injected later via SetTurnContext (E2E-H4). Spawn must fail gracefully
// when neither is configured.

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
