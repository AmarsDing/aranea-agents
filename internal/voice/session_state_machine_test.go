package voice

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVoiceStateMachineLegalTransitions(t *testing.T) {
	cases := []struct {
		from  VoiceState
		event VoiceEvent
		to    VoiceState
	}{
		{StateIdle, EvVoiceStart, StateListening},
		{StateIdle, EvVoiceStartDormant, StateDormant}, // V10：companion 模式进入即待命
		{StateListening, EvASRFinal, StateThinking},
		{StateListening, EvBargeIn, StateListening}, // 忽略（自环）
		{StateListening, EvTurnFailed, StateError},
		{StateListening, EvVoiceStop, StateIdle},
		{StateListening, EvSleepTimeout, StateDormant}, // V10：60s 静默休眠
		{StateListening, EvExitWord, StateDormant},     // V10：退出词休眠
		{StateDormant, EvWake, StateListening},         // V10：唤醒（KWS/手动/委派系统唤醒）
		{StateDormant, EvVoiceStop, StateIdle},
		{StateThinking, EvFirstTTSAudio, StateSpeaking},
		{StateThinking, EvTTSEnd, StateListening}, // 无文本 Turn
		{StateThinking, EvBargeIn, StateListening},
		{StateThinking, EvTurnFailed, StateError},
		{StateThinking, EvVoiceStop, StateIdle},
		{StateSpeaking, EvTTSEnd, StateListening},
		{StateSpeaking, EvBargeIn, StateInterrupted},
		{StateSpeaking, EvTurnFailed, StateError},
		{StateSpeaking, EvVoiceStop, StateIdle},
		{StateInterrupted, EvVoiceStop, StateIdle},
		{StateError, EvVoiceStart, StateListening},
		{StateError, EvVoiceStop, StateIdle},
	}
	for _, c := range cases {
		to, err := Transition(c.from, c.event)
		require.NoError(t, err, "%s --%s", c.from, c.event)
		require.Equal(t, c.to, to, "%s --%s", c.from, c.event)
	}
}

func TestVoiceStateMachineIllegalTransitions(t *testing.T) {
	illegal := []struct {
		from  VoiceState
		event VoiceEvent
	}{
		{StateIdle, EvASRFinal},
		{StateIdle, EvFirstTTSAudio},
		{StateIdle, EvBargeIn},
		{StateIdle, EvWake},
		{StateIdle, EvSleepTimeout},
		{StateIdle, EvExitWord},
		{StateDormant, EvVoiceStart},        // dormant 重入 start 忽略（Start 入口拦截）
		{StateDormant, EvVoiceStartDormant}, // 自环不允许
		{StateDormant, EvASRFinal},          // 迟到 ASR 事件拒绝
		{StateDormant, EvFirstTTSAudio},
		{StateDormant, EvTTSEnd},
		{StateDormant, EvBargeIn},
		{StateDormant, EvTurnFailed},
		{StateDormant, EvSleepTimeout}, // dormant 无 sleep timer
		{StateDormant, EvExitWord},
		{StateListening, EvVoiceStartDormant},
		{StateListening, EvWake}, // wake 幂等：仅 dormant 受理
		{StateThinking, EvWake},
		{StateThinking, EvSleepTimeout}, // 交互中不休眠
		{StateThinking, EvExitWord},
		{StateSpeaking, EvWake},
		{StateSpeaking, EvSleepTimeout},
		{StateSpeaking, EvExitWord},
		{StateInterrupted, EvWake},
		{StateInterrupted, EvSleepTimeout},
		{StateInterrupted, EvExitWord},
		{StateInterrupted, EvASRFinal},
		{StateInterrupted, EvFirstTTSAudio},
		{StateInterrupted, EvTTSEnd},
		{StateError, EvWake},
		{StateError, EvVoiceStartDormant},
		{StateError, EvSleepTimeout},
		{StateError, EvExitWord},
		{StateSpeaking, EvVoiceStart},
		{StateSpeaking, EvASRFinal},
		{StateError, EvFirstTTSAudio},
		{StateError, EvTTSEnd},
	}
	for _, c := range illegal {
		_, err := Transition(c.from, c.event)
		require.Error(t, err, "%s --%s should be illegal", c.from, c.event)
	}
}
