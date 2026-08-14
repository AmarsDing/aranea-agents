package biz

import (
	"context"
	"reflect"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestSpeechConfigured(t *testing.T) {
	full := SpeechSetting{
		ASR: ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "ak", AccessKey: "sk"},
		TTS: TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "ak", AccessKey: "sk", Voice: "v"},
	}
	if !SpeechASRConfigured(full) || !SpeechTTSConfigured(full) {
		t.Fatal("expected configured")
	}
	if SpeechASRConfigured(SpeechSetting{}) {
		t.Fatal("empty ASR must not be configured")
	}
	// TTS additionally requires Voice.
	noVoice := full
	noVoice.TTS.Voice = ""
	if SpeechTTSConfigured(noVoice) {
		t.Fatal("TTS without voice must not be configured")
	}
}

// X-Api-Key 鉴权模式：单 APIKey 即视为凭据完整。
func TestSpeechConfiguredAPIKeyMode(t *testing.T) {
	s := SpeechSetting{
		ASR: ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://x", APIKey: "ak-1"},
		TTS: TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://x", APIKey: "ak-1", Voice: "v"},
	}
	if !SpeechASRConfigured(s) || !SpeechTTSConfigured(s) {
		t.Fatal("api-key mode must be configured")
	}
}

func TestApplySpeechPatch_EmptyPreserves(t *testing.T) {
	cur := SpeechSetting{
		ASR:              ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://a", AppKey: "ak", AccessKey: "sk", ResourceID: "rid", Language: "zh-CN"},
		TTS:              TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://t", AppKey: "ak", AccessKey: "sk", Voice: "v", SpeedRatio: 1.2},
		ArchiveUserAudio: boolPtr(true),
	}
	out := ApplySpeechPatch(cur, SpeechSetting{}, false, false)
	if !reflect.DeepEqual(out, cur) {
		t.Fatalf("empty patch must preserve everything, got %#v", out)
	}
}

func TestApplySpeechPatch_NonCredFieldsMerge(t *testing.T) {
	cur := SpeechSetting{
		ASR: ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://a", Language: "zh-CN"},
		TTS: TTSProviderConfig{Driver: "volcengine", SpeedRatio: 1.0},
	}
	out := ApplySpeechPatch(cur, SpeechSetting{
		ASR: ASRProviderConfig{Language: "en-US"},
		TTS: TTSProviderConfig{Voice: "zh_female_x", SpeedRatio: 1.5},
	}, false, false)
	if out.ASR.Language != "en-US" || out.ASR.Driver != "volcengine" || out.ASR.Endpoint != "wss://a" {
		t.Fatalf("ASR merge wrong: %#v", out.ASR)
	}
	if out.TTS.Voice != "zh_female_x" || out.TTS.SpeedRatio != 1.5 || out.TTS.Driver != "volcengine" {
		t.Fatalf("TTS merge wrong: %#v", out.TTS)
	}
	// SpeedRatio 0 = unset → preserved.
	out2 := ApplySpeechPatch(out, SpeechSetting{TTS: TTSProviderConfig{Driver: "volcengine"}}, false, false)
	if out2.TTS.SpeedRatio != 1.5 {
		t.Fatalf("SpeedRatio 0 must preserve stored value, got %v", out2.TTS.SpeedRatio)
	}
}

