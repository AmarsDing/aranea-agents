// Package speech implements the biz.SpeechProvider ports (M74).
package speech

import (
	"context"
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
)

const (
	defaultASRDriver     = "volcengine"
	defaultASREndpoint   = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel"
	defaultASRResourceID = "volc.bigasr.sauc.duration"
	defaultASRLanguage   = "zh-CN"

	defaultTTSDriver     = "volcengine"
	defaultTTSEndpoint   = "wss://openspeech.bytedance.com/api/v3/tts/unidirectional/stream"
	defaultTTSResourceID = "volc.service_type.10029"
	defaultTTSSpeedRatio = 1.0
)

// EnvSpeechConfigReader implements biz.SpeechConfigReader from environment
// variables (V1; System Settings speech 分组在 V2-T7)。凭据只经 env 注入，
// 禁止写入日志（DB-N8 同语义）。
type EnvSpeechConfigReader struct{}

func NewEnvSpeechConfigReader() *EnvSpeechConfigReader { return &EnvSpeechConfigReader{} }

var _ biz.SpeechConfigReader = (*EnvSpeechConfigReader)(nil)

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

// loadEnvASRConfig reads raw ASR config from env without validation. Shared by
// EnvSpeechConfigReader (V1, validating) and SystemSpeechConfigReader (V2-T7,
// DB-first field-level merge fallback).
func loadEnvASRConfig() biz.ASRProviderConfig {
	return biz.ASRProviderConfig{
		Driver:     envOr("SPEECH_ASR_DRIVER", defaultASRDriver),
		Endpoint:   envOr("SPEECH_ASR_ENDPOINT", defaultASREndpoint),
		APIKey:     strings.TrimSpace(os.Getenv("SPEECH_ASR_API_KEY")),
		AppKey:     strings.TrimSpace(os.Getenv("SPEECH_ASR_APP_KEY")),
		AccessKey:  strings.TrimSpace(os.Getenv("SPEECH_ASR_ACCESS_KEY")),
		ResourceID: envOr("SPEECH_ASR_RESOURCE_ID", defaultASRResourceID),
		Language:   envOr("SPEECH_ASR_LANGUAGE", defaultASRLanguage),
	}
}

// loadEnvTTSConfig reads raw TTS config from env without validation.
func loadEnvTTSConfig() biz.TTSProviderConfig {
	speed := defaultTTSSpeedRatio
	if raw := strings.TrimSpace(os.Getenv("SPEECH_TTS_SPEED_RATIO")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			speed = v
		}
	}
	return biz.TTSProviderConfig{
		Driver:     envOr("SPEECH_TTS_DRIVER", defaultTTSDriver),
		Endpoint:   envOr("SPEECH_TTS_ENDPOINT", defaultTTSEndpoint),
		APIKey:     strings.TrimSpace(os.Getenv("SPEECH_TTS_API_KEY")),
		AppKey:     strings.TrimSpace(os.Getenv("SPEECH_TTS_APP_KEY")),
		AccessKey:  strings.TrimSpace(os.Getenv("SPEECH_TTS_ACCESS_KEY")),
		ResourceID: envOr("SPEECH_TTS_RESOURCE_ID", defaultTTSResourceID),
		Voice:      strings.TrimSpace(os.Getenv("SPEECH_TTS_VOICE")),
		SpeedRatio: speed,
	}
}

// loadEnvArchiveUserAudio reads SPEECH_ARCHIVE_USER_AUDIO（V2-T6 语音留档开关）。
// 默认 false；非法值按 false 处理（配置笔误不应打断语音链路）。
func loadEnvArchiveUserAudio() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SPEECH_ARCHIVE_USER_AUDIO"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (r *EnvSpeechConfigReader) ASRConfig(_ context.Context) (biz.ASRProviderConfig, error) {
	cfg := loadEnvASRConfig()
	if err := cfg.Validate(); err != nil {
		return biz.ASRProviderConfig{}, err
	}
	return cfg, nil
}

func (r *EnvSpeechConfigReader) TTSConfig(_ context.Context) (biz.TTSProviderConfig, error) {
	cfg := loadEnvTTSConfig()
	if err := cfg.Validate(); err != nil {
		return biz.TTSProviderConfig{}, err
	}
	return cfg, nil
}

// ArchiveUserAudio 读取 SPEECH_ARCHIVE_USER_AUDIO（V2-T6 语音留档开关）。
// 默认 false；非法值按 false 处理（配置笔误不应打断语音链路）。
func (r *EnvSpeechConfigReader) ArchiveUserAudio(_ context.Context) (bool, error) {
	return loadEnvArchiveUserAudio(), nil
}
