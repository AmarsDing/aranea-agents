package voice

// session_wake.go — V10 唤醒/休眠（小媛）领域逻辑（设计 §16.4②）。
// 从 session.go 抽离（2026-08-13，AS-COG-01 债务控制）：唤醒入口、退出词、
// 静默休眠计时、ASR 上游关停。状态机定义见 session_state（dormant 状态/事件）。

import (
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// sleepTimeout V10：listening 态静默休眠阈值（需求 §2.12：60s 无交互回待命）。
// 包级变量（非 const）供测试缩短（ttsSentenceTimeout 同款先例）。
var sleepTimeout = 60 * time.Second

// V10 自足 TTS 应答文案（不经 Chat Turn，不占消息流）。
const wakeAckText = "我在"
const exitAckText = "好的，我先休息了"

// Wake 处理 voice.wake：dormant → listening，懒启动 ASR 上游（V10，设计 §16.4②）。
// source ∈ kws/manual/system；kws/manual 播自足唤醒应答「我在」，
// system（委派终态唤醒）不应答——委派播报本身即内容。非 dormant 幂等忽略。
func (s *Session) Wake(source string) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != StateDormant {
		return
	}
	if err := s.transition(EvWake); err != nil {
		return
	}
	if err := s.openASR(); err != nil {
		// 对齐 Start 错误路径：报 ASR_UNAVAILABLE，停留 listening 由前端重试。
		s.sendError("ASR_UNAVAILABLE", err, true)
	}
	s.flow.LogDone("voice.wake.detect", "语音唤醒", event.P("source", source))
	s.lg.Info("voice session woken", loggateway.StepID("voice.wake.detect"), loggateway.Str("source", source))
	s.broadcastState()
	if source != "system" {
		s.speakSelfSufficient(wakeAckText, "voice.wake.detect", "语音唤醒应答", false)
	}
}

// speakSelfSufficient 自足播报（R15 机制泛化，V10 唤醒/退出应答复用）：
// listening 态无活跃 turn/chunker flush 源，必须一次性 ensureTTS → Write →
// Flush → flush 哨兵（驱动 OnDrained → tts.end）。状态停留 listening：
// TTS 播放期间用户说话照常进 ASR。
// sleepAfter=true（退出词）时追加休眠哨兵：应答播报完的 drain 驱动 dormant
// （设计 §16.4②），休眠意图绑定 TTS 任务与会话状态机解耦——前序 flush
// drain（如唤醒应答）不会抢消费（2026-08-13 竞态修复）。
// ensure+快照单临界区、全程使用快照引用：陈旧调度器的迟到 drain 置空会话
// 字段不影响本次播报（同次修复）。
func (s *Session) speakSelfSufficient(text, stepID, title string, sleepAfter bool) {
	s.mu.Lock()
	if err := s.ensureTTSLocked(); err != nil {
		s.mu.Unlock()
		s.sendError("TTS_UNAVAILABLE", err, true)
		return
	}
	// 自足路径无 turn 起点复位（handleASRFinal），显式清零防陈旧 true 跳哨兵。
	s.flushEnqueued = false
	ch := s.chunker
	sch := s.scheduler
	s.mu.Unlock()
	s.flow.LogDone(stepID, title, event.P("chars", len(text)))
	s.lg.Info("voice self-sufficient speech",
		loggateway.StepID(stepID), loggateway.Int("chars", len(text)))
	ch.Write(text)
	ch.Flush()
	s.mu.Lock()
	tailSent := s.flushEnqueued
	s.mu.Unlock()
	switch {
	case !tailSent && sleepAfter:
		// Flush 无残余（尾句不可播报等）：休眠哨兵一并承载 drain（tts.end）与 dormant
		if err := sch.EnqueueSleepSentinel(s.ctx); err != nil {
			s.lg.Warn("voice tts sleep sentinel enqueue failed", loggateway.StepID("voice.tts.enqueue_fail"), loggateway.Err(err))
		}
	case !tailSent:
		// Flush 无残余：补普通 flush 哨兵驱动 OnDrained（对齐 flushTTSTail）
		if err := sch.Enqueue(s.ctx, "", true); err != nil {
			s.lg.Warn("voice tts flush sentinel enqueue failed", loggateway.StepID("voice.tts.enqueue_fail"), loggateway.Err(err))
		}
	case sleepAfter:
		// 应答尾句已带 flush（drain → tts.end），追加休眠哨兵驱动 dormant
		if err := sch.EnqueueSleepSentinel(s.ctx); err != nil {
			s.lg.Warn("voice tts sleep sentinel enqueue failed", loggateway.StepID("voice.tts.enqueue_fail"), loggateway.Err(err))
		}
	}
}

