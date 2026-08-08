package biz

import "strings"

// SpeechSetting holds the voice companion (M74) ASR/TTS provider configuration
// as stored on the system_settings singleton (V2-T7). Empty string fields mean
// "unset" — the runtime reader (data/speech) falls back to SPEECH_* env vars
// via field-level merge, so a partially-filled DB row never shadows env values.
//
// 凭据（AppKey/AccessKey）与 knowledge_embed/web_research 同惯例：明文存储、
// 禁入日志、API 仅以 has_api_key 暴露，updateXxxCred=false 时空值不覆盖。
type SpeechSetting struct {
	ASR ASRProviderConfig
	TTS TTSProviderConfig
	// ArchiveUserAudio is tri-state: nil = unset（回退 env
	// SPEECH_ARCHIVE_USER_AUDIO）；non-nil = 管理员显式设置。三态避免升级后
	// 静默覆盖 V1 env 开关（DDL 列 DEFAULT NULL）。
	ArchiveUserAudio *bool
}

// SpeechASRConfigured reports whether the stored ASR sub-config is complete
// enough to open a session without env fallback (credential + driver + endpoint).
// 凭据双模式：X-Api-Key 单 key，或 legacy AppKey+AccessKey 对（SpeechCredOK）。
func SpeechASRConfigured(s SpeechSetting) bool {
	return SpeechCredOK(s.ASR.APIKey, s.ASR.AppKey, s.ASR.AccessKey) &&
		strings.TrimSpace(s.ASR.Driver) != "" &&
		strings.TrimSpace(s.ASR.Endpoint) != ""
}

// SpeechTTSConfigured reports whether the stored TTS sub-config is complete
// enough to open a session without env fallback.
func SpeechTTSConfigured(s SpeechSetting) bool {
	return SpeechCredOK(s.TTS.APIKey, s.TTS.AppKey, s.TTS.AccessKey) &&
		strings.TrimSpace(s.TTS.Driver) != "" &&
		strings.TrimSpace(s.TTS.Endpoint) != "" &&
		strings.TrimSpace(s.TTS.Voice) != ""
}

// ApplySpeechPatch merges an admin update onto the currently stored speech
// settings. Empty patch fields preserve current values (proto3 zero-value =
// "not provided" semantics, same as ApplyWebResearchPatch); credentials are
// replaced only when the matching updateXxxCred flag is set and the value is
// non-empty. ArchiveUserAudio: nil patch preserves, non-nil replaces
// (explicit true/false both persisted).
func ApplySpeechPatch(cur SpeechSetting, patch SpeechSetting, updateASRCred, updateTTSCred bool) SpeechSetting {
	out := cur
	// ASR non-credential fields.
	if v := strings.TrimSpace(patch.ASR.Driver); v != "" {
		out.ASR.Driver = v
	}
	if v := strings.TrimSpace(patch.ASR.Endpoint); v != "" {
		out.ASR.Endpoint = v
	}
	if v := strings.TrimSpace(patch.ASR.ResourceID); v != "" {
		out.ASR.ResourceID = v
	}
	if v := strings.TrimSpace(patch.ASR.Language); v != "" {
		out.ASR.Language = v
	}
	if updateASRCred {
		if v := strings.TrimSpace(patch.ASR.APIKey); v != "" {
			out.ASR.APIKey = v
		}
		if v := strings.TrimSpace(patch.ASR.AppKey); v != "" {
			out.ASR.AppKey = v
		}
		if v := strings.TrimSpace(patch.ASR.AccessKey); v != "" {
			out.ASR.AccessKey = v
		}
	}
	// TTS non-credential fields.
	if v := strings.TrimSpace(patch.TTS.Driver); v != "" {
		out.TTS.Driver = v
	}
	if v := strings.TrimSpace(patch.TTS.Endpoint); v != "" {
		out.TTS.Endpoint = v
	}
	if v := strings.TrimSpace(patch.TTS.ResourceID); v != "" {
		out.TTS.ResourceID = v
	}
	if v := strings.TrimSpace(patch.TTS.Voice); v != "" {
		out.TTS.Voice = v
	}
	if patch.TTS.SpeedRatio > 0 {
		out.TTS.SpeedRatio = patch.TTS.SpeedRatio
	}
	if updateTTSCred {
		if v := strings.TrimSpace(patch.TTS.APIKey); v != "" {
			out.TTS.APIKey = v
		}
		if v := strings.TrimSpace(patch.TTS.AppKey); v != "" {
			out.TTS.AppKey = v
		}
		if v := strings.TrimSpace(patch.TTS.AccessKey); v != "" {
			out.TTS.AccessKey = v
		}
	}
	if patch.ArchiveUserAudio != nil {
		v := *patch.ArchiveUserAudio
		out.ArchiveUserAudio = &v
	}
	return out
}
