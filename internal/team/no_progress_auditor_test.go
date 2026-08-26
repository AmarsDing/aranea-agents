package team

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

func TestNormalizeNoProgressStatus(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		"   \n\t ":                 "",
		"FAILED   to   Connect":    "failed to connect",
		"  Blocked：\t等待  上游 。 ": "blocked: 等待 上游", // 全角冒号折半角 + 空白压缩 + 尾句号去除
		"无法连接１２３。":             "无法连接123", // 全角数字折半角 + 尾句号去除
		"！！失败！！":                 "失败",       // 首尾标点去除
	}
	for in, want := range cases {
		if got := normalizeNoProgressStatus(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNoProgressStatusText(t *testing.T) {
	// ErrorMessage 优先于产出文本。
	asst := biz.ChatMessage{ErrorMessage: " dial tcp: timeout ", ContentMarkdown: "line1\nline2"}
	if got := noProgressStatusText(asst); got != "dial tcp: timeout" {
		t.Errorf("error-priority = %q", got)
	}
	// 取末个非空行。
	asst = biz.ChatMessage{ContentMarkdown: "过程行\n结论：仍无法推进\n\n  "}
	if got := noProgressStatusText(asst); got != "结论：仍无法推进" {
		t.Errorf("last-non-empty-line = %q", got)
	}
	// 全空 → 空串。
	if got := noProgressStatusText(biz.ChatMessage{}); got != "" {
		t.Errorf("empty = %q, want \"\"", got)
	}
	// 超长 ErrorMessage 截 400。
	long := strings.Repeat("x", 500)
	if got := noProgressStatusText(biz.ChatMessage{ErrorMessage: long}); len(got) != 400 {
		t.Errorf("truncation len = %d, want 400", len(got))
	}
}

func TestLooksStalled(t *testing.T) {
	for _, s := range []string{"无法连接数据库", "blocked by upstream", "仍在等待上游回复", "request timed out"} {
		if !looksStalled(normalizeNoProgressStatus(s)) {
			t.Errorf("looksStalled(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"已完成部署并验证", "结果如上所述", "任务完成"} {
		if looksStalled(normalizeNoProgressStatus(s)) {
			t.Errorf("looksStalled(%q) = true, want false", s)
		}
	}
}

// newNoProgressTestRunner 装配最小可观测 Runner：真实 RunRegistry（cancel
// 经 StoreCancelable 的 cancelFunc 观测）+ 记录型 followup enqueuer。
func newNoProgressTestRunner(t *testing.T, cfg NoProgressAuditorConfig) (*Runner, *rt.RunRegistry, *[]string) {
	t.Helper()
	r := &Runner{lg: loggateway.NewNoop()}
	r.cfg.NoProgressAudit = cfg
	reg := rt.NewRunRegistry()
	r.cfg.Runs = reg
	enqueued := &[]string{}
	r.SetNoProgressEnqueuer(func(_ context.Context, sessionID, content string) error {
		*enqueued = append(*enqueued, sessionID+"|"+content)
		return nil
	})
	return r, reg, enqueued
}

func stalledStep(msg string) biz.ChatMessage {
	return biz.ChatMessage{Status: biz.TeamMemberStepStatusError, ErrorMessage: msg}
}

// TestNoProgressAuditor_Ladder 验证核心剧本：同指纹阻塞 3 次纠偏 → 再 2 次
// 单发 Cancel；纠偏注记只发一次；Cancel 后同指纹继续到达不重发（单发守卫）。
func TestNoProgressAuditor_Ladder(t *testing.T) {
	cfg := NoProgressAuditorConfig{Enabled: true, CorrectAfter: 3, CancelAfter: 2}
	r, reg, enqueued := newNoProgressTestRunner(t, cfg)

	cancelled := make(chan string, 2)
	reg.StoreCancelable("sess-1", "run-1", func() { cancelled <- "run-1" })
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}
	ctx := context.Background()

	r.registerNoProgressAudit(run.ID, cfg)
	defer r.releaseNoProgressAudit(run.ID)

	step := stalledStep("无法连接数据库")
	// 第 1、2 次：阶梯未达，无纠偏。
	for i := 1; i <= 2; i++ {
		r.auditMemberNoProgress(ctx, run, "team-1", ag, step)
		if len(*enqueued) != 0 {
			t.Fatalf("step %d: nudge fired early (%d notes)", i, len(*enqueued))
		}
	}
	// 第 3 次：纠偏注记经 followup 入队，含成员 key 与连续计数。
	r.auditMemberNoProgress(ctx, run, "team-1", ag, step)
	if len(*enqueued) != 1 {
		t.Fatalf("step 3: nudge count = %d, want 1", len(*enqueued))
	}
	note := (*enqueued)[0]
	if !strings.HasPrefix(note, "sess-1|") || !strings.Contains(note, "worker-a") || !strings.Contains(note, "已连续 3 轮") {
		t.Errorf("nudge note malformed: %q", note)
	}
	// 第 4 次：纠偏单发不再重复；Cancel 阶梯未满。
	r.auditMemberNoProgress(ctx, run, "team-1", ag, step)
	if len(*enqueued) != 1 {
		t.Fatalf("step 4: nudge re-fired (%d notes)", len(*enqueued))
	}
	select {
	case <-cancelled:
		t.Fatal("step 4: cancel fired before CorrectAfter+CancelAfter")
	case <-time.After(50 * time.Millisecond):
	}
	// 第 5 次：纠偏后再 2 次同指纹 → 单发 Cancel（reason=no_progress）。
	r.auditMemberNoProgress(ctx, run, "team-1", ag, step)
	select {
	case got := <-cancelled:
		if got != "run-1" {
			t.Fatalf("cancel func fired for %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("step 5: cancel must fire via RunRegistry")
	}
	// 第 6 次：noProgTripped 单发守卫，不重复 Cancel。
	r.auditMemberNoProgress(ctx, run, "team-1", ag, step)
	select {
	case <-cancelled:
		t.Fatal("step 6: cancel must be single-fire")
	case <-time.After(50 * time.Millisecond):
	}
	if len(*enqueued) != 1 {
		t.Fatalf("step 6: nudge must stay single-fire, got %d", len(*enqueued))
	}
}

// TestNoProgressAuditor_SuccessResetsStreak 验证正常推进/成功结论重置该成员
// 连续计数（保留 corrected 语义——新阻塞指纹重走阶梯）。
func TestNoProgressAuditor_SuccessResetsStreak(t *testing.T) {
	cfg := NoProgressAuditorConfig{Enabled: true, CorrectAfter: 3, CancelAfter: 2}
	r, reg, enqueued := newNoProgressTestRunner(t, cfg)
	reg.StoreCancelable("sess-1", "run-1", func() {})
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}
	ctx := context.Background()
	r.registerNoProgressAudit(run.ID, cfg)
	defer r.releaseNoProgressAudit(run.ID)

	r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("无法连接数据库"))
	r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("无法连接数据库"))
	// 正常推进：成功结论无阻塞语义 → 重置。
	r.auditMemberNoProgress(ctx, run, "team-1", ag, biz.ChatMessage{
		Status: biz.TeamMemberStepStatusOK, ContentMarkdown: "已完成 srv-01 部署并验证",
	})
	// 再 2 次同指纹阻塞：连续计数从 0 重计，阶梯未达。
	r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("无法连接数据库"))
	r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("无法连接数据库"))
	if len(*enqueued) != 0 {
		t.Fatalf("success must reset streak; nudge count = %d", len(*enqueued))
	}
}

