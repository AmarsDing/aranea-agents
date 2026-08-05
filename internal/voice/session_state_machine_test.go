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
		{StateListening, EvASRFinal, StateThinking},
		{StateListening, EvBargeIn, StateListening}, // 忽略（自环）
		{StateListening, EvTurnFailed, StateError},
		{StateListening, EvVoiceStop, StateIdle},
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
		{StateInterrupted, EvASRFinal},
		{StateInterrupted, EvFirstTTSAudio},
		{StateInterrupted, EvTTSEnd},
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
