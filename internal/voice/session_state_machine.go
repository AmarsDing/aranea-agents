package voice

import "fmt"

// 保留理由（2026-08-20 P2-2 盘点）：不迁移到 biz/shared.GenericStateMachine——
// voice 包为独立领域，不依赖 biz 层；表内含自环（listening --barge_in--> listening
// 表示忽略）与 interrupted 无出口事件（由会话定时器直接置位）等设计 §16.2 特定
// 语义，迁移收益仅为形式统一。AS-FSM-01 合规：显式转换表。

// VoiceState 是语音会话状态（设计 §5 + §16.2，7 状态 > 3，AS-FSM-01 显式状态机）。
type VoiceState string

const (
	StateIdle        VoiceState = "idle"
	StateDormant     VoiceState = "dormant" // V10：待命（KWS 本地监听，ASR 关闭）
	StateListening   VoiceState = "listening"
	StateThinking    VoiceState = "thinking"
	StateSpeaking    VoiceState = "speaking"
	StateInterrupted VoiceState = "interrupted"
	StateError       VoiceState = "error"
)

type VoiceEvent string

const (
	EvVoiceStart        VoiceEvent = "voice_start"         // idle/error→listening（dictation + 恢复路径）
	EvVoiceStartDormant VoiceEvent = "voice_start_dormant" // idle→dormant（companion 默认入口）
	EvWake              VoiceEvent = "wake"                // dormant→listening（KWS/手动/委派系统唤醒）
	EvSleepTimeout      VoiceEvent = "sleep_timeout"       // listening→dormant（60s 静默）
	EvExitWord          VoiceEvent = "exit_word"           // listening→dormant（退出词）
	EvASRFinal          VoiceEvent = "asr_final"
	EvFirstTTSAudio     VoiceEvent = "first_tts_audio"
	EvTTSEnd            VoiceEvent = "tts_end"
	EvBargeIn           VoiceEvent = "barge_in"
	EvTurnFailed        VoiceEvent = "turn_failed"
	EvVoiceStop         VoiceEvent = "voice_stop"
)

// transitions 转换表（设计 §5 + §16.2）。interrupted 为过渡态：进入后由会话定时器
// ~300ms 直接置位回 listening（设计明确"无需事件"），故表中无其出口事件。
var transitions = map[VoiceState]map[VoiceEvent]VoiceState{
	StateIdle: {
		EvVoiceStart:        StateListening,
		EvVoiceStartDormant: StateDormant,
	},
	StateDormant: {
		EvWake:      StateListening,
		EvVoiceStop: StateIdle,
	},
	StateListening: {
		EvASRFinal:     StateThinking,
		EvBargeIn:      StateListening, // 忽略（自环）
		EvTurnFailed:   StateError,
		EvVoiceStop:    StateIdle,
		EvSleepTimeout: StateDormant,
		EvExitWord:     StateDormant,
	},
	StateThinking: {
		EvFirstTTSAudio: StateSpeaking,
		EvTTSEnd:        StateListening, // 无文本 Turn
		EvBargeIn:       StateListening,
		EvTurnFailed:    StateError,
		EvVoiceStop:     StateIdle,
	},
	StateSpeaking: {
		EvTTSEnd:     StateListening,
		EvBargeIn:    StateInterrupted,
		EvTurnFailed: StateError,
		EvVoiceStop:  StateIdle,
	},
	StateInterrupted: {
		EvVoiceStop: StateIdle,
	},
	StateError: {
		EvVoiceStart: StateListening,
		EvVoiceStop:  StateIdle,
	},
}

// Transition 校验并返回目标状态；非法转换返回错误。
func Transition(from VoiceState, event VoiceEvent) (VoiceState, error) {
	if events, ok := transitions[from]; ok {
		if to, ok := events[event]; ok {
			return to, nil
		}
	}
	return "", fmt.Errorf("voice: illegal transition %s --%s", from, event)
}