// TestNoProgressAuditor_RephraseResetsFingerprint 验证改述轮换产生新指纹、
// 连续计数重置（有意保守：宁漏勿冤，止损由 token 预算闸兜底）。
func TestNoProgressAuditor_RephraseResetsFingerprint(t *testing.T) {
	cfg := NoProgressAuditorConfig{Enabled: true, CorrectAfter: 3, CancelAfter: 2}
	r, reg, enqueued := newNoProgressTestRunner(t, cfg)
	reg.StoreCancelable("sess-1", "run-1", func() {})
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}
	ctx := context.Background()
	r.registerNoProgressAudit(run.ID, cfg)
	defer r.releaseNoProgressAudit(run.ID)

	r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("无法连接数据库"))
	r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("无法连接数据库"))
	// 改述：同语义不同措辞 → 新指纹，阶梯重走。
	r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("仍然无法连上 DB"))
	r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("仍然无法连上 DB"))
	if len(*enqueued) != 0 {
		t.Fatalf("rephrased fingerprint must reset streak; nudge count = %d", len(*enqueued))
	}
}

// TestNoProgressAuditor_StallMarkerWithoutErrorStatus 验证 ok 状态但结论带
// 阻塞语义（"等待上游"类）同样计入——成员每轮"成功"返回恰是本审计器场景。
func TestNoProgressAuditor_StallMarkerWithoutErrorStatus(t *testing.T) {
	cfg := NoProgressAuditorConfig{Enabled: true, CorrectAfter: 3, CancelAfter: 2}
	r, reg, enqueued := newNoProgressTestRunner(t, cfg)
	reg.StoreCancelable("sess-1", "run-1", func() {})
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}
	ctx := context.Background()
	r.registerNoProgressAudit(run.ID, cfg)
	defer r.releaseNoProgressAudit(run.ID)

	waiting := biz.ChatMessage{Status: biz.TeamMemberStepStatusOK, ContentMarkdown: "已尝试多种方案。\n仍在等待上游团队回复"}
	for i := 0; i < 3; i++ {
		r.auditMemberNoProgress(ctx, run, "team-1", ag, waiting)
	}
	if len(*enqueued) != 1 {
		t.Fatalf("ok-status stall text must count; nudge count = %d, want 1", len(*enqueued))
	}
}

