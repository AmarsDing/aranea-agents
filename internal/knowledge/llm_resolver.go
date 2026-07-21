package knowledge

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type RefineLLMSettingsGetter interface {
	Get(ctx context.Context) (biz.SystemSetting, error)
}

type LLMCatalogLister interface {
	List(ctx context.Context) ([]biz.ProviderModel, error)
}

func ResolveLLM(ctx context.Context, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, purpose string, lg loggateway.Logger) (string, string, error) {
	if sys != nil {
		s, err := sys.Get(ctx)
		if err != nil {
			lg.Warn("系统设置获取失败",
				loggateway.StepID("knowledge.resolve_llm"),
				loggateway.Str("purpose", purpose),
				loggateway.Err(err))
		} else if strings.TrimSpace(s.DefaultRefineLLM.Provider) != "" && strings.TrimSpace(s.DefaultRefineLLM.Model) != "" {
			return s.DefaultRefineLLM.Provider, s.DefaultRefineLLM.Model, nil
		}
	}
	if catalog != nil {
		models, err := catalog.List(ctx)
		if err != nil {
			lg.Warn("模型目录获取失败",
				loggateway.StepID("knowledge.resolve_llm"),
				loggateway.Str("purpose", purpose),
				loggateway.Err(err))
		} else {
			for _, m := range models {
				if m.Provider != "" && m.Model != "" && m.Enabled {
					return m.Provider, m.Model, nil
				}
			}
		}
	}
	return "", "", apierror.Unavailable(apierror.DomainKnowledge, "no LLM available for "+purpose+"; configure DefaultRefineLLM in system settings")
}

// ResolveVisionLLM 解析具备视觉能力的 LLM（Phase 9 VisionExtractor）。
// 优先目录中显式声明 Vision 能力的启用模型；无则回退 DefaultRefineLLM
// （用户显式配置，尽力而为）；两者皆无返回明确错误（NFR-12）。
func ResolveVisionLLM(ctx context.Context, sys RefineLLMSettingsGetter, catalog LLMCatalogLister, purpose string, lg loggateway.Logger) (string, string, error) {
	if catalog != nil {
		models, err := catalog.List(ctx)
		if err != nil {
			lg.Warn("模型目录获取失败",
				loggateway.StepID("knowledge.resolve_vision_llm"),
				loggateway.Str("purpose", purpose),
				loggateway.Err(err))
		} else {
			for _, m := range models {
				if m.Provider != "" && m.Model != "" && m.Enabled && m.Capabilities.Vision {
					return m.Provider, m.Model, nil
				}
			}
		}
	}
	if sys != nil {
		s, err := sys.Get(ctx)
		if err != nil {
			lg.Warn("系统设置获取失败",
				loggateway.StepID("knowledge.resolve_vision_llm"),
				loggateway.Str("purpose", purpose),
				loggateway.Err(err))
		} else if strings.TrimSpace(s.DefaultRefineLLM.Provider) != "" && strings.TrimSpace(s.DefaultRefineLLM.Model) != "" {
			return s.DefaultRefineLLM.Provider, s.DefaultRefineLLM.Model, nil
		}
	}
	return "", "", apierror.Unavailable(apierror.DomainKnowledge, "no vision-capable LLM available for "+purpose+"; enable a vision model in the catalog or configure DefaultRefineLLM")
}