func TestApplySpeechPatch_CredUpdateFlag(t *testing.T) {
	cur := SpeechSetting{
		ASR: ASRProviderConfig{AppKey: "old-ak", AccessKey: "old-sk"},
		TTS: TTSProviderConfig{AppKey: "old-ak", AccessKey: "old-sk"},
	}
	// updateXxxCred=false → credentials preserved even when patch carries values.
	out := ApplySpeechPatch(cur, SpeechSetting{
		ASR: ASRProviderConfig{AppKey: "new-ak", AccessKey: "new-sk"},
		TTS: TTSProviderConfig{AppKey: "new-ak", AccessKey: "new-sk"},
	}, false, false)
	if out.ASR.AppKey != "old-ak" || out.TTS.AccessKey != "old-sk" {
		t.Fatalf("cred flags off must not rotate credentials: %#v", out)
	}
	// flag on but empty value → preserved (avoid clobbering by accidental empty submit).
	out2 := ApplySpeechPatch(cur, SpeechSetting{}, true, true)
	if out2.ASR.AppKey != "old-ak" || out2.TTS.AppKey != "old-ak" {
		t.Fatalf("empty cred with flag on must preserve: %#v", out2)
	}
	// flag on + value → rotated.
	out3 := ApplySpeechPatch(cur, SpeechSetting{
		ASR: ASRProviderConfig{AppKey: "new-ak"},
		TTS: TTSProviderConfig{AccessKey: "new-sk"},
	}, true, true)
	if out3.ASR.AppKey != "new-ak" || out3.ASR.AccessKey != "old-sk" {
		t.Fatalf("ASR cred rotation wrong: %#v", out3.ASR)
	}
	if out3.TTS.AccessKey != "new-sk" || out3.TTS.AppKey != "old-ak" {
		t.Fatalf("TTS cred rotation wrong: %#v", out3.TTS)
	}
	// APIKey 同样走 cred flag 合并语义。
	out4 := ApplySpeechPatch(cur, SpeechSetting{
		ASR: ASRProviderConfig{APIKey: "api-1"},
		TTS: TTSProviderConfig{APIKey: "api-2"},
	}, true, true)
	if out4.ASR.APIKey != "api-1" || out4.TTS.APIKey != "api-2" {
		t.Fatalf("APIKey rotation wrong: %#v", out4)
	}
	out5 := ApplySpeechPatch(cur, SpeechSetting{
		ASR: ASRProviderConfig{APIKey: "api-1"},
	}, false, false)
	if out5.ASR.APIKey != "" {
		t.Fatalf("APIKey without cred flag must not rotate: %#v", out5.ASR)
	}
}

func TestApplySpeechPatch_ArchiveTriState(t *testing.T) {
	cur := SpeechSetting{ArchiveUserAudio: boolPtr(true)}
	// nil patch preserves stored true.
	out := ApplySpeechPatch(cur, SpeechSetting{}, false, false)
	if out.ArchiveUserAudio == nil || !*out.ArchiveUserAudio {
		t.Fatal("nil patch must preserve stored true")
	}
	// explicit false persisted (tri-state: distinguishable from unset).
	out2 := ApplySpeechPatch(cur, SpeechSetting{ArchiveUserAudio: boolPtr(false)}, false, false)
	if out2.ArchiveUserAudio == nil || *out2.ArchiveUserAudio {
		t.Fatal("explicit false must be persisted")
	}
	// nil stored stays nil under empty patch.
	out3 := ApplySpeechPatch(SpeechSetting{}, SpeechSetting{}, false, false)
	if out3.ArchiveUserAudio != nil {
		t.Fatal("unset must stay unset")
	}
}

// ── Usecase: UpdateSpeech merge + validation ────────────────────────────────

// speechStubRepo is a stateful SystemSettingRepo stub for usecase tests; only
// Get/UpdateSpeech are exercised, the rest satisfy the interface.
type speechStubRepo struct{ stored SpeechSetting }

func (r *speechStubRepo) GetSpeech(context.Context) (SpeechSetting, error) { return r.stored, nil }
func (r *speechStubRepo) UpdateSpeech(_ context.Context, patch SpeechSetting, _, _ bool) (SpeechSetting, error) {
	r.stored = patch
	return patch, nil
}

