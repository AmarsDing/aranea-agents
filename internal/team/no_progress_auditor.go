package team

// Run 级无进展审计器（79-runtime-governance R5，2026-08-26）。
//
// 背景：agent 级 max_tool_iterations 限单成员单 turn 轮数，调用级
// tool_loop_guard 拦同参数工具重发，token 预算闸限累计 input tokens——
// 但「成员连续多轮输出同语义阻塞状态」（每轮措辞微调、语义不动）此前无
// 任何 run 级检测：成员每轮都"成功"返回，预算缓慢燃烧，run 永不终止。
//
// 本审计器在成员 step 落库点（persistStep，与 recordMemberUsage 同点）
// 提取成员本轮结论状态文本（ErrorMessage 优先，否则取产出末行），归一化
// （大小写折叠 + NFC + 全角折半角 + 空白压缩 + 首尾标点去除，复用
// voice/normalizeConfirmWord 与 biz.NormalizeDeliverableTopic 的规则并集，
// 本场景允许大小写折叠——阻塞语义与大小写无关）后取 sha1 前 16 hex 作
// 状态指纹。O(1)，无额外 LLM 调用。
//
// 动作阶梯（per run × member）：
//   - 同指纹连续 CorrectAfter（默认 3）次 → 发布纠偏系统注记
//     （team_no_progress_nudge，指明"已 N 轮同状态，须改变策略或声明无法
//     推进"），单 run 单次；
//   - 纠偏后再连续 CancelAfter（默认 2）次同指纹 → RunRegistry.Cancel
//     （reason=team_no_progress），noProgTripped 单发守卫（复用
//     token_budget 的 budgetTripped 模式）。
//
// 与既有机制边界（design §6.4）：tool_loop_guard=调用级同参数；
// max_tool_iterations=agent 级轮数；token_budget=run 级 token 累计；
// 本审计器=run 级成员维度同语义无进展。
//
// 指纹归一化只覆盖码点/空白/标点/大小写微差异，不做改述级语义归并
// （无 LLM 调用约束）；同一阻塞语义的改述轮换（"无法连接"→"连不上"）
// 会产生不同指纹而重置计数——这是有意的保守：宁漏勿冤，止损由
// token 预算闸兜底。

import (
	"context"
	"crypto/sha1" //nolint:gosec // 非安全用途：状态指纹只需抗巧合碰撞
	"encoding/hex"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"golang.org/x/text/unicode/norm"
)

// RunCancelReasonNoProgress 是审计器终止 run 的 Cancel reason（P2.5 进
// run 事件流/状态，与 team_token_budget_exceeded、loop-guard 拦截计数分列）。
const RunCancelReasonNoProgress = "no_progress"

// NoProgressAuditorConfig 是消费侧配置（team 包不依赖 internal/conf，
// 由 wire 翻译 conf.RuntimeNoProgressAuditorConfig 注入）。Enabled=false
// 时 register 不装配、audit 调用全 no-op（一键回退）。
type NoProgressAuditorConfig struct {
	Enabled      bool
	CorrectAfter int // 同指纹连续 N 次触发纠偏注记（0 值由 conf 解析为 3）
	CancelAfter  int // 纠偏后再 N 次同指纹终止 run（0 值由 conf 解析为 2）
}

// noProgressMemberState 单成员在给定 run 内的连续状态追踪。
type noProgressMemberState struct {
	lastFP    string // 最近一次计入的状态指纹
	streak    int    // 同指纹连续计数
	corrected bool   // 纠偏注记是否已发（单 run 单次）
}

// noProgressStallMarkers 阻塞/失败/等待类语义标记（在归一化小写文本上
// 子串匹配）。指纹恒等才是真正的防冤闸（健康 run 几乎不可能同成员连续
// 5 轮产出逐字相同的结论）；本词表只是第一道粗筛，避免把"任务完成"类
// 重复成功结论误计为无进展。
var noProgressStallMarkers = []string{
	// EN
	"blocked", "failed", "failure", "awaiting", "waiting for", "unable",
	"cannot", "can't", "stuck", "no progress", "not progressing",
	"timed out", "timeout", "still failing",
	// ZH
	"无法", "失败", "阻塞", "等待", "未能", "卡住", "无进展", "暂无法",
	"重试仍", "仍未",
}

// registerNoProgressAudit 在 run 开始时装配审计状态（cfg 由调用方从
// r.cfg.NoProgressAudit 取出传入，与 registerRunTokenBudget 同型）；
// Enabled=false 或阈值非法时不装配（audit 调用对未登记 run 一律 no-op）。
func (r *Runner) registerNoProgressAudit(runID string, cfg NoProgressAuditorConfig) {
	if r == nil || runID == "" || !cfg.Enabled || cfg.CorrectAfter <= 0 || cfg.CancelAfter <= 0 {
		return
	}
	r.noProgMu.Lock()
	defer r.noProgMu.Unlock()
	if r.noProgRuns == nil {
		r.noProgRuns = make(map[string]map[string]*noProgressMemberState)
		r.noProgTripped = make(map[string]bool)
	}
	r.noProgRuns[runID] = make(map[string]*noProgressMemberState)
	r.noProgTripped[runID] = false
}

