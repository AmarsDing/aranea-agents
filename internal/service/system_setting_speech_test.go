package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/system_setting/v1"
	"aranea-agents/internal/biz"
)

func TestToProtoSpeech_NeverExposesCredentials(t *testing.T) {
	row := biz.SpeechSetting{
		ASR: biz.ASRProviderConfig{
			Driver: "volcengine", Endpoint: "wss://asr", AppKey: "secret-ak", AccessKey: "secret-sk",
			ResourceID: "rid", Language: "zh-CN",
		},
		TTS: biz.TTSProviderConfig{
			Driver: "volcengine", Endpoint: "wss://tts", AppKey: "secret-ak", AccessKey: "secret-sk",
			Voice: "zh_female_x", SpeedRatio: 1.5,
		},
	}
	out := toProtoSpeech(row)
	if out.GetAsr().GetHasApiKey() != true || out.GetTts().GetHasApiKey() != true {
		t.Fatalf("has_api_key must be set: %#v", out)
	}
	if out.GetAsr().GetConfigured() != true || out.GetTts().GetConfigured() != true {
		t.Fatalf("configured must be set: %#v", out)
	}
	// Proto messages carry no credential fields at all — verify by absence of
	// any accessor returning the secret (compile-time guarantee; runtime we
	// assert the non-secret fields survived).
	if out.GetAsr().GetEndpoint() != "wss://asr" || out.GetTts().GetVoice() != "zh_female_x" {
		t.Fatalf("non-secret fields must pass through: %#v", out)
	}
	if out.GetTts().GetSpeedRatio() != 1.5 {
		t.Fatalf("speed ratio must pass through: %v", out.GetTts().GetSpeedRatio())
	}
}

func TestToProtoSpeech_ArchiveMapping(t *testing.T) {
	// nil (unset) → false at API edge.
	if got := toProtoSpeech(biz.SpeechSetting{}).GetArchiveUserAudio(); got {
		t.Fatal("unset archive must map to false")
	}
	on := true
	if got := toProtoSpeech(biz.SpeechSetting{ArchiveUserAudio: &on}).GetArchiveUserAudio(); !got {
		t.Fatal("stored true must map to true")
	}
	off := false
	if got := toProtoSpeech(biz.SpeechSetting{ArchiveUserAudio: &off}).GetArchiveUserAudio(); got {
		t.Fatal("stored false must map to false")
	}
}

func TestHasSpeechUpdate(t *testing.T) {
	if hasSpeechUpdate(nil) {
		t.Fatal("nil req must be false")
	}
	if hasSpeechUpdate(&v1.UpdateSystemSettingsRequest{}) {
		t.Fatal("empty req must be false")
	}
	// Any single field triggers.
	if !hasSpeechUpdate(&v1.UpdateSystemSettingsRequest{SpeechAsrLanguage: "zh-CN"}) {
		t.Fatal("asr language must trigger")
	}
	if !hasSpeechUpdate(&v1.UpdateSystemSettingsRequest{SpeechTtsSpeedRatio: 1.2}) {
		t.Fatal("tts speed must trigger")
	}
	// Whitespace-only credential must NOT trigger (would rotate to empty).
	if hasSpeechUpdate(&v1.UpdateSystemSettingsRequest{SpeechAsrAppKey: "   "}) {
		t.Fatal("blank credential must not trigger")
	}
	// Explicit archive false must trigger (proto3 optional presence).
	explicitFalse := false
	if !hasSpeechUpdate(&v1.UpdateSystemSettingsRequest{SpeechArchiveUserAudio: &explicitFalse}) {
		t.Fatal("explicit archive false must trigger via optional presence")
	}
}
