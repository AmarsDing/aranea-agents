package speech_test

import (
	"context"
	"testing"

	speech "aranea-agents/internal/data/speech"
	"aranea-agents/pkg/apierror"

	"github.com/stretchr/testify/require"
)

func TestEnvSpeechConfigReaderMissingCreds(t *testing.T) {
	r := speech.NewEnvSpeechConfigReader()
	_, err := r.ASRConfig(context.Background())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "got %v", err)
	_, err = r.TTSConfig(context.Background())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "got %v", err)
}

func TestEnvSpeechConfigReaderDefaults(t *testing.T) {
	t.Setenv("SPEECH_ASR_APP_KEY", "ak")
	t.Setenv("SPEECH_ASR_ACCESS_KEY", "sk")
	t.Setenv("SPEECH_TTS_APP_KEY", "ak")
	t.Setenv("SPEECH_TTS_ACCESS_KEY", "sk")
	t.Setenv("SPEECH_TTS_VOICE", "zh_female_test")

	r := speech.NewEnvSpeechConfigReader()
	asr, err := r.ASRConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "volcengine", asr.Driver)
	require.Equal(t, "zh-CN", asr.Language)
	require.Contains(t, asr.Endpoint, "wss://")

	tts, err := r.TTSConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "volcengine", tts.Driver)
	require.Equal(t, 1.0, tts.SpeedRatio)
	require.Equal(t, "zh_female_test", tts.Voice)
}

func TestEnvSpeechConfigReaderOverrides(t *testing.T) {
	t.Setenv("SPEECH_TTS_APP_KEY", "ak")
	t.Setenv("SPEECH_TTS_ACCESS_KEY", "sk")
	t.Setenv("SPEECH_TTS_VOICE", "v")
	t.Setenv("SPEECH_TTS_SPEED_RATIO", "1.25")
	t.Setenv("SPEECH_ASR_APP_KEY", "ak")
	t.Setenv("SPEECH_ASR_ACCESS_KEY", "sk")
	t.Setenv("SPEECH_ASR_LANGUAGE", "en-US")

	r := speech.NewEnvSpeechConfigReader()
	tts, err := r.TTSConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1.25, tts.SpeedRatio)
	asr, err := r.ASRConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "en-US", asr.Language)
}
