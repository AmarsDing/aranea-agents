package telemetry

import (
	"context"
	"strings"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"

	trpclangfuse "trpc.group/trpc-go/trpc-agent-go/telemetry/langfuse"
)

type LangfuseRuntime struct {
	shutdown func(context.Context) error
	enabled  bool
}

func NewLangfuseRuntime(c *conf.Bootstrap, lg loggateway.Logger) *LangfuseRuntime {
	if c == nil || c.Langfuse == nil || !c.Langfuse.Enable {
		lg.Debug("langfuse: not enabled")
		return &LangfuseRuntime{}
	}

	opts := buildLangfuseOptions(c.Langfuse)
	ctx := context.Background()
	clean, err := trpclangfuse.Start(ctx, opts...)
	if err != nil {
		lg.Error("langfuse: initialization failed", loggateway.Err(err))
		return &LangfuseRuntime{}
	}

	lg.Info("langfuse: enabled", loggateway.Str("base_url", c.Langfuse.BaseUrl))
	return &LangfuseRuntime{
		shutdown: clean,
		enabled:  true,
	}
}

func (r *LangfuseRuntime) Enabled() bool {
	return r != nil && r.enabled
}

func (r *LangfuseRuntime) Shutdown(ctx context.Context) error {
	if r == nil || r.shutdown == nil {
		return nil
	}
	return r.shutdown(ctx)
}

func buildLangfuseOptions(c *conf.Langfuse) []trpclangfuse.Option {
	var opts []trpclangfuse.Option
	if pk := strings.TrimSpace(c.PublicKey); pk != "" {
		opts = append(opts, trpclangfuse.WithPublicKey(pk))
	}
	if sk := strings.TrimSpace(c.SecretKey); sk != "" {
		opts = append(opts, trpclangfuse.WithSecretKey(sk))
	}
	if host := strings.TrimSpace(c.BaseUrl); host != "" {
		opts = append(opts, trpclangfuse.WithHost(host))
	}
	return opts
}
