package service

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"

	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"
)

const (
	// speculativeResolveWaitCap 是 final 到达而投机仍在途时的有界等待上限。
	// 收益窗口 ≈ 常规意图耗时 - 构建耗时（预热后 ~0.3s）；超过即放弃投机，
	// Turn 走常规意图路径（有界兜底，不无限阻塞 Turn 派发）。
	speculativeResolveWaitCap = 2 * time.Second
	// speculativeSlotTTL 是投机槽过期时间（兜底清理 final 从未到达的孤儿槽）。
	speculativeSlotTTL = 30 * time.Second
	// speculativeCallTimeout 是单次投机意图 LLM 调用超时（独立于连接存活）。
	speculativeCallTimeout = 15 * time.Second
)

// speculativeIntentSlot 是一次投机意图的槽位（每会话单槽，新 partial 取代旧槽）。
type speculativeIntentSlot struct {
	sourceText string // 触发投机的 partial 归一化文本（final 精确匹配校验）
	matchText  string // sourceText 的匹配归一化形（小写、去标点/空白）
	createdAt  time.Time
	done       chan struct{}    // 完成（含失败）时关闭
	art        *intent.Artifact // done 关闭后有效；nil = 跳过/失败
}

// normalizeSpeculativeMatchText 归一化 ASR 文本用于 partial/final 匹配（P1-F）：
// 转小写、去 Unicode 标点与所有空白。ASR final 常对 partial 做标点增删润色
// （补句号、去逗号），归一化后语义等价即复用投机产物 —— refined_goal 语义
// 不变，零误复用风险；文本实体差异（用户改口/ASR 纠错）归一化后仍不同，
// 照常失配丢弃。
func normalizeSpeculativeMatchText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsPunct(r) || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// VoiceIntentSpeculator 实现 voice.IntentSpeculator（C2，2026-08-11）。
//
// ASR partial 稳定 500ms 后后台预跑意图识别（投机阶梯 L2）：意图识别
// 1-2s 的 LLM 调用从「final → Turn」关键路径移到「用户说话尾部」空窗期。
// ASR final 时（L3 判定）文本与投机源一致则注入产物复用，失配/超时/失败
// 即丢弃，Turn 走常规意图路径。
//
// 一致性保障（设计 §14.5）：槽位携带 sourceText（sourceHash 语义）+
// createdAt（expiresAt 语义），final 精确匹配校验，失配即丢弃。
// 非阻断容错（K3）：任何失败仅记日志，语音链路回退常规路径，零感知。
//
// 缓存 key 一致性：session/agent/provider/model/历史的解析与 Turn 侧
// runIntentPass 完全同源，保证投机产物与真实 Turn 的意图语义一致。
type VoiceIntentSpeculator struct {
	orch    *ChatOrchestrator
	lg      loggateway.Logger
	waitCap time.Duration
	slotTTL time.Duration

	// runIntentFn 执行真实意图识别（测试缝；默认 defaultRunIntent）。
	// 返回 nil 表示跳过/失败——复用方走常规意图路径。
	runIntentFn func(ctx context.Context, ag biz.Agent, sess biz.Session, sessionID, text string) *intent.Artifact

	mu    sync.Mutex
	slots map[string]*speculativeIntentSlot // key: sessionID
}

// NewVoiceIntentSpeculator 构造投机器；chatService/orchestrator 为 nil 时返回
// nil（接线方视为关闭投机）。
func NewVoiceIntentSpeculator(chatService *ChatService) *VoiceIntentSpeculator {
	if chatService == nil || chatService.orch == nil {
		return nil
	}
	lg := chatService.lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	sp := &VoiceIntentSpeculator{
		orch:    chatService.orch,
		lg:      lg.With(loggateway.Domain("voice")),
		waitCap: speculativeResolveWaitCap,
		slotTTL: speculativeSlotTTL,
		slots:   make(map[string]*speculativeIntentSlot),
	}
	sp.runIntentFn = sp.defaultRunIntent
	return sp
}

// SpeculateIntent 对稳定 partial 文本后台预跑意图识别。非阻断容错：
// 任何前置失败（会话/agent 解析、门控）仅记日志并放弃，不存槽。
func (s *VoiceIntentSpeculator) SpeculateIntent(ctx context.Context, sessionID, text string) {
	start := time.Now()
	lg := s.lg.With(loggateway.SessionID(sessionID))
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	sess, err := s.orch.td().Sessions.Get(ctx, sessionID)
	if err != nil {
		lg.Warn("voice speculate: session fetch failed",
			loggateway.StepID("chat.voice_speculate"), loggateway.Err(err))
		return
	}
	// 团队会话走 executeTeamTurnViaHooks 独立意图路径，不消费投机产物。
	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		return
	}
	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		return
	}
	ag, err := s.orch.hydratedAgent(ctx, agentID)
	if err != nil {
		lg.Warn("voice speculate: agent hydrate failed",
			loggateway.StepID("chat.voice_speculate"), loggateway.Err(err))
		return
	}
	// 与 Turn 侧门控同源：A2A 代理 / intent 未启用 / 空文本不投机。
	if biz.IsA2AProxyAgent(ag) || !intent.ShouldRun(ag, text) {
		return
	}

	slot := &speculativeIntentSlot{sourceText: text, matchText: normalizeSpeculativeMatchText(text), createdAt: time.Now(), done: make(chan struct{})}
	s.mu.Lock()
	// 去重：同一稳定文本（含标点变体，P1-F 归一化）的投机已在槽，不重复调 LLM。
	if cur := s.slots[sessionID]; cur != nil && cur.matchText == slot.matchText && time.Since(cur.createdAt) <= s.slotTTL {
		s.mu.Unlock()
		return
	}
	// 顺带清扫过期孤儿槽（final 从未到达的残留，每会话至多一条）。
	for id, sl := range s.slots {
		if time.Since(sl.createdAt) > s.slotTTL {
			delete(s.slots, id)
		}
	}
	s.slots[sessionID] = slot
	s.mu.Unlock()

	lg.Info("voice speculative intent started",
		loggateway.StepID("chat.voice_speculate"),
		loggateway.Int("text_len", len(text)))
	safego.Go(ctx, "voice.intent_speculate", func() {
		defer close(slot.done)
		callCtx, cancel := context.WithTimeout(ctx, speculativeCallTimeout)
		defer cancel()
		art := s.runIntentFn(callCtx, ag, sess, sessionID, text)
		slot.art = art // 先于 close(done) 写入，与复用方构成 happens-before
		lg.Info("voice speculative intent finished",
			loggateway.StepID("chat.voice_speculate"),
			loggateway.Any("elapsed_ms", time.Since(start).Milliseconds()),
			loggateway.Any("completed", art != nil))
	})
}

