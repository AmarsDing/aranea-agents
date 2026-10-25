package agent

// finish_observed_emit_test.go —— Q5 批 D3 发射侧钉板（94 三件套；
// 2026-10-22-review-framework-finish-reason-eof §4 验收口径）：
//  1. kind 判定边界：finish_reason ∈ {length, content_filter} 走小写化字面
//     （gemini UPPER_SNAKE 枚举拼写不映射）；EOF 仅当共享分类器同一判序
//     得出 StreamInvalidHTTPEOF——overflow+eof 共存、stall 耗尽、通用
//     stream error、safety 错误文本、silent 净关恒静默（排除语义随判序
//     免费获得）。
//  2. 会话级 5min 桶去重：同 session 同桶第二次零记录零日志；异 session
//     各自第一封；空 session 桶键回落 "unknown" 但 SourceRef.SessionID
//     不污染（保持空——占位仅用于防桶坍缩）。
//  3. ctx 取消 neutral：不发射且不占桶（恢复后同 session 第一封仍发）。
//  4. origin 三方推导（runID→team_turn / session→chat_turn / background）+
//     obs.Origin 显式值优先 + ctx 注入 session 优先覆盖 obs（T5 契约）。

import (
	"context"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

// finishEmitWarnLogger 只收 Warn（观测纪律：净关与去重静默必须零日志）。
type finishEmitWarnLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *finishEmitWarnLogger) Debug(string, ...loggateway.Field) {}
func (l *finishEmitWarnLogger) Info(string, ...loggateway.Field)  {}
func (l *finishEmitWarnLogger) Warn(msg string, _ ...loggateway.Field) {
	l.mu.Lock()
	l.warns = append(l.warns, msg)
	l.mu.Unlock()
}
func (l *finishEmitWarnLogger) Error(string, ...loggateway.Field)          {}
func (l *finishEmitWarnLogger) With(...loggateway.Field) loggateway.Logger { return l }

func (l *finishEmitWarnLogger) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.warns))
	copy(out, l.warns)
	return out
}

// ---------------------------------------------------------------------------
// kind 判定边界（纯函数）
// ---------------------------------------------------------------------------

