package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type sensitiveMaskConfig struct {
	MaskEmail       bool `json:"mask_email"`
	MaskPhone       bool `json:"mask_phone"`
	MaskSecret      bool `json:"mask_secret"`
	BlockLeakOutput bool `json:"block_leak_output"`
}

type SensitiveDataMaskPlugin struct {
	name   string
	cfg    sensitiveMaskConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
}

var _ trpcplugin.Plugin = (*SensitiveDataMaskPlugin)(nil)

func NewSensitiveDataMaskPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *SensitiveDataMaskPlugin {
	var cfg sensitiveMaskConfig
	cfg.MaskEmail = true
	cfg.MaskPhone = true
	cfg.MaskSecret = true
	cfg.BlockLeakOutput = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &SensitiveDataMaskPlugin{name: p.Key, cfg: cfg, stats: stats, logger: NewPluginSafeLogger(p.Key, bus)}
}

func (s *SensitiveDataMaskPlugin) Name() string { return s.name }

func (s *SensitiveDataMaskPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeModel(s.beforeModel)
	r.AfterModel(s.afterModel)
}

func (s *SensitiveDataMaskPlugin) beforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	if args == nil || args.Request == nil {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	}
	msgCount := len(args.Request.Messages)
	for i := range args.Request.Messages {
		args.Request.Messages[i].Content = redactText(
			args.Request.Messages[i].Content,
			s.cfg.MaskEmail, s.cfg.MaskPhone, s.cfg.MaskSecret,
		)
	}
	s.logger.Info("plugin.sensitive_mask.before_model", "status", "ok", "messages", msgCount, "mask_email", s.cfg.MaskEmail, "mask_phone", s.cfg.MaskPhone, "mask_secret", s.cfg.MaskSecret)
	s.record(ctx, "before_model", "ok")
	return &trpcmodel.BeforeModelResult{Context: ctx}, nil
}

func (s *SensitiveDataMaskPlugin) afterModel(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
	if args == nil || args.Response == nil {
		return &trpcmodel.AfterModelResult{Context: ctx}, nil
	}
	text := responseText(args.Response)
	if text == "" {
		s.record(ctx, "after_model", "ok")
		return &trpcmodel.AfterModelResult{Context: ctx}, nil
	}
	if secretRE.MatchString(text) && s.cfg.BlockLeakOutput {
		s.logger.Info("plugin.sensitive_mask.after_model", "status", "blocked", "reason", "secret_leak_detected")
		s.record(ctx, "after_model", "blocked")
		return &trpcmodel.AfterModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("sensitive_data_mask: possible secret leak in model output"),
		}, nil
	}
	masked := redactText(text, s.cfg.MaskEmail, s.cfg.MaskPhone, s.cfg.MaskSecret)
	if masked != text {
		applyResponseText(args.Response, masked)
		s.logger.Info("plugin.sensitive_mask.after_model", "status", "ok", "masked", true)
	} else {
		s.logger.Info("plugin.sensitive_mask.after_model", "status", "ok", "masked", false)
	}
	s.record(ctx, "after_model", "ok")
	return &trpcmodel.AfterModelResult{Context: ctx}, nil
}

func (s *SensitiveDataMaskPlugin) record(ctx context.Context, point, status string) {
	if s.stats != nil {
		s.stats.Record(ctx, s.name, point, status)
	}
}

func responseText(resp *trpcmodel.Response) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	for _, ch := range resp.Choices {
		if ch.Message.Content != "" {
			b.WriteString(ch.Message.Content)
		}
	}
	return b.String()
}

func applyResponseText(resp *trpcmodel.Response, text string) {
	if resp == nil || len(resp.Choices) == 0 {
		return
	}
	resp.Choices[0].Message.Content = text
}

func blockedModelResponse(msg string) *trpcmodel.Response {
	return &trpcmodel.Response{
		Choices: []trpcmodel.Choice{{
			Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: msg},
		}},
	}
}
