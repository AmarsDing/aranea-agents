package speech

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// SystemSpeechConfigReader implements biz.SpeechConfigReader with DB-first /
// env-fallback field-level merge (M74 V2-T7): each field uses the stored
// system_settings value when non-empty, otherwise the SPEECH_* env value.
// This keeps V1 pure-env deployments working while letting the admin UI
// override individual fields; changes take effect on the next voice.start
// (the WS provider factories re-read config per session — 配置热生效).
//
// 凭据合并后明文仅存在于内存返回值中，禁止入日志（DB-N8 同语义）。
type SystemSpeechConfigReader struct {
	repo biz.SystemSettingRepo
	lg   loggateway.Logger
}

// NewSystemSpeechConfigReader builds the DB-first reader. repo may be nil in
// tests — then it degrades to pure env behavior (same as EnvSpeechConfigReader).
func NewSystemSpeechConfigReader(repo biz.SystemSettingRepo, lg loggateway.Logger) *SystemSpeechConfigReader {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SystemSpeechConfigReader{repo: repo, lg: lg.With(loggateway.Domain("speech"))}
}

var _ biz.SpeechConfigReader = (*SystemSpeechConfigReader)(nil)

// stored loads the stored speech settings; any read failure degrades to an
// empty setting (K3: env fallback keeps the voice pipeline alive).
func (r *SystemSpeechConfigReader) stored(ctx context.Context) biz.SpeechSetting {
	if r.repo == nil {
		return biz.SpeechSetting{}
	}
	s, err := r.repo.GetSpeech(ctx)
	if err != nil {
		r.lg.Warn("speech settings read failed, falling back to env",
			loggateway.StepID("speech.config.read_fallback"),
			loggateway.Err(err))
		return biz.SpeechSetting{}
	}
	return s
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return b
}

func (r *SystemSpeechConfigReader) ASRConfig(ctx context.Context) (biz.ASRProviderConfig, error) {
	db := r.stored(ctx).ASR
	env := loadEnvASRConfig()
	cfg := biz.ASRProviderConfig{
		Driver:     firstNonEmptyStr(db.Driver, env.Driver),
		Endpoint:   firstNonEmptyStr(db.Endpoint, env.Endpoint),
		APIKey:     firstNonEmptyStr(db.APIKey, env.APIKey),
		AppKey:     firstNonEmptyStr(db.AppKey, env.AppKey),
		AccessKey:  firstNonEmptyStr(db.AccessKey, env.AccessKey),
		ResourceID: firstNonEmptyStr(db.ResourceID, env.ResourceID),
		Language:   firstNonEmptyStr(db.Language, env.Language),
	}
	if err := cfg.Validate(); err != nil {
		return biz.ASRProviderConfig{}, err
	}
	return cfg, nil
}

func (r *SystemSpeechConfigReader) TTSConfig(ctx context.Context) (biz.TTSProviderConfig, error) {
	db := r.stored(ctx).TTS
	env := loadEnvTTSConfig()
	speed := env.SpeedRatio
	if db.SpeedRatio > 0 {
		speed = db.SpeedRatio
	}
	cfg := biz.TTSProviderConfig{
		Driver:     firstNonEmptyStr(db.Driver, env.Driver),
		Endpoint:   firstNonEmptyStr(db.Endpoint, env.Endpoint),
		APIKey:     firstNonEmptyStr(db.APIKey, env.APIKey),
		AppKey:     firstNonEmptyStr(db.AppKey, env.AppKey),
		AccessKey:  firstNonEmptyStr(db.AccessKey, env.AccessKey),
		ResourceID: firstNonEmptyStr(db.ResourceID, env.ResourceID),
		Voice:      firstNonEmptyStr(db.Voice, env.Voice),
		SpeedRatio: speed,
	}
	if err := cfg.Validate(); err != nil {
		return biz.TTSProviderConfig{}, err
	}
	return cfg, nil
}

// ArchiveUserAudio resolves the voice-archive toggle: stored non-nil value
// wins; NULL (unset) falls back to env SPEECH_ARCHIVE_USER_AUDIO.
func (r *SystemSpeechConfigReader) ArchiveUserAudio(ctx context.Context) (bool, error) {
	if v := r.stored(ctx).ArchiveUserAudio; v != nil {
		return *v, nil
	}
	return loadEnvArchiveUserAudio(), nil
}