func TestClassifyFinishObserved_Q5EmitSetBoundaries(t *testing.T) {
	cases := []struct {
		name string
		obs  provider.FinishObservation
		want string
	}{
		{"stop 正常收官不在事件面", provider.FinishObservation{SawFinishReason: true, FinishReason: "stop"}, ""},
		{"tool_calls 正常收官不在事件面", provider.FinishObservation{SawFinishReason: true, FinishReason: "tool_calls"}, ""},
		{"silent 净关不在事件面", provider.FinishObservation{}, ""},
		{"length 小写字面", provider.FinishObservation{SawFinishReason: true, FinishReason: "length"}, "length"},
		{"LENGTH 大写归一小写化", provider.FinishObservation{SawFinishReason: true, FinishReason: "LENGTH"}, "length"},
		{"length 带空白先修剪", provider.FinishObservation{SawFinishReason: true, FinishReason: " length "}, "length"},
		{"content_filter 小写字面", provider.FinishObservation{SawFinishReason: true, FinishReason: "content_filter"}, "content_filter"},
		{"CONTENT_FILTER 大写归一", provider.FinishObservation{SawFinishReason: true, FinishReason: "CONTENT_FILTER"}, "content_filter"},
		{"gemini MAX_TOKENS 不映射", provider.FinishObservation{SawFinishReason: true, FinishReason: "MAX_TOKENS"}, ""},
		{"max_tokens 枚举拼写不映射", provider.FinishObservation{SawFinishReason: true, FinishReason: "max_tokens"}, ""},
		{"纯 EOF 错误帧", provider.FinishObservation{HasError: true, LastError: "Post \"https://api.x/v1\": unexpected EOF", LastErrorType: "stream_error"}, "eof"},
		{"EOF 大小写混合错误文本", provider.FinishObservation{HasError: true, LastError: "stream error: Unexpected EOF", LastErrorType: "stream_error"}, "eof"},
		{"stall 耗尽恒排除", provider.FinishObservation{HasError: true, LastError: "liveness exhausted: reconnect budget spent", LastErrorType: "stream_error"}, ""},
		{"overflow+eof 共存判序防误报", provider.FinishObservation{HasError: true, LastError: "this model's maximum context length is 8192 tokens ... unexpected eof", LastErrorType: "stream_error"}, ""},
		{"通用 stream error 不在事件面", provider.FinishObservation{HasError: true, LastError: "500 upstream boom", LastErrorType: "stream_error"}, ""},
		{"safety 错误文本不进 EOF 面", provider.FinishObservation{HasError: true, LastError: "content_filter violation: blocked", LastErrorType: "stream_error"}, ""},
		{"stop+eof 共存按正常收官排除", provider.FinishObservation{SawFinishReason: true, FinishReason: "stop", HasError: true, LastError: "unexpected eof", LastErrorType: "stream_error"}, ""},
		{"finish length 优先于 EOF 错误帧", provider.FinishObservation{SawFinishReason: true, FinishReason: "length", HasError: true, LastError: "unexpected eof", LastErrorType: "stream_error"}, "length"},
	}
	for _, tc := range cases {
		if got := classifyFinishObserved(tc.obs); got != tc.want {
			t.Errorf("%s: classify = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 发射行为
// ---------------------------------------------------------------------------

// length 记录字段全钉 + 同桶第二封静默 + 异 session 各自一封。
func TestFinishObservedEmit_LengthRecordFieldsAndBucketDedupe(t *testing.T) {
	cc := &captureDecisionCollector{}
	lg := &finishEmitWarnLogger{}
	hooks := NewFinishObservedEmitHooks(cc, lg)
	obs := provider.FinishObservation{
		Provider: "openai", Model: "gpt-x", SessionID: "s-1",
		SawFinishReason: true, FinishReason: "length",
	}
	hooks.OnAbnormalFinish(context.Background(), obs)
	hooks.OnAbnormalFinish(context.Background(), obs) // 同 session 同桶：静默
	hooks.OnAbnormalFinish(context.Background(), provider.FinishObservation{
		SessionID: "s-2", SawFinishReason: true, FinishReason: "length",
	}) // 异 session：各自第一封
	if len(cc.recs) != 2 {
		t.Fatalf("records = %d, want 2（同桶 second 静默、异 session 各一封）", len(cc.recs))
	}
	if got := lg.messages(); len(got) != 2 {
		t.Fatalf("warns = %d, want 2（去重静默必须零日志）", len(got))
	}
	r := cc.recs[0]
	if r.Category != decision.CategorySystemGuard {
		t.Errorf("category = %q, want system_guard", r.Category)
	}
	if r.Outcome != "tripped" {
		t.Errorf("outcome = %q, want tripped", r.Outcome)
	}
	if r.ActorKey != "system:llm_finish_observer" {
		t.Errorf("actor_key = %q, want system:llm_finish_observer", r.ActorKey)
	}
	if r.SourceRef.SessionID != "s-1" {
		t.Errorf("source_ref.session_id = %q, want s-1", r.SourceRef.SessionID)
	}
	if r.SourceRef.RunID != "" {
		t.Errorf("source_ref.run_id = %q, want empty (chat 轮无 run)", r.SourceRef.RunID)
	}
	if !strings.Contains(r.Scenario, "异常收官") {
		t.Errorf("scenario = %q, want 含 异常收官", r.Scenario)
	}
	if !strings.Contains(r.Reasoning, `finish_reason="length"`) || !strings.Contains(r.Reasoning, "kind=length") {
		t.Errorf("reasoning = %q, want 含 finish_reason/kind", r.Reasoning)
	}
	md := r.Metadata
	if md["trigger_rule"] != decision.TriggerLLMAbnormalFinish {
		t.Errorf("metadata.trigger_rule = %v, want llm_abnormal_finish", md["trigger_rule"])
	}
	if md["action"] != "observe" {
		t.Errorf("metadata.action = %v, want observe", md["action"])
	}
	if md["kind"] != "length" || md["finish_reason"] != "length" {
		t.Errorf("metadata kind/finish_reason = %v/%v, want length/length", md["kind"], md["finish_reason"])
	}
	if md["provider"] != "openai" || md["model"] != "gpt-x" {
		t.Errorf("metadata provider/model = %v/%v", md["provider"], md["model"])
	}
	if md["origin"] != "chat_turn" {
		t.Errorf("metadata.origin = %v, want chat_turn（session 存在且无 runID）", md["origin"])
	}
	if md["session_id"] != "s-1" {
		t.Errorf("metadata.session_id = %v, want s-1", md["session_id"])
	}
	key, _ := md["dedupe_key"].(string)
	if !strings.HasPrefix(key, "llm_abnormal_finish:s-1:") {
		t.Errorf("dedupe_key = %q, want 前缀 llm_abnormal_finish:s-1:", key)
	}
}

// stop / silent 净关：复合零观测纪律——零记录零日志（Q5 §4 验收组合断言）。
func TestFinishObservedEmit_StopAndSilentCloseCompositeZero(t *testing.T) {
	cc := &captureDecisionCollector{}
	lg := &finishEmitWarnLogger{}
	hooks := NewFinishObservedEmitHooks(cc, lg)
	for i := 0; i < 3; i++ {
		hooks.OnAbnormalFinish(context.Background(), provider.FinishObservation{
			SessionID: "s-9", SawFinishReason: true, FinishReason: "stop",
		})
		hooks.OnAbnormalFinish(context.Background(), provider.FinishObservation{SessionID: "s-9"}) // silent 净关
	}
	if len(cc.recs) != 0 || len(lg.messages()) != 0 {
		t.Fatalf("stop/净关必须零记录零日志, recs=%d warns=%d", len(cc.recs), len(lg.messages()))
	}
}

// 空 session：桶键回落 unknown（防全进程坍缩同桶互吃），但 SourceRef/
// metadata.session_id 不被占位污染——审计口径只写真值。
func TestFinishObservedEmit_EmptySessionBucketUnknownNoSourceRefPollution(t *testing.T) {
	cc := &captureDecisionCollector{}
	lg := &finishEmitWarnLogger{}
	hooks := NewFinishObservedEmitHooks(cc, lg)
	obs := provider.FinishObservation{SawFinishReason: true, FinishReason: "content_filter"}
	hooks.OnAbnormalFinish(context.Background(), obs)
	hooks.OnAbnormalFinish(context.Background(), obs) // unknown 桶第二封静默
	if len(cc.recs) != 1 {
		t.Fatalf("records = %d, want 1（unknown 桶占位生效，第二封被压）", len(cc.recs))
	}
	r := cc.recs[0]
	if r.SourceRef.SessionID != "" {
		t.Errorf("source_ref.session_id = %q, want empty（unknown 仅桶键占位）", r.SourceRef.SessionID)
	}
	if _, has := r.Metadata["session_id"]; has {
		t.Errorf("metadata.session_id must be absent for empty session, got %v", r.Metadata["session_id"])
	}
	key, _ := r.Metadata["dedupe_key"].(string)
	if !strings.HasPrefix(key, "llm_abnormal_finish:unknown:") {
		t.Errorf("dedupe_key = %q, want 前缀 llm_abnormal_finish:unknown:", key)
	}
	if r.Metadata["origin"] != "background" {
		t.Errorf("origin = %v, want background（无 run 无 session）", r.Metadata["origin"])
	}
	if r.Metadata["kind"] != "content_filter" {
		t.Errorf("kind = %v, want content_filter", r.Metadata["kind"])
	}
}

// ctx 取消 neutral：不发射且不占同桶第一封名额（恢复后第一封仍发）。
func TestFinishObservedEmit_CancelledCtxNeutralDoesNotConsumeBucket(t *testing.T) {
	cc := &captureDecisionCollector{}
	lg := &finishEmitWarnLogger{}
	hooks := NewFinishObservedEmitHooks(cc, lg)
	obs := provider.FinishObservation{SessionID: "s-c", SawFinishReason: true, FinishReason: "length"}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	hooks.OnAbnormalFinish(canceled, obs)
	if len(cc.recs) != 0 || len(lg.messages()) != 0 {
		t.Fatalf("cancelled ctx must be silent, recs=%d warns=%d", len(cc.recs), len(lg.messages()))
	}
	hooks.OnAbnormalFinish(context.Background(), obs)
	if len(cc.recs) != 1 {
		t.Fatalf("桶未被取消帧消费：恢复后第一封应发, recs=%d", len(cc.recs))
	}
}

// origin 三方推导 + obs.Origin 显式值优先 + ctx session 注入覆盖 obs（T5）。
func TestFinishObservedEmit_OriginDerivationAndCtxOverride(t *testing.T) {
	cc := &captureDecisionCollector{}
	lg := &finishEmitWarnLogger{}
	hooks := NewFinishObservedEmitHooks(cc, lg)
	// team turn：runner 同坐标注入 run+session → origin=team_turn。
	teamCtx := decision.WithGateRunID(decision.WithGateSessionID(context.Background(), "s-team"), "run-9")
	hooks.OnAbnormalFinish(teamCtx, provider.FinishObservation{
		Provider: "p", Model: "m", SessionID: "s-team", SawFinishReason: true, FinishReason: "length",
	})
	// chat：仅 session → origin=chat_turn；ctx 注入 session 覆盖 obs 值。
	chatCtx := decision.WithGateSessionID(context.Background(), "s-ctx")
	hooks.OnAbnormalFinish(chatCtx, provider.FinishObservation{
		SessionID: "s-obs", SawFinishReason: true, FinishReason: "content_filter",
	})
	// 显式 Origin 优先于推导（eval_job 等后台命名面）。
	hooks.OnAbnormalFinish(context.Background(), provider.FinishObservation{
		Origin: "eval_job", SessionID: "s-x", SawFinishReason: true, FinishReason: "length",
	})
	if len(cc.recs) != 3 {
		t.Fatalf("records = %d, want 3", len(cc.recs))
	}
	if got := cc.recs[0].Metadata["origin"]; got != "team_turn" {
		t.Errorf("team origin = %v, want team_turn", got)
	}
	if got := cc.recs[0].SourceRef.RunID; got != "run-9" {
		t.Errorf("team run_id = %q, want run-9", got)
	}
	if got := cc.recs[1].SourceRef.SessionID; got != "s-ctx" {
		t.Errorf("ctx session override = %q, want s-ctx（T5：ctx 注入优先）", got)
	}
	if got := cc.recs[1].Metadata["origin"]; got != "chat_turn" {
		t.Errorf("chat origin = %v, want chat_turn", got)
	}
	if got := cc.recs[2].Metadata["origin"]; got != "eval_job" {
		t.Errorf("explicit origin = %v, want eval_job（显式值优先）", got)
	}
}

// EOF kind 记录：无 finish_reason 的 EOF 错误帧 → kind=eof、finish_reason 空。
func TestFinishObservedEmit_EOFErrorFrameMapsKindEOF(t *testing.T) {
	cc := &captureDecisionCollector{}
	lg := &finishEmitWarnLogger{}
	hooks := NewFinishObservedEmitHooks(cc, lg)
	hooks.OnAbnormalFinish(context.Background(), provider.FinishObservation{
		Provider: "openai", Model: "gpt-x", SessionID: "s-e",
		HasError: true, LastError: "stream error: unexpected EOF", LastErrorType: "stream_error",
	})
	if len(cc.recs) != 1 {
		t.Fatalf("records = %d, want 1", len(cc.recs))
	}
	r := cc.recs[0]
	if r.Metadata["kind"] != "eof" {
		t.Errorf("kind = %v, want eof", r.Metadata["kind"])
	}
	if r.Metadata["finish_reason"] != "" {
		t.Errorf("finish_reason = %v, want empty（无 finish 帧）", r.Metadata["finish_reason"])
	}
	if !strings.Contains(r.Reasoning, "kind=eof") {
		t.Errorf("reasoning = %q, want 含 kind=eof", r.Reasoning)
	}
}

// D3-f 对抗评审钉板（wf_a9d43a4f-956 双 major 确认，钉死旧盲区）：生产
// chat/graph 轮 ctx 恒带 invocation（runner.NewInvocationContext，内含
// 恒非空的 InvocationID）但绝无 WithGateRunID 注入。旧实现走 gateRunID 的
// invocation-id 回落 → origin 首支 `runID != ""` 恒命中 team_turn 且
// SourceRef.RunID 落 invocation uuid。钉板必须用生产 ctx 形态（invocation
// 在场），断言 origin 归 chat_turn/background 且 RunID 为空；合成裸 ctx
// 的旧用例结构性测不到这个分叉。
func TestFinishObservedEmit_ProductionCtxInvocationNotTeam(t *testing.T) {
	cc := &captureDecisionCollector{}
	lg := &finishEmitWarnLogger{}
	hooks := NewFinishObservedEmitHooks(cc, lg)
	// 生产 chat 形态：invocation 在场（runner 恒注入）+ session 归属
	// （gateSessionID 回落 inv.Session.ID）；无 WithGateRunID。
	inv := &trpcagent.Invocation{
		InvocationID: "inv-chat-uuid-1",
		Session:      &session.Session{ID: "s-prod-chat"},
	}
	chatCtx := trpcagent.NewInvocationContext(context.Background(), inv)
	hooks.OnAbnormalFinish(chatCtx, provider.FinishObservation{
		Provider: "openai", Model: "gpt-x",
		SawFinishReason: true, FinishReason: "length",
	})
	// 生产 graph/背景形态：invocation 在场但 session 为空 → background。
	graphInv := &trpcagent.Invocation{InvocationID: "inv-graph-uuid-2", Session: &session.Session{ID: ""}}
	hooks.OnAbnormalFinish(trpcagent.NewInvocationContext(context.Background(), graphInv), provider.FinishObservation{
		SawFinishReason: true, FinishReason: "content_filter",
	})
	if len(cc.recs) != 2 {
		t.Fatalf("records = %d, want 2（异 session/unknown 桶各一封）", len(cc.recs))
	}
	r := cc.recs[0]
	if got := r.Metadata["origin"]; got != "chat_turn" {
		t.Errorf("生产 chat ctx origin = %v, want chat_turn（invocation 非空不得被判 team）", got)
	}
	if got := r.SourceRef.RunID; got != "" {
		t.Errorf("生产 chat ctx SourceRef.RunID = %q, want empty（invocation uuid 不得落审计 RunID）", got)
	}
	if got := r.SourceRef.SessionID; got != "s-prod-chat" {
		t.Errorf("生产 chat ctx SourceRef.SessionID = %q, want s-prod-chat（gateSessionID 回落 inv.Session.ID 正用）", got)
	}
	r2 := cc.recs[1]
	if got := r2.Metadata["origin"]; got != "background" {
		t.Errorf("生产 graph ctx origin = %v, want background", got)
	}
	if got := r2.SourceRef.RunID; got != "" {
		t.Errorf("生产 graph ctx SourceRef.RunID = %q, want empty", got)
	}
}