func (r *speechStubRepo) Get(context.Context) (SystemSetting, error) { return SystemSetting{}, nil }
func (r *speechStubRepo) Update(context.Context, string, string, int64, string, bool) (SystemSetting, error) {
	return SystemSetting{}, nil
}
func (r *speechStubRepo) UpdateKnowledgeEmbed(context.Context, KnowledgeEmbedSetting, bool) (KnowledgeEmbedSetting, error) {
	return KnowledgeEmbedSetting{}, nil
}
func (r *speechStubRepo) GetKnowledgeEmbed(context.Context) (KnowledgeEmbedSetting, error) {
	return KnowledgeEmbedSetting{}, nil
}
func (r *speechStubRepo) UpdateEvalLLM(context.Context, EvalLLMSetting) (EvalLLMSetting, error) {
	return EvalLLMSetting{}, nil
}
func (r *speechStubRepo) GetWebResearch(context.Context) (WebResearchSetting, error) {
	return WebResearchSetting{}, nil
}
func (r *speechStubRepo) UpdateWebResearch(context.Context, WebResearchSetting, bool) (WebResearchSetting, error) {
	return WebResearchSetting{}, nil
}
func (r *speechStubRepo) UpdateMemoryPlatform(context.Context, MemoryPlatformSetting) (MemoryPlatformSetting, error) {
	return MemoryPlatformSetting{}, nil
}
func (r *speechStubRepo) EnsureCredentialEncryptionKey(context.Context) (string, error) {
	return "", nil
}
func (r *speechStubRepo) GetRefineLLM(context.Context) (RefineLLMSetting, error) {
	return RefineLLMSetting{}, nil
}
func (r *speechStubRepo) UpdateRefineLLM(context.Context, RefineLLMSetting, bool) (RefineLLMSetting, error) {
	return RefineLLMSetting{}, nil
}
func (r *speechStubRepo) GetPlannerModel(context.Context) (PlannerModelSetting, error) {
	return PlannerModelSetting{}, nil
}
func (r *speechStubRepo) UpdatePlannerModel(context.Context, PlannerModelSetting) (PlannerModelSetting, error) {
	return PlannerModelSetting{}, nil
}

func TestUsecaseUpdateSpeech_RejectsNegativeSpeed(t *testing.T) {
	uc := NewSystemSettingUsecase(&speechStubRepo{}, nil)
	_, err := uc.UpdateSpeech(context.Background(), SpeechSetting{
		TTS: TTSProviderConfig{SpeedRatio: -0.5},
	}, false, false)
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("negative speed must be BadRequest, got %v", err)
	}
}

func TestUsecaseUpdateSpeech_MergesOntoStored(t *testing.T) {
	repo := &speechStubRepo{stored: SpeechSetting{
		ASR:              ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://a", AppKey: "ak", AccessKey: "sk", Language: "zh-CN"},
		TTS:              TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://t", AppKey: "ak", AccessKey: "sk", Voice: "v1", SpeedRatio: 1.0},
		ArchiveUserAudio: boolPtr(true),
	}}
	uc := NewSystemSettingUsecase(repo, nil)
	// Admin changes only TTS voice + speed; no credentials in the form submit.
	got, err := uc.UpdateSpeech(context.Background(), SpeechSetting{
		TTS: TTSProviderConfig{Voice: "v2", SpeedRatio: 1.3},
	}, false, false)
	if err != nil {
		t.Fatalf("UpdateSpeech: %v", err)
	}
	if got.TTS.Voice != "v2" || got.TTS.SpeedRatio != 1.3 {
		t.Fatalf("patched fields wrong: %#v", got.TTS)
	}
	if got.TTS.AppKey != "ak" || got.ASR.AppKey != "ak" {
		t.Fatalf("credentials must survive cred-less submit: %#v", got)
	}
	if got.ASR.Language != "zh-CN" || got.ASR.Driver != "volcengine" {
		t.Fatalf("untouched ASR fields must be preserved: %#v", got.ASR)
	}
	if got.ArchiveUserAudio == nil || !*got.ArchiveUserAudio {
		t.Fatalf("archive must be preserved: %#v", got.ArchiveUserAudio)
	}
	if repo.stored.TTS.Endpoint != "wss://t" {
		t.Fatalf("stored TTS endpoint must be preserved: %#v", repo.stored.TTS)
	}
}
