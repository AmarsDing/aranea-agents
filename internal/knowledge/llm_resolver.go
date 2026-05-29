package knowledge

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type RefineLLMSettingsGetter interface {
	Get(ctx context.Context) (biz.SystemSetting, error)
}

type LLMCatalogLister interface {
	List(ctx context.Context) ([]biz.ProviderModel, error)
}

func ResolveLLM(ctx context.Context, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, purpose string) (string, string, error) {
	if sys != nil {
		s, err := sys.Get(ctx)
		if err != nil {
			event.SysLogWarn("knowledge.resolve_llm", "系统设置获取失败", event.P("purpose", purpose), event.P("error", err.Error()))
		} else if strings.TrimSpace(s.DefaultRefineLLM.Provider) != "" && strings.TrimSpace(s.DefaultRefineLLM.Model) != "" {
			return s.DefaultRefineLLM.Provider, s.DefaultRefineLLM.Model, nil
		}
	}
	if catalog != nil {
		models, err := catalog.List(ctx)
		if err != nil {
			event.SysLogWarn("knowledge.resolve_llm", "模型目录获取失败", event.P("purpose", purpose), event.P("error", err.Error()))
		} else {
			for _, m := range models {
				if m.Provider != "" && m.Model != "" && m.Enabled {
					return m.Provider, m.Model, nil
				}
			}
		}
	}
	return "", "", kerrors.ServiceUnavailable("KNOWLEDGE", "no LLM available for "+purpose+"; configure DefaultRefineLLM in system settings")
}