// WithSpeculativeIntent 判定 final 文本是否复用投机产物（L3）：
//   - 无槽 / 文本失配 / 槽过期 → 原样返回（失配槽丢弃，Turn 走常规意图路径）
//   - 投机已完成且成功 → 注入产物（fresh 语义：保留澄清残留，澄清门照常评估）
//   - 投机在途 → 有界等待（waitCap）；超时/失败 → 丢弃走常规路径
func (s *VoiceIntentSpeculator) WithSpeculativeIntent(ctx context.Context, sessionID, finalText string) context.Context {
	finalText = strings.TrimSpace(finalText)
	if finalText == "" {
		return ctx
	}
	lg := s.lg.With(loggateway.SessionID(sessionID))

	s.mu.Lock()
	slot := s.slots[sessionID]
	if slot != nil && slot.matchText != normalizeSpeculativeMatchText(finalText) {
		delete(s.slots, sessionID)
		s.mu.Unlock()
		// K3 降级：final≠partial（用户改口/ASR 纠错），丢弃投机走常规路径。
		lg.Info("voice speculative intent discarded: final text mismatch",
			loggateway.StepID("chat.voice_speculate"),
			loggateway.Int("partial_len", len(slot.sourceText)),
			loggateway.Int("final_len", len(finalText)))
		return ctx
	}
	if slot != nil && time.Since(slot.createdAt) > s.slotTTL {
		delete(s.slots, sessionID)
		slot = nil
	}
	s.mu.Unlock()
	if slot == nil {
		return ctx
	}

	// 在途投机：有界等待其完成（收益窗口内完成即净赚，超时放弃）。
	timer := time.NewTimer(s.waitCap)
	defer timer.Stop()
	select {
	case <-slot.done:
	case <-ctx.Done():
		return ctx
	case <-timer.C:
		s.mu.Lock()
		if s.slots[sessionID] == slot {
			delete(s.slots, sessionID)
		}
		s.mu.Unlock()
		lg.Warn("voice speculative intent wait cap exceeded, falling back to fresh pass (K3)",
			loggateway.StepID("chat.voice_speculate"),
			loggateway.Any("wait_cap_ms", s.waitCap.Milliseconds()))
		return ctx
	}

	s.mu.Lock()
	if s.slots[sessionID] == slot {
		delete(s.slots, sessionID)
	}
	s.mu.Unlock()
	if slot.art == nil {
		lg.Info("voice speculative intent incomplete, falling back to fresh pass (K3)",
			loggateway.StepID("chat.voice_speculate"))
		return ctx
	}
	lg.Info("voice speculative intent reused",
		loggateway.StepID("chat.voice_speculate"),
		loggateway.Str("intent_kind", slot.art.IntentKind))
	return intent.WithSpeculativeArtifact(ctx, slot.art)
}

// slotCount 返回当前槽位数（测试观测用）。
func (s *VoiceIntentSpeculator) slotCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.slots)
}

// defaultRunIntent 是真实意图识别：provider/model 与历史的解析与 Turn 侧
// runIntentPass 同源（含 ARANEA_INTENT_PASS_MODEL 轻量模型覆盖）。
func (s *VoiceIntentSpeculator) defaultRunIntent(ctx context.Context, ag biz.Agent, sess biz.Session, sessionID, text string) *intent.Artifact {
	prov := strutil.FirstNonEmpty("", sess.DefaultProvider, ag.Provider)
	mod := strutil.FirstNonEmpty("", sess.DefaultModel, ag.Model)
	prov, mod = s.orch.resolveProviderModelFallback(ctx, prov, mod)
	prov, mod = resolveIntentPassProviderModel(ctx, s.orch.td().ReadDeps.LLM, prov, mod, s.lg)
	res := intent.RunForAgent(ctx, ag, s.orch.td().ReadDeps.LLM, s.orch.td().LLMHTTP,
		prov, mod, text, s.orch.recentIntentHistory(ctx, sessionID, text), s.lg)
	// P1-2 (2026-08-19): 记录语音预推测 intent pass 旁路用量（与 Turn 侧同源）。
	s.orch.turnMetrics().RecordAuxUsage(ctx, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxIntent,
		SessionID:     sessionID,
		AgentID:       ag.ID,
		AgentKey:      ag.AgentKey,
		Provider:      prov,
		Model:         mod,
		Status:        "success",
		PromptTok:     res.PromptTok,
		CompletionTok: res.CompletionTok,
		UsageSource:   biz.UsageSourceResponse,
		Latency:       res.Duration,
	})
	return res.Artifact
}