// TestNoProgressAuditor_DisabledAndUnregisteredNoop 验证一键回退：Enabled=false
// 不装配审计状态；未登记/已释放 run 的 audit 调用全 no-op。
func TestNoProgressAuditor_DisabledAndUnregisteredNoop(t *testing.T) {
	cfg := NoProgressAuditorConfig{Enabled: false, CorrectAfter: 3, CancelAfter: 2}
	r, reg, enqueued := newNoProgressTestRunner(t, cfg)
	cancelled := make(chan string, 1)
	reg.StoreCancelable("sess-1", "run-1", func() { cancelled <- "run-1" })
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}
	ctx := context.Background()

	// Enabled=false：register 不装配。
	r.registerNoProgressAudit(run.ID, cfg)
	for i := 0; i < 6; i++ {
		r.auditMemberNoProgress(ctx, run, "team-1", ag, stalledStep("无法连接数据库"))
	}
	if len(*enqueued) != 0 {
		t.Fatalf("disabled auditor must be no-op; nudge count = %d", len(*enqueued))
	}
	// 启用配置但 run 未登记（或已释放）：no-op。
	r.registerNoProgressAudit("run-2", NoProgressAuditorConfig{Enabled: true, CorrectAfter: 3, CancelAfter: 2})
	r.releaseNoProgressAudit("run-2")
	run2 := biz.TeamRunRecord{ID: "run-2", SessionID: "sess-1"}
	for i := 0; i < 6; i++ {
		r.auditMemberNoProgress(ctx, run2, "team-1", ag, stalledStep("无法连接数据库"))
	}
	if len(*enqueued) != 0 {
		t.Fatalf("released run must be no-op; nudge count = %d", len(*enqueued))
	}
	select {
	case <-cancelled:
		t.Fatal("no-op paths must never cancel")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestNoProgressAuditor_CancelHitsRunSessionID 锁定 P2.5 会话口径：子会话
// 场景（SpiritSessionID=父会话）下 Cancel 与纠偏注记必须打 run.SessionID
// ——RunRegistry 以 sess.ID 登记 cancelable，打 SpiritSessionID 会落空或
// 误取消父会话的 run。
func TestNoProgressAuditor_CancelHitsRunSessionID(t *testing.T) {
	cfg := NoProgressAuditorConfig{Enabled: true, CorrectAfter: 1, CancelAfter: 1}
	r, reg, enqueued := newNoProgressTestRunner(t, cfg)
	childCancelled := make(chan string, 1)
	parentCancelled := make(chan string, 1)
	reg.StoreCancelable("child-sess", "run-1", func() { childCancelled <- "run-1" })
	reg.StoreCancelable("parent-spirit", "run-parent", func() { parentCancelled <- "run-parent" })
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "child-sess", SpiritSessionID: "parent-spirit"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}
	r.registerNoProgressAudit(run.ID, cfg)
	defer r.releaseNoProgressAudit(run.ID)

	// CorrectAfter=1 → 第 1 次纠偏；CancelAfter=1 → 第 2 次 Cancel。
	r.auditMemberNoProgress(context.Background(), run, "team-1", ag, stalledStep("无法连接数据库"))
	r.auditMemberNoProgress(context.Background(), run, "team-1", ag, stalledStep("无法连接数据库"))

	// 纠偏注记经 followup 打 run.SessionID（成员真正读历史的会话）。
	if len(*enqueued) != 1 || !strings.HasPrefix((*enqueued)[0], "child-sess|") {
		t.Fatalf("nudge must enqueue to run.SessionID, got %v", *enqueued)
	}
	select {
	case <-childCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel must hit run.SessionID registry entry")
	}
	select {
	case <-parentCancelled:
		t.Fatal("cancel must NOT touch SpiritSessionID registry entry")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestCancelReason_ReadsBackFromRegistry 锁定 P2.5 终态分列：审计器 Cancel
// 后，run 终态分支经 cancelReason 读回 no_progress（与 budget/user_cancel
// 分列），无条目/未取消时回退空串。
func TestCancelReason_ReadsBackFromRegistry(t *testing.T) {
	cfg := NoProgressAuditorConfig{Enabled: true, CorrectAfter: 1, CancelAfter: 1}
	r, reg, _ := newNoProgressTestRunner(t, cfg)
	reg.StoreCancelable("sess-1", "run-1", func() {})
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}

	if got := r.cancelReason("sess-1"); got != "" {
		t.Fatalf("before cancel: cancelReason = %q, want empty", got)
	}
	r.registerNoProgressAudit(run.ID, cfg)
	defer r.releaseNoProgressAudit(run.ID)
	r.auditMemberNoProgress(context.Background(), run, "team-1", ag, stalledStep("无法连接数据库"))
	r.auditMemberNoProgress(context.Background(), run, "team-1", ag, stalledStep("无法连接数据库"))
	if got := r.cancelReason("sess-1"); got != RunCancelReasonNoProgress {
		t.Fatalf("after auditor cancel: cancelReason = %q, want %q", got, RunCancelReasonNoProgress)
	}
	// nil registry / 未知会话 → 空串。
	var nilRunner *Runner
	if got := nilRunner.cancelReason("sess-1"); got != "" {
		t.Fatalf("nil runner: cancelReason = %q, want empty", got)
	}
	if got := r.cancelReason("sess-unknown"); got != "" {
		t.Fatalf("unknown session: cancelReason = %q, want empty", got)
	}
}
