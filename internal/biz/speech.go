package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// SpeechProvider ports for the voice companion (M74). Pure Go interfaces —
// biz 不依赖任何 Provider SDK（火山等 SDK 仅在 internal/data/speech）。

// ---- ASR ----

type ASREventType int

const (
	ASREventPartial ASREventType = iota
	ASREventFinal
	ASREventError
	ASREventVadEnd // 服务端 VAD 端点（无终稿文本时的纯端点信号）
)

// ASREvent is the transport-neutral recognition event emitted by ASRSession.
type ASREvent struct {
	Type       ASREventType
	Text       string // Partial=当前 partial 全文；Final=终稿全文
	DurationMs int    // Final: 本语句音频时长
	Err        error  // Error 事件非 nil
}

type ASRSessionConfig struct {
	Driver     string // Provider 驱动名（如 volcengine），随消息元数据落库（V2-T6 asr_provider）
	Language   string // 默认 zh-CN
	SampleRate int    // 默认 16000
}

// Stability:evolving
type StreamingASRProvider interface {
	Open(ctx context.Context, cfg ASRSessionConfig) (ASRSession, error)
}

// Stability:evolving
type ASRSession interface {
	Write(pcm []byte) error  // 20ms PCM s16le 帧
	Finish() error           // 标记当前语句音频结束（voice.commit / PTT）
	Events() <-chan ASREvent // Partial/Final/Error/VadEnd
	Close() error
}

// ---- TTS ----

type TTSAudioChunkType int

const (
	TTSAudioChunkData TTSAudioChunkType = iota
	TTSAudioChunkEnd                    // 全部句子合成完毕（或取消）
	TTSAudioChunkError
)

// TTSAudioChunk carries one audio chunk. PCM 编码固定 f32le 16kHz mono
// （适配器负责从 Provider 原始编码转换）。
type TTSAudioChunk struct {
	Type TTSAudioChunkType
	PCM  []byte
	Err  error
}

type TTSSessionConfig struct {
	Voice      string
	SpeedRatio float64
	SampleRate int // 默认 16000
}

// Stability:evolving
type StreamingTTSProvider interface {
	Open(ctx context.Context, cfg TTSSessionConfig) (TTSSession, error)
}

// Stability:evolving
type TTSSession interface {
	Write(text string, flush bool) error // 分句写入；flush=尾句强制合成
	Audio() <-chan TTSAudioChunk
	Close() error
}

// ---- 配置（V1 环境变量注入，见 data/speech/env_config.go）----

type ASRProviderConfig struct {
	Driver     string
	Endpoint   string
	APIKey     string // sensitive，禁止入日志。X-Api-Key 鉴权模式（火山控制台新 API Key）
	AppKey     string // sensitive，禁止入日志（legacy 模式：AppKey+AccessKey 对）
	AccessKey  string // sensitive，禁止入日志
	ResourceID string
	Language   string
	// Hotwords ASR 热词直传表（V11-T4）：非空时覆盖 Provider 默认表。
	// 预留配置通道，暂不接 DB/UI（YAGNI）。
	Hotwords []string
}

func (c ASRProviderConfig) Validate() error {
	if strings.TrimSpace(c.Driver) == "" {
		return apierror.FailedPrecondition("speech", "asr driver is required")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return apierror.FailedPrecondition("speech", "asr endpoint is required")
	}
	if !SpeechCredOK(c.APIKey, c.AppKey, c.AccessKey) {
		return apierror.FailedPrecondition("speech", "asr credential is required (SPEECH_ASR_API_KEY，或 SPEECH_ASR_APP_KEY + SPEECH_ASR_ACCESS_KEY)")
	}
	return nil
}

// SpeechCredOK 判定凭据完整性：X-Api-Key 单 key 模式，或 legacy AppKey+AccessKey 对。
func SpeechCredOK(apiKey, appKey, accessKey string) bool {
	if strings.TrimSpace(apiKey) != "" {
		return true
	}
	return strings.TrimSpace(appKey) != "" && strings.TrimSpace(accessKey) != ""
}

type TTSProviderConfig struct {
	Driver     string
	Endpoint   string
	APIKey     string // sensitive
	AppKey     string // sensitive
	AccessKey  string // sensitive
	ResourceID string
	Voice      string
	SpeedRatio float64
}

func (c TTSProviderConfig) Validate() error {
	if strings.TrimSpace(c.Driver) == "" {
		return apierror.FailedPrecondition("speech", "tts driver is required")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return apierror.FailedPrecondition("speech", "tts endpoint is required")
	}
	if !SpeechCredOK(c.APIKey, c.AppKey, c.AccessKey) {
		return apierror.FailedPrecondition("speech", "tts credential is required (SPEECH_TTS_API_KEY，或 SPEECH_TTS_APP_KEY + SPEECH_TTS_ACCESS_KEY)")
	}
	if strings.TrimSpace(c.Voice) == "" {
		return apierror.FailedPrecondition("speech", "tts voice is required (SPEECH_TTS_VOICE)")
	}
	if c.SpeedRatio <= 0 {
		return apierror.FailedPrecondition("speech", "tts speed_ratio must be > 0")
	}
	return nil
}

// Stability:evolving — 配置读取端口。V1 实现 = env（data/speech/env_config.go）；
// V2-T7 换 System Settings speech 分组实现，端口不变。
type SpeechConfigReader interface {
	ASRConfig(ctx context.Context) (ASRProviderConfig, error)
	TTSConfig(ctx context.Context) (TTSProviderConfig, error)
	// ArchiveUserAudio 返回语音留档开关（speech.archive_user_audio，V2-T6）。
	// 默认 false；读取失败时调用方按 Warn 降级为不留档（K3）。
	ArchiveUserAudio(ctx context.Context) (bool, error)
}
