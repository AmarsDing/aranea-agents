package speech_test

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	speech "aranea-agents/internal/data/speech"
	"aranea-agents/pkg/apierror"

	"github.com/stretchr/testify/require"
)

// stubSettingRepo implements biz.SystemSettingRepo; only GetSpeech is used by
// SystemSpeechConfigReader, the rest satisfy the interface.
type stubSettingRepo struct {
	speech biz.SpeechSetting
	err    error
}

func (s stubSettingRepo) GetSpeech(context.Context) (biz.SpeechSetting, error) {
	return s.speech, s.err
}

func (stubSettingRepo) Get(context.Context) (biz.SystemSetting, error) {
	return biz.SystemSetting{}, nil
}
func (stubSettingRepo) Update(context.Context, string, string, int64, string, bool) (biz.SystemSetting, error) {
	return biz.SystemSetting{}, nil
}
func (stubSettingRepo) UpdateKnowledgeEmbed(context.Context, biz.KnowledgeEmbedSetting, bool) (biz.KnowledgeEmbedSetting, error) {
	return biz.KnowledgeEmbedSetting{}, nil
}
func (stubSettingRepo) GetKnowledgeEmbed(context.Context) (biz.KnowledgeEmbedSetting, error) {
	return biz.KnowledgeEmbedSetting{}, nil
}
func (stubSettingRepo) UpdateEvalLLM(context.Context, biz.EvalLLMSetting) (biz.EvalLLMSetting, error) {
	return biz.EvalLLMSetting{}, nil
}
func (stubSettingRepo) GetWebResearch(context.Context) (biz.WebResearchSetting, error) {
	return biz.WebResearchSetting{}, nil
}
func (stubSettingRepo) UpdateWebResearch(context.Context, biz.WebResearchSetting, bool) (biz.WebResearchSetting, error) {
	return biz.WebResearchSetting{}, nil
}
func (stubSettingRepo) UpdateMemoryPlatform(context.Context, biz.MemoryPlatformSetting) (biz.MemoryPlatformSetting, error) {
	return biz.MemoryPlatformSetting{}, nil
}
func (stubSettingRepo) EnsureCredentialEncryptionKey(context.Context) (string, error) { return "", nil }
func (stubSettingRepo) GetRefineLLM(context.Context) (biz.RefineLLMSetting, error) {
	return biz.RefineLLMSetting{}, nil
}
func (stubSettingRepo) UpdateRefineLLM(context.Context, biz.RefineLLMSetting, bool) (biz.RefineLLMSetting, error) {
	return biz.RefineLLMSetting{}, nil
}
func (stubSettingRepo) GetPlannerModel(context.Context) (biz.PlannerModelSetting, error) {
	return biz.PlannerModelSetting{}, nil
}
func (stubSettingRepo) UpdatePlannerModel(context.Context, biz.PlannerModelSetting) (biz.PlannerModelSetting, error) {
	return biz.PlannerModelSetting{}, nil
}
func (stubSettingRepo) UpdateSpeech(context.Context, biz.SpeechSetting, bool, bool) (biz.SpeechSetting, error) {
	return biz.SpeechSetting{}, nil
}

func TestSystemSpeechConfigReader_DBWinsOverEnv(t *testing.T) {
	t.Setenv("SPEECH_ASR_APP_KEY", "env-ak")
	t.Setenv("SPEECH_ASR_ACCESS_KEY", "env-sk")
	t.Setenv("SPEECH_ASR_LANGUAGE", "zh-CN")
	t.Setenv("SPEECH_TTS_APP_KEY", "env-ak")
	t.Setenv("SPEECH_TTS_ACCESS_KEY", "env-sk")
	t.Setenv("SPEECH_TTS_VOICE", "env-voice")

	r := speech.NewSystemSpeechConfigReader(stubSettingRepo{speech: biz.SpeechSetting{
		ASR: biz.ASRProviderConfig{Language: "en-US"},
		TTS: biz.TTSProviderConfig{Voice: "db-voice", SpeedRatio: 1.5},
	}}, nil)

	asr, err := r.ASRConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "en-US", asr.Language, "DB field must win")
	require.Equal(t, "env-ak", asr.AppKey, "unset DB field must fall back to env")
	require.Equal(t, "volcengine", asr.Driver, "unset DB+env field must use built-in default")

	tts, err := r.TTSConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "db-voice", tts.Voice)
	require.Equal(t, 1.5, tts.SpeedRatio, "DB speed > 0 must win")
	require.Equal(t, "env-ak", tts.AppKey)
}

func TestSystemSpeechConfigReader_EnvFallbackWhenDBEmpty(t *testing.T) {
	t.Setenv("SPEECH_ASR_APP_KEY", "env-ak")
	t.Setenv("SPEECH_ASR_ACCESS_KEY", "env-sk")
	t.Setenv("SPEECH_TTS_APP_KEY", "env-ak")
	t.Setenv("SPEECH_TTS_ACCESS_KEY", "env-sk")
	t.Setenv("SPEECH_TTS_VOICE", "env-voice")
	t.Setenv("SPEECH_TTS_SPEED_RATIO", "1.25")

	r := speech.NewSystemSpeechConfigReader(stubSettingRepo{}, nil)
	asr, err := r.ASRConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "env-ak", asr.AppKey)

	tts, err := r.TTSConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "env-voice", tts.Voice)
	require.Equal(t, 1.25, tts.SpeedRatio, "DB speed = 0 (unset) must fall back to env")
}

func TestSystemSpeechConfigReader_ReadErrorDegradesToEnv(t *testing.T) {
	t.Setenv("SPEECH_ASR_APP_KEY", "env-ak")
	t.Setenv("SPEECH_ASR_ACCESS_KEY", "env-sk")

	r := speech.NewSystemSpeechConfigReader(stubSettingRepo{err: errors.New("db down")}, nil)
	asr, err := r.ASRConfig(context.Background())
	require.NoError(t, err, "repo failure must degrade to env, not break voice pipeline")
	require.Equal(t, "env-ak", asr.AppKey)
}

func TestSystemSpeechConfigReader_NilRepoPureEnv(t *testing.T) {
	r := speech.NewSystemSpeechConfigReader(nil, nil)
	_, err := r.ASRConfig(context.Background())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition),
		"nil repo + no env creds must fail validation like env reader, got %v", err)
}

func TestSystemSpeechConfigReader_ArchiveTriState(t *testing.T) {
	on := true
	off := false

	// DB non-nil wins over env.
	t.Setenv("SPEECH_ARCHIVE_USER_AUDIO", "false")
	r := speech.NewSystemSpeechConfigReader(stubSettingRepo{speech: biz.SpeechSetting{ArchiveUserAudio: &on}}, nil)
	got, err := r.ArchiveUserAudio(context.Background())
	require.NoError(t, err)
	require.True(t, got, "stored true must win over env false")

	// DB NULL falls back to env.
	t.Setenv("SPEECH_ARCHIVE_USER_AUDIO", "true")
	r2 := speech.NewSystemSpeechConfigReader(stubSettingRepo{}, nil)
	got, err = r2.ArchiveUserAudio(context.Background())
	require.NoError(t, err)
	require.True(t, got, "NULL must fall back to env true")

	// DB explicit false wins over env true.
	r3 := speech.NewSystemSpeechConfigReader(stubSettingRepo{speech: biz.SpeechSetting{ArchiveUserAudio: &off}}, nil)
	got, err = r3.ArchiveUserAudio(context.Background())
	require.NoError(t, err)
	require.False(t, got, "stored explicit false must win over env true")
}
