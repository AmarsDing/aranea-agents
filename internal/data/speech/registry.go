package speech

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Registry 是 driver 名 → Speech Provider 工厂的注册表（设计 §3.2）。
// 新增 Provider（阿里/OpenAI）只改注册，不动网关与前端。
type Registry struct {
	asrFactories map[string]ASRFactory
	ttsFactories map[string]TTSFactory
}

type ASRFactory func(cfg biz.ASRProviderConfig, lg loggateway.Logger) (biz.StreamingASRProvider, error)
type TTSFactory func(cfg biz.TTSProviderConfig, lg loggateway.Logger) (biz.StreamingTTSProvider, error)

func NewRegistry() *Registry {
	r := &Registry{
		asrFactories: map[string]ASRFactory{},
		ttsFactories: map[string]TTSFactory{},
	}
	r.RegisterASR("volcengine", func(cfg biz.ASRProviderConfig, lg loggateway.Logger) (biz.StreamingASRProvider, error) {
		return newVolcASRProvider(cfg, lg), nil
	})
	r.RegisterTTS("volcengine", func(cfg biz.TTSProviderConfig, lg loggateway.Logger) (biz.StreamingTTSProvider, error) {
		return newVolcTTSProvider(cfg, lg), nil
	})
	return r
}

func (r *Registry) RegisterASR(driver string, f ASRFactory) { r.asrFactories[driver] = f }
func (r *Registry) RegisterTTS(driver string, f TTSFactory) { r.ttsFactories[driver] = f }

func (r *Registry) ASRProvider(cfg biz.ASRProviderConfig, lg loggateway.Logger) (biz.StreamingASRProvider, error) {
	f, ok := r.asrFactories[cfg.Driver]
	if !ok {
		return nil, apierror.FailedPrecondition("speech", "unknown asr driver %q", cfg.Driver)
	}
	return f(cfg, lg)
}

func (r *Registry) TTSProvider(cfg biz.TTSProviderConfig, lg loggateway.Logger) (biz.StreamingTTSProvider, error) {
	f, ok := r.ttsFactories[cfg.Driver]
	if !ok {
		return nil, apierror.FailedPrecondition("speech", "unknown tts driver %q", cfg.Driver)
	}
	return f(cfg, lg)
}