// releaseNoProgressAudit 在 run 结束时丢弃审计状态。
func (r *Runner) releaseNoProgressAudit(runID string) {
	if r == nil || runID == "" {
		return
	}
	r.noProgMu.Lock()
	defer r.noProgMu.Unlock()
	delete(r.noProgRuns, runID)
	delete(r.noProgTripped, runID)
}

// normalizeNoProgressStatus 状态指纹归一化：小写折叠 + NFC + 全角 ASCII
// 折半角 + 全部空白压缩为单空格 + 首尾标点去除。与 topic 归一（大小写
// 敏感）不同，阻塞状态语义与大小写无关，允许折叠。
func normalizeNoProgressStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	s = norm.NFC.String(s)
	s = strings.Map(func(r rune) rune {
		if r >= 0xFF01 && r <= 0xFF5E {
			return r - 0xFEE0
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	// 先 Trim 标点再 TrimSpace：压缩后形如 "...上游 。" 的尾标点被去除时
	// 会残留尾随空格（空格不在标点 cutset 内），指纹文本不留尾巴。
	s = strings.TrimSpace(strings.Trim(s, "。！？!?,.，、~～;；…:："))
	return s
}

// noProgressFingerprint 取归一化文本的 sha1 前 16 hex 作状态指纹。
func noProgressFingerprint(normalized string) string {
	sum := sha1.Sum([]byte(normalized)) //nolint:gosec // 非安全用途
	return hex.EncodeToString(sum[:8])
}

// noProgressStatusText 提取成员本轮结论状态文本：ErrorMessage 优先（错误
// 即最终结论），否则取产出文本末个非空行（成员结论惯于收尾），预截 400
// 字符防超长产出拖慢归一（指纹只需可区分，无需全文）。
func noProgressStatusText(asst biz.ChatMessage) string {
	if em := strings.TrimSpace(asst.ErrorMessage); em != "" {
		if len(em) > 400 {
			em = em[:400]
		}
		return em
	}
	content := strings.TrimSpace(asst.ContentMarkdown)
	if content == "" {
		return ""
	}
	if len(content) > 4000 {
		content = content[len(content)-4000:]
	}
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			if len(line) > 400 {
				line = line[:400]
			}
			return line
		}
	}
	return ""
}

// looksStalled 判定状态文本是否带阻塞/失败/等待语义（归一化小写文本上
// 子串匹配）。错误状态由调用方直接放行，不走本判定。
func looksStalled(normalized string) bool {
	for _, m := range noProgressStallMarkers {
		if strings.Contains(normalized, m) {
			return true
		}
	}
	return false
}