// handleExitWord V10：退出词命中 —— 自足 TTS 应答确认 + 休眠哨兵，应答播报完
// 的 drain 驱动 dormant（设计 §16.4②）；TTS 不可用降级为立即休眠（不阻塞）。
func (s *Session) handleExitWord() {
	s.mu.Lock()
	if s.state != StateListening {
		s.mu.Unlock()
		return // 残余 final（竞态已离 listening），忽略
	}
	s.mu.Unlock()
	// 流程日志统一在 enterDormant 发射（应答后/TTS 降级两路径各一次，此处不重复）。
	s.lg.Info("voice session exit word matched", loggateway.StepID("voice.sleep.exit_word"))
	if err := s.ensureTTS(); err != nil {
		s.sendError("TTS_UNAVAILABLE", err, true)
		s.enterDormant(EvExitWord, "voice.sleep.exit_word", "退出词休眠（TTS 降级直休眠）")
		return
	}
	s.speakSelfSufficient(exitAckText, "voice.sleep.exit_word", "退出词应答", true)
}

// enterDormant listening → dormant 公共路径：转换 + 关 ASR 上游 + 广播。
// 仅 onSleepTimeout / onTTSDrained（退出词应答后）/ handleExitWord 降级调用。
func (s *Session) enterDormant(ev VoiceEvent, stepID, msg string) {
	if err := s.transition(ev); err != nil {
		return
	}
	s.closeASRUpstream()
	s.flow.LogDone(stepID, msg)
	s.lg.Info("voice session dormant", loggateway.StepID(stepID))
	s.broadcastState()
}

// manageSleepTimerLocked V10：companion 静默休眠计时集中管理（设计 §16.4②）。
// 进入 listening 重置 60s；离开 listening 停止（thinking/speaking 交互中不休眠，
// dormant/idle 无需计时）。听写/非 companion 模式无 dormant 语义，恒不计时。
// 调用方须持有 s.mu。
func (s *Session) manageSleepTimerLocked(to VoiceState) {
	if s.sleepTimer != nil {
		s.sleepTimer.Stop()
		s.sleepTimer = nil
	}
	if to != StateListening || s.params.Mode != ModeCompanion {
		return
	}
	s.sleepTimer = time.AfterFunc(sleepTimeout, s.onSleepTimeout)
}

// resetSleepTimer V10：ASR 活动（partial 流）重置静默休眠计时——用户持续
// 说话时（state 停留 listening 无转换）不被 60s 到期误休眠。
func (s *Session) resetSleepTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateListening || s.params.Mode != ModeCompanion {
		return
	}
	if s.sleepTimer != nil {
		s.sleepTimer.Stop()
	}
	s.sleepTimer = time.AfterFunc(sleepTimeout, s.onSleepTimeout)
}

// onSleepTimeout V10：静默到期 → dormant（关闭 ASR 上游，零占用）。
// 竞态窗口内已离开 listening（交互中）时转换表拒绝即忽略。
func (s *Session) onSleepTimeout() {
	if err := s.transition(EvSleepTimeout); err != nil {
		return
	}
	s.closeASRUpstream()
	s.flow.LogDone("voice.sleep.timeout", "静默休眠")
	s.lg.Info("voice session dormant (sleep timeout)", loggateway.StepID("voice.sleep.timeout"))
	s.broadcastState()
}

// closeASRUpstream 关闭并摘除当前 ASR 上游（dormant 零占用；asrPump 的
// 流终结 CAS 摘表发现已摘除后不重复 Close）。
func (s *Session) closeASRUpstream() {
	s.mu.Lock()
	asr := s.asr
	s.asr = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
	}
}