// auditMemberNoProgress 在成员 step 落库点记录状态指纹并执行动作阶梯。
// 与 recordMemberUsage 同点挂载（persistStep）；未登记/已终止 run、空状态
// 文本、无阻塞语义的成功结论一律快速返回（成功结论重置该成员连续计数）。
func (r *Runner) auditMemberNoProgress(ctx context.Context, run biz.TeamRunRecord, teamID string, ag biz.Agent, asst biz.ChatMessage) {
	if r == nil || run.ID == "" {
		return
	}
	raw := noProgressStatusText(asst)
	normalized := normalizeNoProgressStatus(raw)
	if normalized == "" {
		return
	}
	// 阻塞门：error 状态直接计入；否则须带阻塞语义标记。非阻塞结论
	// （成功/正常推进）重置该成员连续计数——"连续"语义只对同指纹阻塞计数。
	// skipped 是路由决策（成员本轮未被调度），不算阻塞也不算推进——其状态
	// 文本通常为空，已在上方空文本早退；走到这按非阻塞重置处理。
	isError := asst.Status == biz.TeamMemberStepStatusError
	stalled := isError || looksStalled(normalized)

	r.noProgMu.Lock()
	members, registered := r.noProgRuns[run.ID]
	if !registered {
		r.noProgMu.Unlock()
		return
	}
	key := ag.AgentKey
	if key == "" {
		key = ag.ID
	}
	st := members[key]
	if st == nil {
		st = &noProgressMemberState{}
		members[key] = st
	}
	fp := noProgressFingerprint(normalized)
	if !stalled {
		// 正常推进：重置连续计数（保留条目，避免 map 抖动）。
		st.lastFP = ""
		st.streak = 0
		st.corrected = false
		r.noProgMu.Unlock()
		return
	}
	if fp == st.lastFP {
		st.streak++
	} else {
		st.lastFP = fp
		st.streak = 1
		st.corrected = false // 新阻塞指纹重走阶梯
	}
	streak := st.streak
	corrected := st.corrected
	tripped := r.noProgTripped[run.ID]
	cfg := r.cfg.NoProgressAudit
	if !corrected && streak >= cfg.CorrectAfter {
		st.corrected = true
	}
	shouldCancel := !tripped && streak >= cfg.CorrectAfter+cfg.CancelAfter
	if shouldCancel {
		r.noProgTripped[run.ID] = true
	}
	r.noProgMu.Unlock()

	// 会话口径分列：Cancel 与纠偏注记必须打 run.SessionID（=sess.ID）——
	// RunRegistry 以 sess.ID 登记 cancelable（runner_team_trpc.go StoreRunner），
	// 子会话场景 SpiritSessionID 是父/根会话，打它会取消错目标或落空；
	// 纠偏注记走 followup 通道，与质量门修订同约定（runner_quality_gate.go
	// 用 sess.ID）。系统注记事件仍归因 SpiritSessionID（UI/审计轨）。
	spiritSID := run.SpiritSessionID
	if spiritSID == "" {
		spiritSID = run.SessionID
	}
	// 阶梯第一级：纠偏系统注记（单 run 单成员单次）。双通道投递：
	// followup 入队（成员下一 turn 携历史上下文真正读到，与质量门修订同
	// 通道）+ 系统注记事件（UI/审计轨）。enqueuer 拒收仅记日志，不阻断
	// 审计计数与后续 Cancel 裁决（service 装配注释同一约定）。
	if !corrected && streak >= cfg.CorrectAfter {
		msg := fmt.Sprintf("成员 %s 已连续 %d 轮处于相同阻塞状态，须改变策略或明确声明无法推进", key, streak)
		r.lg.Warn("run 级无进展审计：成员连续同指纹阻塞，注入纠偏注记",
			loggateway.StepID("team.no_progress.nudge"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("team_id", teamID),
			loggateway.Str("agent_key", key),
			loggateway.Int("streak", streak),
			loggateway.Str("fingerprint", fp),
		)
		if r.noProgressEnqueuer != nil {
			if err := r.noProgressEnqueuer(ctx, run.SessionID, msg); err != nil {
				r.lg.Warn("无进展纠偏注记入队失败（审计计数与 Cancel 裁决不受影响）",
					loggateway.StepID("team.no_progress.nudge_enqueue"),
					loggateway.Str("run_id", run.ID),
					loggateway.Str("agent_key", key),
					loggateway.Err(err),
				)
			}
		}
		r.publishEvent(ctx, biz.NewSystemNoticeEvent(spiritSID, "team_no_progress_nudge", msg, map[string]any{
			"run_id":      run.ID,
			"team_id":     teamID,
			"agent_key":   key,
			"streak":      streak,
			"fingerprint": fp,
		}))
		return
	}
	// 阶梯第二级：纠偏后仍同指纹 → 单发 Cancel（noProgTripped 守卫）。
	if shouldCancel {
		r.lg.Warn("run 级无进展审计：纠偏后仍无进展，取消 run",
			loggateway.StepID("team.no_progress.cancel"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("team_id", teamID),
			loggateway.Str("session_id", run.SessionID),
			loggateway.Str("agent_key", key),
			loggateway.Int("streak", streak),
			loggateway.Str("fingerprint", fp),
		)
		// M80：系统闸决策双写（设计 §3.2 row 3）。
		decision.EmitGate(ctx, r.cfg.DecisionCollector, decision.GateDecision{
			TriggerRule:   decision.TriggerNoProgressTripped,
			Outcome:       "tripped",
			Scenario:      "成员连续同语义阻塞，纠偏后仍无进展",
			Reasoning:     fmt.Sprintf("成员 %s 连续 %d 轮同指纹阻塞（纠偏 %d 轮后仍无进展），取消 run", key, streak, cfg.CorrectAfter),
			GuardName:     "no_progress_auditor",
			RunID:         run.ID,
			Entities:      []decision.EntityRef{{Type: "team", Key: teamID}, {Type: "agent", Key: key}},
			ObservedValue: streak,
			Threshold:     cfg.CorrectAfter + cfg.CancelAfter,
			Action:        "cancel_run",
			Extra:         map[string]any{"fingerprint": fp, "session_id": run.SessionID},
		})
		if em := event.TraceEmitterFromContext(ctx); em != nil {
			em.LogCritical("chat.team.no_progress", "成员连续同语义阻塞，纠偏无效，已取消 run",
				event.P("run_id", run.ID), event.P("agent_key", key), event.P("streak", streak))
		}
		r.publishEvent(ctx, biz.NewSystemNoticeEvent(spiritSID, "team_run_no_progress",
			"成员连续同语义阻塞状态，纠偏后仍无进展，run 已终止", map[string]any{
				"run_id":      run.ID,
				"team_id":     teamID,
				"agent_key":   key,
				"streak":      streak,
				"fingerprint": fp,
				"reason":      RunCancelReasonNoProgress,
			}))
		if r.cfg.Runs != nil {
			r.cfg.Runs.Cancel(run.SessionID, RunCancelReasonNoProgress)
		}
	}
}
